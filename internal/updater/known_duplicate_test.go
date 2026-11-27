package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/game"
	"github.com/robby-barton/stats-go/internal/sport"
)

const (
	canonicalArmyTempleID int64 = 323220349
	duplicateArmyTempleID int64 = 400368353
)

func testDuplicateRule(t *testing.T, u *Updater, id int64) duplicateRule {
	t.Helper()
	rule, ok := u.duplicateRule(id)
	if !ok {
		t.Fatalf("no duplicate rule for %s event %d", u.sportDB(), id)
	}
	return rule
}

// repairedArmyTemple is the reviewed canonical game, including the six passing
// rows, the single rusher, and both corrected punt inside_20 values. It seeds
// canonical stats without touching the protected duplicate.
func repairedArmyTemple() *game.ParsedGameInfo {
	const id = canonicalArmyTempleID
	return &game.ParsedGameInfo{
		Info: game.Info{
			GameID: id, Season: 2012, Week: 12,
			StartTime: time.Date(2012, 11, 17, 17, 0, 0, 0, time.UTC),
			HomeID:    349, HomeScore: 32, AwayID: 218, AwayScore: 63,
		},
		TeamStats: []game.TeamGameStats{
			{GameID: id, TeamID: 218, Drives: 12, Possession: 1831, FourthDownsConv: 2},
			{GameID: id, TeamID: 349, Drives: 13, Fumbles: 2, Possession: 1728, FourthDownsConv: 1},
		},
		PassingStats: []game.PassingStats{
			{GameID: id, PlayerID: -1, TeamID: 218, Completions: 2, Attempts: 4, Yards: 31},
			{GameID: id, PlayerID: 513397, TeamID: 218, Completions: 2, Attempts: 4, Yards: 31},
			{GameID: id, PlayerID: -1, TeamID: 349, Completions: 4, Attempts: 9, Yards: 88},
			{GameID: id, PlayerID: 483260, TeamID: 349, Attempts: 1},
			{GameID: id, PlayerID: 483302, TeamID: 349, Completions: 3, Attempts: 7, Yards: 70},
			{GameID: id, PlayerID: 535646, TeamID: 349, Completions: 1, Attempts: 1, Yards: 18},
		},
		RushingStats: []game.RushingStats{{GameID: id, PlayerID: 2586749, TeamID: 218, Carries: 1, RushYards: -2}},
		PuntStats: []game.PuntStats{
			{GameID: id, PlayerID: 482164, TeamID: 218, PuntNo: 1, PuntYards: 52, PuntLong: 52, Inside20: 1},
			{GameID: id, PlayerID: -1, TeamID: 218, PuntNo: 1, PuntYards: 52, PuntLong: 52, Inside20: 1},
		},
		ReceivingStats:    []game.ReceivingStats{{GameID: id, PlayerID: 483302, TeamID: 349, RecLong: 4}},
		KickStats:         []game.KickStats{{GameID: id, PlayerID: -1, TeamID: 349, FGLong: 30}},
		ReturnStats:       []game.ReturnStats{{GameID: id, PlayerID: 503597, TeamID: 349, PuntKick: "punt", RetLong: -2}},
		FumbleStats:       []game.FumbleStats{{GameID: id, PlayerID: 503597, TeamID: 349, Fumbles: 1}},
		DefensiveStats:    []game.DefensiveStats{{GameID: id, PlayerID: 503597, TeamID: 349, Sacks: 0.5}},
		InterceptionStats: []game.InterceptionStats{{GameID: id, PlayerID: 503597, TeamID: 349, Interceptions: 1}},
	}
}

// duplicateSchedule is the retired duplicate as it appears in a scoreboard.
func duplicateSchedule() espn.Game {
	return finalScheduleGame(duplicateArmyTempleID, []espn.Competitor{
		{ID: 218, Team: espn.ScheduleTeam{ID: 218}, Score: 63, HomeAway: scheduleAway},
		{ID: 349, Team: espn.ScheduleTeam{ID: 349}, Score: 32, HomeAway: "home"},
	})
}

