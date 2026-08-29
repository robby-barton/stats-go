package load

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/sport"
)

// setupTestDB opens an in-memory SQLite database with the ranking-relevant
// tables migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&database.Game{},
		&database.TeamSeason{},
		&database.TeamName{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// seedTestData seeds 5 football teams (4 FBS, 1 FCS) and games across the
// 2022-2023 seasons.
func seedTestData(t *testing.T, db *gorm.DB) {
	t.Helper()

	// 5 teams: 4 FBS, 1 FCS
	teamNames := []database.TeamName{
		{TeamID: 1, Name: "Alpha", Sport: "ncaaf"},
		{TeamID: 2, Name: "Beta", Sport: "ncaaf"},
		{TeamID: 3, Name: "Gamma", Sport: "ncaaf"},
		{TeamID: 4, Name: "Delta", Sport: "ncaaf"},
		{TeamID: 5, Name: "Epsilon", Sport: "ncaaf"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 1, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 2, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 3, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
		{TeamID: 4, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
		{TeamID: 5, Year: 2023, FBS: 0, Conf: "FCS", Sport: "ncaaf"},
		// Historical team_seasons for 2022
		{TeamID: 1, Year: 2022, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 2, Year: 2022, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 3, Year: 2022, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
		{TeamID: 4, Year: 2022, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed team_seasons: %v", err)
	}

	// Base time: Tuesday of week 1, 2023 season
	base := time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour

	games := []database.Game{
		// 2023 season games
		{
			GameID: 1001, Season: 2023, Week: 1, HomeID: 1, AwayID: 2,
			HomeScore: 28, AwayScore: 14, ConfGame: true, Sport: "ncaaf", StartTime: base,
		},
		{
			GameID: 1002, Season: 2023, Week: 1, HomeID: 3, AwayID: 4,
			HomeScore: 21, AwayScore: 10, ConfGame: true, Sport: "ncaaf", StartTime: base.Add(time.Hour),
		},
		{
			GameID: 1003, Season: 2023, Week: 2, HomeID: 1, AwayID: 3,
			HomeScore: 35, AwayScore: 17, Sport: "ncaaf", StartTime: base.Add(week),
		},
		{
			GameID: 1004, Season: 2023, Week: 2, HomeID: 2, AwayID: 4,
			HomeScore: 24, AwayScore: 21, ConfGame: true, Sport: "ncaaf", StartTime: base.Add(week + time.Hour),
		},
		{
			GameID: 1005, Season: 2023, Week: 3, HomeID: 1, AwayID: 4,
			HomeScore: 42, AwayScore: 7, Sport: "ncaaf", StartTime: base.Add(2 * week),
		},
		{
			GameID: 1006, Season: 2023, Week: 3, HomeID: 2, AwayID: 3,
			HomeScore: 17, AwayScore: 14, Sport: "ncaaf", StartTime: base.Add(2*week + time.Hour),
		},
		{
			GameID: 1007, Season: 2023, Week: 4, HomeID: 3, AwayID: 2,
			HomeScore: 28, AwayScore: 21, Sport: "ncaaf", StartTime: base.Add(3 * week),
		},
		{
			GameID: 1008, Season: 2023, Week: 4, HomeID: 1, AwayID: 5,
			HomeScore: 31, AwayScore: 10, Sport: "ncaaf", StartTime: base.Add(3*week + time.Hour),
		},
		{
			GameID: 1009, Season: 2023, Week: 5, HomeID: 4, AwayID: 3,
			HomeScore: 14, AwayScore: 14, Sport: "ncaaf", StartTime: base.Add(4 * week),
		},
		{
			GameID: 1010, Season: 2023, Week: 5, HomeID: 2, AwayID: 5,
			HomeScore: 35, AwayScore: 7, Sport: "ncaaf", StartTime: base.Add(4*week + time.Hour),
		},
		// 2022 historical games
		{
			GameID: 901, Season: 2022, Week: 1, HomeID: 1, AwayID: 2,
			HomeScore: 24, AwayScore: 17, Sport: "ncaaf",
			StartTime: time.Date(2022, 9, 6, 19, 0, 0, 0, time.UTC),
		},
		{
			GameID: 902, Season: 2022, Week: 2, HomeID: 3, AwayID: 4,
			HomeScore: 20, AwayScore: 13, Sport: "ncaaf",
			StartTime: time.Date(2022, 9, 13, 19, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed games: %v", err)
	}
}

// seedBasketballData inserts 5 basketball teams (all FBS=1) and 6 games across
// 2 weeks for the 2024 season. Also inserts one football game to confirm
// sport filtering excludes it.
func seedBasketballData(t *testing.T, db *gorm.DB) {
	t.Helper()

	teamNames := []database.TeamName{
		{TeamID: 101, Name: "Hoops A", Sport: "ncaam"},
		{TeamID: 102, Name: "Hoops B", Sport: "ncaam"},
		{TeamID: 103, Name: "Hoops C", Sport: "ncaam"},
		{TeamID: 104, Name: "Hoops D", Sport: "ncaam"},
		{TeamID: 105, Name: "Hoops E", Sport: "ncaam"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed basketball team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 101, Year: 2024, FBS: 1, Conf: "Big East", Sport: "ncaam"},
		{TeamID: 102, Year: 2024, FBS: 1, Conf: "Big East", Sport: "ncaam"},
		{TeamID: 103, Year: 2024, FBS: 1, Conf: "ACC", Sport: "ncaam"},
		{TeamID: 104, Year: 2024, FBS: 1, Conf: "ACC", Sport: "ncaam"},
		{TeamID: 105, Year: 2024, FBS: 1, Conf: "Big 12", Sport: "ncaam"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed basketball team_seasons: %v", err)
	}

	base := time.Date(2024, 1, 2, 19, 0, 0, 0, time.UTC) // Tuesday
	week := 7 * 24 * time.Hour

	games := []database.Game{
		// Week 1
		{GameID: 3001, Season: 2024, Week: 1, HomeID: 101, AwayID: 102,
			HomeScore: 78, AwayScore: 65, ConfGame: true, Sport: "ncaam", StartTime: base},
		{GameID: 3002, Season: 2024, Week: 1, HomeID: 103, AwayID: 104,
			HomeScore: 70, AwayScore: 68, ConfGame: true, Sport: "ncaam", StartTime: base.Add(time.Hour)},
		{GameID: 3003, Season: 2024, Week: 1, HomeID: 105, AwayID: 101,
			HomeScore: 60, AwayScore: 72, Sport: "ncaam", StartTime: base.Add(2 * time.Hour)},
		// Week 2
		{GameID: 3004, Season: 2024, Week: 2, HomeID: 101, AwayID: 103,
			HomeScore: 80, AwayScore: 75, Sport: "ncaam", StartTime: base.Add(week)},
		{GameID: 3005, Season: 2024, Week: 2, HomeID: 102, AwayID: 104,
			HomeScore: 66, AwayScore: 64, Sport: "ncaam", StartTime: base.Add(week + time.Hour)},
		{GameID: 3006, Season: 2024, Week: 2, HomeID: 105, AwayID: 103,
			HomeScore: 55, AwayScore: 55, Sport: "ncaam", StartTime: base.Add(week + 2*time.Hour)},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed basketball games: %v", err)
	}

	// Football game — should be excluded by sport filter
	footballGame := database.Game{
		GameID: 9001, Season: 2024, Week: 1, HomeID: 101, AwayID: 102,
		HomeScore: 35, AwayScore: 10, Sport: "ncaaf",
		StartTime: base,
	}
	if err := db.Create(&footballGame).Error; err != nil {
		t.Fatalf("seed football game: %v", err)
	}
}

func TestInput_DefaultYear(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Football})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	if in.Year != 2023 {
		t.Errorf("Year = %d, want 2023", in.Year)
	}
}

func TestInput_WeekSpecified(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023, Week: 3})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	if in.Week != 3 {
		t.Errorf("Week = %d, want 3", in.Week)
	}
	// startTime should be set to midnight Eastern on the Tuesday of week 3's
	// first game (2023-09-19 04:00 UTC, September = EDT).
	if in.StartTime.IsZero() {
		t.Error("startTime is zero")
	}
	if in.StartTime.Weekday() != time.Tuesday {
		t.Errorf("startTime weekday = %v, want Tuesday", in.StartTime.Weekday())
	}
	if !in.StartTime.Equal(time.Date(2023, 9, 19, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("startTime = %v, want 2023-09-19 00:00 ET (04:00 UTC)", in.StartTime)
	}
}

func TestInput_WeekZero(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	// The latest game is week 5, so Week should be 6 (latestGame.Week + 1)
	if in.Week != 6 {
		t.Errorf("Week = %d, want 6", in.Week)
	}
}

func TestInput_Postseason(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	// Add a postseason game
	psGame := database.Game{
		GameID:     2001,
		Season:     2023,
		Week:       1,
		Postseason: 1,
		HomeID:     1,
		AwayID:     2,
		HomeScore:  30,
		AwayScore:  20,
		StartTime:  time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&psGame).Error; err != nil {
		t.Fatalf("create postseason game: %v", err)
	}

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	if !in.Postseason {
		t.Error("postseason = false, want true")
	}
}

func TestInput_Teams(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	// All 5 teams for the sport/year should be loaded, with division flags.
	if len(in.Teams) != 5 {
		t.Fatalf("len(Teams) = %d, want 5", len(in.Teams))
	}

	expected := map[int64]struct {
		Name string
		Conf string
		FBS  bool
	}{
		1: {"Alpha", "SEC", true},
		2: {"Beta", "SEC", true},
		3: {"Gamma", "Big Ten", true},
		4: {"Delta", "Big Ten", true},
		5: {"Epsilon", "FCS", false},
	}

	for _, team := range in.Teams {
		want, ok := expected[team.ID]
		if !ok {
			t.Errorf("unexpected team %d", team.ID)
			continue
		}
		if team.Name != want.Name {
			t.Errorf("team %d Name = %q, want %q", team.ID, team.Name, want.Name)
		}
		if team.Conf != want.Conf {
			t.Errorf("team %d Conf = %q, want %q", team.ID, team.Conf, want.Conf)
		}
		if team.FBS != want.FBS {
			t.Errorf("team %d FBS = %v, want %v", team.ID, team.FBS, want.FBS)
		}
	}
}

func TestInput_SportFiltering(t *testing.T) {
	db := setupTestDB(t)
	seedBasketballData(t, db)

	// Also seed football data to ensure it's ignored
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Basketball})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	if in.Year != 2024 {
		t.Errorf("Year = %d, want 2024", in.Year)
	}
	// Latest basketball game is week 2, so Week should be 3
	if in.Week != 3 {
		t.Errorf("Week = %d, want 3", in.Week)
	}

	// The football game must not appear in the loaded games.
	for _, g := range in.Games {
		if g.GameID == 9001 {
			t.Error("football game 9001 leaked into basketball input")
		}
	}

	// All 5 basketball teams should be loaded (all FBS=1 for ncaam).
	if len(in.Teams) != 5 {
		t.Fatalf("len(Teams) = %d, want 5", len(in.Teams))
	}
}

func TestInput_Validation(t *testing.T) {
	db := setupTestDB(t)

	if _, err := Input(Options{DB: nil, Sport: sport.Football}); err == nil {
		t.Error("Input with nil DB: expected error, got nil")
	}
	if _, err := Input(Options{DB: db, Sport: sport.Sport("ncaaw")}); err == nil {
		t.Error("Input with unknown sport: expected error, got nil")
	}
}

func TestInput_Backfill(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	if in.Backfill == nil {
		t.Fatal("Backfill = nil, want a database-backed implementation")
	}

	// The 2022 games involve teams 1-4; a backfill search for team 1 against
	// {2} before 2023 should return game 901.
	games, err := in.Backfill(1, []int64{2}, 2023, 10)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if len(games) != 1 || games[0].GameID != 901 {
		t.Errorf("Backfill = %v, want [game 901]", games)
	}
}

func TestInput_WeekCutoffBoundary(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	// With Week 3 the window start resolves to midnight Eastern on Tuesday
	// 2023-09-19 (04:00 UTC, September = EDT), the Tuesday of week 3's first
	// game. Seed two extra games straddling the cutoff: one starting exactly
	// at it (must be included) and one a second after it (must be excluded).
	cutoff := time.Date(2023, 9, 19, 4, 0, 0, 0, time.UTC)
	boundary := []database.Game{
		{
			GameID: 1011, Season: 2023, Week: 2, HomeID: 3, AwayID: 5,
			HomeScore: 10, AwayScore: 7, Sport: "ncaaf", StartTime: cutoff,
		},
		{
			GameID: 1012, Season: 2023, Week: 2, HomeID: 4, AwayID: 5,
			HomeScore: 13, AwayScore: 3, Sport: "ncaaf", StartTime: cutoff.Add(time.Second),
		},
	}
	if err := db.Create(&boundary).Error; err != nil {
		t.Fatalf("seed boundary games: %v", err)
	}

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023, Week: 3})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}
	if !in.StartTime.Equal(cutoff) {
		t.Fatalf("StartTime = %v, want %v", in.StartTime, cutoff)
	}

	loaded := make(map[int64]bool, len(in.Games))
	for _, g := range in.Games {
		loaded[g.GameID] = true
	}

	// Games at or before the cutoff are included.
	for _, id := range []int64{1003, 1004, 1011} {
		if !loaded[id] {
			t.Errorf("game %d (at/before cutoff) missing from loaded games", id)
		}
	}
	// Games starting after the cutoff are excluded.
	for _, id := range []int64{1005, 1006, 1012} {
		if loaded[id] {
			t.Errorf("game %d (after cutoff) leaked into loaded games", id)
		}
	}
}

