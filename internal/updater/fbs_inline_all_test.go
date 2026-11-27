package updater

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// esRowsByID keys one window's returned rows by team ID (FBS and FCS IDs are
// disjoint), so map-iteration order cannot perturb comparisons.
func esRowsByID(rows []database.TeamWeekResult) map[int64]database.TeamWeekResult {
	out := make(map[int64]database.TeamWeekResult, len(rows))
	for _, r := range rows {
		if _, dup := out[r.TeamID]; dup {
			tHelperPanic("duplicate team row")
		}
		out[r.TeamID] = r
	}
	return out
}

func tHelperPanic(msg string) {
	panic(msg)
}

// esRebuildFixture assembles the freezable 2023 database (target metadata,
// donor seasons 2021/2022, donor games) plus the 2023 games. The scheduled
// freeze must run before seedGames (its midseason guard refuses once any
// target-season game is persisted), so callers freeze first.
func esRebuildFixture(t *testing.T, u *Updater) {
	t.Helper()
	seedTeamsAndSeasons(t, u.db)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
}

// esPriorRowCount returns the number of rows in fbs_season_priors.
func esPriorRowCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&database.FBSSeasonPrior{}).Count(&count).Error; err != nil {
		t.Fatalf("count fbs_season_priors: %v", err)
	}
	return count
}

// esBlendedCount counts FBS rows of a season whose FinalRaw differs from the
// given baseline beyond the ranker's pre-existing noise.
func esBlendedCount(baseline, got map[rowKey]database.TeamWeekResult, year int64) int {
	blended := 0
	for key, gotRow := range got {
		if !key.Fbs || key.Year != year {
			continue
		}
		if base, ok := baseline[key]; ok && math.Abs(gotRow.FinalRaw-base.FinalRaw) > esRowTol {
			blended++
		}
	}
	return blended
}

// esAssertSameRows fails when the two row maps differ beyond the ranker's
// pre-existing run-to-run noise.
func esAssertSameRows(t *testing.T, want, got map[rowKey]database.TeamWeekResult, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: stored rows = %d, want %d", label, len(got), len(want))
	}
	for key, wantRow := range want {
		gotRow, ok := got[key]
		if !ok {
			t.Fatalf("%s: row %v missing", label, key)
		}
		if math.Abs(gotRow.FinalRaw-wantRow.FinalRaw) > esRowTol {
			t.Errorf("%s: row %v FinalRaw = %v, want %v", label, key, gotRow.FinalRaw, wantRow.FinalRaw)
		}
	}
}

// esSeasonLogs returns the early-season prior log entries mentioning the
// given season number. The 2021/2022 donor seasons in the shared fixtures
// cannot be reconstructed (their own donors are absent), so tests filter
// their expected noise by season.
func esSeasonLogs(logs *observer.ObservedLogs, season int64) []observer.LoggedEntry {
	needle := "season " + strconv.FormatInt(season, 10)
	var out []observer.LoggedEntry
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "fbs early-season prior") &&
			strings.Contains(entry.Message, needle) {
			out = append(out, entry)
		}
	}
	return out
}

// esReconstructedLogCount counts the "reconstructed the prior" info entries
// for a season.
func esReconstructedLogCount(logs *observer.ObservedLogs, season int64) int {
	needle := "reconstructed the prior for season " + strconv.FormatInt(season, 10)
	count := 0
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, needle) {
			count++
		}
	}
	return count
}