func observedUpdater(t *testing.T) (*Updater, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	u := newTestUpdaterWithLogger(t, zap.New(core))
	return u, logs
}

func summaryHandler(t *testing.T, wantID int64, calls *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		*calls++
		if got := r.URL.Query().Get("event"); got != strconv.FormatInt(wantID, 10) {
			t.Errorf("unexpected summary event %q, want %d", got, wantID)
			http.Error(w, "unexpected summary", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(newGameInfo(wantID, 1, 28, 2, 14, 1, true)); err != nil {
			t.Error(err)
		}
	}
}

func assertNoDuplicateRow(t *testing.T, u *Updater) {
	t.Helper()
	var count int64
	if err := u.db.Model(&database.Game{}).
		Where("game_id = ?", duplicateArmyTempleID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("protected duplicate row exists (count=%d)", count)
	}
}

// ingestionSnapshot captures every column of every game/stat table so tests can
// prove a refusal performed no writes, not merely that rows happen to match.
func ingestionSnapshot(t *testing.T, db *gorm.DB) string {
	t.Helper()
	all := map[string][]map[string]any{}
	for _, table := range []string{"games", "team_game_stats", "passing_stats", "rushing_stats",
		"receiving_stats", "fumble_stats", "defensive_stats", "interception_stats",
		"return_stats", "kick_stats", "punt_stats"} {
		var rows []map[string]any
		if err := db.Table(table).Order("game_id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		all[table] = rows
	}
	b, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func forbidIngestionDML(t *testing.T, db *gorm.DB) {
	t.Helper()
	check := func(*gorm.DB) { t.Error("protected import attempted DML") }
	if err := db.Callback().Create().Before("gorm:create").Register("test:no_create", check); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:no_update", check); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:no_delete", check); err != nil {
		t.Fatal(err)
	}
}

func siteEventFor(id, homeID, homeScore, awayID, awayScore int64) espn.SiteEvent {
	return newSiteEvent(id, homeID, 0, homeScore, awayID, 0, awayScore)
}

func protectedScoreboard() espn.SiteScoreboardESPN {
	return espn.SiteScoreboardESPN{
		Leagues: []espn.SiteScoreboardLeague{{Season: espn.SiteScoreboardLeagueSeason{Year: 2023}}},
		Season:  espn.SiteSeason{Year: 2023, Type: 2},
		Events: []espn.SiteEvent{
			siteEventFor(duplicateArmyTempleID, 349, 32, 218, 63),
			siteEventFor(fixtureGameID1, 1, 28, 2, 14),
		},
	}
}

func TestProtectedDuplicate_MixedBatchSkipsAndImportsUnrelated(t *testing.T) {
	u, logs := observedUpdater(t)
	if err := u.insertGameInfo(repairedArmyTemple()); err != nil {
		t.Fatal(err)
	}

	var summaryCalls int
	ts := httptest.NewServer(summaryHandler(t, fixtureGameID1, &summaryCalls))
	t.Cleanup(ts.Close)
	u.espn.(*espn.FootballClient).SetURLs(ts.URL+"/summary?event=%d", "", "", "")

	result, err := u.processGames(context.Background(),
		[]espn.Game{duplicateSchedule(), {ID: fixtureGameID1}})
	if err != nil {
		t.Fatalf("processGames: %v", err)
	}
	if !reflect.DeepEqual(result.Processed, []int64{fixtureGameID1}) {
		t.Fatalf("Processed = %v, want [%d]", result.Processed, fixtureGameID1)
	}
	if !reflect.DeepEqual(result.SkippedIDs(), []int64{duplicateArmyTempleID}) {
		t.Fatalf("SkippedIDs = %v, want [%d]", result.SkippedIDs(), duplicateArmyTempleID)
	}
	if len(result.Failed) != 0 {
		t.Fatalf("Failed = %v, want none", result.Failed)
	}
	if summaryCalls != 1 {
		t.Fatalf("summary requests = %d, want 1", summaryCalls)
	}
	if got := logs.FilterMessage(knownEventLogMessage).Len(); got != 1 {
		t.Fatalf("protected warnings = %d, want 1", got)
	}
	assertNoDuplicateRow(t, u)

	var canonical database.Game
	if err := u.db.Where("game_id = ? and sport = ?", canonicalArmyTempleID, sport.Football).
		First(&canonical).Error; err != nil {
		t.Fatal(err)
	}
	if !testDuplicateRule(t, u, duplicateArmyTempleID).matchesHeader(canonical) {
		t.Fatalf("canonical row changed: %+v", canonical)
	}
}

func TestProtectedDuplicate_CurrentWeekDiscoverySkipsAndImportsUnrelated(t *testing.T) {
	u, logs := observedUpdater(t)
	if err := u.insertGameInfo(repairedArmyTemple()); err != nil {
		t.Fatal(err)
	}

	var summaryCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/scoreboard", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(protectedScoreboard()); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/summary", summaryHandler(t, fixtureGameID1, &summaryCalls))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	u.espn.(*espn.FootballClient).SetURLs(
		ts.URL+"/summary?event=%d", "", ts.URL+"/scoreboard", "")

	result, err := u.UpdateCurrentWeek(context.Background())
	if err != nil {
		t.Fatalf("UpdateCurrentWeek: %v", err)
	}
	if !reflect.DeepEqual(result.Processed, []int64{fixtureGameID1}) {
		t.Fatalf("Processed = %v, want [%d]", result.Processed, fixtureGameID1)
	}
	if !reflect.DeepEqual(result.SkippedIDs(), []int64{duplicateArmyTempleID}) {
		t.Fatalf("SkippedIDs = %v, want [%d]", result.SkippedIDs(), duplicateArmyTempleID)
	}
	if summaryCalls != 1 {
		t.Fatalf("summary requests = %d, want 1", summaryCalls)
	}
	if got := logs.FilterMessage(knownEventLogMessage).Len(); got != 1 {
		t.Fatalf("protected warnings = %d, want 1", got)
	}
	assertNoDuplicateRow(t, u)
}

func TestProtectedDuplicate_HistoricalDiscoverySkipsAndImportsUnrelated(t *testing.T) {
	u, logs := observedUpdater(t)
	if err := u.insertGameInfo(repairedArmyTemple()); err != nil {
		t.Fatal(err)
	}

	var summaryCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/scoreboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("dates") == "2012" {
			calendar := espn.SiteScoreboardESPN{
				Leagues: []espn.SiteScoreboardLeague{{
					Season: espn.SiteScoreboardLeagueSeason{Year: 2012},
					Calendar: espn.SiteCalendar{{
						Label: "Regular Season", Value: 2,
						StartDate: "2012-11-17T07:00Z", EndDate: "2012-11-17T07:59Z",
						Entries: []espn.SiteCalendarWeek{{Label: "Week 12", Value: 12}},
					}},
				}},
			}
			if err := json.NewEncoder(w).Encode(calendar); err != nil {
				t.Error(err)
			}
			return
		}
		board := protectedScoreboard()
		for i := range board.Leagues {
			board.Leagues[i].Season.Year = 2012
		}
		if err := json.NewEncoder(w).Encode(board); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/summary", summaryHandler(t, fixtureGameID1, &summaryCalls))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	u.espn.(*espn.FootballClient).SetURLs(
		ts.URL+"/summary?event=%d", "", ts.URL+"/scoreboard", "")

	result, err := u.UpdateGamesForYear(context.Background(), 2012)
	if err != nil {
		t.Fatalf("UpdateGamesForYear: %v", err)
	}
	if !reflect.DeepEqual(result.Processed, []int64{fixtureGameID1}) {
		t.Fatalf("Processed = %v, want [%d]", result.Processed, fixtureGameID1)
	}
	if !reflect.DeepEqual(result.SkippedIDs(), []int64{duplicateArmyTempleID}) {
		t.Fatalf("SkippedIDs = %v, want [%d]", result.SkippedIDs(), duplicateArmyTempleID)
	}
	if summaryCalls != 1 {
		t.Fatalf("summary requests = %d, want 1", summaryCalls)
	}
	if got := logs.FilterMessage(knownEventLogMessage).Len(); got != 1 {
		t.Fatalf("protected warnings = %d, want 1", got)
	}
	assertNoDuplicateRow(t, u)
}

