package load

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ReplayLimits bounds research loading only; Input remains unrestricted.
type ReplayLimits struct {
	MaxTeamRows  int `json:"max_team_rows"`
	MaxGameRows  int `json:"max_game_rows"`
	MaxOlderRows int `json:"max_older_rows"`
}

// ReplayOptions identifies an explicit, regular-season FBS research window.
// Week is a label only: it never resolves or changes Cutoff.
type ReplayOptions struct {
	DB     *gorm.DB
	Year   int64
	Week   int64
	Cutoff time.Time
	Limits ReplayLimits
}

// ReplayMetadata describes bounded returned history, not an all-history scan.
type ReplayMetadata struct {
	Year          int64                `json:"year"`
	Week          int64                `json:"week"`
	Cutoff        time.Time            `json:"cutoff"`
	Sport         sport.Sport          `json:"sport"`
	Fcs           bool                 `json:"fcs"`
	Limits        ReplayLimits         `json:"limits"`
	TeamRows      int                  `json:"team_rows"`
	BaseGameRows  int                  `json:"base_game_rows"`
	OlderGameRows int                  `json:"older_game_rows"`
	OlderRequests []ReplayOlderRequest `json:"older_requests"`
}

// ReplayOlderRequest records one prospective sparse team's bounded coverage.
type ReplayOlderRequest struct {
	TeamID               int64   `json:"team_id"`
	BeforeSeason         int64   `json:"before_season"`
	OpponentIDs          []int64 `json:"opponent_ids"`
	MaterializedLimit    int     `json:"materialized_limit"`
	GameIDs              []int64 `json:"game_ids"`
	OldestReturnedSeason int64   `json:"oldest_returned_season"`
	Empty                bool    `json:"empty"`
}

type replayHistory struct {
	request ReplayOlderRequest
	games   []ranking.Game
}

// ReplaySnapshot owns only private plain values and survives DB rollback/close.
type ReplaySnapshot struct {
	input    ranking.Input
	metadata ReplayMetadata
	history  map[int64]replayHistory
}

func validateReplayOptions(opts ReplayOptions) error {
	if opts.DB == nil {
		return errors.New("load replay: nil DB")
	}
	if opts.Year <= 0 || opts.Cutoff.IsZero() {
		return errors.New("load replay: explicit positive year and nonzero cutoff required")
	}
	if opts.Week < 0 {
		return errors.New("load replay: negative week")
	}
	lim := opts.Limits
	if lim.MaxTeamRows <= 0 || lim.MaxTeamRows > 512 ||
		lim.MaxGameRows <= 0 || lim.MaxGameRows > 8192 ||
		lim.MaxOlderRows <= 0 || lim.MaxOlderRows > 6144 {
		return errors.New("load replay: limits must be positive and within approved maxima")
	}
	return nil
}

func explicitBase(opts ReplayOptions) (ranking.Input, error) {
	if err := validateReplayOptions(opts); err != nil {
		return ranking.Input{}, err
	}
	teams, err := loadTeamRows(teamQuery(opts.DB, sport.Football, opts.Year).Limit(opts.Limits.MaxTeamRows + 1))
	if err != nil {
		return ranking.Input{}, err
	}
	if len(teams) > opts.Limits.MaxTeamRows {
		return ranking.Input{}, errors.New("load replay: team row bound exceeded")
	}
	seenTeams := make(map[int64]bool, len(teams))
	for _, team := range teams {
		if team.ID <= 0 || seenTeams[team.ID] {
			return ranking.Input{}, fmt.Errorf("load replay: invalid or duplicate team ID %d", team.ID)
		}
		seenTeams[team.ID] = true
	}
	games, err := loadGameRows(baseGameQuery(opts.DB, sport.Football, opts.Year, opts.Cutoff).
		Limit(opts.Limits.MaxGameRows + 1))
	if err != nil {
		return ranking.Input{}, err
	}
	if len(games) > opts.Limits.MaxGameRows {
		return ranking.Input{}, errors.New("load replay: base game row bound exceeded")
	}
	if err := validateGames(games, opts.Cutoff, nil); err != nil {
		return ranking.Input{}, err
	}
	for _, game := range games {
		if game.Season > opts.Year || game.Season < opts.Year-ranking.YearsBack(sport.Football) {
			return ranking.Input{}, fmt.Errorf("load replay: base game %d outside season window", game.GameID)
		}
	}
	in := ranking.Input{Year: opts.Year, Week: opts.Week, StartTime: opts.Cutoff,
		Sport: sport.Football, Teams: teams, Games: games}
	if _, err := sparseDeficits(in); err != nil {
		return ranking.Input{}, err
	}
	return in, nil
}

