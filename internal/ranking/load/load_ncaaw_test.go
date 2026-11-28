package load

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ---------------------------------------------------------------------------
// Women's basketball window fixtures
// ---------------------------------------------------------------------------
//
// Live ESPN summary checks verified the women's week calendar: Sundays
// 2026-11-08/15/22 are weeks 1/2/3 while the adjacent Mondays are weeks 2/3/4,
// and a Tuesday game still belongs to the Monday's week. Every week therefore
// opens at Monday 00:00 Eastern, so an explicit week's ranking cutoff is the
// Monday 00:00 Eastern that opens the calendar week after the week's games.

// wbET builds a UTC instant from an Eastern wall-clock time.
func wbET(y int, m time.Month, d, h, minute int) time.Time {
	return time.Date(y, m, d, h, minute, 0, 0, boundaryLoc).UTC()
}

// seedWomensLoadDB seeds four D-I women's teams for a season. The team_names
// rows are shared across seasons (they key on team_id and sport only).
func seedWomensLoadDB(t *testing.T, db *gorm.DB, year int64) {
	t.Helper()

	var names int64
	if err := db.Model(database.TeamName{}).
		Where("sport = ?", sport.WomensBasketball).Count(&names).Error; err != nil {
		t.Fatalf("count team_names: %v", err)
	}
	if names == 0 {
		teamNames := []database.TeamName{
			{TeamID: 21, Name: "WAlpha", Sport: sport.WomensBasketball},
			{TeamID: 22, Name: "WBeta", Sport: sport.WomensBasketball},
			{TeamID: 23, Name: "WGamma", Sport: sport.WomensBasketball},
			{TeamID: 24, Name: "WDelta", Sport: sport.WomensBasketball},
		}
		if err := db.Create(&teamNames).Error; err != nil {
			t.Fatalf("seed team_names: %v", err)
		}
	}
	teamSeasons := []database.TeamSeason{
		{TeamID: 21, Year: year, FBS: 1, Conf: "Big East", Sport: sport.WomensBasketball},
		{TeamID: 22, Year: year, FBS: 1, Conf: "Big East", Sport: sport.WomensBasketball},
		{TeamID: 23, Year: year, FBS: 1, Conf: "SEC", Sport: sport.WomensBasketball},
		{TeamID: 24, Year: year, FBS: 1, Conf: "SEC", Sport: sport.WomensBasketball},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed team_seasons: %v", err)
	}
}

// seedWomensGame inserts one completed women's game row.
func seedWomensGame(t *testing.T, db *gorm.DB, g database.Game) {
	t.Helper()
	if err := db.Create(&g).Error; err != nil {
		t.Fatalf("seed game %d: %v", g.GameID, g)
	}
}

func wbGame(id int64, season, week, postseason int64, homeID, awayID, hs, as int64, start time.Time) database.Game {
	return database.Game{
		GameID: id, Season: season, Week: week, Postseason: postseason,
		HomeID: homeID, HomeScore: hs, AwayID: awayID, AwayScore: as,
		Sport: sport.WomensBasketball, StartTime: start,
	}
}

