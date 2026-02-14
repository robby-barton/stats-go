package ranking

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"

	"gonum.org/v1/gonum/mat"

	"github.com/robby-barton/stats-go/internal/database"
)

const runs int = 10000

type gameResults struct {
	team     int64
	score    int64
	opponent int64
	oScore   int64
}

func (r *Ranker) sos(teamList TeamList) error {
	// range order over a map is not deterministic, so create a slice to ensure
	// order when creating vectors/matrices for SoE
	var teamOrder []int64
	for id := range teamList {
		teamOrder = append(teamOrder, id)
	}

	teamOrderMap := map[int64]int{}
	for idx, team := range teamOrder {
		teamOrderMap[team] = idx
	}

	var gameList []database.Game
	if err := r.DB.
		Where(
			"sport = ? and season = ? and start_time <= ? and home_id in (?) and away_id in (?)",
			r.sportFilter(), r.Year, r.startTime, teamOrder, teamOrder,
		).
		Order("start_time desc").Find(&gameList).Error; err != nil {
		return err
	}

	teamGameInfo := map[int64][]*gameResults{}
	for _, game := range gameList {
		teamGameInfo[game.HomeID] = append(teamGameInfo[game.HomeID], &gameResults{
			team:     game.HomeID,
			score:    game.HomeScore,
			opponent: game.AwayID,
			oScore:   game.AwayScore,
		})
		teamGameInfo[game.AwayID] = append(teamGameInfo[game.AwayID], &gameResults{
			team:     game.AwayID,
			score:    game.AwayScore,
			opponent: game.HomeID,
			oScore:   game.HomeScore,
		})
	}

	var terms []float64
	var solutions []float64
	for _, team := range teamOrder {
		gameSpreads := teamGameInfo[team]
		teamRow := make([]float64, len(teamOrder))

		// recounting wins and losses because we only care about intra-division play
		wins := 0.0
		losses := 0.0
		ties := 0.0
		for _, game := range gameSpreads {
			teamRow[teamOrderMap[game.opponent]]--
			switch {
			case game.score > game.oScore:
				wins++
			case game.oScore > game.score:
				losses++
			default:
				ties++
			}
		}

		teamRow[teamOrderMap[team]] = wins + losses + ties + 2
		terms = append(terms, teamRow...)
		solutions = append(solutions, 1+(wins-losses)/2)
	}

	termsMatrix := mat.NewDense(len(teamOrder), len(teamOrder), terms)

	var a mat.SymDense
	a.SymOuterK(1, termsMatrix)

	var chol mat.Cholesky
	if ok := chol.Factorize(&a); !ok {
		return errors.New("matrix is not positive semi-definite")
	}

	b := mat.NewVecDense(len(teamOrder), solutions)

	// Solve a * x = b for x
	var x mat.VecDense
	if err := chol.SolveVecTo(&x, b); err != nil {
		return fmt.Errorf("matrix is near singular: (%w)", err)
	}

	for idx, team := range teamOrder {
		teamList[team].SOS = x.AtVec(idx)
	}

	sort.Slice(teamOrder, func(i, j int) bool {
		return teamList[teamOrder[i]].SOS > teamList[teamOrder[j]].SOS
	})

	maxSOS := teamList[teamOrder[0]].SOS
	minSOS := teamList[teamOrder[len(teamOrder)-1]].SOS
	var prev float64
	var prevRank int64
	for rank, id := range teamOrder {
		team := teamList[id]

		if team.SOS == prev {
			team.SOSRank = prevRank
		} else {
			team.SOSRank = int64(rank + 1)
			prev = team.SOS
			prevRank = team.SOSRank
		}

		if maxSOS-minSOS != 0 {
			team.SOSNorm = (team.SOS - minSOS) / (maxSOS - minSOS)
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
	reqGames, yrsBack, movs := r.sportConfig()
	sport := r.sportFilter()

	// get previous season games just to be ready
	var allowedTeams []int64
	for id := range teamList {
		allowedTeams = append(allowedTeams, id)
	}
	var allGames []database.Game
	if err := r.DB.
		Where(
			"sport = ? and season >= ? and start_time <= ? and home_id in (?) and away_id in (?)",
			sport,
			r.Year-yrsBack,
			r.startTime,
			allowedTeams,
			allowedTeams,
		).
		Order("start_time desc").Find(&allGames).Error; err != nil {
		return err
	}

	var games []database.Game
	found := make(map[int64]bool)
	for id := range teamList {
		divGames := 0
		for _, game := range allGames {
			if game.Season == r.Year {
				if (game.HomeID == id && teamList.teamExists(game.AwayID)) ||
					(game.AwayID == id && teamList.teamExists(game.HomeID)) {
					divGames++
					if !found[game.GameID] {
						games = append(games, game)
						found[game.GameID] = true
					}
				}
			} else {
				if divGames < reqGames {
					if (game.HomeID == id && teamList.teamExists(game.AwayID)) ||
						(game.AwayID == id && teamList.teamExists(game.HomeID)) {
						divGames++
						if !found[game.GameID] {
							games = append(games, game)
							found[game.GameID] = true
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
		if divGames < reqGames {
			var remainingGames []database.Game
			if err := r.DB.
				Where(
					"sport = ? and season < ? and ((home_id = ? and away_id in (?)) or "+
						"(away_id = ? and home_id in (?)))",
					sport,
					r.Year-yrsBack,
					id,
					allowedTeams,
					id,
					allowedTeams,
				).Limit(reqGames - divGames).Order("start_time desc").
				Find(&remainingGames).Error; err != nil {
				return err
			}
			for _, game := range remainingGames {
				if !found[game.GameID] {
					games = append(games, game)
					found[game.GameID] = true
				}
			}
		}
	}

	for i, mov := range movs {
		ratings := generateAdjRatings(games, mov)
		maxMOV := math.Inf(-1)
		minMOV := math.Inf(1)
		for _, rating := range ratings {
			if rating > maxMOV {
				maxMOV = rating
			}
			if rating < minMOV {
				minMOV = rating
			}
		}
		for id, rating := range ratings {
			team := teamList[id]
			norm := (rating - minMOV) / (maxMOV - minMOV)
			team.SRS = ((team.SRS * float64(i)) + norm) / float64(i+1)
		}
	}

	var teamIDs []int64
	for id := range teamList {
		teamIDs = append(teamIDs, id)
	}
	sort.Slice(teamIDs, func(i, j int) bool {
		return teamList[teamIDs[i]].SRS > teamList[teamIDs[j]].SRS
	})
	maxSRS := teamList[teamIDs[0]].SRS
	minSRS := teamList[teamIDs[len(teamIDs)-1]].SRS
	var prev float64
	var prevRank int64
	for rank, id := range teamIDs {
		team := teamList[id]

		if team.SRS == prev {
			team.SRSRank = prevRank
		} else {
			team.SRSRank = int64(rank + 1)
			prev = team.SRS
			prevRank = team.SRSRank
		}
		if maxSRS-minSRS > 0 {
			team.SRSNorm = (team.SRS - minSRS) / (maxSRS - minSRS)
		}
	}

	return nil
}

func generateAdjRatings(games []database.Game, mov int64) map[int64]float64 {
	teamGameInfo := map[int64][]*gameSpreadSRS{}
	for _, game := range games {
		spread := game.HomeScore - game.AwayScore
		if spread > mov {
			spread = mov
		} else if spread < -mov {
			spread = -mov
		}

		teamGameInfo[game.HomeID] = append(teamGameInfo[game.HomeID], &gameSpreadSRS{
			team:     game.HomeID,
			spread:   spread,
			opponent: game.AwayID,
		})
		teamGameInfo[game.AwayID] = append(teamGameInfo[game.AwayID], &gameSpreadSRS{
			team:     game.AwayID,
			spread:   -spread,
			opponent: game.HomeID,
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
