package ranking

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/robby-barton/stats-go/internal/sport"
)

// Input is the data a ranking run computes over. It is loaded once at the
// edge (internal/updater or cmd/ranker via internal/ranking/load) and passed
// in as plain values — the ranking pipeline performs no database I/O.
type Input struct {
	// Year and Week identify the ranking window. Week 0 means "the latest
	// available week" (resolved by the loader).
	Year  int64
	Week  int64
	Sport sport.Sport

	// Fcs selects the lower-division ranking (football only; ignored for
	// basketball, which has no division split).
	Fcs bool

	// Postseason marks the window as a postseason/final ranking.
	Postseason bool

	// StartTime bounds the games included in the computation (only games
	// starting at or before it count).
	StartTime time.Time

	// Teams is every team for the sport/year, joined with its display name
	// and division flag (team_names ⋈ team_seasons).
	Teams []TeamInfo

	// Games are the games available for computation (the sport's season
	// window, already filtered to StartTime, ordered by start time
	// descending). record and sos further restrict to the current season.
	Games []Game

	// Backfill optionally returns games from seasons older than the loaded
	// window for a team short on division games (see srs). The loader
	// supplies a database-backed implementation; it is nil when the caller
	// has no store to query (tests).
	Backfill BackfillFunc
}

// BackfillFunc returns up to limit games from seasons before beforeSeason
// involving team against any of opponents, most recent first.
type BackfillFunc func(teamID int64, opponents []int64, beforeSeason int64, limit int) ([]Game, error)

// TeamInfo is a team entry loaded at the edge.
type TeamInfo struct {
	ID   int64
	Name string
	Conf string
	FBS  bool
}

// Game is a single game loaded at the edge.
type Game struct {
	GameID    int64
	Season    int64
	Week      int64
	StartTime time.Time
	HomeID    int64
	HomeScore int64
	AwayID    int64
	AwayScore int64
}

type Ranker struct {
	in Input

	// Derived during CalculateRanking; callers must not set these.
	startTime  time.Time
	postseason bool
}

// NewRanker validates the input and returns a Ranker. An unknown sport is
// rejected here instead of panicking mid-computation.
func NewRanker(in Input) (*Ranker, error) {
	if err := in.Sport.Validate(); err != nil {
		return nil, fmt.Errorf("ranking: %w", err)
	}
	return &Ranker{in: in}, nil
}

type sportParams struct {
	RequiredGames int
	YearsBack     int64
	MOVCaps       []int64
	RecordWeight  float64
	SRSWeight     float64
	SOSWeight     float64
}

// sportConfig returns ranking constants appropriate for the sport. The sport
// is validated by NewRanker, so the default branch only covers football.
func sportConfig(s sport.Sport) sportParams {
	switch s {
	case sport.Basketball:
		return sportParams{
			RequiredGames: 25, YearsBack: 1, MOVCaps: []int64{1, 20},
			RecordWeight: 0.25, SRSWeight: 0.60, SOSWeight: 0.15,
		}
	case sport.Football:
		return sportParams{
			RequiredGames: 12, YearsBack: 2, MOVCaps: []int64{1, 30},
			RecordWeight: 0.45, SRSWeight: 0.40, SOSWeight: 0.15,
		}
	default:
		return sportParams{
			RequiredGames: 12, YearsBack: 2, MOVCaps: []int64{1, 30},
			RecordWeight: 0.45, SRSWeight: 0.40, SOSWeight: 0.15,
		}
	}
}

// YearsBack reports how many prior seasons of games the ranking window for a
// sport reaches back, so data loaders can fetch a sufficient window.
func YearsBack(s sport.Sport) int64 {
	return sportConfig(s).YearsBack
}

type Team struct {
	Name       string
	Conf       string
	Year       int64
	Week       int64
	Postseason int64
	Record     Record
	SRS        float64
	SRSNorm    float64
	SRSRank    int64
	SOS        float64
	SOSNorm    float64
	SOSRank    int64
	FinalRaw   float64
	FinalRank  int64
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
	teamList := r.setup()
	r.record(teamList)

	if err := r.srs(teamList); err != nil {
		return nil, err
	}

	if err := r.sos(teamList); err != nil {
		return nil, err
	}

	r.finalRanking(teamList)

	return teamList, nil
}

func (r *Ranker) finalRanking(teamList TeamList) {
	cfg := sportConfig(r.in.Sport)
	for _, team := range teamList {
		team.FinalRaw = (team.Record.Record * cfg.RecordWeight) +
			(team.SRSNorm * cfg.SRSWeight) +
			(team.SOSNorm * cfg.SOSWeight)
	}

	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].FinalRaw > teamList[ids[j]].FinalRaw
	})

	// NaN seed: ensures the first team always takes the rank-assignment branch,
	// even when its score is exactly zero (otherwise it would get rank 0 via
	// the uninitialized prevRank).
	prev := math.NaN()
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