// TestWomensWeekCutoffMondayEastern walks the verified 2026-27 opening
// calendar: ESPN includes the weekend opener in Week 1 along with the next
// Monday slate. Each snapshot ends at midnight Eastern after its last Sunday.
func TestWomensWeekCutoffMondayEastern(t *testing.T) {
	db := setupTestDB(t)
	seedWomensLoadDB(t, db, 2027)

	games := []database.Game{
		// ESPN event 401920983 on Sunday Nov 1 counted as Week 1, as did
		// the Monday Nov 2 slate and Sunday Nov 8's late games.
		wbGame(6101, 2027, 1, 0, 21, 22, 70, 60, wbET(2026, time.November, 1, 12, 0)),
		wbGame(6100, 2027, 1, 0, 21, 22, 71, 60, wbET(2026, time.November, 2, 19, 0)),
		wbGame(6102, 2027, 1, 0, 23, 24, 80, 75, wbET(2026, time.November, 8, 23, 0)),
		// Week 2: Monday opener (the first game of the new week) and a late
		// Sunday close (23:30 ET = 04:30Z Monday UTC).
		wbGame(6103, 2027, 2, 0, 21, 23, 65, 63, wbET(2026, time.November, 9, 19, 0)),
		wbGame(6104, 2027, 2, 0, 22, 24, 90, 88, wbET(2026, time.November, 15, 23, 30)),
		// Week 3: Tuesday game — still that week.
		wbGame(6105, 2027, 3, 0, 21, 24, 55, 50, wbET(2026, time.November, 17, 19, 0)),
	}
	for _, g := range games {
		seedWomensGame(t, db, g)
	}

	cases := []struct {
		week        int64
		wantCutoff  time.Time
		wantInclude []int64
		wantExclude []int64
	}{
		{
			// Windows are cumulative up to the cutoff; only games after the
			// cutoff must be excluded.
			week:        1,
			wantCutoff:  wbET(2026, time.November, 9, 0, 0), // 2026-11-09T05:00Z (EST)
			wantInclude: []int64{6100, 6101, 6102},
			wantExclude: []int64{6103, 6104, 6105},
		},
		{
			week:        2,
			wantCutoff:  wbET(2026, time.November, 16, 0, 0), // 2026-11-16T05:00Z
			wantInclude: []int64{6100, 6101, 6102, 6103, 6104},
			wantExclude: []int64{6105},
		},
		{
			week:        3,
			wantCutoff:  wbET(2026, time.November, 23, 0, 0), // 2026-11-23T05:00Z
			wantInclude: []int64{6100, 6101, 6102, 6103, 6104, 6105},
			wantExclude: nil,
		},
	}

	for _, tc := range cases {
		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: tc.week})
		if err != nil {
			t.Fatalf("week %d: Input: %v", tc.week, err)
		}
		if !in.StartTime.Equal(tc.wantCutoff) {
			t.Errorf("week %d cutoff = %s, want %s", tc.week, in.StartTime, tc.wantCutoff)
		}
		if in.Week != tc.week {
			t.Errorf("week %d: resolved week = %d", tc.week, in.Week)
		}
		if in.Postseason {
			t.Errorf("week %d: regular week resolved postseason", tc.week)
		}
		loaded := make(map[int64]bool, len(in.Games))
		for _, g := range in.Games {
			loaded[g.GameID] = true
		}
		for _, id := range tc.wantInclude {
			if !loaded[id] {
				t.Errorf("week %d: game %d missing from the loaded window", tc.week, id)
			}
		}
		for _, id := range tc.wantExclude {
			if loaded[id] {
				t.Errorf("week %d: game %d must be outside the loaded window", tc.week, id)
			}
		}
	}
}

// TestWomensWeekCutoffDSTBoundaries pins the cutoff to local midnight across
// both United States DST transitions: the boundary is Monday 00:00 in
// America/New_York, which is 04:00Z in daylight time and 05:00Z in standard
// time, so a late Sunday game near either transition stays in its own week.
func TestWomensWeekCutoffDSTBoundaries(t *testing.T) {
	t.Run("fall back", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2027)
		// Sunday 2026-11-01 is the fall-back morning; this Sunday-evening
		// game (20:00 EDT = 2026-11-01T00:00Z) closes a week whose cutoff
		// lands in standard time: Monday 2026-11-02 00:00 EST = 05:00Z.
		seedWomensGame(t, db, wbGame(6111, 2027, 4, 0, 21, 22, 60, 58,
			wbET(2026, time.November, 1, 20, 0)))
		// The following Monday's game (19:00 EST = 2026-11-03T00:00Z) opens
		// the next week and must be excluded.
		seedWomensGame(t, db, wbGame(6112, 2027, 5, 0, 23, 24, 70, 69,
			wbET(2026, time.November, 2, 19, 0)))

		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: 4})
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		if want := wbET(2026, time.November, 2, 0, 0); !in.StartTime.Equal(want) {
			t.Errorf("cutoff = %s, want %s (05:00Z standard time)", in.StartTime, want)
		}
		loaded := make(map[int64]bool, len(in.Games))
		for _, g := range in.Games {
			loaded[g.GameID] = true
		}
		if !loaded[6111] || loaded[6112] {
			t.Errorf("loaded = %v, want game 6111 in and 6112 out", loaded)
		}
	})

	t.Run("spring forward", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2027)
		// Sunday 2027-03-14 is the spring-forward day; a Sunday-evening game
		// (20:00 EDT = 2027-03-15T00:00Z) closes a week whose cutoff is
		// Monday 2027-03-15 00:00 EDT = 04:00Z.
		seedWomensGame(t, db, wbGame(6121, 2027, 18, 0, 21, 22, 61, 59,
			wbET(2027, time.March, 14, 20, 0)))
		seedWomensGame(t, db, wbGame(6122, 2027, 19, 0, 23, 24, 71, 70,
			wbET(2027, time.March, 15, 19, 0)))

		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: 18})
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		if want := wbET(2027, time.March, 15, 0, 0); !in.StartTime.Equal(want) {
			t.Errorf("cutoff = %s, want %s (04:00Z daylight time)", in.StartTime, want)
		}
		loaded := make(map[int64]bool, len(in.Games))
		for _, g := range in.Games {
			loaded[g.GameID] = true
		}
		if !loaded[6121] || loaded[6122] {
			t.Errorf("loaded = %v, want game 6121 in and 6122 out", loaded)
		}
	})
}

