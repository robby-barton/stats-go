package espn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/sport"
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

func overrideURLs(t *testing.T, client *Client, serverURL string) {
	t.Helper()
	t.Cleanup(client.SetURLs(
		serverURL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
		"",
	))
}

func newTestClient() *FootballClient {
	return &FootballClient{Client: &Client{
		MaxAttempts:    2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          CollegeFootball,
	}}
}

func TestGetCurrentWeekGames(t *testing.T) {
	ts := setupTestServer(t)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	games, err := client.GetCurrentWeekGames(context.Background(), FBS)
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
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	res, err := client.GetGamesByWeek(context.Background(), 2023, 1, FBS, Regular)
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
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	games, err := client.GetCompletedGamesByWeek(context.Background(), 2023, 1, FBS, Regular)
	if err != nil {
		t.Fatalf("GetCompletedGamesByWeek: %v", err)
	}

	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}
}

func TestGetWeeksInSeason(t *testing.T) {
	ts := setupTestServer(t)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	weeks, err := client.GetWeeksInSeason(context.Background(), 2023)
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
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	// Postseason starts 2023-12-16T08:00Z
	// Test with time before postseason
	before := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)
	started, err := client.HasPostseasonStarted(context.Background(), 2023, before)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if started {
		t.Error("postseason should not have started before 2023-12-16")
	}

	// Test with time after postseason
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	started, err = client.HasPostseasonStarted(context.Background(), 2023, after)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if !started {
		t.Error("postseason should have started after 2024-01-01")
	}
}

func TestGetGameStats(t *testing.T) {
	ts := setupTestServer(t)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	res, err := client.GetGameStats(context.Background(), 1001)
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
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	res, err := client.GetTeamInfo(context.Background())
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
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	year, err := client.DefaultSeason(context.Background())
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

	client := newTestClient()
	t.Cleanup(client.SetURLs(ts.URL+"/schedule", "", "", ""))

	_, err := client.DefaultSeason(context.Background())
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

	client := newTestClient()
	t.Cleanup(client.SetURLs(ts.URL+"/schedule", "", "", ""))

	_, err := client.DefaultSeason(context.Background())
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

	client := newTestClient()
	t.Cleanup(client.SetURLs(ts.URL+"/schedule", "", "", ""))

	// Empty response fails validation because schedule data is missing.
	_, err := client.DefaultSeason(context.Background())
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
}