func TestProtectedDuplicate_ExistingMatchingScoresStillReported(t *testing.T) {
	u := newTestUpdater(t)
	row := gameInfoToDB(repairedArmyTemple().Info, sport.Football)
	row.GameID = duplicateArmyTempleID
	if err := u.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	before := ingestionSnapshot(t, u.db)
	forbidIngestionDML(t, u.db)
	games, err := u.checkGames([]espn.Game{duplicateSchedule()})
	if err != nil {
		t.Fatalf("checkGames: %v", err)
	}
	result, err := u.processGames(t.Context(), games)
	if err != nil || !reflect.DeepEqual(result.SkippedIDs(), []int64{duplicateArmyTempleID}) {
		t.Fatalf("the existing duplicate was not reported: %+v, %v", result, err)
	}
	if after := ingestionSnapshot(t, u.db); after != before {
		t.Fatal("the guard changed an existing duplicate")
	}
}

func TestProtectedDuplicate_CanonicalReadFailureStillReportsSkip(t *testing.T) {
	u, logs := observedUpdater(t)
	if err := u.db.Migrator().DropTable(&database.Game{}); err != nil {
		t.Fatal(err)
	}
	result, err := u.processGames(t.Context(), []espn.Game{duplicateSchedule()})
	if err != nil || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "could not be verified") {
		t.Fatalf("canonical read error did not yield a skip: %+v, %v", result, err)
	}
	if got := logs.FilterMessage(knownEventLogMessage).Len(); got != 1 {
		t.Fatalf("protected warnings = %d, want 1", got)
	}
}