// InputAt independently loads an explicit-cutoff input with lazy DB history.
// It shares production predicates and returned ordering, not latest resolution.
func InputAt(opts ReplayOptions) (ranking.Input, error) {
	in, err := explicitBase(opts)
	if err != nil {
		return ranking.Input{}, err
	}
	baseValues := gameValues(in.Games)
	in.Backfill = func(teamID int64, opponents []int64, beforeSeason int64, limit int) ([]ranking.Game, error) {
		if limit <= 0 || limit > ranking.RequiredGames(sport.Football) {
			return nil, errors.New("load replay: unsupported older limit")
		}
		games, err := loadGameRows(olderGameQuery(opts.DB, sport.Football, teamID, opponents, beforeSeason, limit))
		if err != nil {
			return nil, err
		}
		if err := validateOlder(games, opts.Cutoff, teamID, opponents, beforeSeason, baseValues); err != nil {
			return nil, err
		}
		return games, nil
	}
	return in, nil
}

// MaterializeAt independently loads base input and the top quota-sized older
// prefix for each prospective sparse team. No DB handle escapes this function.
func MaterializeAt(opts ReplayOptions) (*ReplaySnapshot, error) {
	in, err := explicitBase(opts)
	if err != nil {
		return nil, err
	}
	deficits, err := sparseDeficits(in)
	if err != nil {
		return nil, err
	}
	opponents := FBSIDs(in.Teams)
	quota := ranking.RequiredGames(sport.Football)
	before := in.Year - ranking.YearsBack(sport.Football)
	snapshot := &ReplaySnapshot{input: in, history: make(map[int64]replayHistory), metadata: ReplayMetadata{
		Year: in.Year, Week: in.Week, Cutoff: in.StartTime, Sport: in.Sport, Limits: opts.Limits,
		TeamRows: len(in.Teams), BaseGameRows: len(in.Games), OlderRequests: []ReplayOlderRequest{},
	}}
	values := gameValues(in.Games)
	for _, id := range opponents {
		if deficits[id] == 0 {
			continue
		}
		games, err := loadGameRows(olderGameQuery(opts.DB, sport.Football, id, opponents, before, quota))
		if err != nil {
			return nil, err
		}
		if len(games) > quota || snapshot.metadata.OlderGameRows+len(games) > opts.Limits.MaxOlderRows {
			return nil, errors.New("load replay: older game row bound exceeded")
		}
		if err := validateOlder(games, opts.Cutoff, id, opponents, before, values); err != nil {
			return nil, err
		}
		req := ReplayOlderRequest{TeamID: id, BeforeSeason: before, OpponentIDs: slices.Clone(opponents),
			MaterializedLimit: quota, GameIDs: []int64{}, Empty: len(games) == 0}
		for _, game := range games {
			req.GameIDs = append(req.GameIDs, game.GameID)
			if req.OldestReturnedSeason == 0 || game.Season < req.OldestReturnedSeason {
				req.OldestReturnedSeason = game.Season
			}
			values[game.GameID] = game
		}
		snapshot.history[id] = replayHistory{request: req, games: games}
		snapshot.metadata.OlderRequests = append(snapshot.metadata.OlderRequests, req)
		snapshot.metadata.OlderGameRows += len(games)
	}
	return snapshot, nil
}

// Input returns fresh slices and a strict offline callback over private values.
func (s *ReplaySnapshot) Input() ranking.Input {
	in := s.input
	in.Teams = slices.Clone(in.Teams)
	in.Games = slices.Clone(in.Games)
	in.Backfill = func(teamID int64, opponents []int64, beforeSeason int64, limit int) ([]ranking.Game, error) {
		history, ok := s.history[teamID]
		if !ok || beforeSeason != history.request.BeforeSeason || limit <= 0 ||
			limit > history.request.MaterializedLimit || !sameSet(opponents, history.request.OpponentIDs) {
			return nil, errors.New("load replay: unexpected offline older request")
		}
		return slices.Clone(history.games[:min(limit, len(history.games))]), nil
	}
	return in
}

// Metadata returns a defensive copy including every nested diagnostics slice.
func (s *ReplaySnapshot) Metadata() ReplayMetadata {
	meta := s.metadata
	meta.OlderRequests = slices.Clone(meta.OlderRequests)
	for i := range meta.OlderRequests {
		meta.OlderRequests[i].OpponentIDs = slices.Clone(meta.OlderRequests[i].OpponentIDs)
		meta.OlderRequests[i].GameIDs = slices.Clone(meta.OlderRequests[i].GameIDs)
	}
	return meta
}