// esAllLevelLogger captures every log level so tests can assert on the
// reconstruction info entries as well as warnings.
func esAllLevelLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	return zap.New(core), logs
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestInlineRebuild_ScheduledVsRecomputedAgreement verifies the core
// override: on the same fixture inputs and cutoff, the scheduled stored-prior
// blend and the inline rebuild's in-memory reconstruction agree within the
// ranker's pre-existing float noise. The 2023 games are persisted before the
// rebuild runs, so agreement also proves no target-season game leaked into
// the reconstructed prior input.
func TestInlineRebuild_ScheduledVsRecomputedAgreement(t *testing.T) {
	logger, logs := esAllLevelLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
	stubNow(t, freezeClock())

	// Freeze the scheduled prior before any target-season game exists (the
	// scheduler's midseason guard), then persist the season's games and
	// observe the rankings at the normal early-season clock.
	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
		t.Fatalf("freezeFBSPrior: %v", err)
	}
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	// Baseline snapshot of the latest window.
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// Scheduled blended run.
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	scheduled := storedRows(t, u.db)
	if esBlendedCount(baseline, scheduled, 2023) == 0 {
		t.Fatal("scheduled run stayed entirely on baseline")
	}

	// The inline rebuild must not touch the stored table.
	artifactBefore := storedPriorRow(t, u.db, 2023).Artifact
	if err := u.db.Where("1 = 1").Delete(&database.TeamWeekResult{}).Error; err != nil {
		t.Fatalf("clear rows: %v", err)
	}
	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior: %v", err)
	}
	got := storedRows(t, u.db)

	// Scheduled vs recomputed agreement on the same latest window: the
	// rebuild also writes every historical window, so compare the scheduled
	// keys (the latest window) within the rebuild's rows.
	if len(got) < len(scheduled) {
		t.Fatalf("rebuild rows = %d, want at least %d", len(got), len(scheduled))
	}
	for key, want := range scheduled {
		gotRow, ok := got[key]
		if !ok {
			t.Fatalf("rebuild row %v missing", key)
		}
		if math.Abs(gotRow.FinalRaw-want.FinalRaw) > esRowTol {
			t.Errorf("scheduled vs rebuild row %v FinalRaw = %v, want %v", key, gotRow.FinalRaw, want.FinalRaw)
		}
	}

	// FCS rows stay at baseline.
	for key, want := range baseline {
		if !key.Fbs {
			if math.Abs(got[key].FinalRaw-want.FinalRaw) > esRowTol {
				t.Errorf("FCS row %v FinalRaw = %v, want baseline %v", key, got[key].FinalRaw, want.FinalRaw)
			}
		}
	}

	// No prior table writes, and the stored row is byte-identical.
	if got := esPriorRowCount(t, u.db); got != 1 {
		t.Errorf("fbs_season_priors rows = %d, want 1", got)
	}
	if got := storedPriorRow(t, u.db, 2023).Artifact; got != artifactBefore {
		t.Error("the stored prior artifact changed during the rebuild")
	}

	// The rebuild reconstructed the 2023 prior in memory (the current
	// Eastern season) despite a valid stored row being present.
	if esReconstructedLogCount(logs, 2023) == 0 {
		t.Error("the rebuild did not reconstruct the 2023 prior in memory")
	}
}

// TestInlineRebuild_IgnoresStoredRow verifies the rebuild is independent of
// the stored table: a deliberately corrupt row is neither read nor repaired,
// the rebuild still blends, and the same rows are produced with the prior
// table dropped entirely.
func TestInlineRebuild_IgnoresStoredRow(t *testing.T) {
	u := newTestUpdater(t)
	esRebuildFixture(t, u)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// Deliberately different but valid stored strengths. A rebuild that
	// used them would disagree with the no-table run below.
	seedEspPrior(t, u.db, 2023, func(out *fbsprior.FreezeOutput) {
		out.ZStrength = map[int64]float64{1: -5, 2: 4, 3: -3, 4: 2}
	})
	valid := storedPriorRow(t, u.db, 2023).Artifact
	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior (different stored strengths): %v", err)
	}
	withValid := storedRows(t, u.db)
	if got := storedPriorRow(t, u.db, 2023).Artifact; got != valid {
		t.Fatal("the rebuild changed the frozen artifact")
	}

	// Make the existing row corrupt without changing ranking inputs.
	if err := u.db.Model(&database.FBSSeasonPrior{}).Where("season = ?", 2023).
		Update("artifact", "{not json").Error; err != nil {
		t.Fatalf("corrupt fixture: %v", err)
	}

	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior: %v", err)
	}
	withCorrupt := storedRows(t, u.db)
	esAssertSameRows(t, withValid, withCorrupt, "different valid stored prior vs corrupt prior")
	if esBlendedCount(baseline, withCorrupt, 2023) == 0 {
		t.Fatal("rebuild stayed entirely on baseline with a corrupt stored row")
	}
	// The corrupt row was left untouched.
	if got := esPriorRowCount(t, u.db); got != 1 {
		t.Errorf("fbs_season_priors rows = %d, want 1", got)
	}

	// Drop the prior table entirely: the rebuild still works and produces
	// the same rows.
	if err := u.db.Migrator().DropTable(&database.FBSSeasonPrior{}); err != nil {
		t.Fatalf("drop fbs_season_priors: %v", err)
	}
	if err := u.db.Where("1 = 1").Delete(&database.TeamWeekResult{}).Error; err != nil {
		t.Fatalf("clear rows: %v", err)
	}
	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior (table dropped): %v", err)
	}
	esAssertSameRows(t, withCorrupt, storedRows(t, u.db), "corrupt row vs dropped table")
}