func TestInput_WeekCutoffIsEasternMidnight(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	// Pins finding: the week boundary is midnight Eastern expressed as a UTC
	// instant, not midnight UTC. Week 3's first game is Tuesday 2023-09-19
	// 19:00 UTC, so the cutoff is 2023-09-19 00:00 ET = 04:00 UTC (EDT).
	// A Monday 8pm ET tip (00:00 UTC Tuesday) must be included even though
	// its UTC instant is past the naive UTC-midnight cutoff that once
	// dropped such games; a 12:30am ET Tuesday tip (04:30 UTC) must be
	// excluded. Boundary games sit in week 2 so the week-3 anchor game is
	// untouched.
	boundary := []database.Game{
		{
			GameID: 1013, Season: 2023, Week: 2, HomeID: 1, AwayID: 3,
			HomeScore: 21, AwayScore: 20, Sport: "ncaaf",
			// Monday 2023-09-18 20:00 ET = 2023-09-19 00:00 UTC. Built in ET
			// and converted to a UTC instant (how the DB round-trips rows;
			// SQLite compares stored strings, so offsets must not leak in).
			StartTime: time.Date(2023, 9, 18, 20, 0, 0, 0, nyLoc(t)).UTC(),
		},
		{
			GameID: 1014, Season: 2023, Week: 2, HomeID: 2, AwayID: 4,
			HomeScore: 30, AwayScore: 28, Sport: "ncaaf",
			// Tuesday 2023-09-19 00:30 ET = 2023-09-19 04:30 UTC.
			StartTime: time.Date(2023, 9, 19, 0, 30, 0, 0, nyLoc(t)).UTC(),
		},
	}
	if err := db.Create(&boundary).Error; err != nil {
		t.Fatalf("seed boundary games: %v", err)
	}

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023, Week: 3})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}
	want := time.Date(2023, 9, 19, 4, 0, 0, 0, time.UTC)
	if !in.StartTime.Equal(want) {
		t.Fatalf("StartTime = %v, want %v (midnight ET as a UTC instant)", in.StartTime, want)
	}

	loaded := make(map[int64]bool, len(in.Games))
	for _, g := range in.Games {
		loaded[g.GameID] = true
	}
	if !loaded[1013] {
		t.Error("game 1013 (8pm ET on the day before the cutoff) excluded; boundary must be ET midnight")
	}
	if loaded[1014] {
		t.Error("game 1014 (12:30am ET on the cutoff day) included; boundary must be ET midnight")
	}
}

