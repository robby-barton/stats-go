package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Schedule endpoint — handles all weekURL-based requests
	mux.HandleFunc("/core/college-football/schedule", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(testScheduleResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Game stats endpoint
	mux.HandleFunc("/core/college-football/playbyplay", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(testGameInfoResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Team info endpoint
	mux.HandleFunc("/apis/site/v2/sports/football/college-football/teams", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(testTeamInfoResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func overrideURLs(t *testing.T, serverURL string) {
	t.Helper()
	restore := SetTestURLs(
		serverURL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
	)
	t.Cleanup(restore)
}

func TestGetCurrentWeekGames(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	games, err := GetCurrentWeekGames(FBS)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}

	// Only STATUS_FINAL games should be returned (2 of 3 in fixture)
	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}

	ids := map[int64]bool{}
	for _, g := range games {
		ids[g.ID] = true
	}
	if !ids[1001] || !ids[1002] {
		t.Errorf("expected game IDs 1001 and 1002, got %v", ids)
	}
	if ids[1003] {
		t.Error("in-progress game 1003 should not be included")
	}
}

func TestGetGamesByWeek(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	res, err := GetGamesByWeek(2023, 1, FBS, Regular)
	if err != nil {
		t.Fatalf("GetGamesByWeek: %v", err)
	}

	if res == nil {
		t.Fatal("result is nil")
	}
	if res.Content.Parameters.Year != 2023 {
		t.Errorf("Year = %d, want 2023", res.Content.Parameters.Year)
	}
	if res.Content.Parameters.Week != 1 {
		t.Errorf("Week = %d, want 1", res.Content.Parameters.Week)
	}
}

func TestGetCompletedGamesByWeek(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	games, err := GetCompletedGamesByWeek(2023, 1, FBS, Regular)
	if err != nil {
		t.Fatalf("GetCompletedGamesByWeek: %v", err)
	}

	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}
}

func TestGetWeeksInSeason(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	weeks, err := GetWeeksInSeason(2023)
	if err != nil {
		t.Fatalf("GetWeeksInSeason: %v", err)
	}

	// Calendar[0] has 15 weeks
	if weeks != 15 {
		t.Errorf("weeks = %d, want 15", weeks)
	}
}

func TestHasPostseasonStarted(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	// Postseason starts 2023-12-16T08:00Z
	// Test with time before postseason
	before := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)
	started, err := HasPostseasonStarted(2023, before)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if started {
		t.Error("postseason should not have started before 2023-12-16")
	}

	// Test with time after postseason
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	started, err = HasPostseasonStarted(2023, after)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if !started {
		t.Error("postseason should have started after 2024-01-01")
	}
}

func TestGetGameStats(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	res, err := GetGameStats(1001)
	if err != nil {
		t.Fatalf("GetGameStats: %v", err)
	}

	if res == nil {
		t.Fatal("result is nil")
	}
	if res.GamePackage.Header.ID != 1001 {
		t.Errorf("Header.ID = %d, want 1001", res.GamePackage.Header.ID)
	}
	if res.GamePackage.Header.Season.Year != 2023 {
		t.Errorf("Season.Year = %d, want 2023", res.GamePackage.Header.Season.Year)
	}
	if res.GamePackage.Header.Week != 1 {
		t.Errorf("Week = %d, want 1", res.GamePackage.Header.Week)
	}
	if len(res.GamePackage.Header.Competitions) != 1 {
		t.Fatalf("len(Competitions) = %d, want 1", len(res.GamePackage.Header.Competitions))
	}
	comp := res.GamePackage.Header.Competitions[0]
	if !comp.ConfGame {
		t.Error("ConfGame = false, want true")
	}
}

func TestGetTeamInfo(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	res, err := GetTeamInfo()
	if err != nil {
		t.Fatalf("GetTeamInfo: %v", err)
	}

	if res == nil {
		t.Fatal("result is nil")
	}
	if len(res.Sports) != 1 {
		t.Fatalf("len(Sports) = %d, want 1", len(res.Sports))
	}
	if len(res.Sports[0].Leagues) != 1 {
		t.Fatalf("len(Leagues) = %d, want 1", len(res.Sports[0].Leagues))
	}
	teams := res.Sports[0].Leagues[0].Teams
	if len(teams) != 2 {
		t.Fatalf("len(Teams) = %d, want 2", len(teams))
	}
	if teams[0].Team.ID != 1 {
		t.Errorf("teams[0].ID = %d, want 1", teams[0].Team.ID)
	}
}

func TestDefaultSeason(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)

	year, err := DefaultSeason()
	if err != nil {
		t.Fatalf("DefaultSeason: %v", err)
	}

	if year != 2023 {
		t.Errorf("year = %d, want 2023", year)
	}
}