// FBSIDs returns the FBS team IDs of an input, in input order. It is shared
// by replay materialization and external parity tooling.
func FBSIDs(teams []ranking.TeamInfo) []int64 {
	ids := make([]int64, 0, len(teams))
	for _, team := range teams {
		if team.FBS {
			ids = append(ids, team.ID)
		}
	}
	return ids
}

// sparseDeficits mirrors only SRS's per-team traversal, including its break
// on ANY prior row once the quota is met. It does not compute any rating.
func sparseDeficits(in ranking.Input) (map[int64]int, error) {
	ids := FBSIDs(in.Teams)
	field := make(map[int64]bool, len(ids))
	for _, id := range ids {
		field[id] = true
	}
	var games []ranking.Game
	for _, game := range in.Games {
		if field[game.HomeID] && field[game.AwayID] {
			games = append(games, game)
		}
	}
	deficits := make(map[int64]int)
	for _, id := range ids {
		// A current game behind a prior game invalidates the current-first
		// traversal. Reject, never reorder or impose calendar-year rules.
		seenPrior := false
		currentTotal := 0
		for _, game := range games {
			if game.HomeID != id && game.AwayID != id {
				continue
			}
			if game.Season == in.Year && seenPrior {
				return nil, fmt.Errorf("load replay: inconsistent current/prior traversal for team %d", id)
			}
			if game.Season != in.Year {
				seenPrior = true
			} else {
				currentTotal++
			}
		}
		count := 0
		currentSelected := 0
		for _, game := range games {
			if game.Season != in.Year && count >= ranking.RequiredGames(in.Sport) {
				break
			}
			if game.HomeID == id || game.AwayID == id {
				count++
				if game.Season == in.Year {
					currentSelected++
				}
			}
		}
		if currentSelected != currentTotal {
			return nil, fmt.Errorf("load replay: prior-row break hides current games for team %d", id)
		}
		if count < ranking.RequiredGames(in.Sport) {
			deficits[id] = ranking.RequiredGames(in.Sport) - count
		}
	}
	return deficits, nil
}

func gameValues(games []ranking.Game) map[int64]ranking.Game {
	values := make(map[int64]ranking.Game, len(games))
	for _, game := range games {
		values[game.GameID] = game
	}
	return values
}

func equalGame(a, b ranking.Game) bool {
	return a.GameID == b.GameID && a.Season == b.Season && a.Week == b.Week &&
		a.StartTime.Equal(b.StartTime) && a.HomeID == b.HomeID && a.AwayID == b.AwayID &&
		a.HomeScore == b.HomeScore && a.AwayScore == b.AwayScore
}

func validateGames(games []ranking.Game, cutoff time.Time, known map[int64]ranking.Game) error {
	seen := make(map[int64]bool, len(games))
	previous := cutoff
	for _, game := range games {
		if game.GameID <= 0 || seen[game.GameID] || game.HomeID <= 0 || game.AwayID <= 0 ||
			game.HomeID == game.AwayID || game.StartTime.IsZero() || game.StartTime.After(cutoff) {
			return fmt.Errorf("load replay: invalid or duplicate game %d", game.GameID)
		}
		if game.StartTime.After(previous) {
			return errors.New("load replay: games not in descending returned order")
		}
		previous = game.StartTime
		if prev, ok := known[game.GameID]; ok && !equalGame(prev, game) {
			return fmt.Errorf("load replay: inconsistent duplicate game %d", game.GameID)
		}
		seen[game.GameID] = true
	}
	return nil
}

func validateOlder(games []ranking.Game, cutoff time.Time, teamID int64, opponents []int64,
	before int64, known map[int64]ranking.Game,
) error {
	if err := validateGames(games, cutoff, known); err != nil {
		return err
	}
	for _, game := range games {
		homeMatch := game.HomeID == teamID && slices.Contains(opponents, game.AwayID)
		awayMatch := game.AwayID == teamID && slices.Contains(opponents, game.HomeID)
		if game.Season >= before || (!homeMatch && !awayMatch) {
			return fmt.Errorf("load replay: older game %d outside request", game.GameID)
		}
	}
	return nil
}

func sameSet(actual, expected []int64) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := make(map[int64]bool, len(actual))
	for _, id := range actual {
		if seen[id] || !slices.Contains(expected, id) {
			return false
		}
		seen[id] = true
	}
	return true
}