func TestProtectedDuplicate_ExplicitSingleGameRefusedWithoutFetch(t *testing.T) {
	u, logs := observedUpdater(t)
	if err := u.insertGameInfo(repairedArmyTemple()); err != nil {
		t.Fatal(err)
	}

	var summaryCalls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		summaryCalls++
		http.Error(w, "unexpected summary", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)
	u.espn.(*espn.FootballClient).SetURLs(ts.URL+"/summary?event=%d", "", "", "")

	err := u.UpdateSingleGame(context.Background(), duplicateArmyTempleID)
	if err == nil || !strings.Contains(err.Error(), "known-event protection") {
		t.Fatalf("expected known-event refusal, got %v", err)
	}
	if summaryCalls != 0 {
		t.Fatalf("summary requests = %d, want 0", summaryCalls)
	}
	if got := logs.FilterMessage(knownEventLogMessage).Len(); got != 1 {
		t.Fatalf("protected warnings = %d, want 1", got)
	}
	assertNoDuplicateRow(t, u)
}

func TestProtectedDuplicate_DirectWriteAndRetryRefused(t *testing.T) {
	u := newTestUpdater(t)
	if err := u.insertGameInfo(repairedArmyTemple()); err != nil {
		t.Fatal(err)
	}
	before := ingestionSnapshot(t, u.db)
	forbidIngestionDML(t, u.db)

	direct := repairedArmyTemple()
	direct.Info.GameID = duplicateArmyTempleID
	if err := u.insertGameInfo(direct); err == nil ||
		!strings.Contains(err.Error(), "known-event protection") {
		t.Fatalf("stat write not refused: %v", err)
	}
	if err := u.markGameRetry(duplicateSchedule()); err == nil ||
		!strings.Contains(err.Error(), "known-event protection") {
		t.Fatalf("retry write not refused: %v", err)
	}
	if after := ingestionSnapshot(t, u.db); after != before {
		t.Fatal("guard performed DML")
	}
}