func TestGameScheduleValidate(t *testing.T) {
	tests := []struct {
		name    string
		resp    GameScheduleESPN
		wantErr bool
	}{
		{name: "empty response", resp: GameScheduleESPN{}, wantErr: true},
		{name: "empty calendar", resp: GameScheduleESPN{Content: Content{Calendar: []Calendar{{}}}}, wantErr: true},
		{
			name: "valid schedule",
			resp: GameScheduleESPN{Content: Content{Schedule: map[string]Day{"2026-01-01": {}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.validate()
			if tt.wantErr && err == nil {
				t.Error("validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate() returned unexpected error: %v", err)
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

func TestSportDB(t *testing.T) {
	if got := CollegeFootball.DBSport(); got != sport.Football {
		t.Errorf("CollegeFootball.DBSport() = %q, want %q", got, sport.Football)
	}
	if got := CollegeBasketball.DBSport(); got != sport.Basketball {
		t.Errorf("CollegeBasketball.DBSport() = %q, want %q", got, sport.Basketball)
	}
}

func TestGroups(t *testing.T) {
	fbGroups := CollegeFootball.Groups()
	if len(fbGroups) != 2 {
		t.Fatalf("CollegeFootball.Groups() len = %d, want 2", len(fbGroups))
	}
	if fbGroups[0] != FBS || fbGroups[1] != FCS {
		t.Errorf("CollegeFootball.Groups() = %v, want [FBS, FCS]", fbGroups)
	}

	bbGroups := CollegeBasketball.Groups()
	if len(bbGroups) != 1 {
		t.Fatalf("CollegeBasketball.Groups() len = %d, want 1", len(bbGroups))
	}
	if bbGroups[0] != D1Basketball {
		t.Errorf("CollegeBasketball.Groups() = %v, want [D1Basketball]", bbGroups)
	}
}

func TestHasDivisionSplit(t *testing.T) {
	if !CollegeFootball.HasDivisionSplit() {
		t.Error("CollegeFootball.HasDivisionSplit() = false, want true")
	}
	if CollegeBasketball.HasDivisionSplit() {
		t.Error("CollegeBasketball.HasDivisionSplit() = true, want false")
	}
}

func TestScoreboardValidate(t *testing.T) {
	tests := []struct {
		name    string
		resp    ScoreboardESPN
		wantErr string
	}{
		{
			name:    "no leagues",
			resp:    ScoreboardESPN{},
			wantErr: "missing leagues",
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

// ---------------------------------------------------------------------------
// Basketball tests using scoreboard endpoint
// ---------------------------------------------------------------------------

func testScoreboardResponse() ScoreboardESPN {
	return ScoreboardESPN{
		Leagues: []ScoreboardLeague{{
			Season: ScoreboardSeason{
				Year:      2024,
				StartDate: "2023-11-06T08:00Z",
				EndDate:   "2024-04-08T06:59Z",
				Type:      ScoreboardSeasonType{ID: 2, Name: "Regular Season"},
			},
			Calendar: []string{"2023-11-06T08:00Z", "2023-11-07T08:00Z"},
		}},
	}
}

func basketballScheduleResponse(r *http.Request) GameScheduleESPN {
	date := r.URL.Query().Get("date")
	games := map[string]Day{
		"2024-01-06": {Games: []Game{{
			ID:     2001,
			Status: Status{StatusType: StatusType{Name: "STATUS_FINAL", Completed: true}},
		}}},
	}

	// When a specific date is requested, return date-specific games.
	// Game 3001 appears on both dates to verify deduplication.
	if date != "" {
		today := time.Now().Format("20060102")
		yesterday := time.Now().AddDate(0, 0, -1).Format("20060102")
		switch date {
		case today:
			games = map[string]Day{
				today: {Games: []Game{
					{ID: 3001, Status: Status{StatusType: StatusType{Name: "STATUS_FINAL", Completed: true}}},
					{ID: 3002, Status: Status{StatusType: StatusType{Name: "STATUS_FINAL", Completed: true}}},
				}},
			}
		case yesterday:
			games = map[string]Day{
				yesterday: {Games: []Game{
					{ID: 3001, Status: Status{StatusType: StatusType{Name: "STATUS_FINAL", Completed: true}}},
					{ID: 3003, Status: Status{StatusType: StatusType{Name: "STATUS_FINAL", Completed: true}}},
				}},
			}
		}
	}

	return GameScheduleESPN{
		Content: Content{
			Schedule: games,
			Defaults: Parameters{Week: 10, Year: 2024, SeasonType: 2, Group: FlexInt64(50)},
			ConferenceAPI: ConferenceAPI{
				Conferences: []Conference{
					{GroupID: 300, Name: "Big East", ShortName: "Big East", ParentGroupID: FlexInt64(50)},
				},
			},
		},
	}
}

func setupBasketballTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/core/mens-college-basketball/schedule", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(basketballScheduleResponse(r)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	scoreboardPath := "/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard"
	mux.HandleFunc(scoreboardPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(testScoreboardResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func newBasketballTestClient(t *testing.T, serverURL string) *BasketballClient {
	t.Helper()

	c := &BasketballClient{Client: &Client{
		MaxAttempts:    2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          CollegeBasketball,
	}}
	t.Cleanup(c.SetURLs(
		serverURL+"/core/mens-college-basketball/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/mens-college-basketball/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/basketball/mens-college-basketball/teams?limit=1000",
		serverURL+"/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard",
	))
	return c
}

func TestBasketball_DefaultSeason(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := newBasketballTestClient(t, ts.URL)

	year, err := client.DefaultSeason(context.Background())
	if err != nil {
		t.Fatalf("DefaultSeason: %v", err)
	}
	if year != 2024 {
		t.Errorf("year = %d, want 2024", year)
	}
}

func TestBasketball_GetWeeksInSeason(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := newBasketballTestClient(t, ts.URL)

	weeks, err := client.GetWeeksInSeason(context.Background(), 2024)
	if err != nil {
		t.Fatalf("GetWeeksInSeason: %v", err)
	}

	// Season: 2023-11-06 to 2024-04-08 ≈ 154 days ≈ 22 weeks + 1 = 23
	if weeks < 20 || weeks > 25 {
		t.Errorf("weeks = %d, expected roughly 22-23", weeks)
	}
}

func TestBasketball_HasPostseasonStarted(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := newBasketballTestClient(t, ts.URL)

	// Scoreboard fixture has season type 2 (regular), so postseason has not started
	started, err := client.HasPostseasonStarted(context.Background(), 2024, time.Now())
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if started {
		t.Error("postseason should not have started (season type = 2)")
	}
}

func TestBasketball_GetCurrentWeekGames(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := newBasketballTestClient(t, ts.URL)

	games, err := client.GetCurrentWeekGames(context.Background(), D1Basketball)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}

	// Today returns 3001+3002, yesterday returns 3001+3003.
	// After dedup we should have 3 unique games.
	if len(games) != 3 {
		t.Fatalf("len(games) = %d, want 3", len(games))
	}

	ids := map[int64]bool{}
	for _, g := range games {
		ids[g.ID] = true
	}
	for _, want := range []int64{3001, 3002, 3003} {
		if !ids[want] {
			t.Errorf("missing game ID %d", want)
		}
	}
}

// TestGetGamesBySeason_CoversAllRegularSeasonWeeks verifies that the season
// fetch requests every regular-season week (1..N, including the final week)
// plus postseason week 1.
func TestGetGamesBySeason_CoversAllRegularSeasonWeeks(t *testing.T) {
	var mu sync.Mutex
	var regularWeeks, postseasonWeeks []int64

	mux := http.NewServeMux()
	mux.HandleFunc("/core/college-football/schedule", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(testScheduleResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		week := r.URL.Query().Get("week")
		seasonType := r.URL.Query().Get("seasonType")
		// GetWeeksInSeason probes the same endpoint without week/seasonType
		// params; only count actual week fetches.
		if week == "" {
			return
		}
		n, _ := strconv.ParseInt(week, 10, 64)
		if seasonType == strconv.FormatInt(int64(Postseason), 10) {
			postseasonWeeks = append(postseasonWeeks, n)
		} else {
			regularWeeks = append(regularWeeks, n)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	if _, err := client.GetGamesBySeason(context.Background(), 2023, FBS); err != nil {
		t.Fatalf("GetGamesBySeason: %v", err)
	}

	// Regular season: calendar has 15 weeks (1..15) — every week must be
	// requested, including the final one.
	if len(regularWeeks) != 15 {
		t.Errorf("regular season requests = %d, want 15", len(regularWeeks))
	}
	requested := map[int64]bool{}
	for _, week := range regularWeeks {
		requested[week] = true
	}
	for week := int64(1); week <= 15; week++ {
		if !requested[week] {
			t.Errorf("regular season week %d was never requested", week)
		}
	}

	// Postseason week 1 is fetched separately.
	if len(postseasonWeeks) != 1 || postseasonWeeks[0] != 1 {
		t.Errorf("postseason requests = %v, want [1]", postseasonWeeks)
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
			resp:    TeamInfoESPN{Sports: []TeamInfoSport{{}}},
			wantErr: "missing leagues",
		},
		{
			name:    "no teams",
			resp:    TeamInfoESPN{Sports: []TeamInfoSport{{Leagues: []League{{}}}}},
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

// TestMakeRequestMaxAttempts verifies that MaxAttempts is the total number of
// request tries (not retries), and that no backoff sleep happens after the
// final failed attempt — with InitialBackoff=30ms and 3 attempts the elapsed
// time should be 30ms+60ms of sleeps (plus request overhead), never the
// additional 120ms a post-final sleep would add.
func TestMakeRequestMaxAttempts(t *testing.T) {
	var mu sync.Mutex
	var requests int

	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := &FootballClient{Client: &Client{
		MaxAttempts:    3,
		InitialBackoff: 30 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          CollegeFootball,
	}}
	t.Cleanup(client.SetURLs(ts.URL+"/schedule", "", "", ""))

	start := time.Now()
	_, err := client.DefaultSeason(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after exhausting attempts, got nil")
	}
	if requests != 3 {
		t.Errorf("requests = %d, want 3 (MaxAttempts is total tries)", requests)
	}
	if elapsed >= 150*time.Millisecond {
		t.Errorf("elapsed = %v, want < 150ms (no backoff sleep after the final attempt)", elapsed)
	}
}

func TestMakeRequestRetries202Challenge(t *testing.T) {
	var mu sync.Mutex
	var requests int

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		n := requests
		mu.Unlock()
		if n < 3 {
			// ESPN's edge occasionally answers with an empty-body 202 bot
			// challenge; makeRequest must back off and retry, not decode.
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(testScheduleResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := &FootballClient{Client: &Client{
		MaxAttempts:    4,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          CollegeFootball,
	}}
	t.Cleanup(client.SetURLs(ts.URL+"/schedule", "", "", ""))

	games, err := client.GetCurrentWeekGames(context.Background(), FBS)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3 (two 202 challenges then success)", requests)
	}
	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}
}
