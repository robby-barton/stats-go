package ranking

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/robby-barton/stats-api/internal/config"
	"github.com/robby-barton/stats-api/internal/database"
	"github.com/robby-barton/stats-api/internal/games"

	"gorm.io/gorm"
)

var (
	year       int64
	week       int64
	startTime  time.Time
	postseason bool
)

type Ranker struct {
	DB  *gorm.DB
	CFG *config.Config
}

type Team struct {
	Name          string
	Conf          string
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

func NewRanker() (*Ranker, error) {
	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		return nil, err
	}

	return &Ranker{
		DB:  db,
		CFG: cfg,
	}, nil
}

type CalculateRankingParams struct {
	Year int64
	Week int64
	Fbs  bool
}

func PrintRankings(teamList TeamList) {
	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].FinalRank < teamList[ids[j]].FinalRank
	})

	if postseason {
		fmt.Printf("%d Final\n", year)
	} else {
		fmt.Printf("%d Week %d\n", year, week)
	}
	fmt.Printf("Games up to %v\n", startTime)
	fmt.Printf("%-5s %-25s %-7s %-8s %-5s %-5s %-5s %7s\n",
		"Rank", "Team", "Conf", "Record", "SRS", "SoS", "SoV", "Total")
	for _, id := range ids {
		team := teamList[id]
		fmt.Printf("%-5d %-25s %-7s %-8s %-5d %-5d %-5d %.5f\n",
			team.FinalRank, team.Name, team.Conf, team.Record, team.SRSRank,
			team.SOSRank, team.SOVRank, team.FinalRaw)
	}
}

func (r *Ranker) CalculateRanking(globals CalculateRankingParams) (TeamList, error) {
	var teamList TeamList

	teamList, err := r.setup(globals)
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

	if err = finalRanking(teamList); err != nil {
		return nil, err
	}

	return teamList, nil
}

func finalRanking(teamList TeamList) error {
	numWeeks, err := games.GetSeasonWeeksInYear(year)
	if err != nil {
		return err
	}

	numWeeks -= 2
	sharedPct := 0.2
	compositePct := 0.0
	if !postseason {
		compositePct = (math.Max(float64(numWeeks-week+1), 0.0) / float64(numWeeks)) * sharedPct
	}
	schedulePct := sharedPct - compositePct
	for _, team := range teamList {
		team.FinalRaw = (team.Record.Record * 0.50) + (team.SRSNorm * 0.30) +
			(team.CompositeNorm * compositePct) + (team.SOS * team.SOV * schedulePct)
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
