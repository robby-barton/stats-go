package ranking

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"

	"gonum.org/v1/gonum/mat"
)

const runs int = 10000

type gameResults struct {
	team     int64
	score    int64
	opponent int64
	oScore   int64
}

// inSeasonGames returns the loaded games restricted to the current season and
// to games between the ranked teams.
func (r *Ranker) inSeasonGames(teamList TeamList) []Game {
	return r.windowGames(teamList, r.in.Year)
}

// windowGames returns the loaded games restricted to games between the ranked
// teams, optionally filtered to a season (season 0 means no filter). The
// loaded order (start time descending) is preserved.
func (r *Ranker) windowGames(teamList TeamList, season int64) []Game {
	var games []Game
	for _, game := range r.in.Games {
		if season != 0 && game.Season != season {
			continue
		}
		if teamList.teamExists(game.HomeID) && teamList.teamExists(game.AwayID) {
			games = append(games, game)
		}
	}
	return games
}

func (r *Ranker) sos(teamList TeamList) error {
	gameList := r.inSeasonGames(teamList)

	// range order over a map is not deterministic, so create a slice to ensure
	// order when creating vectors/matrices for SoE
	var teamOrder []int64
	for id := range teamList {
		teamOrder = append(teamOrder, id)
	}

	if len(teamOrder) == 0 {
		return nil // nothing to rank
	}

	teamOrderMap := map[int64]int{}
	for idx, team := range teamOrder {
		teamOrderMap[team] = idx
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
	// Initialize prev to NaN so the first team always takes the rank-assignment
	// branch, even when its score is exactly zero (0 == 0 would otherwise
	// assign rank 0 via the uninitialized prevRank).
	prev := math.NaN()
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
	cfg := sportConfig(r.in.Sport)

	// get previous season games just to be ready
	var allowedTeams []int64
	for id := range teamList {
		allowedTeams = append(allowedTeams, id)
	}

	allGames := r.windowGames(teamList, 0)

	var games []Game
	found := make(map[int64]bool)
	for id := range teamList {
		divGames := 0
		for _, game := range allGames {
			if game.Season == r.in.Year {
				if (game.HomeID == id && teamList.teamExists(game.AwayID)) ||
					(game.AwayID == id && teamList.teamExists(game.HomeID)) {
					divGames++
					if !found[game.GameID] {
						games = append(games, game)
						found[game.GameID] = true
					}
				}
			} else {
				if divGames < cfg.RequiredGames {
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
		if divGames < cfg.RequiredGames && r.in.Backfill != nil {
			remainingGames, err := r.in.Backfill(
				id, allowedTeams, r.in.Year-cfg.YearsBack, cfg.RequiredGames-divGames)
			if err != nil {
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

	for i, mov := range cfg.MOVCaps {
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
			norm := 0.0
			// Guard against a degenerate range: when all MOVs are equal
			// (maxMOV == minMOV) the normalization would be 0/0 = NaN.
			if maxMOV > minMOV {
				norm = (rating - minMOV) / (maxMOV - minMOV)
			}
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

	if len(teamIDs) == 0 {
		return nil // nothing to rank
	}

	maxSRS := teamList[teamIDs[0]].SRS
	minSRS := teamList[teamIDs[len(teamIDs)-1]].SRS
	// NaN seed: see the note in sos about first-team rank assignment.
	prev := math.NaN()
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

func generateAdjRatings(games []Game, mov int64) map[int64]float64 {
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
