package updater

import (
	"math"
	"sort"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
)

// ---------------------------------------------------------------------------
// Clock stub
// ---------------------------------------------------------------------------

// esObservedClock is inside the eligible window: after the latest fixture
// kickoff (2023-09-09) and before the day-56 expiry (2023-10-05).
func esObservedClock() time.Time {
	return time.Date(2023, 9, 12, 0, 0, 0, 0, time.UTC)
}

// stubNow pins the observed snapshot clock for the duration of the test.
func stubNow(t *testing.T, at time.Time) {
	t.Helper()
	prev := nowFunc
	nowFunc = func() time.Time { return at }
	t.Cleanup(func() { nowFunc = prev })
}

// esUniqueStartTimes breaks the seed fixture's tied start times: SQLite
// returns tied rows in unspecified order, which perturbs baseline float
// accumulation between runs and would break exact baseline comparisons.
func esUniqueStartTimes(t *testing.T, db *gorm.DB) {
	t.Helper()
	updates := map[int64]time.Time{
		fixtureGameID2: time.Date(2023, 9, 2, 23, 30, 0, 0, time.UTC),
		fixtureGameID5: time.Date(2023, 9, 9, 23, 30, 0, 0, time.UTC),
		501002:         time.Date(2023, 9, 9, 20, 30, 0, 0, time.UTC),
	}
	for id, at := range updates {
		if err := db.Model(&database.Game{}).
			Where("game_id = ?", id).
			Update("start_time", at).Error; err != nil {
			t.Fatalf("unique start time for game %d: %v", id, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Stored prior row fixture
// ---------------------------------------------------------------------------

// esPriorField returns the fixture FBS field rows matching team_names.
func esPriorField() []fbsprior.FieldRow {
	return []fbsprior.FieldRow{
		{TeamID: 1, TeamName: "Alpha"},
		{TeamID: 2, TeamName: "Beta"},
		{TeamID: 3, TeamName: "Gamma"},
		{TeamID: 4, TeamName: "Delta"},
	}
}

// esPriorZStrength returns an arbitrary nonzero strength map over the fixture
// FBS field; the expected values are derived from the baseline run, not here.
func esPriorZStrength() map[int64]float64 {
	return map[int64]float64{1: 1.2, 2: -0.8, 3: 0.4, 4: -0.9}
}

// esPriorSeasonFixture builds a valid freeze output for season 2023 inside
// the August 10-16 Eastern window (anchor 2023-08-10T04:00Z, expiry
// 2023-10-05T04:00Z).
func esPriorSeasonFixture(mutate func(*fbsprior.FreezeOutput)) fbsprior.FreezeOutput {
	out := fbsprior.FreezeOutput{
		Season:      2023,
		FreezeDate:  "2023-08-12",
		AsOf:        time.Date(2023, 8, 12, 12, 0, 0, 0, time.UTC),
		CapturedAt:  time.Date(2023, 8, 12, 13, 0, 0, 0, time.UTC),
		Field:       esPriorField(),
		ZStrength:   esPriorZStrength(),
		InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	if mutate != nil {
		mutate(&out)
	}
	return out
}

// seedEspPrior encodes a freeze output through the production builder and
// stores the resulting immutable row; it returns the parsed prior.
func seedEspPrior(t *testing.T, db *gorm.DB, season int64, mutate func(*fbsprior.FreezeOutput)) *fbsprior.Prior {
	t.Helper()
	data, prior, err := fbsprior.Build(esPriorSeasonFixture(mutate))
	if err != nil {
		t.Fatalf("build prior fixture: %v", err)
	}
	if err := db.Create(&database.FBSSeasonPrior{
		Season:   season,
		Artifact: string(data),
		Checksum: prior.SHA256,
	}).Error; err != nil {
		t.Fatalf("seed stored prior: %v", err)
	}
	return prior
}

// seedEspPriorRaw stores a raw (possibly invalid) payload with the given
// checksum for direct corruption tests.
func seedEspPriorRaw(t *testing.T, db *gorm.DB, season int64, artifact, checksum string) {
	t.Helper()
	if err := db.Create(&database.FBSSeasonPrior{
		Season: season, Artifact: artifact, Checksum: checksum,
	}).Error; err != nil {
		t.Fatalf("seed raw stored prior: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Stored-row helpers
// ---------------------------------------------------------------------------

type rowKey struct {
	TeamID     int64
	Year       int64
	Week       int64
	Postseason int64
	Fbs        bool
}

func storedRows(t *testing.T, db *gorm.DB) map[rowKey]database.TeamWeekResult {
	t.Helper()
	var results []database.TeamWeekResult
	if err := db.Find(&results).Error; err != nil {
		t.Fatalf("query team_week_results: %v", err)
	}
	rows := make(map[rowKey]database.TeamWeekResult, len(results))
	for _, r := range results {
		key := rowKey{r.TeamID, r.Year, r.Week, r.Postseason, r.Fbs}
		if _, dup := rows[key]; dup {
			t.Fatalf("duplicate stored row for %v", key)
		}
		rows[key] = r
	}
	return rows
}

// esRowTol is the comparison tolerance for baseline identity. The baseline
// ranker resolves a Cholesky solve over a map-derived team order, so two runs
// of the identical fixture differ by ~1 ulp and can permute exactly-tied
// SRS/SOS ranks; only that pre-existing noise is tolerated here. The blend's
// own exactness (bypass identity, all-bypass rank preservation) is asserted
// bit-exactly in the fbsprior unit tests.
const esRowTol = 1e-9

// expectBaseline fails the test when the current stored rows differ from the
// given baseline snapshot beyond the ranker's pre-existing run-to-run noise.
func expectBaseline(t *testing.T, baseline, got map[rowKey]database.TeamWeekResult) {
	t.Helper()
	if len(got) != len(baseline) {
		t.Fatalf("stored rows = %d, want %d", len(got), len(baseline))
	}
	for key, want := range baseline {
		gotRow, ok := got[key]
		if !ok {
			t.Fatalf("row %v missing", key)
		}
		if gotRow.Name != want.Name || gotRow.Conf != want.Conf || gotRow.Year != want.Year ||
			gotRow.Week != want.Week || gotRow.Postseason != want.Postseason ||
			gotRow.Wins != want.Wins || gotRow.Losses != want.Losses || gotRow.Ties != want.Ties ||
			math.Abs(gotRow.FinalRaw-want.FinalRaw) > esRowTol {
			t.Errorf("row %v = %+v, want baseline %+v", key, gotRow, want)
		}
	}
	// Sub-ranks are not compared across runs: the ranker's 1-ulp score noise
	// changes exact-tie group membership, which is pre-existing behavior.
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// esObservedClock is inside the eligible window: after the latest fixture
// kickoff (2023-09-09) and before the day-56 expiry (2023-10-05).
// TestEarlySeasonPrior_AppliesBlend seeds an extra zero-game FBS team and an
// FBS-FCS game, then verifies the stored blended rows against the researched
// formula computed from the baseline run, and that FCS rows and the
// non-score columns are untouched.
func TestEarlySeasonPrior_AppliesBlend(t *testing.T) {
	u := newTestUpdater(t)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)

	// Extra FBS team with zero games; must be in the prior field too.
	teamNames := []database.TeamName{
		{TeamID: 7, Name: "Eta", DisplayName: "Eta Panthers", Abbreviation: "ETA",
			Location: "Eta", Slug: "eta", IsActive: true, Sport: "ncaaf"},
	}
	if err := u.db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed extra team_name: %v", err)
	}
	teamSeasons := []database.TeamSeason{{TeamID: 7, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"}}
	if err := u.db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed extra team_season: %v", err)
	}
	// FBS-FCS game: team 1 (FBS) vs team 5 (FCS) — the record count for the
	// blend includes it.
	fbsFcs := database.Game{
		GameID: 401007, Season: 2023, Week: 1, HomeID: 1, AwayID: 5,
		HomeScore: 42, AwayScore: 7, Sport: "ncaaf",
		StartTime: time.Date(2023, 9, 2, 21, 0, 0, 0, time.UTC),
	}
	if err := u.db.Create(&fbsFcs).Error; err != nil {
		t.Fatalf("seed FBS-FCS game: %v", err)
	}

	stubNow(t, esObservedClock())
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("baseline UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	zStrength := map[int64]float64{1: 1.2, 2: -0.8, 3: 0.4, 4: -0.9, 7: 0.0}
	seedEspPrior(t, u.db, 2023, func(out *fbsprior.FreezeOutput) {
		out.Field = append(append([]fbsprior.FieldRow(nil), esPriorField()...),
			fbsprior.FieldRow{TeamID: 7, TeamName: "Eta"})
		out.ZStrength = zStrength
		out.MissingHistory = []int64{7}
	})

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	got := storedRows(t, u.db)
	if len(got) != len(baseline) {
		t.Fatalf("stored rows = %d, want %d", len(got), len(baseline))
	}

	// Recompute the blend from the baseline rows in ascending team-ID order.
	fbsIDs := []int64{1, 2, 3, 4, 7}
	var sum float64
	for _, id := range fbsIDs {
		sum += baseline[rowKey{id, 2023, 3, 0, true}].FinalRaw
	}
	mean := sum / float64(len(fbsIDs))
	var sq float64
	for _, id := range fbsIDs {
		dev := baseline[rowKey{id, 2023, 3, 0, true}].FinalRaw - mean
		sq += dev * dev
	}
	sd := math.Max(math.Sqrt(sq/float64(len(fbsIDs))), fbsprior.SDFloor)

	type expected struct {
		raw  float64
		rank int64
	}
	expectedRows := make(map[int64]*expected, len(fbsIDs))
	for _, id := range fbsIDs {
		base := baseline[rowKey{id, 2023, 3, 0, true}]
		games := base.Wins + base.Losses + base.Ties
		lambda := math.Max(0, 1-float64(games)/float64(fbsprior.ExpirationGames))
		if lambda == 0 {
			expectedRows[id] = &expected{raw: base.FinalRaw, rank: base.FinalRank}
			continue
		}
		z := (base.FinalRaw - mean) / sd
		expectedRows[id] = &expected{
			raw: mean + sd*((1-lambda)*z+lambda*(fbsprior.Rho*zStrength[id])),
		}
	}
	// Competition ranks over the blended scores, ascending team ID ties.
	ids := append([]int64(nil), fbsIDs...)
	sort.Slice(ids, func(i, j int) bool {
		if expectedRows[ids[i]].raw != expectedRows[ids[j]].raw {
			return expectedRows[ids[i]].raw > expectedRows[ids[j]].raw
		}
		return ids[i] < ids[j]
	})
	prev := math.NaN()
	var prevRank int64
	for idx, id := range ids {
		rank := int64(idx + 1)
		if expectedRows[id].raw == prev {
			rank = prevRank
		} else {
			prev = expectedRows[id].raw
			prevRank = rank
		}
		expectedRows[id].rank = rank
	}

	blended := 0
	for _, id := range fbsIDs {
		want := expectedRows[id]
		gotRow := got[rowKey{id, 2023, 3, 0, true}]
		if math.Abs(gotRow.FinalRaw-want.raw) > esRowTol {
			t.Errorf("team %d blended raw = %v, want %v", id, gotRow.FinalRaw, want.raw)
		}
		if gotRow.FinalRank != want.rank {
			t.Errorf("team %d blended rank = %d, want %d", id, gotRow.FinalRank, want.rank)
		}
		// Only FinalRaw/FinalRank are substituted. SRS/SOS ranks can permute
		// exact ties between runs (pre-existing ranker noise); records must
		// not change.
		base := baseline[rowKey{id, 2023, 3, 0, true}]
		if gotRow.Wins != base.Wins || gotRow.Losses != base.Losses || gotRow.Ties != base.Ties {
			t.Errorf("team %d: record columns changed", id)
		}
		if math.Abs(gotRow.FinalRaw-base.FinalRaw) > esRowTol {
			blended++
		}
	}
	if blended == 0 {
		t.Fatal("no FBS row changed; the blend did not apply")
	}

	// FCS rows are untouched by the blend.
	fcsBaseline := map[rowKey]database.TeamWeekResult{}
	fcsGot := map[rowKey]database.TeamWeekResult{}
	for key, row := range baseline {
		if !key.Fbs {
			fcsBaseline[key] = row
			fcsGot[key] = got[key]
		}
	}
	expectBaseline(t, fcsBaseline, fcsGot)
}

// TestEarlySeasonPrior_DisabledMatchesBaseline runs the scheduler path with
// no stored prior and verifies byte-identical stored rows.
func TestEarlySeasonPrior_DisabledMatchesBaseline(t *testing.T) {
	u := newTestUpdater(t)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// No stored row must behave exactly like the one-shot path.
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
}

// TestEarlySeasonPrior_RowCreatedAfterStartupIsUsable verifies the stored
// prior is read fresh on every run: a row frozen after the (simulated)
// startup applies on the next run without any restart.
func TestEarlySeasonPrior_RowCreatedAfterStartupIsUsable(t *testing.T) {
	u := newTestUpdater(t)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	baseline := storedRows(t, u.db)

	// First enabled run before the freeze: baseline, silently.
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("pre-freeze run: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	// The freeze task stores the row (real production builder over the
	// blend fixture so the frozen field matches the loaded one). The row
	// appears after the first enabled run, with no restart in between.
	seedEspPrior(t, u.db, 2023, nil)

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("post-freeze run: %v", err)
	}
	got := storedRows(t, u.db)
	changed := 0
	for key, row := range got {
		if row.FinalRaw-baseline[key].FinalRaw > esRowTol {
			changed++
		}
	}
	if changed == 0 {
		t.Fatal("post-freeze run stayed entirely on baseline")
	}
}

// TestEarlySeasonPrior_WrongSeasonKeyedRowWarnsAndStaysBaseline checks that
// a row keyed by the loaded season that contains another season's payload
// warns and never touches the stored rows.
func TestEarlySeasonPrior_WrongSeasonKeyedRowWarnsAndStaysBaseline(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// Build a valid 2022 payload but store it under the 2023 key.
	seedEspPrior(t, u.db, 2023, func(out *fbsprior.FreezeOutput) {
		out.Season = 2022
		out.FreezeDate = "2022-08-11"
		out.AsOf = time.Date(2022, 8, 11, 12, 0, 0, 0, time.UTC)
		out.CapturedAt = time.Date(2022, 8, 11, 13, 0, 0, 0, time.UTC)
	})

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) == 0 {
		t.Error("wrong-season payload logged no warning")
	}
}

// TestEarlySeasonPrior_FieldDivergenceWarnsAndStaysBaseline verifies the
// exact atomic fallback when the current FBS field diverges from the prior.
func TestEarlySeasonPrior_FieldDivergenceWarnsAndStaysBaseline(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, func(out *fbsprior.FreezeOutput) {
		out.Field = []fbsprior.FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
			{TeamID: 3, TeamName: "Gamma"},
			{TeamID: 4, TeamName: "Renamed"}, // name divergence
		}
	})

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) == 0 {
		t.Error("field-divergence fallback logged no warning")
	}
}

// TestEarlySeasonPrior_CorruptRowWarnsAndStaysBaseline verifies a present
// but unreadable stored row: warn once, publish the exact baseline, and
// never repair or replace the stored row.
func TestEarlySeasonPrior_CorruptRowWarnsAndStaysBaseline(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPriorRaw(t, u.db, 2023, "{not json", "0000")

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) == 0 {
		t.Error("corrupt stored row logged no warning")
	}
	var count int64
	if err := u.db.Model(&database.FBSSeasonPrior{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Error("the corrupt row was not left untouched")
	}
}

// TestEarlySeasonPrior_ChecksumMismatchWarnsAndStaysBaseline verifies the
// exact-byte checksum contract on read.
func TestEarlySeasonPrior_ChecksumMismatchWarnsAndStaysBaseline(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// Store a valid artifact with a wrong checksum.
	data, _, err := fbsprior.Build(esPriorSeasonFixture(nil))
	if err != nil {
		t.Fatal(err)
	}
	seedEspPriorRaw(t, u.db, 2023, string(data), "ffff")

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) == 0 {
		t.Error("checksum mismatch logged no warning")
	}
}

// TestEarlySeasonPrior_FutureKickoffWarnsAndStaysBaseline covers the
// inconsistent-clock fallback: the observed clock precedes the latest game.
func TestEarlySeasonPrior_FutureKickoffWarnsAndStaysBaseline(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC))

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, nil)

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) == 0 {
		t.Error("future-kickoff fallback logged no warning")
	}
}

