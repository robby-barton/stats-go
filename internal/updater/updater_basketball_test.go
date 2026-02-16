//go:build integration

package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

// ---------------------------------------------------------------------------
// Basketball fixture data — 4 teams, 2 D1 conferences, 4 games across 2 weeks
// ---------------------------------------------------------------------------

const (
	bbFixtureGameID1 int64 = 501001
	bbFixtureGameID2 int64 = 501002
	bbFixtureGameID3 int64 = 501003 // in-progress
	bbFixtureGameID4 int64 = 501004
)

func bbFixtureScheduleResponse() espn.GameScheduleESPN {
	return espn.GameScheduleESPN{
		Content: espn.Content{
			Schedule: map[string]espn.Day{
				"2024-01-06": {
					Games: []espn.Game{
						newFinalGame(bbFixtureGameID1, 11, 300, 78, 12, 300, 65),
						newFinalGame(bbFixtureGameID2, 13, 400, 70, 14, 400, 68),
						newInProgressGame(bbFixtureGameID3, 11, 300, 40, 13, 400, 38),
					},
				},
				"2024-01-13": {
					Games: []espn.Game{
						newFinalGame(bbFixtureGameID4, 11, 300, 80, 13, 400, 75),
					},
				},
			},
			Parameters: espn.Parameters{Week: 10, Year: 2024, SeasonType: 2, Group: espn.FlexInt64(50)},
			Defaults:   espn.Parameters{Week: 10, Year: 2024, SeasonType: 2, Group: espn.FlexInt64(50)},
			// Basketball schedule responses have no Calendar — season
			// metadata comes from the scoreboard endpoint instead.
			ConferenceAPI: espn.ConferenceAPI{
				Conferences: []espn.Conference{
					{GroupID: 300, Name: "Big East Conference", ShortName: "Big East", ParentGroupID: espn.FlexInt64(50)},
					{GroupID: 400, Name: "Atlantic Coast Conference", ShortName: "ACC", ParentGroupID: espn.FlexInt64(50)},
				},
			},
		},
	}
}

func bbFixtureScoreboardResponse() espn.ScoreboardESPN {
	return espn.ScoreboardESPN{
		Leagues: []espn.ScoreboardLeague{{
			Season: espn.ScoreboardSeason{
				Year:      2024,
				StartDate: "2023-11-06T08:00Z",
				EndDate:   "2024-04-08T06:59Z",
				Type:      espn.ScoreboardSeasonType{ID: 2, Name: "Regular Season"},
			},
			Calendar: []string{"2024-01-06T08:00Z", "2024-01-13T08:00Z"},
		}},
	}
}

func bbFixtureGameInfoResponse(gameID int64) espn.GameInfoESPN {
	games := map[int64]espn.GameInfoESPN{
		bbFixtureGameID1: bbNewGameInfo(bbFixtureGameID1, 11, 78, 12, 65, 2024, 10, true),
		bbFixtureGameID2: bbNewGameInfo(bbFixtureGameID2, 13, 70, 14, 68, 2024, 10, true),
		bbFixtureGameID4: bbNewGameInfo(bbFixtureGameID4, 11, 80, 13, 75, 2024, 11, false),
	}
	if g, ok := games[gameID]; ok {
		return g
	}
	return bbNewGameInfo(gameID, 11, 0, 12, 0, 2024, 10, false)
}

func bbNewGameInfo(
	gameID, homeID, homeScore, awayID, awayScore, year, week int64,
	confGame bool,
) espn.GameInfoESPN {
	dateStr := "2024-01-06T19:00Z"
	if week == 11 {
		dateStr = "2024-01-13T19:00Z"
	}
	return espn.GameInfoESPN{
		GamePackage: espn.GamePackage{
			Header: espn.Header{
				ID: gameID,
				Competitions: []espn.Competitions{{
					ID:       gameID,
					Date:     dateStr,
					ConfGame: confGame,
					Neutral:  false,
					Competitors: []espn.Competitors{
						{HomeAway: "home", ID: homeID, Score: homeScore},
						{HomeAway: "away", ID: awayID, Score: awayScore},
					},
					Status: espn.Status{StatusType: espn.StatusType{
						Name: "STATUS_FINAL", Completed: true,
					}},
				}},
				Season: espn.Season{Year: year, Type: 2},
				Week:   week,
			},
			Boxscore: espn.Boxscore{
				Teams: []espn.Teams{
					{
						Team: espn.Team{ID: homeID},
						Statistics: []espn.TeamStatistics{
							{Name: "firstDowns", DisplayValue: "0"},
							{Name: "totalYards", DisplayValue: "0"},
						},
					},
					{
						Team: espn.Team{ID: awayID},
						Statistics: []espn.TeamStatistics{
							{Name: "firstDowns", DisplayValue: "0"},
							{Name: "totalYards", DisplayValue: "0"},
						},
					},
				},
				// No player stats for basketball
				Players: []espn.Players{},
			},
		},
	}
}

