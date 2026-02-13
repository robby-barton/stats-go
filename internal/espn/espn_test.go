package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func newTestClient() *Client {
	return &Client{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
	}
}

func TestGetCurrentWeekGames(t *testing.T) {
	ts := setupTestServer(t)
	overrideURLs(t, ts.URL)
	client := newTestClient()

	games, err := client.GetCurrentWeekGames(FBS)
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
	client := newTestClient()

	res, err := client.GetGamesByWeek(2023, 1, FBS, Regular)
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
	client := newTestClient()

	games, err := client.GetCompletedGamesByWeek(2023, 1, FBS, Regular)
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
	client := newTestClient()

	weeks, err := client.GetWeeksInSeason(2023)
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
	client := newTestClient()

	// Postseason starts 2023-12-16T08:00Z
	// Test with time before postseason
	before := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)
	started, err := client.HasPostseasonStarted(2023, before)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if started {
		t.Error("postseason should not have started before 2023-12-16")
	}

	// Test with time after postseason
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	started, err = client.HasPostseasonStarted(2023, after)
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
	client := newTestClient()

	res, err := client.GetGameStats(1001)
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
	client := newTestClient()

	res, err := client.GetTeamInfo()
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
	client := newTestClient()

	year, err := client.DefaultSeason()
	if err != nil {
		t.Fatalf("DefaultSeason: %v", err)
	}

	if year != 2023 {
		t.Errorf("year = %d, want 2023", year)
	}
}

func TestMakeRequestNon2xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	restore := SetTestURLs(ts.URL+"/schedule", "", "")
	t.Cleanup(restore)
	client := newTestClient()

	_, err := client.DefaultSeason()
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 404") {
		t.Errorf("error = %q, want it to contain 'unexpected status 404'", err)
	}
}

func TestMakeRequestMalformedJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`)) //nolint:errcheck // test helper
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	restore := SetTestURLs(ts.URL+"/schedule", "", "")
	t.Cleanup(restore)
	client := newTestClient()

	_, err := client.DefaultSeason()
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("error = %q, want it to contain 'decoding response'", err)
	}
}

func TestMakeRequestEmptyResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`)) //nolint:errcheck // test helper
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	restore := SetTestURLs(ts.URL+"/schedule", "", "")
	t.Cleanup(restore)
	client := newTestClient()

	_, err := client.DefaultSeason()
	if err == nil {
		t.Fatal("expected validation error for empty response, got nil")
	}
	if !strings.Contains(err.Error(), "missing calendar") {
		t.Errorf("error = %q, want it to contain 'missing calendar'", err)
	}
}

func TestGameScheduleValidate(t *testing.T) {
	tests := []struct {
		name    string
		resp    GameScheduleESPN
		wantErr string
	}{
		{
			name:    "empty calendar",
			resp:    GameScheduleESPN{},
			wantErr: "missing calendar",
		},
		{
			name: "empty weeks",
			resp: GameScheduleESPN{
				Content: Content{Calendar: []Calendar{{}}},
			},
			wantErr: "empty weeks",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestGameInfoValidate(t *testing.T) {
	tests := []struct {
		name    string
		resp    GameInfoESPN
		wantErr string
	}{
		{
			name:    "zero header ID",
			resp:    GameInfoESPN{},
			wantErr: "zero header ID",
		},
		{
			name: "no competitions",
			resp: GameInfoESPN{
				GamePackage: GamePackage{Header: Header{ID: 1}},
			},
			wantErr: "missing competitions",
		},
		{
			name: "too few competitors",
			resp: GameInfoESPN{
				GamePackage: GamePackage{Header: Header{
					ID:           1,
					Competitions: []Competitions{{Competitors: []Competitors{{ID: 1}}}},
				}},
			},
			wantErr: "fewer than 2 competitors",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestTeamInfoValidate(t *testing.T) {
	tests := []struct {
		name    string
		resp    TeamInfoESPN
		wantErr string
	}{
		{
			name:    "no sports",
			resp:    TeamInfoESPN{},
			wantErr: "missing sports",
		},
		{
			name:    "no leagues",
			resp:    TeamInfoESPN{Sports: []Sport{{}}},
			wantErr: "missing leagues",
		},
		{
			name:    "no teams",
			resp:    TeamInfoESPN{Sports: []Sport{{Leagues: []League{{}}}}},
			wantErr: "missing teams",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
