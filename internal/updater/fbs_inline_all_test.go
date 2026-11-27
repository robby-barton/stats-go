package updater

import (
	"errors"
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
//
//nolint:unparam // keyed by season for call-site clarity
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

	// The inline rebuild replaces the frozen row with its freshly computed
	// rebuilt artifact.
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

	// The failed 2021/2022 attempts staged nothing, and the authoritative
	// rebuild replaced the frozen 2023 row with a valid rebuilt artifact.
	if got := esPriorRowCount(t, u.db); got != 1 {
		t.Errorf("fbs_season_priors rows = %d, want 1", got)
	}
	published := storedPriorRow(t, u.db, 2023)
	if published.Artifact == artifactBefore {
		t.Error("the rebuild did not replace the frozen artifact")
	}
	rebuilt, err := fbsprior.Parse([]byte(published.Artifact))
	if err != nil {
		t.Fatalf("published artifact does not parse: %v", err)
	}
	if published.Checksum != sha256Sum([]byte(published.Artifact)) {
		t.Error("published checksum does not match the stored bytes")
	}
	if !rebuilt.RebuiltAt.Equal(esObservedClock()) {
		t.Errorf("rebuilt at = %s, want the observed rebuild clock", rebuilt.RebuiltAt)
	}
	if rebuilt.FreezeDate != "" || !rebuilt.CapturedAt.Equal(esObservedClock()) {
		t.Errorf("rebuilt capture metadata = %q/%s, want no fabricated August capture and the rebuild instant",
			rebuilt.FreezeDate, rebuilt.CapturedAt)
	}
	if rebuilt.Anchor != "2023-08-10" ||
		!rebuilt.CalendarExpiry.Equal(time.Date(2023, 10, 5, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("rebuilt anchor/expiry = %s/%s", rebuilt.Anchor, rebuilt.CalendarExpiry)
	}

	// The rebuild reconstructed the 2023 prior in memory (the current
	// Eastern season) despite a valid stored row being present.
	if esReconstructedLogCount(logs, 2023) == 0 {
		t.Error("the rebuild did not reconstruct the 2023 prior in memory")
	}
}

// TestInlineRebuild_ReplacesStoredRowsWithoutReusingThem verifies the
// rebuild never uses stored prior values as inputs — a different but valid
// saved set of scores changes nothing — and that, as the authoritative
// rebuild, it replaces both a valid and a corrupt stored row with the
// freshly computed rebuilt artifact. With the prior store unavailable the
// whole publication aborts and the rebuilt rankings roll back.
func TestInlineRebuild_ReplacesStoredRowsWithoutReusingThem(t *testing.T) {
	u := newTestUpdater(t)
	esRebuildFixture(t, u)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// Deliberately different but valid stored strengths. A rebuild that
	// used them would disagree with the recomputed run below.
	seedEspPrior(t, u.db, 2023, func(out *fbsprior.FreezeOutput) {
		out.ZStrength = map[int64]float64{1: -5, 2: 4, 3: -3, 4: 2}
	})
	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior (different stored strengths): %v", err)
	}
	withValid := storedRows(t, u.db)
	if esBlendedCount(baseline, withValid, 2023) == 0 {
		t.Fatal("rebuild stayed entirely on baseline")
	}
	published := storedPriorRow(t, u.db, 2023)
	validPrior, err := fbsprior.Parse([]byte(published.Artifact))
	if err != nil {
		t.Fatalf("published artifact does not parse: %v", err)
	}
	if published.Checksum != sha256Sum([]byte(published.Artifact)) {
		t.Error("published checksum does not match the stored bytes")
	}
	if validPrior.FreezeDate != "" || validPrior.RebuiltAt.IsZero() {
		t.Errorf("published artifact = freeze %q rebuilt %s, want a rebuilt artifact",
			validPrior.FreezeDate, validPrior.RebuiltAt)
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
	// The corrupt row was replaced in place by the authoritative rebuild.
	repaired := storedPriorRow(t, u.db, 2023)
	if _, err := fbsprior.Parse([]byte(repaired.Artifact)); err != nil {
		t.Fatalf("corrupt row was not replaced with a valid artifact: %v", err)
	}
	if repaired.Checksum != sha256Sum([]byte(repaired.Artifact)) {
		t.Error("repaired checksum does not match the stored bytes")
	}

	// Without the prior store the whole publication aborts and the rebuilt
	// rankings roll back: nothing is half-rebuilt.
	if err := u.db.Migrator().DropTable(&database.FBSSeasonPrior{}); err != nil {
		t.Fatalf("drop fbs_season_priors: %v", err)
	}
	if err := u.UpdateAllRankingsWithPrior(); err == nil {
		t.Fatal("rebuild succeeded without the prior store")
	}
	esAssertSameRows(t, withCorrupt, storedRows(t, u.db), "store-unavailable rollback")
}

// TestInlineRebuild_RefreshesCorrectedDonorResults verifies the rebuild
// recomputes the prior from the current stored donor data and publishes the
// refreshed artifact over the one frozen from the pre-correction data.
func TestInlineRebuild_RefreshesCorrectedDonorResults(t *testing.T) {
	u := newTestUpdater(t)
	seedTeamsAndSeasons(t, u.db)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
	stubNow(t, freezeClock())
	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
		t.Fatalf("freezeFBSPrior: %v", err)
	}
	frozen := storedPriorRow(t, u.db, 2023)
	frozenPrior, err := fbsprior.Parse([]byte(frozen.Artifact))
	if err != nil {
		t.Fatalf("frozen artifact does not parse: %v", err)
	}

	// Correct a donor result: team 4's 2022 loss becomes a win.
	if err := u.db.Model(&database.Game{}).Where("game_id = ?", int64(402204)).
		Updates(map[string]any{"home_score": 6, "away_score": 34}).Error; err != nil {
		t.Fatalf("correct donor result: %v", err)
	}

	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior: %v", err)
	}
	published := storedPriorRow(t, u.db, 2023)
	if published.Artifact == frozen.Artifact {
		t.Fatal("the rebuild did not refresh the prior from the corrected donor results")
	}
	refreshed, err := fbsprior.Parse([]byte(published.Artifact))
	if err != nil {
		t.Fatalf("published artifact does not parse: %v", err)
	}
	if refreshed.ZStrength[4] <= frozenPrior.ZStrength[4] {
		t.Errorf("refreshed team 4 strength = %v, want above the frozen %v (its corrected win was folded in)",
			refreshed.ZStrength[4], frozenPrior.ZStrength[4])
	}
}

