package ranking

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/robby-barton/stats-go/internal/espn"

	"gorm.io/gorm"
)

type Ranker struct {
	DB   *gorm.DB
	Year int64
	Week int64
	Fcs  bool

	startTime  time.Time
	postseason bool
}

type Team struct {
	Name          string
	Conf          string
	Year          int64
	Week          int64
	Postseason    int64
	Record        Record
	Composite     float64
	CompositeNorm float64
	CompositeRank int64
	SRS           float64
	SRSNorm       float64
	SRSRank       int64
	SOS           float64
	SOSNorm       float64
	SOSRank       int64
	SOV           float64
	SOVNorm       float64
	SOVRank       int64
	FinalRaw      float64
	FinalRank     int64
}

type Record struct {
	Wins   int64
	Losses int64
	Record float64
}

func (r Record) String() string {
	return fmt.Sprintf("%d-%d", r.Wins, r.Losses)
}

type TeamList map[int64]*Team

func (t TeamList) teamExists(team int64) bool {
	_, ok := t[team]
	return ok
}

func (r *Ranker) CalculateRanking() (TeamList, error) {
	var teamList TeamList
	teamList, err := r.setup()
	if err != nil {
		return nil, err
	}

	if err = r.getComposites(teamList); err != nil {
		return nil, err
	}

	if err = r.srs(teamList); err != nil {
		return nil, err
	}

	if err = r.recordAndSos(teamList); err != nil {
		return nil, err
	}

	if err = r.finalRanking(teamList); err != nil {
		return nil, err
	}

	return teamList, nil
}

func (r *Ranker) finalRanking(teamList TeamList) error {
	numWeeks, err := espn.GetWeeksInSeason(r.Year)
	if err != nil {
		return err
	}

	numWeeks -= 2
	sharedPct := 0.2
	compositePct := 0.0
	if !r.postseason {
		compositePct = (math.Max(float64(numWeeks-r.Week+1), 0.0) / float64(numWeeks)) * sharedPct
	}
	schedulePct := sharedPct - compositePct
	for _, team := range teamList {
		team.FinalRaw = (team.Record.Record * 0.50) + (team.SRSNorm * 0.30) +
			(team.CompositeNorm * compositePct) + ((team.SOSNorm + team.SOVNorm) / 2 * schedulePct)
	}

	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].FinalRaw > teamList[ids[j]].FinalRaw
	})

	var prev float64
	var prevRank int64
	for rank, id := range ids {
		team := teamList[id]
		if team.FinalRaw == prev {
			team.FinalRank = prevRank
		} else {
			team.FinalRank = int64(rank + 1)
			prev = team.FinalRaw
			prevRank = team.FinalRank
		}
	}
	return nil
}
