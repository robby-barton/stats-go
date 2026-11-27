package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
// Donor-season fixture (target 2023, donors 2021/2022)
// ---------------------------------------------------------------------------

// freezeClock is an in-window observed clock: Monday 2023-08-14 13:00Z
// (09:00 EDT), inside August 10-16 Eastern.
func freezeClock() time.Time {
	return time.Date(2023, 8, 14, 13, 0, 0, 0, time.UTC)
}

// seedDonorSeasons inserts prior-year FBS/FCS metadata for the two donor
// seasons 2021 and 2022. Teams 1-4 are the continuing FBS members; teams 5-6
// remain FCS.
func seedDonorSeasons(t *testing.T, db *gorm.DB) {
	t.Helper()
	var seasons []database.TeamSeason
	for _, year := range []int64{2021, 2022} {
		seasons = append(seasons,
			database.TeamSeason{TeamID: 1, Year: year, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
			database.TeamSeason{TeamID: 2, Year: year, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
			database.TeamSeason{TeamID: 3, Year: year, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
			database.TeamSeason{TeamID: 4, Year: year, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
			database.TeamSeason{TeamID: 5, Year: year, FBS: 0, Conf: "MVFC", Sport: "ncaaf"},
			database.TeamSeason{TeamID: 6, Year: year, FBS: 0, Conf: "MVFC", Sport: "ncaaf"},
		)
	}
	if err := db.Create(&seasons).Error; err != nil {
		t.Fatalf("seed donor seasons: %v", err)
	}
}

// seedDonorGames inserts completed donor games between the continuing FBS
// teams in both donor seasons, with unique start times for determinism.
// Team 1 dominates, team 4 loses throughout.
func seedDonorGames(t *testing.T, db *gorm.DB) {
	t.Helper()
	games := []database.Game{
		// 2021 (year-2): two weekends.
		{GameID: 402101, Season: 2021, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 42, AwayScore: 10,
			Sport: "ncaaf", StartTime: time.Date(2021, 9, 4, 17, 0, 0, 0, time.UTC)},
		{GameID: 402102, Season: 2021, Week: 1, HomeID: 3, AwayID: 4, HomeScore: 31, AwayScore: 7,
			Sport: "ncaaf", StartTime: time.Date(2021, 9, 4, 20, 0, 0, 0, time.UTC)},
		{GameID: 402103, Season: 2021, Week: 5, HomeID: 1, AwayID: 3, HomeScore: 35, AwayScore: 14,
			Sport: "ncaaf", StartTime: time.Date(2021, 10, 2, 17, 0, 0, 0, time.UTC)},
		{GameID: 402104, Season: 2021, Week: 5, HomeID: 4, AwayID: 2, HomeScore: 7, AwayScore: 28,
			Sport: "ncaaf", StartTime: time.Date(2021, 10, 2, 20, 0, 0, 0, time.UTC)},
		// 2022 (year-1).
		{GameID: 402201, Season: 2022, Week: 1, HomeID: 2, AwayID: 1, HomeScore: 10, AwayScore: 38,
			Sport: "ncaaf", StartTime: time.Date(2022, 9, 3, 17, 0, 0, 0, time.UTC)},
		{GameID: 402202, Season: 2022, Week: 1, HomeID: 4, AwayID: 3, HomeScore: 3, AwayScore: 27,
			Sport: "ncaaf", StartTime: time.Date(2022, 9, 3, 20, 0, 0, 0, time.UTC)},
		{GameID: 402203, Season: 2022, Week: 6, HomeID: 3, AwayID: 1, HomeScore: 10, AwayScore: 31,
			Sport: "ncaaf", StartTime: time.Date(2022, 10, 8, 17, 0, 0, 0, time.UTC)},
		{GameID: 402204, Season: 2022, Week: 6, HomeID: 2, AwayID: 4, HomeScore: 24, AwayScore: 6,
			Sport: "ncaaf", StartTime: time.Date(2022, 10, 8, 20, 0, 0, 0, time.UTC)},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed donor games: %v", err)
	}
}

// seedFreezeFixture assembles a complete freezable database: target-year
// metadata (seedTeamsAndSeasons), donor metadata, and donor games.
func seedFreezeFixture(t *testing.T, u *Updater) {
	t.Helper()
	seedTeamsAndSeasons(t, u.db)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
}

// storedPriorRow returns the single stored prior row for a season.
//
//nolint:unparam // keyed by season for call-site clarity
func storedPriorRow(t *testing.T, db *gorm.DB, season int64) database.FBSSeasonPrior {
	t.Helper()
	var row database.FBSSeasonPrior
	if err := db.Where("season = ?", season).First(&row).Error; err != nil {
		t.Fatalf("query stored prior for %d: %v", season, err)
	}
	return row
}

// ---------------------------------------------------------------------------
// Freeze tests
// ---------------------------------------------------------------------------

// TestFreezeFBSPrior_CreatesFrozenRow runs the freeze against a complete
// fixture and verifies the stored artifact contract, the fixed anchor and
// expiry, the observed capture date/instant, and the strength ordering.
func TestFreezeFBSPrior_CreatesFrozenRow(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())

	now := freezeClock()
	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, now); err != nil {
		t.Fatalf("freezeFBSPrior: %v", err)
	}

	row := storedPriorRow(t, u.db, 2023)
	prior, err := fbsprior.Parse([]byte(row.Artifact))
	if err != nil {
		t.Fatalf("stored artifact does not parse: %v", err)
	}
	sum := sha256Sum([]byte(row.Artifact))
	if row.Checksum != sum {
		t.Errorf("stored checksum %q != digest %q", row.Checksum, sum)
	}
	if prior.SHA256 != sum {
		t.Errorf("parsed digest %q != digest %q", prior.SHA256, sum)
	}
	if prior.Season != 2023 || prior.Anchor != "2023-08-10" || prior.FreezeDate != "2023-08-14" {
		t.Errorf("season/anchor/freeze = %d/%s/%s", prior.Season, prior.Anchor, prior.FreezeDate)
	}
	if !prior.AnchorInstant.Equal(time.Date(2023, 8, 10, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("anchor instant = %s", prior.AnchorInstant)
	}
	if !prior.CalendarExpiry.Equal(time.Date(2023, 10, 5, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("calendar expiry = %s", prior.CalendarExpiry)
	}
	if !prior.CapturedAt.Equal(freezeClock()) {
		t.Errorf("captured at = %s, want the observed instant", prior.CapturedAt)
	}
	// Target field: the four FBS teams, sorted.
	if len(prior.Field) != 4 || prior.Field[0].TeamID != 1 || prior.Field[3].TeamID != 4 {
		t.Errorf("field = %v", prior.Field)
	}
	// Strength ordering: dominant team 1 positive, winless team 4 lowest.
	if prior.ZStrength[1] <= 0 || prior.ZStrength[4] >= 0 ||
		prior.ZStrength[4] >= prior.ZStrength[1] {
		t.Errorf("strength ordering wrong: %v", prior.ZStrength)
	}
	if len(prior.MissingHistory) != 0 {
		t.Errorf("missing history = %v, want empty (all teams covered)", prior.MissingHistory)
	}
}

// TestFreezeFBSPrior_NewcomersImputed seeds an extra 2023 FBS team absent
// from both donor seasons; the frozen prior must cover it with an exactly
// zero z-score and list it in MissingHistory.
func TestFreezeFBSPrior_NewcomersImputed(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	teamNames := []database.TeamName{
		{TeamID: 7, Name: "Eta", DisplayName: "Eta Panthers", Abbreviation: "ETA",
			Location: "Eta", Slug: "eta", IsActive: true, Sport: "ncaaf"},
	}
	if err := u.db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed newcomer: %v", err)
	}
	teamSeasons := []database.TeamSeason{{TeamID: 7, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"}}
	if err := u.db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed newcomer season: %v", err)
	}
	stubNow(t, freezeClock())

	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
		t.Fatalf("freezeFBSPrior: %v", err)
	}

	prior, err := fbsprior.Parse([]byte(storedPriorRow(t, u.db, 2023).Artifact))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prior.Field) != 5 || prior.Field[4].TeamID != 7 {
		t.Errorf("field = %v, want the newcomer included and sorted", prior.Field)
	}
	if prior.ZStrength[7] != 0 {
		t.Errorf("newcomer z = %v, want exactly 0", prior.ZStrength[7])
	}
	if len(prior.MissingHistory) != 1 || prior.MissingHistory[0] != 7 {
		t.Errorf("missing history = %v, want [7]", prior.MissingHistory)
	}
}

// TestFreezeFBSPrior_ImmutableRepeat verifies repeat attempts never replace
// the stored row: a second, differently dated attempt leaves the exact bytes.
func TestFreezeFBSPrior_ImmutableRepeat(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())

	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
		t.Fatalf("first freeze: %v", err)
	}
	first := storedPriorRow(t, u.db, 2023)

	// A later in-window attempt (Aug 16) must not touch the stored row.
	later := time.Date(2023, 8, 16, 15, 0, 0, 0, time.UTC)
	stubNow(t, later)
	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, later); err != nil {
		t.Fatalf("repeat freeze: %v", err)
	}
	second := storedPriorRow(t, u.db, 2023)
	if second.Artifact != first.Artifact || second.Checksum != first.Checksum {
		t.Error("repeat attempt replaced the stored row")
	}
}