// TestWomensExplicitMissingWeekStrict pins the strictness contract: an
// explicit women's regular week (or season) with no stored games is an error,
// never a silent fall-through to the latest window, and the latest window
// never builds a phantom preview out of a previous season's games.
func TestWomensExplicitMissingWeekStrict(t *testing.T) {
	db := setupTestDB(t)
	seedWomensLoadDB(t, db, 2027)
	seedWomensGame(t, db, wbGame(6131, 2027, 1, 0, 21, 22, 70, 60,
		wbET(2026, time.November, 8, 19, 0)))

	if _, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: 2}); err == nil {
		t.Error("missing week 2: expected an error, got nil")
	}

	// Absent season, explicit week.
	if _, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2026, Week: 1}); err == nil {
		t.Error("absent season 2026, explicit week: expected an error, got nil")
	}

	// A games-less season must not leak a phantom preview through the
	// latest window either, even though older seasons have games the old
	// cross-season fallback would have consumed.
	seedWomensLoadDB(t, db, 2026)
	seedWomensGame(t, db, wbGame(6132, 2026, 20, 0, 21, 22, 66, 64,
		wbET(2026, time.March, 1, 19, 0)))
	seedWomensLoadDB(t, db, 2028)
	if _, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2028, Week: 0}); err == nil {
		t.Error("latest for a season with no games: expected an error, got nil")
	}
}

// TestMensWindowsUnchanged pins the pre-existing window semantics for every
// other sport: the Tuesday-derived explicit cutoff, and the silent
// latest-game fall-through for a missing week or season.
func TestMensWindowsUnchanged(t *testing.T) {
	db := setupTestDB(t)

	teamNames := []database.TeamName{
		{TeamID: 31, Name: "MAlpha", Sport: sport.Basketball},
		{TeamID: 32, Name: "MBeta", Sport: sport.Basketball},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed team_names: %v", err)
	}
	teamSeasons := []database.TeamSeason{
		{TeamID: 31, Year: 2027, FBS: 1, Conf: "Big East", Sport: sport.Basketball},
		{TeamID: 32, Year: 2027, FBS: 1, Conf: "Big East", Sport: sport.Basketball},
		{TeamID: 31, Year: 2026, FBS: 1, Conf: "Big East", Sport: sport.Basketball},
		{TeamID: 32, Year: 2026, FBS: 1, Conf: "Big East", Sport: sport.Basketball},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed team_seasons: %v", err)
	}
	mbGame := func(id int64, season, week int64, start time.Time) database.Game {
		return database.Game{
			GameID: id, Season: season, Week: week,
			HomeID: 31, HomeScore: 70, AwayID: 32, AwayScore: 60,
			Sport: sport.Basketball, StartTime: start,
		}
	}
	games := []database.Game{
		// A Wednesday game: the historical cutoff is the Tuesday midnight
		// opening its calendar week.
		mbGame(6141, 2027, 5, wbET(2027, time.January, 6, 19, 0)),
		// A prior-season game for the silent latest fall-through.
		mbGame(6142, 2026, 20, wbET(2026, time.March, 1, 19, 0)),
	}
	for _, g := range games {
		if err := db.Create(&g).Error; err != nil {
			t.Fatalf("seed game %d: %v", g.GameID, err)
		}
	}

	in, err := Input(Options{DB: db, Sport: sport.Basketball, Year: 2027, Week: 5})
	if err != nil {
		t.Fatalf("men's explicit week: Input: %v", err)
	}
	if want := wbET(2027, time.January, 5, 0, 0); !in.StartTime.Equal(want) {
		t.Errorf("men's cutoff = %s, want %s (Tuesday of the game's week)", in.StartTime, want)
	}

	// A missing week silently resolves the latest game (the week-5 game).
	in, err = Input(Options{DB: db, Sport: sport.Basketball, Year: 2027, Week: 9})
	if err != nil {
		t.Fatalf("men's missing week: Input: %v", err)
	}
	if in.Week != 6 {
		t.Errorf("men's missing week resolved week = %d, want 6 (latest game 5 + 1)", in.Week)
	}
	if !in.StartTime.Equal(wbET(2027, time.January, 6, 19, 0)) {
		t.Errorf("men's missing week cutoff = %s, want the latest game's start time", in.StartTime)
	}
}