// TestEarlySeasonPrior_CapturePrecedesObservationWarns covers a prior whose
// capture instant is after the observed clock: baseline with a warning.
func TestEarlySeasonPrior_CapturePrecedesObservationWarns(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	// Observed clock before the fixture capture instant (2023-08-12).
	stubNow(t, time.Date(2023, 8, 11, 0, 0, 0, 0, time.UTC))

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, nil)

	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) == 0 {
		t.Error("capture-after-observation fallback logged no warning")
	}
}

// TestEarlySeasonPrior_CalendarExpiryBypassesSilently verifies the safe
// normal expiry: a kickoff long before day 56 but an observation exactly at
// (and after) the expiry instant leaves the exact baseline, without warnings
// — and without reading the stored row at all.
func TestEarlySeasonPrior_CalendarExpiryBypassesSilently(t *testing.T) {
	// The fixed day-56 expiry for season 2023 is midnight Eastern on
	// 2023-10-05 (04:00Z).
	expiry := time.Date(2023, 10, 5, 4, 0, 0, 0, time.UTC)

	for _, observed := range []time.Time{
		expiry,
		expiry.Add(24 * time.Hour),
	} {
		t.Run(observed.Format(time.RFC3339), func(t *testing.T) {
			logger, logs := esObservedLogger(t)
			u := newTestUpdaterWithLogger(t, logger)
			seedTeamsAndSeasons(t, u.db)
			seedGames(t, u.db)
			stubNow(t, observed)

			if err := u.UpdateRecentRankings(); err != nil {
				t.Fatalf("UpdateRecentRankings: %v", err)
			}
			baseline := storedRows(t, u.db)

			seedEspPrior(t, u.db, 2023, nil)
			if err := u.UpdateRecentRankingsWithPrior(); err != nil {
				t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
			}
			expectBaseline(t, baseline, storedRows(t, u.db))
			if esWarnCount(logs) != 0 {
				t.Errorf("normal expiry logged %d warnings, want 0", esWarnCount(logs))
			}
		})
	}
}