// TestFreezeFBSPrior_CorruptRowIsNeverReplaced checks that an existing but
// invalid stored row blocks the freeze (immutability) and stays untouched.
func TestFreezeFBSPrior_CorruptRowIsNeverReplaced(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())

	corrupt := database.FBSSeasonPrior{Season: 2023, Artifact: "{not json", Checksum: "bad"}
	if err := u.db.Create(&corrupt).Error; err != nil {
		t.Fatalf("seed corrupt row: %v", err)
	}

	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
		t.Fatalf("freeze with a corrupt row present: %v", err)
	}
	row := storedPriorRow(t, u.db, 2023)
	if row.Artifact != "{not json" || row.Checksum != "bad" {
		t.Error("corrupt stored row was replaced")
	}
}

// TestFreezeFBSPrior_OutsideWindowRefused covers the window guard: any date
// outside August 10-16 Eastern refuses, and nothing is stored.
func TestFreezeFBSPrior_OutsideWindowRefused(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)

	for _, at := range []time.Time{
		time.Date(2023, 8, 9, 23, 59, 0, 0, time.UTC), // Aug 9 EDT
		time.Date(2023, 8, 17, 4, 0, 0, 0, time.UTC),  // Aug 17 ET
		time.Date(2023, 9, 12, 0, 0, 0, 0, time.UTC),  // midseason
		time.Date(2022, 8, 14, 13, 0, 0, 0, time.UTC), // prior year
	} {
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, at); err == nil {
			t.Errorf("freeze at %s succeeded", at)
		}
		var count int64
		if err := u.db.Model(&database.FBSSeasonPrior{}).Where("season = 2023").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("a row was stored by an out-of-window attempt")
		}
	}
}

