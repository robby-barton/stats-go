package ranking

import (
	"errors"
	"fmt"
	"sort"

	"github.com/robby-barton/stats-api/internal/database"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

type gameSpread struct {
	team     int64
	spread   float64
	opponent int64
}

func PrintSRS(teamList TeamList, top int) {
	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].SRSRank < teamList[ids[j]].SRSRank
	})

	if postseason {
		fmt.Printf("%d Final\n", year)
	} else {
		fmt.Printf("%d Week %d\n", year, week)
	}
	fmt.Printf("Games up to %v\n", startTime)
	fmt.Printf("%-5s %-25s %-7s %9s\n", "Rank", "Team", "Conf", "SRS")
	for i := 0; i < top; i++ {
		team := teamList[ids[i]]
		fmt.Printf("%-5d %-25s %-7s % 7.5f\n",
			team.SRSRank, team.Name, team.Conf, team.SRS)
	}
}

func (r *Ranker) srs(teamList TeamList) error {
	// range order over a map is not deterministic, so create a slice to ensure
	// order when creating vectors/matrices for SoE
	var teamOrder []int64
	for id := range teamList {
		teamOrder = append(teamOrder, id)
	}

	requiredGames := 6
	// get previous season games just to be ready
	var prevSeason []database.Game
	if err := r.DB.Where("season = ?", year-1).
		Order("start_time desc").Find(&prevSeason).Error; err != nil {

		return err
	}

	teamGames := make(map[int64][]int64)
	var games []int64
	found := make(map[int64]bool)
	for id, team := range teamList {
		divGames := 0
		for _, g := range team.Schedule {
			if _, ok := teamList[g.Opponent]; ok {
				divGames++
				teamGames[id] = append(teamGames[id], g.GameId)
				if _, ok := found[g.GameId]; !ok {
					games = append(games, g.GameId)
					found[g.GameId] = true
				}
			}
		}
		if divGames < requiredGames {
			for _, game := range prevSeason {
				if game.HomeId == id {
					if _, ok := teamList[game.AwayId]; ok {
						divGames++
						teamGames[id] = append(teamGames[id], game.GameId)
						if _, ok := found[game.GameId]; !ok {
							games = append(games, game.GameId)
							found[game.GameId] = true
						}
					}
				} else if game.AwayId == id {
					if _, ok := teamList[game.HomeId]; ok {
						divGames++
						teamGames[id] = append(teamGames[id], game.GameId)
						if _, ok := found[game.GameId]; !ok {
							games = append(games, game.GameId)
							found[game.GameId] = true
						}
					}
				}
				if divGames >= requiredGames {
					break
				}
			}

			/*
				This solves the James Madison problem. In 2022 JMU moved to FBS and won it's
				first 5 games. Since the rating system only takes into account games played
				between division-mates (and only goes back through the previous year to backfill
				in the beginning of the season) JMU ended up having fewer than the required amount
				of games but all wins throwing off the rating scale. For teams in this situation
				we can individually search for their remaining games against division-mates.
			*/
			if divGames < requiredGames {
				var remainingGames []int64
				if err := r.DB.Model(&database.Game{}).
					Where(
						"season < ? and ((home_id = ? and away_id in (?)) or "+
							"(away_id = ? and home_id in (?)))",
						year-1,
						id,
						teamOrder,
						id,
						teamOrder,
					).Limit(requiredGames-divGames).Order("start_time desc").
					Pluck("game_id", &remainingGames).Error; err != nil {

					return nil
				}
				teamGames[id] = append(teamGames[id], remainingGames...)
				for _, game := range remainingGames {
					if _, ok := found[game]; !ok {
						games = append(games, game)
						found[game] = true
					}
				}
			}
		}
	}

	var gameStats []database.TeamGameStats
	gameStatsMap := make(map[int64][]database.TeamGameStats)
	if err := r.DB.Where("game_id in (?)", games).Find(&gameStats).Error; err != nil {
		return err
	}
	for _, stat := range gameStats {
		gameStatsMap[stat.GameId] = append(gameStatsMap[stat.GameId], stat)
	}

	teamGameInfo := map[int64][]*gameSpread{}
	var spreads []float64
	for id, stats := range gameStatsMap {
		if len(stats) != 2 {
			return errors.New(fmt.Sprintf("Not correct number of game stats (%d)", id))
		}

		spread := stats[0].Score - stats[1].Score
		spreads = append(spreads, float64(spread), float64(-spread))
		teamGameInfo[stats[0].TeamId] = append(teamGameInfo[stats[0].TeamId], &gameSpread{
			team:     stats[0].TeamId,
			spread:   float64(spread),
			opponent: stats[1].TeamId,
		})
		teamGameInfo[stats[1].TeamId] = append(teamGameInfo[stats[1].TeamId], &gameSpread{
			team:     stats[1].TeamId,
			spread:   float64(-spread),
			opponent: stats[0].TeamId,
		})
	}

	mean, stdDev := stat.MeanStdDev(spreads, nil)
	mov := mean + 1.5*stdDev
	for _, spreadList := range teamGameInfo {
		for _, spread := range spreadList {
			if spread.spread > mov {
				spread.spread = mov
			}
			if spread.spread < -mov {
				spread.spread = -mov
			}
		}
	}

	var terms []float64
	var solutions []float64
	for _, team := range teamOrder {
		avg := 0.0
		gameSpreads := teamGameInfo[team]
		opponents := make(map[int64]struct{})
		for _, spread := range gameSpreads {
			avg += spread.spread
			opponents[spread.opponent] = struct{}{}
		}
		avg /= float64(len(gameSpreads))
		solutions = append(solutions, avg)

		for _, term := range teamOrder {
			if term == team {
				terms = append(terms, 1.0)
			} else if _, ok := opponents[term]; ok {
				terms = append(terms, -1.0/float64(len(opponents)))
			} else {
				terms = append(terms, 0.0)
			}
		}
	}
	termsMatrix := mat.NewDense(len(teamList), len(teamList), terms)
	solutionsMatrix := mat.NewVecDense(len(teamList), solutions)
	resultsMatrix := mat.NewVecDense(len(teamList), nil)

	/*
		The terms matrix is near singular and has infinitely many solutions but the distances
		between individual results (e.g. South Carolina - Clemson) is always the same, so I can
		shift whatever solution I get to center on 0 and it ends up being the same between runs.
		(Hopefully this never changes, though if it does I'd assume math is dead.)
	*/
	_ = resultsMatrix.SolveVec(termsMatrix, solutionsMatrix)
	results := mat.Col(nil, 0, resultsMatrix)
	floats.AddConst(-floats.Sum(results)/float64(len(results)), results)
	for idx, team := range teamOrder {
		teamList[team].SRS = results[idx]
	}

	sort.Slice(teamOrder, func(i, j int) bool {
		return teamList[teamOrder[i]].SRS > teamList[teamOrder[j]].SRS
	})

	max := teamList[teamOrder[0]].SRS
	min := teamList[teamOrder[len(teamOrder)-1]].SRS
	var prev float64
	var prevRank int64
	for rank, id := range teamOrder {
		team := teamList[id]

		if team.SRS == prev {
			team.SRSRank = prevRank
		} else {
			team.SRSRank = int64(rank + 1)
			prev = float64(team.SRS)
			prevRank = team.SRSRank
		}

		if max-min > 0 {
			team.SRSNorm = (team.SRS - min) / (max - min)
		}
	}

	return nil
}