// TestInlineRebuild_CurrentSeasonWithoutStoredRow verifies the rebuild
// reconstructs the current Eastern season's prior in memory when the
// scheduler has no stored row, and writes nothing to the prior table.
func TestInlineRebuild_CurrentSeasonWithoutStoredRow(t *testing.T) {
	logger, logs := esAllLevelLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	esRebuildFixture(t, u)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior: %v", err)
	}
	got := storedRows(t, u.db)
	if esBlendedCount(baseline, got, 2023) == 0 {
		t.Fatal("rebuild stayed entirely on baseline without a stored row")
	}
	if got := esPriorRowCount(t, u.db); got != 0 {
		t.Errorf("fbs_season_priors rows = %d, want 0 (no table writes)", got)
	}
	if esReconstructedLogCount(logs, 2023) == 0 {
		t.Error("the rebuild did not reconstruct the 2023 prior in memory")
	}
}

// TestInlineRebuild_HistoricalWeeks verifies the rebuild's window semantics:
// the explicit week-1 window (Tuesday cutoff before any regular game) is the
// exact baseline, the explicit week-2 window (cutoff within the early window)
// is blended, and FCS rows are untouched. No prior table writes happen.
func TestInlineRebuild_HistoricalWeeks(t *testing.T) {
	u := newTestUpdater(t)
	esRebuildFixture(t, u)
	stubNow(t, esObservedClock())

	// Week 1: the Tuesday cutoff precedes the earliest regular kickoff
	// (2023-09-02), so the window must stay at the exact baseline.
	week1Base, err := u.rankingForWeek(2023, 1, nil)
	if err != nil {
		t.Fatalf("week 1 baseline: %v", err)
	}
	week1Run := newPriorRun(esObservedClock(), true)
	week1Prior, err := u.rankingForWeek(2023, 1, week1Run)
	if err != nil {
		t.Fatalf("week 1 with prior: %v", err)
	}
	for id, want := range esRowsByID(week1Base) {
		got := esRowsByID(week1Prior)[id]
		if math.Abs(got.FinalRaw-want.FinalRaw) > esRowTol {
			t.Errorf("week 1 team %d FinalRaw = %v, want baseline %v", id, got.FinalRaw, want.FinalRaw)
		}
	}

	if len(week1Run.priors) != 0 {
		t.Fatal("the preseason cutoff triggered a reconstruction")
	}

	// Week 2: the Tuesday cutoff (2023-09-05) is inside the early window and
	// the season has regular games through it, so the window is blended.
	week2Base, err := u.rankingForWeek(2023, 2, nil)
	if err != nil {
		t.Fatalf("week 2 baseline: %v", err)
	}
	week2Run := newPriorRun(esObservedClock(), true)
	week2Prior, err := u.rankingForWeek(2023, 2, week2Run)
	if err != nil {
		t.Fatalf("week 2 with prior: %v", err)
	}
	blended := 0
	for id, want := range esRowsByID(week2Base) {
		got := esRowsByID(week2Prior)[id]
		diff := math.Abs(got.FinalRaw - want.FinalRaw)
		if want.Fbs {
			if diff > esRowTol {
				blended++
			}
		} else if diff > esRowTol {
			t.Errorf("FCS team %d changed at week 2", id)
		}
	}
	if blended == 0 {
		t.Fatal("week 2 stayed entirely on baseline")
	}
	if got := esPriorRowCount(t, u.db); got != 0 {
		t.Errorf("fbs_season_priors rows = %d, want 0 (no table writes)", got)
	}
}

