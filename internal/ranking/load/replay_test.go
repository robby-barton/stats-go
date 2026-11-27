package load

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/sport"
)

func replayOpts(db *gorm.DB) ReplayOptions {
	return ReplayOptions{DB: db, Year: 2023, Week: 7,
		Cutoff: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
		Limits: ReplayLimits{MaxTeamRows: 512, MaxGameRows: 8192, MaxOlderRows: 6144}}
}

func seedOlder(t *testing.T, db *gorm.DB) {
	t.Helper()
	var games []database.Game
	// More than a quota, with no lower year floor. FBS status is determined
	// from the target season, not the historical opponents' divisions.
	for i := range 14 {
		games = append(games, database.Game{GameID: int64(4000 + i), Season: int64(2020 - i), Week: 1,
			Sport: "ncaaf", HomeID: 1, AwayID: 2, HomeScore: int64(20 + i), AwayScore: 7,
			StartTime: time.Date(2020-i, 9, 1, 0, 0, 0, 0, time.UTC)})
	}
	games = append(games,
		database.Game{GameID: 5001, Season: 2020, Sport: "ncaam", HomeID: 1, AwayID: 2,
			StartTime: time.Date(2020, 12, 1, 0, 0, 0, 0, time.UTC)},
		database.Game{GameID: 5002, Season: 2020, Sport: "ncaaf", HomeID: 1, AwayID: 5,
			StartTime: time.Date(2020, 11, 1, 0, 0, 0, 0, time.UTC)},
		database.Game{GameID: 5003, Season: 2020, Sport: "ncaaf", HomeID: 1, AwayID: 999,
			StartTime: time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)})
	if err := db.Create(&games).Error; err != nil {
		t.Fatal(err)
	}
}

type capturedRequest struct {
	Before    int64
	Opponents []int64
	Limit     int
	Games     []ranking.Game
}