// TestFreezeFBSPrior_GamesAlreadyPersistedRefused covers the midseason
// guard: any persisted football game for the target season — regular,
// preseason phase, postseason, FCS — refuses the freeze, as does an
// unclassified retry placeholder (season=0).
func TestFreezeFBSPrior_GamesAlreadyPersistedRefused(t *testing.T) {
	tests := []struct {
		name string
		game database.Game
	}{
		{
			name: "regular season FBS game",
			game: database.Game{GameID: 401501, Season: 2023, Week: 1, HomeID: 1, AwayID: 2,
				HomeScore: 28, AwayScore: 14, Sport: "ncaaf",
				StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC)},
		},
		{
			name: "preseason phase game",
			game: database.Game{GameID: 401502, Season: 2023, Week: 0, Postseason: -1, HomeID: 1, AwayID: 2,
				HomeScore: 21, AwayScore: 10, Sport: "ncaaf",
				StartTime: time.Date(2023, 8, 26, 23, 0, 0, 0, time.UTC)},
		},
		{
			name: "postseason FCS game",
			game: database.Game{GameID: 401503, Season: 2023, Week: 1, Postseason: 1, HomeID: 5, AwayID: 6,
				HomeScore: 17, AwayScore: 10, Sport: "ncaaf",
				StartTime: time.Date(2023, 12, 16, 20, 0, 0, 0, time.UTC)},
		},
		{
			name: "unclassified retry placeholder (season 0)",
			game: database.Game{GameID: 401504, Retry: 1, Sport: "ncaaf"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newTestUpdater(t)
			seedFreezeFixture(t, u)
			stubNow(t, freezeClock())
			if err := u.db.Create(&tt.game).Error; err != nil {
				t.Fatalf("seed refusing game: %v", err)
			}

			if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
				t.Fatal("freeze succeeded with persisted games")
			} else if !strings.Contains(err.Error(), "already persisted") {
				t.Errorf("unexpected error: %v", err)
			}
			var count int64
			if err := u.db.Model(&database.FBSSeasonPrior{}).Where("season = 2023").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Error("a row was stored despite persisted games")
			}
		})
	}
}