// TestWomensLatestWindow covers the women's latest resolution: season-scoped
// regular labeling, the stable Final key once the anchor game is postseason,
// and the exclusion of unparsed retry rows and preseason games from both the
// anchor and the loaded game set.
func TestWomensLatestWindow(t *testing.T) {
	t.Run("regular label convention", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2027)
		seedWomensGame(t, db, wbGame(6151, 2027, 1, 0, 21, 22, 70, 60,
			wbET(2026, time.November, 8, 19, 0)))
		seedWomensGame(t, db, wbGame(6152, 2027, 11, 0, 21, 23, 65, 63,
			wbET(2027, time.January, 10, 19, 0)))

		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: 0})
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		// The latest regular snapshot shares the week's normal label.
		if in.Week != 11 || in.Postseason {
			t.Errorf("latest regular window = week %d postseason %v, want week 11 regular",
				in.Week, in.Postseason)
		}
		if !in.StartTime.Equal(wbET(2027, time.January, 10, 19, 0)) {
			t.Errorf("latest cutoff = %s, want the latest game's start time", in.StartTime)
		}
	})

	t.Run("stable Final key", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2027)
		seedWomensGame(t, db, wbGame(6161, 2027, 1, 0, 21, 22, 70, 60,
			wbET(2026, time.November, 8, 19, 0)))
		// Semifinal numbered by ESPN; championship omits header.week (the
		// observed 2026-04-05 summary shape) and must still anchor the same
		// stable Final window.
		seedWomensGame(t, db, wbGame(6162, 2027, 4, 1, 21, 23, 75, 74,
			wbET(2027, time.April, 1, 19, 0)))
		seedWomensGame(t, db, wbGame(6163, 2027, 0, 1, 22, 24, 78, 77,
			wbET(2027, time.April, 5, 19, 0)))

		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: 0})
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		if !in.Postseason || in.Week != 0 {
			t.Errorf("Final window = week %d postseason %v, want week 0 postseason",
				in.Week, in.Postseason)
		}
		if !in.StartTime.Equal(wbET(2027, time.April, 5, 19, 0)) {
			t.Errorf("Final cutoff = %s, want the championship start time", in.StartTime)
		}
		loaded := make(map[int64]bool, len(in.Games))
		for _, g := range in.Games {
			loaded[g.GameID] = true
		}
		for _, id := range []int64{6161, 6162, 6163} {
			if !loaded[id] {
				t.Errorf("Final window missing game %d", id)
			}
		}
	})

	t.Run("retry and preseason rows excluded", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2027)
		seedWomensGame(t, db, wbGame(6171, 2027, 1, 0, 21, 22, 70, 60,
			wbET(2026, time.November, 8, 19, 0)))
		// An unparsed retry placeholder with schedule-seeded scores: never
		// rankable input, never the latest anchor.
		retry := wbGame(6172, 2027, 2, 0, 23, 24, 99, 99,
			wbET(2026, time.November, 20, 19, 0))
		retry.Retry = 1
		seedWomensGame(t, db, retry)
		// A preseason exhibition stored after the last real game.
		seedWomensGame(t, db, wbGame(6173, 2027, 0, -1, 21, 24, 88, 44,
			wbET(2026, time.November, 25, 19, 0)))

		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2027, Week: 0})
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		if in.Week != 1 {
			t.Errorf("latest week = %d, want 1 (anchor on the last rankable game)", in.Week)
		}
		loaded := make(map[int64]bool, len(in.Games))
		for _, g := range in.Games {
			loaded[g.GameID] = true
		}
		if loaded[6172] || loaded[6173] {
			t.Errorf("retry/preseason rows loaded into the window: %v", loaded)
		}
	})

	t.Run("latest is season-scoped", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2026)
		seedWomensLoadDB(t, db, 2027)
		seedWomensGame(t, db, wbGame(6181, 2026, 20, 0, 21, 22, 66, 64,
			wbET(2026, time.March, 1, 19, 0)))
		seedWomensGame(t, db, wbGame(6182, 2027, 1, 0, 21, 22, 70, 60,
			wbET(2026, time.November, 8, 19, 0)))

		in, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2026, Week: 0})
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		if in.Week != 20 {
			t.Errorf("2026 latest week = %d, want 20 (2026's own latest game)", in.Week)
		}
		if !in.StartTime.Equal(wbET(2026, time.March, 1, 19, 0)) {
			t.Errorf("2026 latest cutoff = %s, want 2026's latest game", in.StartTime)
		}
	})
}