func calculateCaptured(t *testing.T, in ranking.Input) map[int64]capturedRequest {
	t.Helper()
	requests := map[int64]capturedRequest{}
	callback := in.Backfill
	in.Backfill = func(id int64, opponents []int64, before int64, limit int) ([]ranking.Game, error) {
		games, err := callback(id, opponents, before, limit)
		canonical := slices.Clone(opponents)
		slices.Sort(canonical)
		requests[id] = capturedRequest{Before: before, Opponents: canonical, Limit: limit, Games: slices.Clone(games)}
		return games, err
	}
	ranker, err := ranking.NewRanker(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ranker.CalculateRanking(); err != nil {
		t.Fatal(err)
	}
	return requests
}

func TestReplayLiveRequestsAndOfflineAfterClose(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)
	seedOlder(t, db)
	// FCS in the historical season -> FBS in the target season is intentional.
	if err := db.Create(&database.TeamSeason{TeamID: 1, Year: 2020, FBS: 0, Sport: "ncaaf"}).Error; err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	opts := replayOpts(tx)
	lazy, err := Input(Options{DB: tx, Sport: sport.Football, Year: opts.Year})
	if err != nil {
		t.Fatal(err)
	}
	opts.Cutoff = lazy.StartTime
	opts.Week = lazy.Week
	live := calculateCaptured(t, lazy)
	explicit, err := InputAt(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(live, calculateCaptured(t, explicit)) {
		t.Fatal("explicit lazy requests differ from unmodified production Input")
	}
	predicted, err := sparseDeficits(lazy)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != len(predicted) || len(live) != 4 {
		t.Fatalf("requests live=%v predicted=%v", live, predicted)
	}
	for id, req := range live {
		if req.Limit != predicted[id] || req.Before != 2021 || !reflect.DeepEqual(req.Opponents, []int64{1, 2, 3, 4}) {
			t.Fatalf("request %d: %+v vs deficit %d", id, req, predicted[id])
		}
	}
	if len(live[1].Games) == 0 || live[1].Games[0].GameID != 4000 || len(live[3].Games) != 0 {
		t.Fatalf("expected nonempty/empty histories: %v", live)
	}
	snapshot, err := MaterializeAt(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	offline := calculateCaptured(t, snapshot.Input())
	if !reflect.DeepEqual(live, offline) {
		t.Fatalf("live/offline trace mismatch: live=%v offline=%v", live, offline)
	}
	meta := snapshot.Metadata()
	if meta.TeamRows != 5 || meta.BaseGameRows != 12 || meta.OlderGameRows != 24 || len(meta.OlderRequests) != 4 {
		t.Fatalf("metadata: %+v", meta)
	}
	for _, req := range meta.OlderRequests {
		if req.MaterializedLimit != 12 || (req.TeamID <= 2 && req.OldestReturnedSeason != 2009) {
			t.Fatalf("bounded top12 metadata: %+v", req)
		}
	}
}

func TestReplayDefensiveCopiesAndCallbackContract(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)
	seedOlder(t, db)
	snapshot, err := MaterializeAt(replayOpts(db))
	if err != nil {
		t.Fatal(err)
	}
	in := snapshot.Input()
	in.Teams[0].ID = 999
	in.Games[0].HomeScore = 999
	meta := snapshot.Metadata()
	meta.OlderRequests[0].OpponentIDs[0] = 999
	meta.OlderRequests[0].GameIDs[0] = 999
	meta.OlderRequests[0].TeamID = 999
	meta.OlderRequests = nil
	games, err := in.Backfill(1, []int64{4, 3, 2, 1}, 2021, 3)
	if err != nil || len(games) != 3 || games[0].GameID != 4000 || games[2].GameID != 4002 {
		t.Fatalf("ordered prefix: %v %v", games, err)
	}
	games[0].HomeScore = 999
	fresh := snapshot.Input()
	freshGames, err := fresh.Backfill(1, []int64{1, 2, 3, 4}, 2021, 1)
	if err != nil || freshGames[0].HomeScore == 999 || fresh.Teams[0].ID == 999 || fresh.Games[0].HomeScore == 999 {
		t.Fatal("mutable input/callback storage escaped")
	}
	if snapshot.Metadata().OlderRequests[0].OpponentIDs[0] == 999 ||
		snapshot.Metadata().OlderRequests[0].GameIDs[0] == 999 {
		t.Fatal("mutable metadata storage escaped")
	}
	for _, req := range []struct {
		id     int64
		opp    []int64
		before int64
		limit  int
	}{
		{999, []int64{1, 2, 3, 4}, 2021, 1},
		{1, []int64{1, 2, 3, 4}, 2020, 1},
		{1, []int64{1, 2, 3, 4}, 2021, 0},
		{1, []int64{1, 2, 3, 4}, 2021, 13},
		{1, []int64{1, 2, 3, 3}, 2021, 1},
		{1, []int64{1, 2, 3}, 2021, 1},
		{1, []int64{1, 2, 3, 5}, 2021, 1},
	} {
		if _, err := fresh.Backfill(req.id, req.opp, req.before, req.limit); err == nil {
			t.Fatalf("accepted unexpected request: %+v", req)
		}
	}
	empty, err := fresh.Backfill(3, []int64{1, 2, 3, 4}, 2021, 12)
	if err != nil || len(empty) != 0 {
		t.Fatalf("known empty history: %v %v", empty, err)
	}
}

func TestInputAtSharedSelectionAndInclusiveCutoff(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)
	seedBasketballData(t, db)
	// Cross-sport display names with the same ID cannot fan out the join.
	if err := db.Create(&database.TeamName{TeamID: 1, Sport: "ncaam", Name: "wrong"}).Error; err != nil {
		t.Fatal(err)
	}
	opts := replayOpts(db)
	opts.Cutoff = time.Date(2023, 9, 19, 4, 0, 0, 0, time.UTC)
	boundary := []database.Game{
		{GameID: 6001, Season: 2023, Sport: "ncaaf", HomeID: 1, AwayID: 999, StartTime: opts.Cutoff},
		{GameID: 6002, Season: 2023, Sport: "ncaaf", HomeID: 1, AwayID: 999, StartTime: opts.Cutoff.Add(time.Second)},
		{GameID: 6003, Season: 2023, Sport: "ncaam", HomeID: 1, AwayID: 2, StartTime: opts.Cutoff},
	}
	if err := db.Create(&boundary).Error; err != nil {
		t.Fatal(err)
	}
	explicit, err := InputAt(opts)
	if err != nil {
		t.Fatal(err)
	}
	weekly, err := Input(Options{DB: db, Sport: sport.Football, Year: 2023, Week: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(explicit.Teams, weekly.Teams) || !reflect.DeepEqual(explicit.Games, weekly.Games) {
		t.Fatal("explicit and weekly queries differ at identical cutoff")
	}
	if explicit.Year != 2023 || explicit.Week != 7 || explicit.Sport != sport.Football || explicit.Fcs ||
		explicit.Postseason || !explicit.StartTime.Equal(opts.Cutoff) {
		t.Fatalf("explicit window resolved unexpectedly: %+v", explicit)
	}
	if explicit.Games[0].GameID != 6001 || explicit.Games[0].AwayID != 999 {
		t.Fatal("inclusive/lower-division opponent missing")
	}
}

func TestReplayInvalidOptionsAndBounds(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)
	seedOlder(t, db)
	changes := []func(*ReplayOptions){
		func(o *ReplayOptions) { o.DB = nil },
		func(o *ReplayOptions) { o.Year = 0 },
		func(o *ReplayOptions) { o.Cutoff = time.Time{} },
		func(o *ReplayOptions) { o.Week = -1 },
		func(o *ReplayOptions) { o.Limits.MaxTeamRows = 0 },
		func(o *ReplayOptions) { o.Limits.MaxTeamRows = 513 },
		func(o *ReplayOptions) { o.Limits.MaxGameRows = 0 },
		func(o *ReplayOptions) { o.Limits.MaxGameRows = 8193 },
		func(o *ReplayOptions) { o.Limits.MaxOlderRows = 0 },
		func(o *ReplayOptions) { o.Limits.MaxOlderRows = 6145 },
		func(o *ReplayOptions) { o.Limits.MaxTeamRows = 4 },
		func(o *ReplayOptions) { o.Limits.MaxGameRows = 11 },
	}
	for i, change := range changes {
		opts := replayOpts(db)
		change(&opts)
		if _, err := InputAt(opts); err == nil {
			t.Errorf("InputAt accepted invalid option/bound %d", i)
		}
		if _, err := MaterializeAt(opts); err == nil {
			t.Errorf("MaterializeAt accepted invalid option/bound %d", i)
		}
	}
	opts := replayOpts(db)
	opts.Limits.MaxOlderRows = 23
	if _, err := MaterializeAt(opts); err == nil {
		t.Fatal("older aggregate overflow silently truncated")
	}
	opts.Limits.MaxOlderRows = 24
	opts.Limits.MaxTeamRows = 5
	opts.Limits.MaxGameRows = 12
	if _, err := MaterializeAt(opts); err != nil {
		t.Fatalf("exact bounds should pass: %v", err)
	}
}

func TestReplayRejectsCorruptRows(t *testing.T) {
	for _, kind := range []string{"future-season", "inverted-season", "old-after-cutoff", "zero-time", "bad-endpoint"} {
		t.Run(kind, func(t *testing.T) {
			db := setupTestDB(t)
			seedTestData(t, db)
			opts := replayOpts(db)
			row := database.Game{GameID: 7001, Season: 2023, Sport: "ncaaf", HomeID: 1, AwayID: 2,
				StartTime: opts.Cutoff}
			switch kind {
			case "future-season":
				row.Season = 2024
			case "inverted-season":
				row.Season = 2022 // Prior ahead of current games, without a calendar-year floor.
			case "old-after-cutoff":
				row.Season = 2020
				row.StartTime = opts.Cutoff.Add(time.Hour)
			case "zero-time":
				row.StartTime = time.Time{}
			case "bad-endpoint":
				row.AwayID = row.HomeID
			}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := MaterializeAt(opts); err == nil {
				t.Fatal("materialized corrupt input")
			}
			lazy, err := InputAt(opts)
			if kind == "old-after-cutoff" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := lazy.Backfill(1, []int64{1, 2, 3, 4}, 2021, 12); err == nil {
					t.Fatal("lazy path accepted old game beyond cutoff")
				}
			} else if err == nil {
				t.Fatal("lazy base accepted corrupt input")
			}
		})
	}
}