// TestInlineRebuild_CapBypass verifies the day-56 cap for the rebuild: the
// current/latest week-0 slot observed exactly at the fixed expiry instant is
// the exact baseline, silently — the prior is not even reconstructed for it —
// while the earlier historical week-2 window is still blended.
func TestInlineRebuild_CapBypass(t *testing.T) {
	logger, logs := esAllLevelLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	esRebuildFixture(t, u)
	// The fixed day-56 expiry for season 2023 is midnight Eastern on
	// 2023-10-05 (04:00Z).
	stubNow(t, time.Date(2023, 10, 5, 4, 0, 0, 0, time.UTC))

	week3Base, err := u.rankingForWeek(2023, 0, nil)
	if err != nil {
		t.Fatalf("week 3 baseline: %v", err)
	}
	week3Run := newPriorRun(nowFunc(), true)
	week3Prior, err := u.rankingForWeek(2023, 0, week3Run)
	if err != nil {
		t.Fatalf("week 3 with prior: %v", err)
	}
	for id, want := range esRowsByID(week3Base) {
		got := esRowsByID(week3Prior)[id]
		if math.Abs(got.FinalRaw-want.FinalRaw) > esRowTol {
			t.Errorf("week 3 team %d FinalRaw = %v, want baseline %v", id, got.FinalRaw, want.FinalRaw)
		}
	}
	// The cap bypass is silent for the current season.
	if got := esSeasonLogs(logs, 2023); len(got) != 0 {
		t.Errorf("cap bypass logged %d entries for season 2023, want 0", len(got))
	}

	// The earlier historical week-2 window (cutoff inside the early window)
	// is still blended at the cap-observed clock.
	week2Base, err := u.rankingForWeek(2023, 2, nil)
	if err != nil {
		t.Fatalf("week 2 baseline: %v", err)
	}
	week2Run := newPriorRun(nowFunc(), true)
	week2Prior, err := u.rankingForWeek(2023, 2, week2Run)
	if err != nil {
		t.Fatalf("week 2 with prior: %v", err)
	}
	blended := 0
	for id, want := range esRowsByID(week2Base) {
		got := esRowsByID(week2Prior)[id]
		if want.Fbs && math.Abs(got.FinalRaw-want.FinalRaw) > esRowTol {
			blended++
		}
	}
	if blended == 0 {
		t.Fatal("week 2 stayed entirely on baseline")
	}
}

