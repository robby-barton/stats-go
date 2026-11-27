package ncaafreplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/ranking/load"
	"github.com/robby-barton/stats-go/internal/sport"
)

const (
	// Schema identifies the parity report format.
	Schema = "ncaaf-replay-parity/1"
	// ParityTolerance is the fixed absolute numeric tolerance.
	ParityTolerance = 1e-9

	// Report statuses. blocked means capture could not run; failed means a
	// parity mismatch; passed means every listed window matched exactly.
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusBlocked = "blocked"

	// Baseline names the comparison target: the default, unblended ranking
	// path (no opt-in prior blend; that is applied only after ranking in the
	// updater and is not modeled here).
	Baseline = "unblended-default-ranking"
	// HistoryMode names the replay's independent bounded history loading.
	HistoryMode = "independent_materialized_older_backfill"

	componentSRS   = "srs"
	componentSOS   = "sos"
	nonfiniteLabel = "nonfinite"
)

// Counts describe this run only; they are never efficacy evidence.
type Counts struct {
	Windows                  int `json:"windows"`
	TeamComparisons          int `json:"team_comparisons"`
	BackfillRequests         int `json:"actual_backfill_requests"`
	NonemptyResponses        int `json:"nonempty_responses"`
	MaterializedOlderRows    int `json:"materialized_older_rows"`
	FinalRankMismatches      int `json:"final_rank_mismatches"`
	ComponentRankDifferences int `json:"component_rank_differences"`
}

// Report is the only artifact this tool produces. On failed parity it
// contains status, failures and counts and nothing else; there is no
// efficacy, ranking, poll or coverage report.
type Report struct {
	Schema            string              `json:"schema"`
	Status            string              `json:"status"`
	Baseline          string              `json:"baseline"`
	HistoryMode       string              `json:"history_mode"`
	AbsoluteTolerance float64             `json:"absolute_tolerance"`
	Limits            load.ReplayLimits   `json:"limits"`
	Transaction       TransactionEvidence `json:"transaction"`
	Counts            Counts              `json:"counts"`
	Windows           []WindowResult      `json:"windows"`
	Failures          []Failure           `json:"failures"`
	Limitations       []string            `json:"limitations"`
}

// WindowResult carries the bounded evidence for one explicitly listed window.
type WindowResult struct {
	Year                 int64               `json:"year"`
	Week                 int64               `json:"week"`
	Cutoff               time.Time           `json:"cutoff"`
	Status               string              `json:"status"`
	Metadata             load.ReplayMetadata `json:"metadata"`
	ReferenceFingerprint string              `json:"reference_sha256"`
	ReplayFingerprint    string              `json:"replay_sha256"`
	ReferenceBaseIDs     []int64             `json:"reference_base_game_ids"`
	ReplayBaseIDs        []int64             `json:"replay_base_game_ids"`
	BaseSequenceEqual    bool                `json:"base_sequence_equal"`
	ReferenceSelected    []Selection         `json:"reference_selected"`
	ReplaySelected       []Selection         `json:"replay_selected"`
	ReferenceOlder       []Trace             `json:"reference_older_traces"`
	ReplayOlder          []Trace             `json:"replay_older_traces"`
	MaxDeltas            map[string]float64  `json:"maximum_numeric_deltas"`
	Mismatches           []Failure           `json:"mismatches"`
	ComponentDiagnostics []Failure           `json:"component_rank_diagnostics"`
}

// Failure is one parity mismatch. Expected/Actual are sanitized plain values.
type Failure struct {
	Year     int64   `json:"year,omitempty"`
	Week     int64   `json:"week,omitempty"`
	Category string  `json:"category"`
	TeamIDs  []int64 `json:"affected_ids,omitempty"`
	Expected any     `json:"expected,omitempty"`
	Actual   any     `json:"actual,omitempty"`
	NearTie  any     `json:"near_tie,omitempty"`
}

// Selection records one team's selected base-game traversal and deficit.
type Selection struct {
	TeamID  int64   `json:"team_id"`
	GameIDs []int64 `json:"game_ids"`
	Deficit int     `json:"deficit"`
}