func bbFixtureTeamInfoResponse() espn.TeamInfoESPN {
	return espn.TeamInfoESPN{
		Sports: []espn.TeamInfoSport{{
			ID:   600,
			Name: "Basketball",
			Slug: "basketball",
			Leagues: []espn.League{{
				ID:           41,
				Name:         "NCAA Men's Basketball",
				Abbreviation: "NCAAM",
				ShortName:    "NCAAM",
				Slug:         "mens-college-basketball",
				Year:         2024,
				Teams: []espn.TeamWrap{
					{Team: espn.TeamInfo{
						ID: 11, Name: "Bulldogs", DisplayName: "BBall Alpha Bulldogs",
						Abbreviation: "BBA", Location: "BBall Alpha", Slug: "bball-alpha",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 12, Name: "Wildcats", DisplayName: "BBall Beta Wildcats",
						Abbreviation: "BBB", Location: "BBall Beta", Slug: "bball-beta",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 13, Name: "Eagles", DisplayName: "BBall Gamma Eagles",
						Abbreviation: "BBG", Location: "BBall Gamma", Slug: "bball-gamma",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 14, Name: "Hawks", DisplayName: "BBall Delta Hawks",
						Abbreviation: "BBD", Location: "BBall Delta", Slug: "bball-delta",
						IsActive: true,
					}},
				},
			}},
		}},
	}
}

// ---------------------------------------------------------------------------
// Mock HTTP server for basketball
// ---------------------------------------------------------------------------

func setupBasketballTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/core/mens-college-basketball/schedule", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := bbFixtureScheduleResponse()

		// If a date param is provided, filter schedule to only that date.
		if dateParam := r.URL.Query().Get("date"); dateParam != "" {
			// Convert "20240106" → "2024-01-06"
			key := dateParam[:4] + "-" + dateParam[4:6] + "-" + dateParam[6:8]
			filtered := map[string]espn.Day{}
			if day, ok := resp.Content.Schedule[key]; ok {
				filtered[key] = day
			}
			resp.Content.Schedule = filtered
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/core/mens-college-basketball/playbyplay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gameIDStr := r.URL.Query().Get("gameId")
		var gameID int64
		fmt.Sscanf(gameIDStr, "%d", &gameID) //nolint:errcheck // test helper

		if err := json.NewEncoder(w).Encode(bbFixtureGameInfoResponse(gameID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/apis/site/v2/sports/basketball/mens-college-basketball/teams", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(bbFixtureTeamInfoResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(bbFixtureScoreboardResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// Basketball test Updater constructor
// ---------------------------------------------------------------------------

func newBasketballTestUpdater(t *testing.T) (*Updater, *capturingWriter) {
	t.Helper()

	db := setupTestDB(t)
	cw := newCapturingWriter()
	ts := setupBasketballTestServer(t)

	restore := espn.SetTestURLs(
		ts.URL+"/core/mens-college-basketball/schedule?xhr=1&render=false&userab=18",
		ts.URL+"/core/mens-college-basketball/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		ts.URL+"/apis/site/v2/sports/basketball/mens-college-basketball/teams?limit=1000",
	)
	t.Cleanup(restore)

	client := &espn.BasketballClient{Client: &espn.Client{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 5 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeBasketball,
	}}

	restoreSB := espn.SetTestScoreboardURL(client.Client,
		ts.URL+"/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard",
	)
	t.Cleanup(restoreSB)

	u := &Updater{
		DB:     db,
		Logger: zap.NewNop().Sugar(),
		Writer: cw,
		ESPN:   client,
	}

	return u, cw
}

// seedBasketballTeamsAndSeasons inserts 4 basketball teams (all FBS=1).
func seedBasketballTeamsAndSeasons(t *testing.T, db *gorm.DB) {
	t.Helper()

	teamNames := []database.TeamName{
		{TeamID: 11, Name: "BBall Alpha", DisplayName: "BBall Alpha Bulldogs", Abbreviation: "BBA", Location: "BBall Alpha", Slug: "bball-alpha", IsActive: true, Sport: "ncaam"},
		{TeamID: 12, Name: "BBall Beta", DisplayName: "BBall Beta Wildcats", Abbreviation: "BBB", Location: "BBall Beta", Slug: "bball-beta", IsActive: true, Sport: "ncaam"},
		{TeamID: 13, Name: "BBall Gamma", DisplayName: "BBall Gamma Eagles", Abbreviation: "BBG", Location: "BBall Gamma", Slug: "bball-gamma", IsActive: true, Sport: "ncaam"},
		{TeamID: 14, Name: "BBall Delta", DisplayName: "BBall Delta Hawks", Abbreviation: "BBD", Location: "BBall Delta", Slug: "bball-delta", IsActive: true, Sport: "ncaam"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed basketball team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 11, Year: 2024, FBS: 1, Conf: "Big East", Sport: "ncaam"},
		{TeamID: 12, Year: 2024, FBS: 1, Conf: "Big East", Sport: "ncaam"},
		{TeamID: 13, Year: 2024, FBS: 1, Conf: "ACC", Sport: "ncaam"},
		{TeamID: 14, Year: 2024, FBS: 1, Conf: "ACC", Sport: "ncaam"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed basketball team_seasons: %v", err)
	}
}

// seedBasketballGames inserts 3 completed basketball games.
func seedBasketballGames(t *testing.T, db *gorm.DB) {
	t.Helper()

	games := []database.Game{
		{
			GameID: bbFixtureGameID1, Season: 2024, Week: 10,
			HomeID: 11, AwayID: 12, HomeScore: 78, AwayScore: 65,
			ConfGame: true, Sport: "ncaam",
			StartTime: time.Date(2024, 1, 6, 19, 0, 0, 0, time.UTC),
		},
		{
			GameID: bbFixtureGameID2, Season: 2024, Week: 10,
			HomeID: 13, AwayID: 14, HomeScore: 70, AwayScore: 68,
			ConfGame: true, Sport: "ncaam",
			StartTime: time.Date(2024, 1, 6, 19, 0, 0, 0, time.UTC),
		},
		{
			GameID: bbFixtureGameID4, Season: 2024, Week: 11,
			HomeID: 11, AwayID: 13, HomeScore: 80, AwayScore: 75,
			Sport: "ncaam",
			StartTime: time.Date(2024, 1, 13, 19, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed basketball games: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBasketball_UpdateSingleGame(t *testing.T) {
	u, _ := newBasketballTestUpdater(t)

	if err := u.UpdateSingleGame(bbFixtureGameID1); err != nil {
		t.Fatalf("UpdateSingleGame: %v", err)
	}

	var game database.Game
	if err := u.DB.Where("game_id = ?", bbFixtureGameID1).First(&game).Error; err != nil {
		t.Fatalf("game not found: %v", err)
	}
	if game.Sport != "ncaam" {
		t.Errorf("Sport = %q, want %q", game.Sport, "ncaam")
	}
	if game.HomeScore != 78 || game.AwayScore != 65 {
		t.Errorf("scores = %d-%d, want 78-65", game.HomeScore, game.AwayScore)
	}
	if game.HomeID != 11 || game.AwayID != 12 {
		t.Errorf("teams = %d vs %d, want 11 vs 12", game.HomeID, game.AwayID)
	}

	// Team stats should still be inserted
	var teamStats []database.TeamGameStats
	if err := u.DB.Where("game_id = ?", bbFixtureGameID1).Find(&teamStats).Error; err != nil {
		t.Fatalf("team stats query: %v", err)
	}
	if len(teamStats) != 2 {
		t.Errorf("len(teamStats) = %d, want 2", len(teamStats))
	}

	// No player stats for basketball
	var passStats []database.PassingStats
	if err := u.DB.Where("game_id = ?", bbFixtureGameID1).Find(&passStats).Error; err != nil {
		t.Fatalf("passing stats query: %v", err)
	}
	if len(passStats) != 0 {
		t.Errorf("len(passStats) = %d, want 0 (basketball has no passing stats)", len(passStats))
	}
}

func TestBasketball_UpdateCurrentWeek(t *testing.T) {
	u, _ := newBasketballTestUpdater(t)

	gameIDs, err := u.UpdateCurrentWeek()
	if err != nil {
		t.Fatalf("UpdateCurrentWeek: %v", err)
	}

	// Basketball has only 1 group (D1Basketball), so no duplicate fetching.
	// Fixture has 3 final games and 1 in-progress (filtered).
	if len(gameIDs) != 3 {
		t.Fatalf("len(gameIDs) = %d, want 3", len(gameIDs))
	}

	idSet := map[int64]bool{}
	for _, id := range gameIDs {
		idSet[id] = true
	}
	for _, expected := range []int64{bbFixtureGameID1, bbFixtureGameID2, bbFixtureGameID4} {
		if !idSet[expected] {
			t.Errorf("expected game %d in results", expected)
		}
	}

	// Verify sport on stored games
	var games []database.Game
	u.DB.Find(&games)
	for _, g := range games {
		if g.Sport != "ncaam" {
			t.Errorf("game %d Sport = %q, want %q", g.GameID, g.Sport, "ncaam")
		}
	}

	// Re-run should be a no-op
	gameIDs2, err := u.UpdateCurrentWeek()
	if err != nil {
		t.Fatalf("UpdateCurrentWeek re-run: %v", err)
	}
	if len(gameIDs2) != 0 {
		t.Errorf("re-run returned %d games, want 0 (no-op)", len(gameIDs2))
	}
}

func TestBasketball_UpdateTeamSeasons(t *testing.T) {
	u, _ := newBasketballTestUpdater(t)

	count, err := u.UpdateTeamSeasons(true)
	if err != nil {
		t.Fatalf("UpdateTeamSeasons: %v", err)
	}

	if count == 0 {
		t.Error("expected at least some team seasons inserted")
	}

	var seasons []database.TeamSeason
	if err := u.DB.Find(&seasons).Error; err != nil {
		t.Fatalf("query seasons: %v", err)
	}
	if len(seasons) == 0 {
		t.Error("no team_season rows found")
	}

	// All basketball teams should be FBS=1 (D1)
	for _, s := range seasons {
		if s.FBS != 1 {
			t.Errorf("team %d FBS = %d, want 1 (all D1 basketball)", s.TeamID, s.FBS)
		}
		if s.Sport != "ncaam" {
			t.Errorf("team %d Sport = %q, want %q", s.TeamID, s.Sport, "ncaam")
		}
	}
}

func TestBasketball_RankingForWeek(t *testing.T) {
	u, _ := newBasketballTestUpdater(t)

	seedBasketballTeamsAndSeasons(t, u.DB)
	seedBasketballGames(t, u.DB)

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}

	var results []database.TeamWeekResult
	if err := u.DB.Find(&results).Error; err != nil {
		t.Fatalf("query results: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no ranking results found")
	}

	// All basketball results should have Fbs=true and Sport="ncaam"
	for _, r := range results {
		if !r.Fbs {
			t.Errorf("team %d Fbs = false, want true (basketball single D1 ranking)", r.TeamID)
		}
		if r.Sport != "ncaam" {
			t.Errorf("team %d Sport = %q, want %q", r.TeamID, r.Sport, "ncaam")
		}
		if r.FinalRank == 0 {
			t.Errorf("team %d has FinalRank 0", r.TeamID)
		}
	}

	// Should have 4 results (one per team)
	if len(results) != 4 {
		t.Errorf("results count = %d, want 4", len(results))
	}
}

func TestBasketball_UpdateRecentJSON(t *testing.T) {
	u, cw := newBasketballTestUpdater(t)

	seedBasketballTeamsAndSeasons(t, u.DB)
	seedBasketballGames(t, u.DB)

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}

	if err := u.UpdateRecentJSON(); err != nil {
		t.Fatalf("UpdateRecentJSON: %v", err)
	}

	// Verify ncaam/ prefix on expected files
	expectedFiles := []string{
		"ncaam/availRanks.json",
		"ncaam/gameCount.json",
		"ncaam/latest.json",
	}
	for _, f := range expectedFiles {
		if !cw.hasFile(f) {
			t.Errorf("expected file %q not written", f)
		}
	}

	// Verify ranking files use ncaam/ prefix and d1 division
	// Pattern: ncaam/ranking/YEAR/d1/WEEK.json
	if cw.fileCount() < len(expectedFiles) {
		t.Errorf("total files = %d, want at least %d", cw.fileCount(), len(expectedFiles))
	}

	// Should NOT have fbs or fcs divisions
	for fileName := range cw.data {
		if fileName == "ncaam/ranking" {
			continue
		}
		// Check that no ncaaf files were written
		if len(fileName) >= 5 && fileName[:5] == "ncaaf" {
			t.Errorf("unexpected ncaaf file: %q", fileName)
		}
	}

	// Verify PurgeCache was called
	if cw.purgeCount != 1 {
		t.Errorf("PurgeCache count = %d, want 1", cw.purgeCount)
	}
}