// TestFreezeFBSPrior_InputRefusals covers the conservative refusal cases:
// missing donor metadata, an unsupported FBS departure, an empty donor
// season, a continuing member without coverage, a degenerate field, and an
// oversized field.
func TestFreezeFBSPrior_InputRefusals(t *testing.T) {
	setup := func(t *testing.T) *Updater {
		u := newTestUpdater(t)
		seedFreezeFixture(t, u)
		stubNow(t, freezeClock())
		return u
	}

	t.Run("missing target metadata", func(t *testing.T) {
		u := setup(t)
		if err := u.db.Where("year = 2023 and sport = 'ncaaf'").
			Delete(&database.TeamSeason{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded without target metadata")
		}
	})

	t.Run("target newcomer missing a name row", func(t *testing.T) {
		u := setup(t)
		if err := u.db.Create(&database.TeamSeason{TeamID: 700, Year: 2023, Sport: "ncaaf", FBS: 1}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil ||
			!strings.Contains(err.Error(), "team 700 has no name") {
			t.Fatalf("expected the unnamed FBS team to block the freeze, got %v", err)
		}
	})

	t.Run("missing donor metadata", func(t *testing.T) {
		u := setup(t)
		// The prior-year (2022) field drives the departure and coverage
		// checks; without it the input is unusable.
		if err := u.db.Where("year = 2022 and sport = 'ncaaf'").
			Delete(&database.TeamSeason{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded without donor metadata")
		}
	})

	t.Run("unsupported departure from the prior-year field", func(t *testing.T) {
		u := setup(t)
		if err := u.db.Where("year = 2023 and team_id = 4 and sport = 'ncaaf'").
			Delete(&database.TeamSeason{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded with a departed FBS team")
		} else if !strings.Contains(err.Error(), "departure") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty donor season", func(t *testing.T) {
		u := setup(t)
		if err := u.db.Where("season = 2021 and sport = 'ncaaf'").
			Delete(&database.Game{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded with an empty donor season")
		}
	})

	t.Run("continuing member without coverage", func(t *testing.T) {
		u := setup(t)
		if err := u.db.Where("sport = 'ncaaf' and (home_id = 4 or away_id = 4) and season <= 2022").
			Delete(&database.Game{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded with an uncovered continuing member")
		} else if !strings.Contains(err.Error(), "no usable game") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("degenerate field", func(t *testing.T) {
		// Two-team field with one identical tie per donor season: every
		// normalized rating is zero, so the strength signal degenerates.
		u := newTestUpdater(t)
		stubNow(t, freezeClock())
		seedTeamsAndSeasons(t, u.db)
		if err := u.db.Where("team_id >= 3").Delete(&database.TeamSeason{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.db.Where("team_id >= 3").Delete(&database.TeamName{}).Error; err != nil {
			t.Fatal(err)
		}
		seasons := []database.TeamSeason{
			{TeamID: 1, Year: 2021, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
			{TeamID: 2, Year: 2021, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
			{TeamID: 1, Year: 2022, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
			{TeamID: 2, Year: 2022, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		}
		if err := u.db.Create(&seasons).Error; err != nil {
			t.Fatal(err)
		}
		games := []database.Game{
			{GameID: 403101, Season: 2021, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 14, AwayScore: 14,
				Sport: "ncaaf", StartTime: time.Date(2021, 9, 4, 17, 0, 0, 0, time.UTC)},
			{GameID: 403201, Season: 2022, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 14, AwayScore: 14,
				Sport: "ncaaf", StartTime: time.Date(2022, 9, 3, 17, 0, 0, 0, time.UTC)},
		}
		if err := u.db.Create(&games).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded with a degenerate field")
		} else if !strings.Contains(err.Error(), "degenerate") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("oversized field", func(t *testing.T) {
		u := setup(t)
		var names []database.TeamName
		var seasons []database.TeamSeason
		for id := int64(100); id <= 100+int64(fbsPriorMaxFieldRows); id++ {
			names = append(names, database.TeamName{TeamID: id, Name: "Bulk",
				Slug: "bulk", IsActive: true, Sport: "ncaaf"})
			seasons = append(seasons, database.TeamSeason{TeamID: id, Year: 2023, FBS: 1,
				Conf: "SEC", Sport: "ncaaf"})
		}
		if err := u.db.Create(&names).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.db.Create(&seasons).Error; err != nil {
			t.Fatal(err)
		}
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err == nil {
			t.Fatal("freeze succeeded with an oversized field")
		} else if !strings.Contains(err.Error(), "bound") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestFreezeFBSPrior_DeterministicBytes verifies two independent freezes of
// the same fixture produce byte-identical artifacts (observed clocks aside:
// the same stub clock is used for both).
func TestFreezeFBSPrior_DeterministicBytes(t *testing.T) {
	build := func(t *testing.T) string {
		u := newTestUpdater(t)
		seedFreezeFixture(t, u)
		stubNow(t, freezeClock())
		if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
			t.Fatalf("freezeFBSPrior: %v", err)
		}
		return storedPriorRow(t, u.db, 2023).Artifact
	}
	first, second := build(t), build(t)
	if first != second {
		t.Error("repeated freezes of the same data produced different bytes")
	}
}

// ---------------------------------------------------------------------------
// Task entry point
// ---------------------------------------------------------------------------

// TestRunFBSPriorFreezeTask_InWindowFreezes runs the task inside the window
// with complete metadata and verifies the freeze.
func TestRunFBSPriorFreezeTask_InWindowFreezes(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())

	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err != nil {
		t.Fatalf("RunFBSPriorFreezeTask: %v", err)
	}
	if _, err := fbsprior.Parse([]byte(storedPriorRow(t, u.db, 2023).Artifact)); err != nil {
		t.Fatalf("stored artifact: %v", err)
	}
}

// TestRunFBSPriorFreezeTask_OutsideWindowNoops verifies the guard runs
// before any work: no freeze, no metadata initialization, no ESPN calls.
func TestRunFBSPriorFreezeTask_OutsideWindowNoops(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())

	for _, at := range []time.Time{
		freezeClock().AddDate(0, 1, 0),  // September
		freezeClock().AddDate(0, 0, -5), // Aug 9 ET
	} {
		if err := u.RunFBSPriorFreezeTask(context.Background(), at); err != nil {
			t.Fatalf("task outside the window returned an error: %v", err)
		}
	}
	var count int64
	if err := u.db.Model(&database.FBSSeasonPrior{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("a row was stored outside the window")
	}
}

// TestRunFBSPriorFreezeTask_InitializesMissingMetadata verifies the task
// initializes wholly absent target-year metadata (through the annual path)
// before freezing; the mocked ESPN season walk supplies the metadata.
func TestRunFBSPriorFreezeTask_InitializesMissingMetadata(t *testing.T) {
	u := newTestUpdater(t)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
	stubNow(t, freezeClock())
	// (stubNow already pins the capture clock.)

	// No 2023 metadata at all.
	var count int64
	if err := u.db.Model(&database.TeamSeason{}).Where("year = 2023").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("fixture unexpectedly has 2023 metadata")
	}

	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err != nil {
		t.Fatalf("RunFBSPriorFreezeTask: %v", err)
	}

	if err := u.db.Model(&database.TeamSeason{}).Where("year = 2023").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("task did not initialize the missing metadata")
	}
	row := storedPriorRow(t, u.db, 2023)
	prior, err := fbsprior.Parse([]byte(row.Artifact))
	if err != nil {
		t.Fatalf("stored artifact: %v", err)
	}
	if len(prior.Field) == 0 {
		t.Error("frozen field is empty")
	}
	// Teams 5-6 appear in the mocked 2023 walk without donor history.
	if len(prior.MissingHistory) == 0 {
		t.Error("expected missing-history newcomers in the frozen prior")
	}
}

// TestRunFBSPriorFreezeTask_ExistingCoverageFailureWarns verifies that
// metadata which exists but fails coverage is never rewritten: the attempt
// warns (error) and no candidate is published.
func TestRunFBSPriorFreezeTask_ExistingCoverageFailureWarns(t *testing.T) {
	logger, logs := freezeObservedLogger(t)
	u := newTestUpdaterWithLogger(t, logger)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())

	// Break coverage for the continuing member without removing metadata.
	if err := u.db.Where("sport = 'ncaaf' and (home_id = 4 or away_id = 4) and season <= 2022").
		Delete(&database.Game{}).Error; err != nil {
		t.Fatal(err)
	}

	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err == nil {
		t.Fatal("task succeeded despite broken coverage")
	}
	if esWarnCount(logs) != 0 {
		t.Errorf("unexpected blend warnings: %v", logs.All())
	}
	var seasons int64
	if err := u.db.Model(&database.TeamSeason{}).Where("year = 2023").Count(&seasons).Error; err != nil {
		t.Fatal(err)
	}
	if seasons == 0 {
		t.Fatal("existing metadata was rewritten")
	}
}

// TestRunFBSPriorFreezeTask_AlreadyFrozenNoops verifies a stored row makes
// the task a no-op even when the clock is later in the window.
func TestRunFBSPriorFreezeTask_AlreadyFrozenNoops(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)
	stubNow(t, freezeClock())
	if err := u.freezeFBSPrior(u.db.WithContext(t.Context()), 2023, freezeClock()); err != nil {
		t.Fatalf("initial freeze: %v", err)
	}
	first := storedPriorRow(t, u.db, 2023)

	later := time.Date(2023, 8, 15, 10, 0, 0, 0, time.UTC)
	stubNow(t, later)
	if err := u.RunFBSPriorFreezeTask(context.Background(), later); err != nil {
		t.Fatalf("task with a stored row: %v", err)
	}
	second := storedPriorRow(t, u.db, 2023)
	if second.Artifact != first.Artifact {
		t.Error("stored row was replaced by the task")
	}
}

// TestRunFBSPriorFreezeTask_OverlappingAttemptSkips verifies the in-process
// overlap guard: while an attempt holds the gate, a second one skips.
func TestRunFBSPriorFreezeTask_OverlappingAttemptSkips(t *testing.T) {
	u := newTestUpdater(t)
	seedFreezeFixture(t, u)

	u.priorFreezeMu.Lock()
	defer u.priorFreezeMu.Unlock()

	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err != nil {
		t.Fatalf("overlapping task returned an error: %v", err)
	}
	var count int64
	if err := u.db.Model(&database.FBSSeasonPrior{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("an overlapping attempt froze a prior")
	}
}

// TestRunFBSPriorFreezeTask_NonFootballRefused verifies the basketball
// updater can never run the football task.
func TestRunFBSPriorFreezeTask_NonFootballRefused(t *testing.T) {
	u := newBasketballTestUpdater(t)
	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err == nil {
		t.Fatal("basketball updater ran the football freeze task")
	}
}

// TestFBSSeasonPrior_MigrationTwice applies schema initialization twice on
// a fresh database (the migrate-schema idempotence contract).
func TestFBSSeasonPrior_MigrationTwice(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&database.FBSSeasonPrior{}); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	row := database.FBSSeasonPrior{Season: 2023, Artifact: "a", Checksum: "b"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert after re-migration: %v", err)
	}
	var got database.FBSSeasonPrior
	if err := db.First(&got, "season = 2023").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Artifact != "a" || got.Checksum != "b" {
		t.Errorf("row = %+v", got)
	}
}

// TestStoredPriorRow_ConflictDoesNothing verifies the insert-once semantics
// directly: a second insert on the same season leaves the original bytes.
func TestStoredPriorRow_ConflictDoesNothing(t *testing.T) {
	db := setupTestDB(t)
	first := database.FBSSeasonPrior{Season: 2023, Artifact: "original", Checksum: "c1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	second := database.FBSSeasonPrior{Season: 2023, Artifact: "replacement", Checksum: "c2"}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("duplicate insert succeeded without a conflict clause")
	}
	var got database.FBSSeasonPrior
	if err := db.First(&got, "season = 2023").Error; err != nil {
		t.Fatal(err)
	}
	if got.Artifact != "original" {
		t.Errorf("stored artifact = %q, want the original", got.Artifact)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func freezeObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func sha256Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