// TestWomensEmptySeasonSentinel pins the fail-closed season-start behavior the
// scheduled women's pipeline depends on: a season with team rows but no
// rankable games resolves to the ErrNoRankableGames sentinel (so the
// scheduler can skip without alerting) instead of falling through to a
// phantom window built from another season, while the men's loader keeps its
// legacy empty-database fallback and never produces the sentinel.
func TestWomensEmptySeasonSentinel(t *testing.T) {
	t.Run("women's season with no games", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2028)

		_, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 2028, Week: 0})
		if !errors.Is(err, ErrNoRankableGames) {
			t.Fatalf("Input error = %v, want ErrNoRankableGames", err)
		}
	})

	t.Run("women's latest request with only a prior season present", func(t *testing.T) {
		db := setupTestDB(t)
		seedWomensLoadDB(t, db, 2027)
		seedWomensGame(t, db, wbGame(6181, 2027, 1, 0, 21, 22, 70, 60,
			wbET(2026, time.November, 8, 19, 0)))
		// A newly initialized 2028 season row must not pull the 2027 window
		// forward: the latest request is scoped to the resolved season.
		if err := db.Create(&database.TeamSeason{
			TeamID: 21, Year: 2028, FBS: 1, Conf: "Big East", Sport: sport.WomensBasketball,
		}).Error; err != nil {
			t.Fatalf("seed 2028 season: %v", err)
		}

		// Year 0 resolves to max(team_seasons.year) = 2028: no rankable games.
		_, err := Input(Options{DB: db, Sport: sport.WomensBasketball, Year: 0, Week: 0})
		if !errors.Is(err, ErrNoRankableGames) {
			t.Fatalf("Input error = %v, want ErrNoRankableGames (no phantom prior-season window)", err)
		}
	})

	t.Run("men's empty database unchanged", func(t *testing.T) {
		db := setupTestDB(t)
		if err := db.Create(&[]database.TeamSeason{
			{TeamID: 31, Year: 2028, FBS: 1, Conf: "Big East", Sport: sport.Basketball},
		}).Error; err != nil {
			t.Fatalf("seed men's season: %v", err)
		}

		_, err := Input(Options{DB: db, Sport: sport.Basketball, Year: 2028, Week: 0})
		if err != nil {
			t.Fatalf("men's empty-season Input: %v", err)
		}
		if errors.Is(err, ErrNoRankableGames) {
			t.Error("men's loader must never produce ErrNoRankableGames")
		}
	})
}