// DTOs preserve plain ranking values without changing the ranking package's API.
type traceGame struct {
	GameID    int64     `json:"game_id"`
	Season    int64     `json:"season"`
	Week      int64     `json:"week"`
	StartTime time.Time `json:"start_time"`
	HomeID    int64     `json:"home_id"`
	HomeScore int64     `json:"home_score"`
	AwayID    int64     `json:"away_id"`
	AwayScore int64     `json:"away_score"`
}

type traceTeam struct {
	ID   int64  `json:"team_id"`
	Name string `json:"name"`
	Conf string `json:"conf"`
	FBS  bool   `json:"fbs"`
}

// Trace records one actual older-backfill request and its ordered response.
type Trace struct {
	TeamID       int64       `json:"team_id"`
	BeforeSeason int64       `json:"before_season"`
	Limit        int         `json:"limit"`
	OpponentIDs  []int64     `json:"opponent_ids"`
	Games        []traceGame `json:"games"`
}

func traceGames(games []ranking.Game) []traceGame {
	rows := make([]traceGame, 0, len(games))
	for _, g := range games {
		rows = append(rows, traceGame{g.GameID, g.Season, g.Week, g.StartTime.UTC(),
			g.HomeID, g.HomeScore, g.AwayID, g.AwayScore})
	}
	return rows
}

func gameIDs(games []ranking.Game) []int64 {
	ids := make([]int64, 0, len(games))
	for _, g := range games {
		ids = append(ids, g.GameID)
	}
	return ids
}

func sortedFBSIDs(in ranking.Input) []int64 {
	ids := load.FBSIDs(in.Teams)
	slices.Sort(ids)
	return ids
}

// NewReport builds the report header with the fixed parity contract.
func NewReport(limits load.ReplayLimits) Report {
	return Report{
		Schema: Schema, Status: StatusBlocked, Baseline: Baseline, HistoryMode: HistoryMode,
		AbsoluteTolerance: ParityTolerance, Limits: limits,
		Windows:  []WindowResult{},
		Failures: []Failure{},
		Limitations: []string{
			"Event-time, not knowledge-time; no stored ranking rows used.",
			"Baseline is the unblended default ranking path; the opt-in early-season prior " +
				"blend is applied only after ranking in the updater and is not modeled here.",
			"Independent SQL order is preserved; selected equal-start-time ambiguity fails " +
				"closed. No production tie-break was added.",
			"Numerical absolute tolerance is fixed at 1e-9; every final-rank mismatch fails, " +
				"including near ties. Component ranks are diagnostic only with pairwise near-tie proof.",
			"Returned older history is bounded top-quota per sparse team without a historical-year " +
				"floor; row counts are per request, not distinct historical scans.",
			"Counts describe this run only; they are not evidence of live historical completeness.",
			"Only explicitly listed manifest windows are compared. This tool makes no full-study " +
				"coverage claim and does not replace or weaken the archived full-study evidence.",
		},
	}
}

// capturedWindow holds the reference side of one window. No callback or GORM
// handle escapes the read-only transaction: only plain values and the
// self-contained replay snapshot remain.
type capturedWindow struct {
	year     int64
	week     int64
	cutoff   time.Time
	input    ranking.Input
	teams    ranking.TeamList
	traces   []Trace
	snapshot *load.ReplaySnapshot
}