func TestReplayValidationHelpers(t *testing.T) {
	cutoff := replayOpts(nil).Cutoff
	valid := ranking.Game{GameID: 1, Season: 2020, HomeID: 1, AwayID: 2, StartTime: cutoff}
	bad := valid
	bad.HomeScore++
	if err := validateGames([]ranking.Game{bad}, cutoff, gameValues([]ranking.Game{valid})); err == nil {
		t.Fatal("inconsistent duplicate values accepted across requests")
	}
	for _, games := range [][]ranking.Game{
		{valid, valid},
		{{GameID: 0, HomeID: 1, AwayID: 2, StartTime: cutoff}},
		{{GameID: 2, HomeID: 1, AwayID: 2, StartTime: cutoff.Add(-time.Hour)}, valid},
	} {
		if err := validateGames(games, cutoff, nil); err == nil {
			t.Fatalf("accepted invalid order/ID/duplicate: %v", games)
		}
	}
	if err := validateOlder([]ranking.Game{valid}, cutoff, 3, []int64{1, 2, 3}, 2021, nil); err == nil {
		t.Fatal("wrong older endpoints accepted")
	}
	if err := validateOlder([]ranking.Game{valid}, cutoff, 1, []int64{1, 2, 3}, 2020, nil); err == nil {
		t.Fatal("wrong older season accepted")
	}
}

