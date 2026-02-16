package ranking

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
)

const (
	sportFootball   = "ncaaf"
	sportBasketball = "ncaambb"
)

type Ranker struct {
	DB    *gorm.DB
	Year  int64
	Week  int64
	Fcs   bool
	Sport string // sportFootball or sportBasketball

	startTime  time.Time
	postseason bool
}

type sportParams struct {
	RequiredGames int
	YearsBack     int64
	MOVCaps       []int64
}

// sportConfig returns ranking constants appropriate for the sport.
func (r *Ranker) sportConfig() sportParams {
	switch r.Sport {
	case sportBasketball:
		return sportParams{RequiredGames: 25, YearsBack: 1, MOVCaps: []int64{1, 20}}
	case sportFootball:
		return sportParams{RequiredGames: 12, YearsBack: 2, MOVCaps: []int64{1, 30}}
	default:
		panic(fmt.Sprintf("unknown sport: %q", r.Sport))
	}
}

func (r *Ranker) sportFilter() string {
	switch r.Sport {
	case sportFootball, sportBasketball:
		return r.Sport
	default:
		panic(fmt.Sprintf("unknown sport: %q", r.Sport))
	}
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
	SRSHigh       float64
	SRSHighNorm   float64
	SRSLow        float64
	SRSLowNorm    float64
	SRSNorm       float64
	SRSRank       int64
	SOS           float64
	SOSNorm       float64
	SOSRank       int64
	SOV           float64
	SOVNorm       float64
	SOVRank       int64
	SOL           float64
	SOLNorm       float64
	SOLRank       int64
	FinalRaw      float64
	FinalRank     int64
}

type Record struct {
	Wins   int64
	Losses int64
	Ties   int64
	Record float64
}

func (r Record) String() string {
	if r.Ties > 0 {
		return fmt.Sprintf("%d-%d-%d", r.Wins, r.Losses, r.Ties)
	}
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

	if err = r.record(teamList); err != nil {
		return nil, err
	}

	if err = r.srs(teamList); err != nil {
		return nil, err
	}

	if err = r.sos(teamList); err != nil {
		return nil, err
	}

	r.finalRanking(teamList)

	return teamList, nil
}

func (r *Ranker) finalRanking(teamList TeamList) {
	for _, team := range teamList {
		team.FinalRaw = (team.Record.Record * 0.60) + (team.SRSNorm * 0.30) + (team.SOSNorm * 0.10)
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
}