// captureWindow resolves the production window and computes the traced
// reference ranking, then independently materializes the bounded replay
// snapshot. It must run inside the verified read-only transaction.
func captureWindow(tx *gorm.DB, spec WindowSpec, limits load.ReplayLimits) (*capturedWindow, []Failure, error) {
	in, window, err := load.InputWindow(load.Options{DB: tx, Sport: sport.Football,
		Year: spec.Year, Week: spec.Week})
	if err != nil {
		return nil, nil, err
	}
	fail := func(category string, expected, actual any) []Failure {
		return []Failure{{Year: spec.Year, Week: spec.Week, Category: category,
			Expected: expected, Actual: actual}}
	}
	if !window.Found || window.Season != spec.Year || window.Week != spec.Week ||
		in.Year != spec.Year || in.Week != spec.Week || window.StartTime.IsZero() {
		// The production loader falls back to the latest game for missing
		// weeks. A manifest check must never certify that fallback window.
		return nil, fail("window_not_found", "the requested regular-season week",
			"no matching game row resolved the window"), nil
	}
	if in.Postseason || window.Postseason != 0 {
		return nil, fail("non_regular_window", "a regular-season window", window.Postseason), nil
	}
	if asserted, err := spec.cutoff(); err != nil {
		return nil, nil, err
	} else if !asserted.IsZero() && !window.StartTime.Equal(asserted) {
		return nil, fail("window_cutoff_assert", asserted.UTC().Format(time.RFC3339),
			window.StartTime.UTC().Format(time.RFC3339)), nil
	}
	if len(in.Teams) > limits.MaxTeamRows || len(in.Games) > limits.MaxGameRows {
		return nil, nil, errors.New("production reference row bounds exceeded")
	}
	teams, traces, err := CalculateTraced(in)
	if err != nil {
		return nil, nil, err
	}
	// Independent query/materialization, never built from captured lazy responses.
	snapshot, err := load.MaterializeAt(load.ReplayOptions{DB: tx, Year: in.Year, Week: in.Week,
		Cutoff: in.StartTime, Limits: limits})
	if err != nil {
		return nil, nil, err
	}
	in.Backfill = nil
	return &capturedWindow{year: in.Year, week: in.Week, cutoff: in.StartTime,
		input: in, teams: teams, traces: traces, snapshot: snapshot}, nil, nil
}

// verifyWindow computes the offline replay ranking from the snapshot and
// compares it with the captured reference under the parity contract.
func verifyWindow(captured *capturedWindow) WindowResult {
	in := captured.snapshot.Input()
	teams, traces, err := CalculateTraced(in)
	result := WindowResult{
		Year: captured.year, Week: captured.week, Cutoff: captured.cutoff,
		Status:               StatusPassed,
		Metadata:             captured.snapshot.Metadata(),
		ReferenceFingerprint: inputHash(captured.input, captured.traces),
		ReplayFingerprint:    inputHash(in, traces),
		ReferenceBaseIDs:     gameIDs(captured.input.Games),
		ReplayBaseIDs:        gameIDs(in.Games),
		ReferenceSelected:    selectedBase(captured.input),
		ReplaySelected:       selectedBase(in),
		ReferenceOlder:       captured.traces,
		ReplayOlder:          traces,
		MaxDeltas:            make(map[string]float64),
		Mismatches:           []Failure{},
		ComponentDiagnostics: []Failure{},
	}
	result.BaseSequenceEqual = slices.Equal(result.ReferenceBaseIDs, result.ReplayBaseIDs)
	fail := func(category string, expected, actual any) {
		result.Mismatches = append(result.Mismatches, Failure{Year: captured.year, Week: captured.week,
			Category: category, Expected: expected, Actual: actual})
	}
	if err != nil {
		fail("offline_ranking", "successful immutable callback", "offline computation failed")
	} else {
		if !reflect.DeepEqual(result.ReferenceSelected, result.ReplaySelected) {
			fail("selected_base_order", result.ReferenceSelected, result.ReplaySelected)
		}
		if !sameTeams(captured.input.Teams, in.Teams) {
			fail("input_teams", captured.input.Teams, in.Teams)
		}
		if !sameGameValues(captured.input.Games, in.Games) {
			fail("base_game_values", captured.input.Games, in.Games)
		}
		if !sameTraces(captured.traces, traces) {
			fail("older_request_or_ordered_response", captured.traces, traces)
		}
		if !requestsMatchAudit(result.ReferenceSelected, captured.traces) ||
			!requestsMatchAudit(result.ReplaySelected, traces) {
			fail("selection_request_audit", result.ReferenceSelected, captured.traces)
		}
		failures, diagnostics := compareTeams(captured.teams, teams, result.MaxDeltas)
		result.Mismatches = append(result.Mismatches, failures...)
		result.ComponentDiagnostics = diagnostics
	}
	if len(result.Mismatches) > 0 {
		result.Status = StatusFailed
	}
	return result
}