func TestProtectedDuplicate_FailClosedOnCanonicalMetadata(t *testing.T) {
	mutations := map[string]func(*database.Game){
		"missing":     nil,
		"season":      func(g *database.Game) { g.Season++ },
		"week":        func(g *database.Game) { g.Week++ },
		"start":       func(g *database.Game) { g.StartTime = g.StartTime.Add(time.Second) },
		"home":        func(g *database.Game) { g.HomeID++ },
		"away":        func(g *database.Game) { g.AwayID++ },
		"home score":  func(g *database.Game) { g.HomeScore++ },
		"away score":  func(g *database.Game) { g.AwayScore++ },
		"postseason":  func(g *database.Game) { g.Postseason = 1 },
		"neutral":     func(g *database.Game) { g.Neutral = true },
		"conference":  func(g *database.Game) { g.ConfGame = true },
		"retry":       func(g *database.Game) { g.Retry = 1 },
		"wrong sport": func(g *database.Game) { g.Sport = sport.Basketball },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			u := newTestUpdater(t)
			if mutate != nil {
				g := gameInfoToDB(repairedArmyTemple().Info, u.sportDB())
				mutate(&g)
				if err := u.db.Create(&g).Error; err != nil {
					t.Fatal(err)
				}
			}
			protected := u.protectedDuplicate(testDuplicateRule(t, u, duplicateArmyTempleID), "")
			if protected.GameID != duplicateArmyTempleID || protected.Reason == "" {
				t.Fatalf("expected a fail-closed diagnostic, got %+v", protected)
			}
		})
	}
}

func TestProtectedDuplicate_SourceIdentityMismatchStillSkips(t *testing.T) {
	u := newTestUpdater(t)
	if err := u.insertGameInfo(repairedArmyTemple()); err != nil {
		t.Fatal(err)
	}

	cases := map[string]espn.Game{
		"malformed": {ID: duplicateArmyTempleID},
		"wrong home score": func() espn.Game {
			g := duplicateSchedule()
			g.Competitions[0].Competitors[1].Score = 31
			return g
		}(),
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			rule := testDuplicateRule(t, u, g.ID)
			protected := u.protectedDuplicate(rule, rule.scheduleSourceDetail(g))
			if !strings.Contains(protected.Reason, "source schedule identity") {
				t.Fatalf("reason = %+v, want a source identity diagnostic", protected)
			}
		})
	}
}

func TestKnownDuplicateHeadersNeverRetireACanonicalEvent(t *testing.T) {
	headers := knownDuplicateHeaders()
	for key, want := range headers {
		if key.sport.Validate() != nil || key.eventID <= 0 || want.GameID <= 0 {
			t.Fatalf("invalid registry entry: %+v -> %+v", key, want)
		}
		if _, retired := headers[duplicateKey{sport: key.sport, eventID: want.GameID}]; retired {
			t.Fatalf("canonical event %s/%d is itself retired", key.sport, want.GameID)
		}
	}
}

func TestProtectedDuplicate_SecondRegistryEntryUsesItsOwnCanonicalGame(t *testing.T) {
	u, logs := observedUpdater(t)
	const retiredID int64 = 700002
	const canonicalID int64 = 700001
	want := database.Game{
		GameID: canonicalID, Sport: sport.Football, Season: 2020, Week: 7,
		StartTime: time.Date(2020, 10, 17, 12, 0, 0, 0, time.UTC),
		HomeID:    11, AwayID: 22, HomeScore: 7, AwayScore: 3, Neutral: true,
	}
	u.duplicateHeaders[duplicateKey{sport: sport.Football, eventID: retiredID}] = want
	if err := u.db.Create(&want).Error; err != nil {
		t.Fatal(err)
	}
	second := finalScheduleGame(retiredID, []espn.Competitor{
		{ID: want.HomeID, Team: espn.ScheduleTeam{ID: want.HomeID}, Score: want.HomeScore, HomeAway: scheduleHome},
		{ID: want.AwayID, Team: espn.ScheduleTeam{ID: want.AwayID}, Score: want.AwayScore, HomeAway: scheduleAway},
	})
	before := ingestionSnapshot(t, u.db)
	forbidIngestionDML(t, u.db)
	result, err := u.processGames(t.Context(), []espn.Game{duplicateSchedule(), second})
	if err != nil || len(result.Skipped) != 2 || len(result.Processed) != 0 || len(result.Failed) != 0 {
		t.Fatalf("expected both registered events skipped: %+v, %v", result, err)
	}
	first, next := result.Skipped[0], result.Skipped[1]
	if first.CanonicalGameID != canonicalArmyTempleID || next.GameID != retiredID ||
		next.CanonicalGameID != canonicalID || next.Sport != sport.Football ||
		!strings.Contains(next.Reason, "canonical stats preserved") {
		t.Fatalf("registry diagnostics did not use each event's own header: %+v", result.Skipped)
	}
	err = u.UpdateSingleGame(t.Context(), retiredID)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("canonical %d", canonicalID)) {
		t.Fatalf("direct refusal did not use the registered canonical ID: %v", err)
	}
	if logs.FilterMessage(knownEventLogMessage).Len() != 3 || ingestionSnapshot(t, u.db) != before {
		t.Fatal("registry addition did not preserve reporting and no-write behavior")
	}
}

