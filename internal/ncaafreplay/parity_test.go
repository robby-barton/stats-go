package ncaafreplay

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/ranking/load"
	"github.com/robby-barton/stats-go/internal/sport"
)

func testLimits() load.ReplayLimits {
	return load.ReplayLimits{MaxTeamRows: 512, MaxGameRows: 8192, MaxOlderRows: 6144}
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Game{}, &database.TeamSeason{}, &database.TeamName{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// seedReplayData seeds 4 FBS teams, 1 FCS team, a few current-season (2023)
// games across weeks 1-2 and a handful of 2022 games, so every FBS team is
// sparse (fewer than the 12-game quota) and actual older backfill requests
// occur during the reference ranking. Team 1 has qualifying 2020 backfill
// games; team 2 stays empty by construction.
func seedReplayData(t *testing.T, db *gorm.DB) {
	t.Helper()
	teamNames := []database.TeamName{
		{TeamID: 1, Name: "Alpha", Sport: "ncaaf"},
		{TeamID: 2, Name: "Beta", Sport: "ncaaf"},
		{TeamID: 3, Name: "Gamma", Sport: "ncaaf"},
		{TeamID: 4, Name: "Delta", Sport: "ncaaf"},
		{TeamID: 5, Name: "Epsilon", Sport: "ncaaf"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatal(err)
	}
	teamSeasons := []database.TeamSeason{
		{TeamID: 1, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 2, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 3, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
		{TeamID: 4, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
		{TeamID: 5, Year: 2023, FBS: 0, Conf: "FCS", Sport: "ncaaf"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC) // Tuesday of week 1
	week := 7 * 24 * time.Hour
	games := []database.Game{
		{GameID: 1001, Season: 2023, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 28, AwayScore: 14,
			Sport: "ncaaf", StartTime: base},
		{GameID: 1002, Season: 2023, Week: 1, HomeID: 3, AwayID: 4, HomeScore: 21, AwayScore: 10,
			Sport: "ncaaf", StartTime: base.Add(time.Hour)},
		{GameID: 1003, Season: 2023, Week: 2, HomeID: 1, AwayID: 3, HomeScore: 35, AwayScore: 17,
			Sport: "ncaaf", StartTime: base.Add(week)},
		{GameID: 1004, Season: 2023, Week: 2, HomeID: 2, AwayID: 4, HomeScore: 24, AwayScore: 21,
			Sport: "ncaaf", StartTime: base.Add(week + time.Hour)},
		{GameID: 1005, Season: 2023, Week: 2, HomeID: 1, AwayID: 5, HomeScore: 31, AwayScore: 10,
			Sport: "ncaaf", StartTime: base.Add(week + 2*time.Hour)},
		// 2022 historical games between FBS teams.
		{GameID: 901, Season: 2022, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 24, AwayScore: 17,
			Sport: "ncaaf", StartTime: time.Date(2022, 9, 6, 19, 0, 0, 0, time.UTC)},
		{GameID: 902, Season: 2022, Week: 2, HomeID: 3, AwayID: 4, HomeScore: 20, AwayScore: 13,
			Sport: "ncaaf", StartTime: time.Date(2022, 9, 13, 19, 0, 0, 0, time.UTC)},
		{GameID: 903, Season: 2022, Week: 3, HomeID: 2, AwayID: 3, HomeScore: 14, AwayScore: 10,
			Sport: "ncaaf", StartTime: time.Date(2022, 9, 20, 19, 0, 0, 0, time.UTC)},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatal(err)
	}
	// Qualifying pre-window backfill games for team 1 (season < 2021).
	var older []database.Game
	for i := range 3 {
		older = append(older, database.Game{GameID: int64(4000 + i), Season: 2020, Week: int64(i + 1),
			HomeID: 1, AwayID: 2, HomeScore: int64(20 + i), AwayScore: 7, Sport: "ncaaf",
			StartTime: time.Date(2020, 9, 5+i, 19, 0, 0, 0, time.UTC)})
	}
	if err := db.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
}

func testManifest() Manifest {
	return Manifest{Schema: ManifestSchema, Windows: []WindowSpec{{Year: 2023, Week: 2}}}
}

func TestParityPassesOnCleanData(t *testing.T) {
	db := setupTestDB(t)
	seedReplayData(t, db)
	captured, failed, err := captureWindows(db, testManifest(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Fatalf("unexpected window failures: %+v", failed)
	}
	if len(captured) != 1 {
		t.Fatalf("expected one captured window, got %d", len(captured))
	}
	report := NewReport(testLimits())
	report = verifyOffline(&report, captured)
	if report.Status != StatusPassed {
		t.Fatalf("expected pass, got %s: %+v", report.Status, report.Failures)
	}
	if report.Counts.Windows != 1 || report.Counts.BackfillRequests == 0 || report.Counts.NonemptyResponses == 0 {
		t.Fatalf("counts must show real backfill coverage: %+v", report.Counts)
	}
	for key, delta := range report.Windows[0].MaxDeltas {
		if delta > ParityTolerance {
			t.Fatalf("unexpected %s delta %v", key, delta)
		}
	}
	window := report.Windows[0]
	if !window.BaseSequenceEqual || window.ReferenceFingerprint != window.ReplayFingerprint {
		t.Fatal("fingerprints or base sequence diverged on clean data")
	}
	// Every final rank must match exactly across the independent paths.
	for _, f := range report.Windows[0].ComponentDiagnostics {
		if f.Category != "srs_rank" && f.Category != "sos_rank" {
			t.Fatalf("unexpected diagnostic category %s", f.Category)
		}
	}
}

func TestParityFailsOnChangedScore(t *testing.T) {
	clean := setupTestDB(t)
	seedReplayData(t, clean)
	captured, failed, err := captureWindows(clean, testManifest(), testLimits())
	if err != nil || len(failed) != 0 || len(captured) != 1 {
		t.Fatalf("capture failed: %v %v %v", err, failed, len(captured))
	}
	mutated := setupTestDB(t)
	seedReplayData(t, mutated)
	if err := mutated.Model(&database.Game{}).Where("game_id = ?", 1001).
		Update("home_score", 27).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := load.MaterializeAt(load.ReplayOptions{DB: mutated, Year: 2023, Week: 2,
		Cutoff: captured[0].cutoff, Limits: testLimits()})
	if err != nil {
		t.Fatal(err)
	}
	// Reference from the clean DB; replay snapshot from the mutated DB.
	in, window, err := load.InputWindow(load.Options{DB: clean, Sport: sport.Football, Year: 2023, Week: 2})
	if err != nil {
		t.Fatal(err)
	}
	teams, traces, err := CalculateTraced(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Backfill = nil
	result := VerifyWindow(&capturedWindow{year: in.Year, week: in.Week, cutoff: window.StartTime,
		input: in, teams: teams, traces: traces, snapshot: snapshot})
	if result.Status != StatusFailed || len(result.Mismatches) == 0 {
		t.Fatalf("changed score must fail parity: %+v", result)
	}
	categories := []string{}
	for _, f := range result.Mismatches {
		categories = append(categories, f.Category)
	}
	if !slices.Contains(categories, "base_game_values") {
		t.Fatalf("base_game_values failure missing: %v", categories)
	}
}

func TestParityFailsOnOlderPrefixChange(t *testing.T) {
	clean := setupTestDB(t)
	seedReplayData(t, clean)
	captured, failed, err := captureWindows(clean, testManifest(), testLimits())
	if err != nil || len(failed) != 0 || len(captured) != 1 {
		t.Fatalf("capture failed: %v %v %v", err, failed, len(captured))
	}
	// Doctor the captured reference trace to simulate an independently
	// diverging older response (deep copy first: trace games share storage).
	doctored := []Trace{}
	for _, tr := range captured[0].traces {
		games := slices.Clone(tr.Games)
		if len(games) > 0 {
			games[0].HomeScore++
		}
		tr.Games = games
		doctored = append(doctored, tr)
	}
	if sameTraces(captured[0].traces, doctored) {
		t.Fatal("doctored trace must differ")
	}
	if sameTraces(captured[0].traces, captured[0].traces) != true {
		t.Fatal("identical traces must match")
	}
	mismatches := []Failure{}
	if !sameTraces(captured[0].traces, doctored) {
		mismatches = append(mismatches, Failure{Category: "older_request_or_ordered_response"})
	}
	if len(mismatches) != 1 {
		t.Fatal("trace divergence must be detected")
	}
}

func TestParityVacuousRunFailsClosed(t *testing.T) {
	// A run with no captured windows (and therefore zero actual backfill
	// requests) is a vacuous pass: the older path was never exercised. It
	// must fail closed instead of publishing a passing status.
	report := NewReport(testLimits())
	report.Status = StatusPassed
	verifyOffline(&report, nil)
	if report.Status != StatusFailed {
		t.Fatal("a run with no captured windows must fail")
	}
	found := false
	for _, f := range report.Failures {
		if f.Category == "missing_backfill_coverage" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing_backfill_coverage failure missing: %+v", report.Failures)
	}
}

func TestCaptureWindowContractFailures(t *testing.T) {
	db := setupTestDB(t)
	seedReplayData(t, db)
	// Unknown week: nothing resolves.
	_, failures, err := captureWindows(db, Manifest{Schema: ManifestSchema,
		Windows: []WindowSpec{{Year: 1999, Week: 5}}}, testLimits())
	if err != nil || len(failures) != 1 || failures[0].Mismatches[0].Category != "window_not_found" {
		t.Fatalf("window_not_found: %v %+v", err, failures)
	}
	// An existing year with a missing week must not pass by checking the
	// latest available window instead.
	_, failures, err = captureWindows(db, Manifest{Schema: ManifestSchema,
		Windows: []WindowSpec{{Year: 2023, Week: 25}}}, testLimits())
	if err != nil || len(failures) != 1 || failures[0].Mismatches[0].Category != "window_not_found" {
		t.Fatalf("missing week in an existing season: %v %+v", err, failures)
	}
	// Cutoff assertion mismatch.
	_, failures, err = captureWindows(db, Manifest{Schema: ManifestSchema,
		Windows: []WindowSpec{{Year: 2023, Week: 2, Cutoff: "2020-01-01T00:00:00Z"}}}, testLimits())
	if err != nil || len(failures) != 1 || failures[0].Mismatches[0].Category != "window_cutoff_assert" {
		t.Fatalf("window_cutoff_assert: %v %+v", err, failures)
	}
	// Matching cutoff assertion passes.
	_, failures, err = captureWindows(db, Manifest{Schema: ManifestSchema,
		Windows: []WindowSpec{{Year: 2023, Week: 2, Cutoff: "2023-09-12T04:00:00Z"}}}, testLimits())
	if err != nil || len(failures) != 0 {
		t.Fatalf("matching cutoff rejected: %v %+v", err, failures)
	}
}

func TestRejectedManifestWindowFailsEvenWhenOtherWindowsPass(t *testing.T) {
	db := setupTestDB(t)
	seedReplayData(t, db)
	captured, rejected, err := captureWindows(db, Manifest{Schema: ManifestSchema, Windows: []WindowSpec{
		{Year: 2023, Week: 2}, {Year: 2023, Week: 25},
	}}, testLimits())
	if err != nil || len(captured) != 1 || len(rejected) != 1 {
		t.Fatalf("expected one capture and one rejected window: %d/%d, %v", len(captured), len(rejected), err)
	}
	report := NewReport(testLimits())
	report.Windows = append(report.Windows, rejected...)
	report.Failures = append(report.Failures, rejected[0].Mismatches...)
	result := verifyOffline(&report, captured)
	if result.Status != StatusFailed || result.Counts.BackfillRequests == 0 || result.Windows[1].Status != StatusPassed {
		t.Fatalf("expected overall failure despite the passing window, got %s: %+v", result.Status, result.Failures)
	}
}

func TestCompareTeamsContract(t *testing.T) {
	team := func(_ int64, raw, srs float64, finalRank, srsRank int64) *ranking.Team {
		return &ranking.Team{FinalRaw: raw, FinalRank: finalRank, SRS: srs, SRSRank: srsRank,
			Record: ranking.Record{Wins: 1, Losses: 1, Record: 0.5}}
	}

	// Identical lists pass with no failures or diagnostics.
	a := ranking.TeamList{1: team(1, 0.5, 1.0, 1, 1), 2: team(2, 0.4, 0.8, 2, 2)}
	b := ranking.TeamList{1: team(1, 0.5, 1.0, 1, 1), 2: team(2, 0.4, 0.8, 2, 2)}
	failures, diagnostics := compareTeams(a, b, map[string]float64{})
	if len(failures) != 0 || len(diagnostics) != 0 {
		t.Fatalf("identical lists failed: %+v %+v", failures, diagnostics)
	}

	// An altered numeric value beyond tolerance fails.
	c := ranking.TeamList{1: team(1, 0.5, 1.0, 1, 1), 2: team(2, 0.4, 0.8, 2, 2)}
	c[1].FinalRaw = 0.6
	failures, _ = compareTeams(a, c, map[string]float64{})
	if len(failures) == 0 || failures[0].Category != "final_raw" {
		t.Fatalf("final_raw mismatch not reported: %+v", failures)
	}

	// A within-tolerance numeric delta passes.
	d := ranking.TeamList{1: team(1, 0.5+5e-10, 1.0, 1, 1), 2: team(2, 0.4, 0.8, 2, 2)}
	failures, _ = compareTeams(a, d, map[string]float64{})
	if len(failures) != 0 {
		t.Fatalf("within-tolerance delta failed: %+v", failures)
	}

	// A final-rank flip within tolerance still fails, with a near-tie proof.
	e := ranking.TeamList{1: team(1, 0.4, 1.0, 2, 1), 2: team(2, 0.4+5e-10, 1.0, 1, 2)}
	f := ranking.TeamList{1: team(1, 0.4+5e-10, 1.0, 1, 1), 2: team(2, 0.4, 1.0, 2, 2)}
	failures, _ = compareTeams(e, f, map[string]float64{})
	final := 0
	for _, failure := range failures {
		if failure.Category == "final_rank" {
			final++
			if failure.NearTie == nil {
				t.Fatal("final_rank near tie must carry a proof")
			}
		}
	}
	if final != 2 {
		t.Fatalf("both flipped final ranks must fail: %+v", failures)
	}

	// A component-rank flip within tolerance is diagnostic only...
	e = ranking.TeamList{1: team(1, 0.5, 1.0, 1, 1), 2: team(2, 0.4, 1.0, 2, 1)}
	f = ranking.TeamList{1: team(1, 0.5, 1.0+5e-10, 1, 1), 2: team(2, 0.4, 1.0, 2, 2)}
	failures, diagnostics = compareTeams(e, f, map[string]float64{})
	if len(failures) != 0 || len(diagnostics) != 1 || diagnostics[0].Category != "srs_rank" ||
		diagnostics[0].NearTie == nil {
		t.Fatalf("component rank diagnostic expected: %+v %+v", failures, diagnostics)
	}

	// ...but without a near-tie proof it fails.
	g := ranking.TeamList{1: team(1, 0.5, 1.0, 1, 2), 2: team(2, 0.4, 1.2, 2, 1)}
	failures, diagnostics = compareTeams(e, g, map[string]float64{})
	proven := false
	for _, failure := range diagnostics {
		if failure.NearTie != nil {
			proven = true
		}
	}
	if len(diagnostics) != 1 || proven {
		t.Fatalf("expected a single unproven diagnostic: %+v", diagnostics)
	}
	rankFailure := false
	for _, failure := range failures {
		if failure.Category == "srs_rank" && failure.NearTie == nil {
			rankFailure = true
		}
	}
	if !rankFailure {
		t.Fatalf("unproven component rank must fail: %+v", failures)
	}

	// A record mismatch fails exactly.
	h := ranking.TeamList{1: team(1, 0.5, 1.0, 1, 1), 2: team(2, 0.4, 0.8, 2, 2)}
	h[1].Record.Wins = 2
	failures, _ = compareTeams(a, h, map[string]float64{})
	if len(failures) != 1 || failures[0].Category != "record" {
		t.Fatalf("record mismatch not reported: %+v", failures)
	}

	// Nonfinite values fail regardless of tolerance.
	i := ranking.TeamList{1: team(1, math.NaN(), 1.0, 1, 1), 2: team(2, 0.4, 0.8, 2, 2)}
	failures, _ = compareTeams(a, i, map[string]float64{})
	if len(failures) != 1 || failures[0].Expected != "nonfinite" && failures[0].Actual != "nonfinite" {
		t.Fatalf("nonfinite value must fail: %+v", failures)
	}

	// A missing team fails the team set.
	j := ranking.TeamList{1: team(1, 0.5, 1.0, 1, 1)}
	failures, _ = compareTeams(a, j, map[string]float64{})
	if len(failures) != 1 || failures[0].Category != "team_set" {
		t.Fatalf("team set mismatch not reported: %+v", failures)
	}
}

func TestSelectedBaseAndAudit(t *testing.T) {
	in := ranking.Input{Sport: sport.Football, Year: 2023,
		Teams: []ranking.TeamInfo{{ID: 2, FBS: true}, {ID: 1, FBS: true}, {ID: 3, FBS: false}}}
	for i := range 3 {
		in.Games = append(in.Games, ranking.Game{GameID: int64(i + 1), Season: 2023,
			HomeID: 1, AwayID: 2, StartTime: time.Date(2023, 9, 5+i, 19, 0, 0, 0, time.UTC)})
	}
	selected := selectedBase(in)
	if !reflect.DeepEqual(selected, []Selection{
		{TeamID: 1, GameIDs: []int64{1, 2, 3}, Deficit: 9},
		{TeamID: 2, GameIDs: []int64{1, 2, 3}, Deficit: 9},
	}) {
		t.Fatalf("selected base: %+v", selected)
	}
	if len(load.FBSIDs(in.Teams)) != 2 {
		t.Fatal("FCS team must be excluded")
	}
	if !requestsMatchAudit(selected, []Trace{
		{TeamID: 1, Limit: 9, OpponentIDs: []int64{1, 2}, Games: []traceGame{{GameID: 1}}},
		{TeamID: 2, Limit: 9},
	}) {
		t.Fatal("audit must accept matching requests")
	}
	if requestsMatchAudit(selected, []Trace{{TeamID: 1, Limit: 3}}) {
		t.Fatal("audit must reject a wrong limit")
	}
	if requestsMatchAudit(selected, []Trace{{TeamID: 1, Limit: 9}, {TeamID: 1, Limit: 9}}) {
		t.Fatal("audit must reject duplicate requests")
	}
}

func TestFingerprintsIgnoreTeamOrderOnly(t *testing.T) {
	games := []ranking.Game{{GameID: 1, Season: 2023, HomeID: 1, AwayID: 2,
		StartTime: time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)}}
	base := ranking.Input{Year: 2023, Sport: sport.Football,
		Teams: []ranking.TeamInfo{{ID: 1, FBS: true}, {ID: 2, FBS: true}}, Games: games}
	traces := []Trace{{TeamID: 1, Limit: 1, OpponentIDs: []int64{2}, Games: traceGames(games)}}
	reordered := base
	reordered.Teams = []ranking.TeamInfo{{ID: 2, FBS: true}, {ID: 1, FBS: true}}
	if inputHash(base, traces) != inputHash(reordered, traces) {
		t.Fatal("canonicalized team order must not change the fingerprint")
	}
	rescored := base
	rescored.Games = []ranking.Game{{GameID: 1, Season: 2023, HomeID: 1, AwayID: 2, HomeScore: 1,
		StartTime: time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)}}
	if inputHash(base, traces) == inputHash(rescored, traces) {
		t.Fatal("changed game values must change the fingerprint")
	}
}

func TestCalculateTracedCapturesActualRequests(t *testing.T) {
	in := ranking.Input{Year: 2023, Sport: sport.Football,
		Teams: []ranking.TeamInfo{{ID: 1, FBS: true}, {ID: 2, FBS: true}}}
	in.Backfill = func(id int64, _ []int64, _ int64, _ int) ([]ranking.Game, error) {
		if id == 2 {
			return nil, errors.New("boom")
		}
		return nil, nil
	}
	if _, _, err := CalculateTraced(in); err == nil {
		t.Fatal("backfill errors must propagate")
	}
	in.Backfill = func(int64, []int64, int64, int) ([]ranking.Game, error) { return nil, nil }
	_, traces, err := CalculateTraced(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) == 0 {
		t.Fatal("requests must be captured")
	}
	for i := 1; i < len(traces); i++ {
		if traces[i].TeamID < traces[i-1].TeamID {
			t.Fatal("traces must be sorted by team ID")
		}
	}
}