// CalculateTraced runs the ranking while capturing every actual backfill
// request and ordered response.
func CalculateTraced(in ranking.Input) (ranking.TeamList, []Trace, error) {
	traces := []Trace{}
	callback := in.Backfill
	if callback != nil {
		in.Backfill = func(id int64, opponents []int64, before int64, limit int) ([]ranking.Game, error) {
			rows, err := callback(id, opponents, before, limit)
			if err != nil {
				return nil, err
			}
			ids := slices.Clone(opponents)
			slices.Sort(ids)
			traces = append(traces, Trace{TeamID: id, BeforeSeason: before, Limit: limit,
				OpponentIDs: ids, Games: traceGames(rows)})
			return rows, nil
		}
	}
	ranker, err := ranking.NewRanker(in)
	if err != nil {
		return nil, nil, err
	}
	teams, err := ranker.CalculateRanking()
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(traces, func(i, j int) bool { return traces[i].TeamID < traces[j].TeamID })
	return teams, traces, nil
}

func inputHash(in ranking.Input, traces []Trace) string {
	// Teams canonicalized; all game/prefix orders remain in the fingerprint.
	teams := slices.Clone(in.Teams)
	sort.Slice(teams, func(i, j int) bool { return teams[i].ID < teams[j].ID })
	teamRows := make([]traceTeam, 0, len(teams))
	for _, t := range teams {
		teamRows = append(teamRows, traceTeam{t.ID, t.Name, t.Conf, t.FBS})
	}
	b, err := json.Marshal(struct {
		Year   int64       `json:"year"`
		Cutoff time.Time   `json:"cutoff"`
		Teams  []traceTeam `json:"teams"`
		Games  []traceGame `json:"games"`
		Older  []Trace     `json:"older"`
	}{in.Year, in.StartTime.UTC(), teamRows, traceGames(in.Games), traces})
	if err != nil {
		return "marshal_error"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sameTeams(a, b []ranking.TeamInfo) bool {
	a = slices.Clone(a)
	b = slices.Clone(b)
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
	sort.Slice(b, func(i, j int) bool { return b[i].ID < b[j].ID })
	return reflect.DeepEqual(a, b)
}

func sameGameValues(a, b []ranking.Game) bool {
	if len(a) != len(b) {
		return false
	}
	values := make(map[int64]ranking.Game, len(a))
	for _, g := range a {
		values[g.GameID] = g
	}
	for _, g := range b {
		v, ok := values[g.GameID]
		if !ok || !equalGameValues(v, g) {
			return false
		}
	}
	return true
}

func equalGameValues(a, b ranking.Game) bool {
	return a.GameID == b.GameID && a.Season == b.Season && a.Week == b.Week &&
		a.StartTime.Equal(b.StartTime) && a.HomeID == b.HomeID && a.AwayID == b.AwayID &&
		a.HomeScore == b.HomeScore && a.AwayScore == b.AwayScore
}

func sameTraces(a, b []Trace) bool {
	if len(a) != len(b) {
		return false
	}
	for i, t := range a {
		u := b[i]
		if t.TeamID != u.TeamID || t.BeforeSeason != u.BeforeSeason || t.Limit != u.Limit ||
			!slices.Equal(t.OpponentIDs, u.OpponentIDs) || len(t.Games) != len(u.Games) {
			return false
		}
		for j, g := range t.Games {
			if g != u.Games[j] {
				return false
			}
		}
	}
	return true
}

func requestsMatchAudit(audit []Selection, traces []Trace) bool {
	expected := make(map[int64]int)
	for _, s := range audit {
		if s.Deficit > 0 {
			expected[s.TeamID] = s.Deficit
		}
	}
	if len(expected) != len(traces) {
		return false
	}
	seen := make(map[int64]bool)
	for _, t := range traces {
		if seen[t.TeamID] || expected[t.TeamID] != t.Limit {
			return false
		}
		seen[t.TeamID] = true
	}
	return true
}

// selectedBase audits only traversal/deficits, not any scoring formula. Per-team
// order is meaningful; global union insertion order depends on ranker's map walk.
func selectedBase(in ranking.Input) []Selection {
	ids := sortedFBSIDs(in)
	allowed := make(map[int64]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}
	filtered := []ranking.Game{}
	for _, g := range in.Games {
		if allowed[g.HomeID] && allowed[g.AwayID] {
			filtered = append(filtered, g)
		}
	}
	result := []Selection{}
	for _, id := range ids {
		row := Selection{TeamID: id, GameIDs: []int64{}}
		count := 0
		for _, g := range filtered {
			if g.Season != in.Year && count >= ranking.RequiredGames(in.Sport) {
				break
			}
			if g.HomeID == id || g.AwayID == id {
				count++
				row.GameIDs = append(row.GameIDs, g.GameID)
			}
		}
		if count < ranking.RequiredGames(in.Sport) {
			row.Deficit = ranking.RequiredGames(in.Sport) - count
		}
		result = append(result, row)
	}
	return result
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func safeValue(v float64) any {
	if finite(v) {
		return v
	}
	return nonfiniteLabel
}

// compareTeams applies the parity contract to two ranking outputs. Every
// final-rank mismatch fails, including within-tolerance near ties; component
// rank differences fail unless a pairwise near-tie proof exists in both runs.
func compareTeams(a, b ranking.TeamList, deltas map[string]float64) ([]Failure, []Failure) {
	failures := []Failure{}
	diagnostics := []Failure{}
	ids := make(map[int64]bool)
	for id := range a {
		ids[id] = true
	}
	for id := range b {
		ids[id] = true
	}
	ordered := []int64{}
	for id := range ids {
		ordered = append(ordered, id)
	}
	slices.Sort(ordered)
	for _, id := range ordered {
		x, xok := a[id]
		y, yok := b[id]
		add := func(category string, expected, actual, near any) {
			failures = append(failures, Failure{Category: category, TeamIDs: []int64{id},
				Expected: expected, Actual: actual, NearTie: near})
		}
		if !xok || !yok {
			add("team_set", xok, yok, nil)
			continue
		}
		if x.Record.Wins != y.Record.Wins || x.Record.Losses != y.Record.Losses || x.Record.Ties != y.Record.Ties {
			add("record", []int64{x.Record.Wins, x.Record.Losses, x.Record.Ties},
				[]int64{y.Record.Wins, y.Record.Losses, y.Record.Ties}, nil)
		}
		values := []struct {
			name string
			x, y float64
		}{
			{"normalized_record", x.Record.Record, y.Record.Record},
			{"final_raw", x.FinalRaw, y.FinalRaw},
			{componentSRS, x.SRS, y.SRS},
			{"srs_norm", x.SRSNorm, y.SRSNorm},
			{componentSOS, x.SOS, y.SOS},
			{"sos_norm", x.SOSNorm, y.SOSNorm},
		}
		for _, v := range values {
			if !finite(v.x) || !finite(v.y) {
				add(v.name, safeValue(v.x), safeValue(v.y), nil)
				continue
			}
			delta := math.Abs(v.x - v.y)
			deltas[v.name] = math.Max(deltas[v.name], delta)
			if delta > ParityTolerance {
				add(v.name, v.x, v.y, nil)
			}
		}
		if x.FinalRank != y.FinalRank {
			add("final_rank", x.FinalRank, y.FinalRank, nearTie(id, a, b, "final"))
		}
		for _, component := range []string{componentSRS, componentSOS} {
			xr, yr := x.SRSRank, y.SRSRank
			if component == componentSOS {
				xr, yr = x.SOSRank, y.SOSRank
			}
			if xr == yr {
				continue
			}
			near := nearTie(id, a, b, component)
			f := Failure{Category: component + "_rank", TeamIDs: []int64{id},
				Expected: xr, Actual: yr, NearTie: near}
			diagnostics = append(diagnostics, f)
			if near == nil {
				failures = append(failures, f)
			}
		}
	}
	return failures, diagnostics
}

// nearTie explains all changed pairwise order/tie relationships. It never
// excuses a final rank mismatch, and must prove small gaps in BOTH runs.
func nearTie(id int64, a, b ranking.TeamList, component string) any {
	score := func(t *ranking.Team) float64 {
		switch component {
		case componentSRS:
			return t.SRS
		case componentSOS:
			return t.SOS
		default:
			return t.FinalRaw
		}
	}
	order := func(x, y float64) int {
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		return 0
	}
	x, xok := a[id]
	y, yok := b[id]
	if !xok || !yok {
		return nil
	}
	pairs := []map[string]any{}
	ids := []int64{}
	for other := range a {
		ids = append(ids, other)
	}
	slices.Sort(ids)
	for _, other := range ids {
		u := a[other]
		v, ok := b[other]
		if !ok {
			return nil
		}
		xa, xb, ya, yb := score(x), score(u), score(y), score(v)
		if !finite(xa) || !finite(xb) || !finite(ya) || !finite(yb) {
			return nil
		}
		if order(xa, xb) == order(ya, yb) {
			continue
		}
		ga, gb := math.Abs(xa-xb), math.Abs(ya-yb)
		if ga > ParityTolerance || gb > ParityTolerance {
			return nil
		}
		pairs = append(pairs, map[string]any{"other_team_id": other,
			"reference_gap": ga, "replay_gap": gb,
			"reference_relationship": order(xa, xb), "replay_relationship": order(ya, yb)})
	}
	if len(pairs) == 0 {
		return nil
	}
	return map[string]any{"classification": "numerical_near_tie_ordering_ambiguity", "pairs": pairs}
}

// Run executes the full parity slice: verified read-only capture, offline
// replay verification, and report assembly. The report is always returned so
// the caller can persist it even on blocked or failed runs.
func Run(ctx context.Context, params *database.DBParams, manifest Manifest,
	limits load.ReplayLimits,
) (Report, error) {
	report := NewReport(limits)
	captured := []*capturedWindow{}
	var failedStubs []WindowResult
	captureErr := WithReadOnlyProof(ctx, params, &report.Transaction, func(tx *gorm.DB) error {
		var err error
		captured, failedStubs, err = captureWindows(tx, manifest, limits)
		return err
	})
	for _, stub := range failedStubs {
		report.Windows = append(report.Windows, stub)
		report.Failures = append(report.Failures, stub.Mismatches...)
	}
	if captureErr != nil {
		// Driver errors can contain connection details. Never serialize or return those.
		report.Failures = append(report.Failures, Failure{Category: "read_only_capture_blocked",
			Expected: "verified bounded read-only capture and rollback",
			Actual:   "connection, context, query, resolution, validation or rollback failed; database details suppressed"})
		report.Status = StatusBlocked
		return report, errors.New("verified read-only capture failed")
	}
	return verifyOffline(&report, captured), nil
}

// captureWindows captures every manifest window inside the given transaction
// (or bare DB handle in tests). Contract failures are returned as failed stub
// results with sanitized failures; only infrastructure errors abort the
// remaining captures.
func captureWindows(tx *gorm.DB, manifest Manifest, limits load.ReplayLimits,
) ([]*capturedWindow, []WindowResult, error) {
	captured := []*capturedWindow{}
	failed := []WindowResult{}
	for _, spec := range manifest.Windows {
		window, specFailures, err := captureWindow(tx, spec, limits)
		if err != nil {
			return captured, failed, err
		}
		if len(specFailures) > 0 {
			failed = append(failed, WindowResult{Year: spec.Year, Week: spec.Week,
				Status: StatusFailed, Mismatches: specFailures})
			continue
		}
		captured = append(captured, window)
	}
	return captured, failed, nil
}

func verifyOffline(report *Report, captured []*capturedWindow) Report {
	report.Status = StatusPassed
	if len(report.Failures) != 0 {
		report.Status = StatusFailed // capture rejected at least one manifest window
	}
	report.Counts.Windows = len(captured)
	for _, window := range captured {
		result := verifyWindow(window)
		report.Windows = append(report.Windows, result)
		report.Counts.TeamComparisons += len(result.ReplaySelected)
		report.Counts.BackfillRequests += len(result.ReferenceOlder)
		for _, t := range result.ReferenceOlder {
			if len(t.Games) > 0 {
				report.Counts.NonemptyResponses++
			}
		}
		report.Counts.MaterializedOlderRows += result.Metadata.OlderGameRows
		for _, f := range result.Mismatches {
			if f.Category == "final_rank" {
				report.Counts.FinalRankMismatches++
			}
		}
		report.Counts.ComponentRankDifferences += len(result.ComponentDiagnostics)
		if result.Status != StatusPassed {
			report.Status = StatusFailed
			report.Failures = append(report.Failures, result.Mismatches...)
		}
	}
	// A run that never exercised the older-backfill path is a vacuous pass.
	if report.Counts.BackfillRequests == 0 {
		report.Status = StatusFailed
		report.Failures = append(report.Failures, Failure{Category: "missing_backfill_coverage",
			Expected: "at least one actual older backfill request across the listed windows",
			Actual:   0})
	}
	return *report
}