// nyLoc loads America/New_York for test assertions, skipping if the zone
// database is unavailable (mirroring the production fallback to UTC).
func nyLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("cannot load America/New_York: %v", err)
	}
	return loc
}

func TestInput_GamesOrderedDescending(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	in, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023})
	if err != nil {
		t.Fatalf("Input: %v", err)
	}

	if len(in.Games) == 0 {
		t.Fatal("no games loaded")
	}
	// srs's game selection depends on this order: newest game first.
	for i := 0; i < len(in.Games)-1; i++ {
		if in.Games[i].StartTime.Before(in.Games[i+1].StartTime) {
			t.Errorf("games not ordered descending: index %d (%d, %v) precedes index %d (%d, %v)",
				i, in.Games[i].GameID, in.Games[i].StartTime,
				i+1, in.Games[i+1].GameID, in.Games[i+1].StartTime)
		}
	}
	if in.Games[0].GameID != 1010 {
		t.Errorf("first game = %d, want 1010 (latest start time)", in.Games[0].GameID)
	}
}

func TestInput_EmptyDB(t *testing.T) {
	db := setupTestDB(t)

	in, err := Input(Options{DB: db, Sport: sport.Football})
	if err != nil {
		// Clean error is acceptable.
		return
	}
	// If it succeeds, it must do so with an empty, usable input — never a panic.
	if len(in.Teams) != 0 || len(in.Games) != 0 {
		t.Errorf("empty DB yielded teams=%d games=%d, want 0/0", len(in.Teams), len(in.Games))
	}
	if in.Backfill == nil {
		t.Error("Backfill = nil, want a database-backed implementation")
	}
}