// TestInlineRebuild_PublishesCurrentSeasonAndSchedulerSeesIt verifies the
// rebuild publishes the freshly computed current-season prior midseason —
// no stored row needed — and that the scheduler reads the published prior on
// its next run and blends the same numbers.
func TestInlineRebuild_PublishesCurrentSeasonAndSchedulerSeesIt(t *testing.T) {
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
	if got := esPriorRowCount(t, u.db); got != 1 {
		t.Errorf("fbs_season_priors rows = %d, want 1 (the published 2023 prior)", got)
	}
	published := storedPriorRow(t, u.db, 2023)
	rebuilt, err := fbsprior.Parse([]byte(published.Artifact))
	if err != nil {
		t.Fatalf("published artifact does not parse: %v", err)
	}
	if rebuilt.RebuiltAt.IsZero() || esReconstructedLogCount(logs, 2023) == 0 {
		t.Error("the rebuild did not reconstruct and publish the 2023 prior")
	}

	// Scheduler visibility: the next scheduled run reads the published
	// prior and blends the same numbers the rebuild computed.
	if err := u.db.Where("1 = 1").Delete(&database.TeamWeekResult{}).Error; err != nil {
		t.Fatalf("clear rows: %v", err)
	}
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	scheduled := storedRows(t, u.db)
	if esBlendedCount(baseline, scheduled, 2023) == 0 {
		t.Fatal("the scheduler did not see the published rebuilt prior")
	}
	for key, want := range scheduled {
		gotRow, ok := got[key]
		if !ok {
			t.Fatalf("rebuild row %v missing for the scheduler comparison", key)
		}
		if math.Abs(gotRow.FinalRaw-want.FinalRaw) > esRowTol {
			t.Errorf("scheduled vs rebuild row %v FinalRaw = %v, want %v", key, gotRow.FinalRaw, want.FinalRaw)
		}
	}
}

// TestInlineRebuild_PublishesCurrentSeasonPregame verifies the rebuild
// publishes the current Eastern season's prior even before its first game —
// its football metadata exists — and that the scheduler can blend it once
// the season's games are persisted.
func TestInlineRebuild_PublishesCurrentSeasonPregame(t *testing.T) {
	u := newTestUpdater(t)
	seedTeamsAndSeasons(t, u.db)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
	// Pregame: after the fixed August 10 anchor, before the first game.
	pregame := time.Date(2023, 8, 20, 12, 0, 0, 0, time.UTC)
	stubNow(t, pregame)

	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior (pregame): %v", err)
	}
	if got := esPriorRowCount(t, u.db); got != 1 {
		t.Fatalf("fbs_season_priors rows = %d, want 1 (the published 2023 prior)", got)
	}
	published := storedPriorRow(t, u.db, 2023)
	rebuilt, err := fbsprior.Parse([]byte(published.Artifact))
	if err != nil {
		t.Fatalf("published artifact does not parse: %v", err)
	}
	if !rebuilt.RebuiltAt.Equal(pregame) {
		t.Errorf("rebuilt at = %s, want the pregame rebuild clock", rebuilt.RebuiltAt)
	}

	// Once the season's games exist, the scheduler reads the published
	// prior and blends.
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	if esBlendedCount(baseline, storedRows(t, u.db), 2023) == 0 {
		t.Fatal("the scheduler did not blend the pregame-published prior")
	}
}

