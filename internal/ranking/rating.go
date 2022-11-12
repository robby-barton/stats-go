package ranking

import (
	"reflect"
	"sort"

	"github.com/robby-barton/stats-go/internal/database"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

const requiredGames int = 6
const runs int = 10000

type gameSpreadSOE struct {
	team     int64
	spread   float64
	opponent int64
}

func (r *Ranker) soe(teamList TeamList) error {
	// range order over a map is not deterministic, so create a slice to ensure
	// order when creating vectors/matrices for SoE
	var teamOrder []int64
	for id := range teamList {
		teamOrder = append(teamOrder, id)
	}

	// get games through last season
	var gameList []database.Game
	if err := r.DB.Where("season >= ? and start_time <= ?", r.Year-1, r.startTime).
		Order("start_time desc").Find(&gameList).Error; err != nil {

		return err
	}

	var games []database.Game
	found := make(map[int64]bool)
	for id := range teamList {
		divGames := 0
		for _, g := range gameList {
			if g.Season == r.Year {
				if g.HomeId == id {
					if _, ok := teamList[g.AwayId]; ok {
						divGames++
						if _, ok := found[g.GameId]; !ok {
							games = append(games, g)
							found[g.GameId] = true
						}
					}
				} else if g.AwayId == id {
					if _, ok := teamList[g.HomeId]; ok {
						divGames++
						if _, ok := found[g.GameId]; !ok {
							games = append(games, g)
							found[g.GameId] = true
						}
					}
				}
			} else {
				if divGames < requiredGames {
					if g.HomeId == id {
						if _, ok := teamList[g.AwayId]; ok {
							divGames++
							if _, ok := found[g.GameId]; !ok {
								games = append(games, g)
								found[g.GameId] = true
							}
						}
					} else if g.AwayId == id {
						if _, ok := teamList[g.HomeId]; ok {
							divGames++
							if _, ok := found[g.GameId]; !ok {
								games = append(games, g)
								found[g.GameId] = true
							}
						}
					}
				} else {
					break
				}
			}
		}

		/*
			This solves the James Madison problem. In 2022 JMU moved to FBS and won its
			first 5 games. Since the rating system only takes into account games played
			between division-mates (and only goes back through the previous year to backfill
			in the beginning of the season) JMU ended up having fewer than the required amount
			of games but all wins throwing off the rating scale. For teams in this situation
			we can individually search for their remaining games against division-mates.
		*/
		if divGames < requiredGames {
			var remainingGames []database.Game
			if err := r.DB.
				Where(
					"season < ? and ((home_id = ? and away_id in (?)) or "+
						"(away_id = ? and home_id in (?)))",
					r.Year-1,
					id,
					teamOrder,
					id,
					teamOrder,
				).Limit(requiredGames - divGames).Order("start_time desc").
				Find(&remainingGames).Error; err != nil {

				return nil
			}
			for _, game := range remainingGames {
				if _, ok := found[game.GameId]; !ok {
					games = append(games, game)
					found[game.GameId] = true
				}
			}
		}
	}

	teamGameInfo := map[int64][]*gameSpreadSOE{}
	var spreads []float64
	for _, game := range games {
		spread := game.HomeScore - game.AwayScore
		spreads = append(spreads, float64(spread), float64(-spread))
		teamGameInfo[game.HomeId] = append(teamGameInfo[game.HomeId], &gameSpreadSOE{
			team:     game.HomeId,
			spread:   float64(spread),
			opponent: game.AwayId,
		})
		teamGameInfo[game.AwayId] = append(teamGameInfo[game.AwayId], &gameSpreadSOE{
			team:     game.AwayId,
			spread:   float64(-spread),
			opponent: game.HomeId,
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

		if max-min != 0 {
			team.SRSNorm = (team.SRS - min) / (max - min)
		}
	}

	return nil
}

type gameSpreadSRS struct {
	team     int64
	spread   int64
	opponent int64
}

func (r *Ranker) srs(teamList TeamList) error {
	// get previous season games just to be ready
	var allGames []database.Game
	if err := r.DB.Where("season >= ? and start_time <= ?", r.Year-1, r.startTime).
		Order("start_time desc").Find(&allGames).Error; err != nil {

		return err
	}

	var games []database.Game
	found := make(map[int64]bool)
	for id := range teamList {
		divGames := 0
		for _, game := range allGames {
			if game.Season == r.Year {
				if game.HomeId == id || game.AwayId == id {
					divGames++
					if _, ok := found[game.GameId]; !ok {
						games = append(games, game)
						found[game.GameId] = true
					}
				}
			} else {
				if divGames < requiredGames {
					if game.HomeId == id || game.AwayId == id {
						divGames++
						if _, ok := found[game.GameId]; !ok {
							games = append(games, game)
							found[game.GameId] = true
						}
					}
				} else {
					break
				}
			}
		}
	}

	adjRatings := generateAdjRatings(teamList, games)

	for id, rating := range adjRatings {
		teamList[id].SRS = rating
	}

	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return teamList[ids[i]].SRS > teamList[ids[j]].SRS
	})

	max := teamList[ids[0]].SRS
	min := teamList[ids[len(ids)-1]].SRS
	var prev float64
	var prevRank int64
	for rank, id := range ids {
		team := teamList[id]

		if team.SRS == prev {
			team.SRSRank = prevRank
		} else {
			team.SRSRank = int64(rank + 1)
			prev = float64(team.SRS)
			prevRank = team.SRSRank
		}

		if max-min != 0 {
			team.SRSNorm = (team.SRS - min) / (max - min)
		}
	}

	return nil
}

func generateAdjRatings(teamList TeamList, games []database.Game) map[int64]float64 {
	teamGameInfo := map[int64][]*gameSpreadSRS{}
	for _, game := range games {
		spread := game.HomeScore - game.AwayScore
		if spread > 24 {
			spread = 24
		} else if spread < 7 && spread > 0 {
			spread = 7
		} else if spread > -7 && spread < 0 {
			spread = -7
		} else if spread < -24 {
			spread = -24
		}

		homeId := game.HomeId
		if _, ok := teamList[homeId]; !ok {
			homeId = 0
		}
		awayId := game.AwayId
		if _, ok := teamList[awayId]; !ok {
			awayId = 0
		}
		teamGameInfo[homeId] = append(teamGameInfo[homeId], &gameSpreadSRS{
			team:     homeId,
			spread:   spread,
			opponent: awayId,
		})
		teamGameInfo[awayId] = append(teamGameInfo[awayId], &gameSpreadSRS{
			team:     awayId,
			spread:   -spread,
			opponent: homeId,
		})
	}

	ratings := map[int64]float64{}
	for id, spreads := range teamGameInfo {
		var avg int64
		for _, spread := range spreads {
			avg += spread.spread
		}
		ratings[id] = float64(avg) / float64(len(spreads))
	}

	adjRatings := ratings
	for i := 0; i < runs; i++ { // guard against oscillating by capping runs
		nextRating := map[int64]float64{}
		for id, games := range teamGameInfo {
			var oppAvg float64
			for _, game := range games {
				oppAvg += adjRatings[game.opponent]
			}
			oppAvg /= float64(len(games))
			nextRating[id] = ratings[id] + oppAvg
		}

		// when they stop changing, we've peaked
		if reflect.DeepEqual(adjRatings, nextRating) {
			break
		}
		adjRatings = nextRating
	}
	delete(adjRatings, 0)

	return adjRatings
}