// TestInlineRebuild_PreseasonAndPostseasonSkipsSilently verifies the rebuild
// skips the current/latest week-0 slot silently when the latest persisted
// game is a preseason (phase -1) or postseason (phase +1) game, while the
// earlier historical week-2 window still blends.
func TestInlineRebuild_PreseasonAndPostseasonSkipsSilently(t *testing.T) {
	for _, tc := range []struct {
		name  string
		phase int64
	}{
		{name: "preseason", phase: -1},
		{name: "postseason", phase: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := esAllLevelLogger(t)
			u := newTestUpdaterWithLogger(t, logger)
			esRebuildFixture(t, u)
			stubNow(t, esObservedClock())
			if err := u.db.Model(&database.Game{}).
				Where("game_id = ?", fixtureGameID5).
				Update("postseason", tc.phase).Error; err != nil {
				t.Fatalf("mark %s: %v", tc.name, err)
			}

			week3Base, err := u.rankingForWeek(2023, 0, nil)
			if err != nil {
				t.Fatalf("week 3 baseline: %v", err)
			}
			week3Run := newPriorRun(esObservedClock(), true)
			week3Prior, err := u.rankingForWeek(2023, 0, week3Run)
			if err != nil {
				t.Fatalf("week 3 with prior: %v", err)
			}
			for id, want := range esRowsByID(week3Base) {
				got := esRowsByID(week3Prior)[id]
				if math.Abs(got.FinalRaw-want.FinalRaw) > esRowTol {
					t.Errorf("week 3 team %d FinalRaw = %v, want baseline %v", id, got.FinalRaw, want.FinalRaw)
				}
			}
			if got := esSeasonLogs(logs, 2023); len(got) != 0 {
				t.Errorf("%s skip logged %d entries for season 2023, want 0", tc.name, len(got))
			}

			// The earlier historical week-2 window still blends.
			week2Base, err := u.rankingForWeek(2023, 2, nil)
			if err != nil {
				t.Fatalf("week 2 baseline: %v", err)
			}
			week2Run := newPriorRun(esObservedClock(), true)
			week2Prior, err := u.rankingForWeek(2023, 2, week2Run)
			if err != nil {
				t.Fatalf("week 2 with prior: %v", err)
			}
			blended := 0
			for id, want := range esRowsByID(week2Base) {
				got := esRowsByID(week2Prior)[id]
				if want.Fbs && math.Abs(got.FinalRaw-want.FinalRaw) > esRowTol {
					blended++
				}
			}
			if blended == 0 {
				t.Fatal("week 2 stayed entirely on baseline")
			}
		})
	}
}

// TestInlineRebuild_BasketballIsolation verifies the opt-in rebuild is
// football-only: the basketball updater refuses it, and the basketball
// scheduled path never blends even when a stored football prior exists.
func TestInlineRebuild_BasketballIsolation(t *testing.T) {
	bu := newBasketballTestUpdater(t)
	err := bu.UpdateAllRankingsWithPrior()
	if err == nil {
		t.Fatal("basketball rebuild with prior succeeded; want an error")
	}
	if !strings.Contains(err.Error(), "football updater required") {
		t.Errorf("basketball rebuild error = %v, want 'football updater required'", err)
	}

	// Basketball scheduled path stays baseline.
	bu2 := newBasketballTestUpdater(t)
	seedBasketballTeamsAndSeasons(t, bu2.db)
	seedBasketballGames(t, bu2.db)
	if err := bu2.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, bu2.db)
	if err := bu2.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, bu2.db))
}

// TestInlineRebuild_NoTargetOrFutureGameLeak verifies the defensive input
// check on the reconstruction path: any target-season or post-anchor game in
// the loaded donor window is refused (exact baseline), never folded into the
// prior.
func TestInlineRebuild_NoTargetOrFutureGameLeak(t *testing.T) {
	anchor, err := fbsprior.AnchorInstantFor(2023)
	if err != nil {
		t.Fatalf("AnchorInstantFor: %v", err)
	}
	clean := &fbsFreezeInput{
		Season: 2023,
		Games: []ranking.Game{
			{GameID: 1, Season: 2022, StartTime: anchor.Add(-time.Hour)},
			{GameID: 2, Season: 2021, StartTime: anchor.Add(-2 * time.Hour)},
		},
	}
	if err := verifyNoTargetOrFutureGames(clean, 2023, anchor); err != nil {
		t.Errorf("clean donor input refused: %v", err)
	}

	for _, leak := range []ranking.Game{
		{GameID: 3, Season: 2023, StartTime: anchor.Add(-time.Hour)},
		{GameID: 4, Season: 2024, StartTime: anchor.Add(-time.Hour)},
		{GameID: 5, Season: 2022, StartTime: anchor.Add(time.Hour)},
	} {
		bad := &fbsFreezeInput{Season: 2023, Games: append([]ranking.Game(nil), clean.Games...)}
		bad.Games = append(bad.Games, leak)
		if err := verifyNoTargetOrFutureGames(bad, 2023, anchor); err == nil {
			t.Errorf("leaked game %d (season %d, start %s) was not refused",
				leak.GameID, leak.Season, leak.StartTime)
		}
	}
}