func TestReplayQuotaTraversal(t *testing.T) {
	teams := []ranking.TeamInfo{{ID: 1, FBS: true}, {ID: 2, FBS: true}, {ID: 3, FBS: true}}
	for _, currentCount := range []int{0, 3, 12, 14} {
		for _, priorCount := range []int{0, 4, 15} {
			in := ranking.Input{Sport: sport.Football, Year: 2023, Teams: teams}
			for i := range currentCount + priorCount {
				season := int64(2022)
				if i < currentCount {
					season = 2023
				}
				in.Games = append(in.Games, ranking.Game{GameID: int64(i + 1), Season: season,
					HomeID: 1, AwayID: 2, HomeScore: int64(i % 3), AwayScore: int64(i % 2)})
			}
			in.Backfill = func(_ int64, _ []int64, _ int64, _ int) ([]ranking.Game, error) { return nil, nil }
			deficits, err := sparseDeficits(in)
			if err != nil {
				t.Fatal(err)
			}
			live := calculateCaptured(t, in)
			if len(live) != len(deficits) || live[3].Limit != 12 {
				t.Fatalf("current=%d prior=%d live=%v deficits=%v", currentCount, priorCount, live, deficits)
			}
			for id, request := range live {
				if request.Limit != deficits[id] {
					t.Fatalf("team %d request=%d deficit=%d", id, request.Limit, deficits[id])
				}
			}
			want := max(0, 12-currentCount-priorCount)
			if deficits[1] != want || deficits[2] != want {
				t.Fatalf("current=%d prior=%d unexpected deficits %v", currentCount, priorCount, deficits)
			}
		}
	}
}

func TestReplayDuplicateTeamRows(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)
	// Model historical corruption without changing the production schema:
	// duplicate joined name rows must fail rather than fan out the team set.
	if err := db.Exec("DROP TABLE team_names").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE team_names (team_id integer, name text, sport text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO team_names VALUES (1, 'Alpha', 'ncaaf'), (1, 'Other', 'ncaaf')").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := InputAt(replayOpts(db)); err == nil {
		t.Fatal("duplicate joined team rows accepted")
	}
}

func TestReplayCurrentGamesBehindUnrelatedPriorBreak(t *testing.T) {
	in := ranking.Input{Sport: sport.Football, Year: 2023,
		Teams: []ranking.TeamInfo{{ID: 1, FBS: true}, {ID: 2, FBS: true}, {ID: 3, FBS: true}}}
	for i := range 12 {
		in.Games = append(in.Games, ranking.Game{GameID: int64(i + 1), Season: 2023, HomeID: 1, AwayID: 2})
	}
	in.Games = append(in.Games,
		ranking.Game{GameID: 13, Season: 2022, HomeID: 2, AwayID: 3},
		ranking.Game{GameID: 14, Season: 2023, HomeID: 1, AwayID: 2})
	if _, err := sparseDeficits(in); err == nil {
		t.Fatal("current game hidden by unrelated prior-row break accepted")
	}
}

func TestReplayRejectsUnmaterializedNonSparseTeam(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)
	var games []database.Game
	for i := range 12 {
		games = append(games, database.Game{GameID: int64(8000 + i), Season: 2023, Sport: "ncaaf",
			HomeID: 1, AwayID: 2, StartTime: time.Date(2023, 9, 1, i, 0, 0, 0, time.UTC)})
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := MaterializeAt(replayOpts(db))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.Input().Backfill(1, []int64{1, 2, 3, 4}, 2021, 1); err == nil {
		t.Fatal("non-sparse team treated as valid empty materialized history")
	}
}

func TestReplaySharedQueryBoundsAndOrdering(t *testing.T) {
	db := setupTestDB(t).Session(&gorm.Session{DryRun: true})
	opts := replayOpts(db)
	var rows []database.Game
	older := olderGameQuery(db, sport.Football, 1, []int64{1, 2, 3, 4}, 2021, 12).Find(&rows).Statement
	sql := older.SQL.String()
	if !strings.Contains(sql, "sport = ? and season < ?") || !strings.Contains(sql, "ORDER BY start_time desc LIMIT 12") ||
		strings.Contains(sql, "start_time <=") || strings.Contains(sql, "season >=") ||
		strings.Contains(sql, "game_id desc") {
		t.Fatalf("older query must retain production search/order and top12 bound: %s", sql)
	}
	base := baseGameQuery(db, sport.Football, opts.Year, opts.Cutoff).Limit(opts.Limits.MaxGameRows + 1).
		Find(&rows).Statement.SQL.String()
	if !strings.Contains(base, "sport = ? and season >= ? and start_time <= ?") ||
		!strings.Contains(base, "ORDER BY start_time desc LIMIT 8193") || strings.Contains(base, "season <=") {
		t.Fatalf("base query must preserve inclusive cutoff/no upper season predicate: %s", base)
	}
}