func newTestBasketballUpdater(t *testing.T) *Updater {
	t.Helper()
	client := &espn.BasketballClient{Client: &espn.Client{
		MaxAttempts:    1,
		InitialBackoff: time.Millisecond,
		RequestTimeout: 5 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeBasketball,
	}}
	u, err := NewUpdater(setupTestDB(t), zap.NewNop().Sugar(), client)
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	return u
}

func TestProtectedDuplicate_CanonicalAndUnrelatedUnaffected(t *testing.T) {
	u, logs := observedUpdater(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id int64
		if _, err := fmt.Sscanf(r.URL.Query().Get("event"), "%d", &id); err != nil {
			t.Error(err)
			http.Error(w, "bad event", http.StatusBadRequest)
			return
		}
		var resp espn.GameInfoESPN
		if id == canonicalArmyTempleID {
			resp = newGameInfo(id, 349, 32, 218, 63, 12, false)
			resp.GamePackage.Header.Season.Year = 2012
			resp.GamePackage.Header.Season.Type = int64(espn.Regular)
			resp.GamePackage.Header.Competitions[0].Date = "2012-11-17T17:00Z"
		} else {
			resp = newGameInfo(id, 1, 28, 2, 14, 1, true)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(ts.Close)
	u.espn.(*espn.FootballClient).SetURLs(ts.URL+"/summary?event=%d", "", "", "")

	if err := u.UpdateSingleGame(context.Background(), canonicalArmyTempleID); err != nil {
		t.Fatalf("canonical fetch blocked: %v", err)
	}
	if err := u.UpdateSingleGame(context.Background(), fixtureGameID1); err != nil {
		t.Fatalf("unrelated fetch blocked: %v", err)
	}
	if got := logs.FilterMessage(knownEventLogMessage).Len(); got != 0 {
		t.Fatalf("unexpected protected warnings: %d", got)
	}

	var canonical database.Game
	if err := u.db.Where("game_id = ? and sport = ?", canonicalArmyTempleID, sport.Football).
		First(&canonical).Error; err != nil {
		t.Fatal(err)
	}
	if !testDuplicateRule(t, u, duplicateArmyTempleID).matchesHeader(canonical) {
		t.Fatalf("canonical identity not stored: %+v", canonical)
	}

	// The same numeric ID in basketball is outside this exact NCAAF rule.
	bb := newTestBasketballUpdater(t)
	bbTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(
			newGameInfo(duplicateArmyTempleID, 349, 32, 218, 63, 12, false)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(bbTS.Close)
	bb.espn.(*espn.BasketballClient).SetURLs(bbTS.URL+"/summary?event=%d", "", "", "")

	bbResult, err := bb.processGames(context.Background(),
		[]espn.Game{{ID: duplicateArmyTempleID}})
	if err != nil {
		t.Fatalf("basketball import: %v", err)
	}
	if !reflect.DeepEqual(bbResult.Processed, []int64{duplicateArmyTempleID}) || len(bbResult.Skipped) != 0 {
		t.Fatalf("basketball same-ID result = %+v, want processed with no skips", bbResult)
	}
}