// TestInlineRebuild_InvalidatesStalePriorWhenReconstructionFails verifies
// that a reconstruction attempted but unable to yield a usable prior warns,
// publishes the exact baseline for the affected windows, and invalidates
// that season's stale persisted prior in the same publication — after which
// the scheduler stays on baseline too.
func TestInlineRebuild_InvalidatesStalePriorWhenReconstructionFails(t *testing.T) {
	logger, logs := esAllLevelLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	esRebuildFixture(t, u)
	stubNow(t, esObservedClock())

	// Break the 2023 reconstruction input: no usable retained games in the
	// season-2 donor window.
	if err := u.db.Where("season = 2021 and sport = 'ncaaf'").Delete(&database.Game{}).Error; err != nil {
		t.Fatalf("break donor season: %v", err)
	}

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, nil)
	if err := u.UpdateAllRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateAllRankingsWithPrior: %v", err)
	}
	// The affected 2023 windows (including the latest slot in the rebuild's
	// rows) stayed at the exact baseline.
	rebuildRows := storedRows(t, u.db)
	for key, want := range baseline {
		gotRow, ok := rebuildRows[key]
		if !ok {
			t.Fatalf("rebuild row %v missing", key)
		}
		if math.Abs(gotRow.FinalRaw-want.FinalRaw) > esRowTol {
			t.Errorf("rebuild row %v FinalRaw = %v, want baseline %v", key, gotRow.FinalRaw, want.FinalRaw)
		}
	}
	if got := esPriorRowCount(t, u.db); got != 0 {
		t.Errorf("fbs_season_priors rows = %d, want 0 (the stale prior was invalidated)", got)
	}
	warned := false
	for _, entry := range esSeasonLogs(logs, 2023) {
		if strings.Contains(entry.Message, "reconstructing the prior") {
			warned = true
		}
	}
	if !warned {
		t.Error("the failed reconstruction did not warn for season 2023")
	}

	// The scheduler sees the invalidated state: no row, exact baseline.
	if err := u.db.Where("1 = 1").Delete(&database.TeamWeekResult{}).Error; err != nil {
		t.Fatalf("clear rows: %v", err)
	}
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
}

// TestInlineRebuild_PublicationRollsBackOnWriteFailure verifies the whole
// publication is atomic: a ranking write failure rolls the prior publication
// back, and an unavailable prior store aborts before anything is written.
func TestInlineRebuild_PublicationRollsBackOnWriteFailure(t *testing.T) {
	t.Run("ranking write failure rolls the prior publication back", func(t *testing.T) {
		u := newTestUpdater(t)
		esRebuildFixture(t, u)
		stubNow(t, esObservedClock())
		seedEspPrior(t, u.db, 2023, nil)
		stale := storedPriorRow(t, u.db, 2023)

		// Fail after the lock and prior upserts, not at lock acquisition.
		if err := u.db.Exec(`CREATE TRIGGER reject_rebuilt_rankings BEFORE INSERT ON team_week_results
			BEGIN SELECT RAISE(ABORT, 'fixture ranking write failure'); END`).Error; err != nil {
			t.Fatalf("install write failure: %v", err)
		}
		err := u.UpdateAllRankingsWithPrior()
		if err == nil || !strings.Contains(err.Error(), "fixture ranking write failure") {
			t.Fatalf("rebuild did not return the ranking write failure: %v", err)
		}
		// The stale prior survived: the prior publication rolled back with
		// the failed ranking write.
		if got := storedPriorRow(t, u.db, 2023); got.Artifact != stale.Artifact || got.Checksum != stale.Checksum {
			t.Error("the prior publication was not rolled back with the ranking write")
		}
	})

	t.Run("unavailable prior store aborts the whole publication", func(t *testing.T) {
		u := newTestUpdater(t)
		esRebuildFixture(t, u)
		stubNow(t, esObservedClock())
		if err := u.UpdateRecentRankings(); err != nil {
			t.Fatalf("UpdateRecentRankings: %v", err)
		}
		before := storedRows(t, u.db)

		if err := u.db.Migrator().DropTable(&database.FBSSeasonPrior{}); err != nil {
			t.Fatalf("drop fbs_season_priors: %v", err)
		}
		if err := u.UpdateAllRankingsWithPrior(); err == nil {
			t.Fatal("rebuild succeeded without the prior store")
		}
		esAssertSameRows(t, before, storedRows(t, u.db), "store-unavailable rollback")
		if err := u.UpdateRecentRankingsWithPrior(); err != nil {
			t.Fatalf("scheduled baseline without the prior table: %v", err)
		}
		esAssertSameRows(t, before, storedRows(t, u.db), "scheduled baseline without prior store")
	})

	t.Run("input database failure is not grounds to invalidate an existing prior", func(t *testing.T) {
		u := newTestUpdater(t)
		esRebuildFixture(t, u)
		stubNow(t, esObservedClock())
		seedEspPrior(t, u.db, 2023, nil)
		stale := storedPriorRow(t, u.db, 2023)
		if err := u.db.Migrator().DropTable(&database.TeamName{}); err != nil {
			t.Fatalf("drop team_names: %v", err)
		}
		if err := u.UpdateAllRankingsWithPrior(); !errors.Is(err, errFBSPriorRead) {
			t.Fatalf("expected the donor input database error, got %v", err)
		}
		if got := storedPriorRow(t, u.db, 2023); got != stale {
			t.Fatal("an input database error should not invalidate a prior")
		}
	})
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
