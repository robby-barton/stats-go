package fbsprior

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/robby-barton/stats-go/internal/ranking"
)

// Adjusted is one team's blended standing: the values to substitute for the
// baseline FinalRaw and FinalRank after a successful blend.
type Adjusted struct {
	FinalRaw  float64
	FinalRank int64
}

// Apply blends the frozen prior into the baseline FBS ranking at the observed
// snapshot clock now, following the researched formula. It never mutates
// teams and returns exactly one entry per team.
//
// Per team (population mean/SD of the baseline FinalRaw in ascending team-ID
// order, SD floored at SDFloor):
//
//	games   = W+L+T (including FBS-FCS games)
//	lambda  = max(0, 1 - games/ExpirationGames); 0 at or after calendar expiry
//	z       = (FinalRaw - mean) / max(populationSD, SDFloor)
//	p       = Rho * prior.ZStrength[teamID]  // no restandardization
//	FinalRaw' = mean + SD * ((1-lambda)*z + lambda*p)
//
// A team with lambda zero (individual 4-game expiry, or the calendar expiry
// has passed) keeps its exact baseline FinalRaw — no float round-trip through
// the common scale. When every team bypasses, the baseline FinalRank values
// are preserved exactly as well. Otherwise FinalRank is a descending
// competition rank (1,1,3) over the blended scores, with ascending team ID
// breaking serialization order within equal scores. There is deliberately no
// rank floor: no team is protected from falling.
func Apply(teams ranking.TeamList, prior *Prior, now time.Time) (map[int64]Adjusted, error) {
	if len(teams) == 0 {
		return nil, errors.New("fbsprior: apply requires a nonempty baseline team list")
	}
	if prior == nil {
		return nil, errors.New("fbsprior: apply requires a loaded prior")
	}
	ids := sortedIDs(teams)
	var season int64
	for i, id := range ids {
		team := teams[id]
		if team == nil {
			return nil, fmt.Errorf("fbsprior: team %d has a nil baseline entry", id)
		}
		if i == 0 {
			season = team.Year
		}
		if team.FinalRank <= 0 || team.Record.Wins < 0 ||
			team.Record.Losses < 0 || team.Record.Ties < 0 {
			return nil, fmt.Errorf("fbsprior: team %d has an invalid rank or record counts", id)
		}
		if team.Year != season {
			return nil, fmt.Errorf("fbsprior: baseline mixes seasons (%d and %d)", season, team.Year)
		}
		if _, ok := prior.ZStrength[id]; !ok {
			return nil, fmt.Errorf("fbsprior: team %d is missing from the prior field", id)
		}
		if !finite(team.FinalRaw) {
			return nil, fmt.Errorf("fbsprior: team %d has a nonfinite baseline score", id)
		}
	}
	if season != prior.Season {
		return nil, fmt.Errorf("fbsprior: baseline season %d does not match prior season %d",
			season, prior.Season)
	}

	mean, sd := baselineScale(teams, ids)

	adjusted := make(map[int64]Adjusted, len(ids))
	allBypassed := true
	expired := !now.Before(prior.CalendarExpiry)
	for _, id := range ids {
		team := teams[id]
		games := team.Record.Wins + team.Record.Losses + team.Record.Ties
		lambda := math.Max(0, 1-float64(games)/float64(ExpirationGames))
		if expired {
			lambda = 0
		}
		if lambda == 0 {
			// Exact baseline: no round-trip through the common scale.
			adjusted[id] = Adjusted{FinalRaw: team.FinalRaw, FinalRank: team.FinalRank}
			continue
		}
		allBypassed = false
		z := (team.FinalRaw - mean) / sd
		score := mean + sd*((1-lambda)*z+lambda*(Rho*prior.ZStrength[id]))
		if !finite(score) {
			return nil, fmt.Errorf("fbsprior: team %d adjustment overflowed", id)
		}
		adjusted[id] = Adjusted{FinalRaw: score}
	}

	if allBypassed {
		// Baseline ranks are already in place for every team.
		return adjusted, nil
	}
	assignRanks(ids, adjusted)
	return adjusted, nil
}

// baselineScale computes the common comparable output scale: the arithmetic
// mean and the floored population SD of the baseline scores, accumulated in
// ascending team-ID order to match the research arithmetic exactly.
func baselineScale(teams ranking.TeamList, ids []int64) (float64, float64) {
	var sum float64
	for _, id := range ids {
		sum += teams[id].FinalRaw
	}
	mean := sum / float64(len(ids))

	var sq float64
	for _, id := range ids {
		dev := teams[id].FinalRaw - mean
		sq += dev * dev
	}
	sd := math.Sqrt(sq / float64(len(ids)))
	return mean, math.Max(sd, SDFloor)
}

// assignRanks writes descending competition ranks (1,1,3) over the blended
// scores into adjusted, ordering ties by ascending team ID.
func assignRanks(ids []int64, adjusted map[int64]Adjusted) {
	type row struct {
		id    int64
		score float64
	}
	rows := make([]row, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, row{id: id, score: adjusted[id].FinalRaw})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].id < rows[j].id
	})

	prev := math.NaN() // NaN seed: the first team always takes the new-rank branch.
	var prevRank int64
	for idx, r := range rows {
		rank := int64(idx + 1)
		if r.score == prev {
			rank = prevRank
		} else {
			prev = r.score
			prevRank = rank
		}
		adj := adjusted[r.id]
		adj.FinalRank = rank
		adjusted[r.id] = adj
	}
}

func sortedIDs(teams ranking.TeamList) []int64 {
	ids := make([]int64, 0, len(teams))
	for id := range teams {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
