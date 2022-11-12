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
	Schedule      []ScheduleGame
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

type ScheduleGame struct {
	GameId   int64
	Opponent int64
	Won      bool
}

type TeamList map[int64]*Team

func (r *Ranker) PrintRankings(teamList TeamList, top int) {
	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].FinalRank < teamList[ids[j]].FinalRank
	})

	if r.postseason {
		fmt.Printf("%d Final\n", r.Year)
	} else {
		fmt.Printf("%d Week %d\n", r.Year, r.Week)
	}
	fmt.Printf("Games up to %v\n", r.startTime)
	fmt.Printf("%-5s %-25s %-7s %-8s %-5s %-5s %-5s %7s\n",
		"Rank", "Team", "Conf", "Record", "SRS", "SoS", "SoV", "Total")
	for i := 0; i < top; i++ {
		team := teamList[ids[i]]
		fmt.Printf("%-5d %-25s %-7s %-8s %-5d %-5d %-5d %.5f\n",
			team.FinalRank, team.Name, team.Conf, team.Record, team.SRSRank,
			team.SOSRank, team.SOVRank, team.FinalRaw)
	}
}

func (r *Ranker) CalculateRanking() (TeamList, error) {
	var teamList TeamList
	teamList, err := r.setup()
	if err != nil {
		return nil, err
	}

	if err = r.addGames(teamList); err != nil {
		return nil, err
	}

	if err = r.getComposites(teamList); err != nil {
		return nil, err
	}

	if err = r.srs(teamList); err != nil {
		return nil, err
	}

	sos(teamList)

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