// TestEarlySeasonPrior_PreseasonPhaseSkipsSilently covers a stored preseason
// game (raw phase -1) as the latest persisted game.
func TestEarlySeasonPrior_PreseasonPhaseSkipsSilently(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.db.Model(&database.Game{}).
		Where("game_id = ?", fixtureGameID5).
		Update("postseason", -1).Error; err != nil {
		t.Fatalf("mark preseason: %v", err)
	}

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, nil)
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) != 0 {
		t.Errorf("preseason phase logged %d warnings, want 0", esWarnCount(logs))
	}
}

// TestEarlySeasonPrior_PostseasonSkipsSilently covers a stored postseason
// game as the latest persisted game.
func TestEarlySeasonPrior_PostseasonSkipsSilently(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.db.Model(&database.Game{}).
		Where("game_id = ?", fixtureGameID5).
		Update("postseason", 1).Error; err != nil {
		t.Fatalf("mark postseason: %v", err)
	}

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, nil)
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) != 0 {
		t.Errorf("postseason logged %d warnings, want 0", esWarnCount(logs))
	}
}

// TestEarlySeasonPrior_NoCurrentSeasonSkipsSilently covers a database whose
// latest game is from an older season (no current-season games yet).
func TestEarlySeasonPrior_NoCurrentSeasonSkipsSilently(t *testing.T) {
	logger, logs := esObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())

	if err := u.db.Where("season = 2023 and sport = 'ncaaf'").
		Delete(&database.Game{}).Error; err != nil {
		t.Fatalf("delete current-season games: %v", err)
	}

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2023, nil)
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
	if esWarnCount(logs) != 0 {
		t.Errorf("no-current-season skip logged %d warnings, want 0", esWarnCount(logs))
	}
}

// TestEarlySeasonPrior_BasketballIsolation verifies the basketball scheduled
// path never blends, even when a stored football prior exists.
func TestEarlySeasonPrior_BasketballIsolation(t *testing.T) {
	u := newBasketballTestUpdater(t)
	seedBasketballTeamsAndSeasons(t, u.db)
	seedBasketballGames(t, u.db)

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	seedEspPrior(t, u.db, 2024, nil)
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	expectBaseline(t, baseline, storedRows(t, u.db))
}

// ---------------------------------------------------------------------------
// Log observation helpers
// ---------------------------------------------------------------------------

func esObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	return zap.New(core), logs
}

func esWarnCount(logs *observer.ObservedLogs) int {
	return len(logs.FilterMessageSnippet("fbs early-season prior").All())
}
