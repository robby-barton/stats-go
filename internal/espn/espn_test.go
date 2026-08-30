package espn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	// site.api scoreboard — plain requests (DefaultSeason, GetCurrentWeekGames)
	// get the week-1 fixture; requests carrying the `dates` parameter
	// (GetWeeksInSeason, HasPostseasonStarted) get the calendar fixture.
	siteScoreboardPath := "/apis/site/v2/sports/football/college-football/scoreboard"
	mux.HandleFunc(siteScoreboardPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fixture := siteScoreboardFixture
		if r.URL.Query().Get("dates") != "" {
			fixture = siteScoreboardCalendarFixture
		}
		if _, err := w.Write([]byte(fixture)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// site.api scoreboard/conferences — the fixture depends on the division
	// group requested; ConferenceMap requests FBS, FCS, DII and DIII.
	mux.HandleFunc(siteScoreboardPath+"/conferences", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var fixture string
		switch r.URL.Query().Get("groups") {
		case "80":
			fixture = siteConferencesFBSFixture
		case "81":
			fixture = siteConferencesFCSFixture
		case "57":
			fixture = siteConferencesDIIFixture
		case "58":
			fixture = siteConferencesDIIIFixture
		default:
			http.Error(w, "unknown groups parameter", http.StatusBadRequest)
			return
		}
		if _, err := w.Write([]byte(fixture)); err != nil {
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
		serverURL+"/apis/site/v2/sports/football/college-football/scoreboard",
		serverURL+"/apis/site/v2/sports/football/college-football/scoreboard/conferences",
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

func TestGetWeeksInSeason(t *testing.T) {
	ts := setupTestServer(t)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	weeks, err := client.GetWeeksInSeason(context.Background(), 2026)
	if err != nil {
		t.Fatalf("GetWeeksInSeason: %v", err)
	}

	// The calendar fixture's Regular Season entry lists 15 week entries
	// (captured live for the 2026 season). The postseason is a separate
	// calendar entry and must not be counted.
	if weeks != 15 {
		t.Errorf("weeks = %d, want 15", weeks)
	}
}

func TestHasPostseasonStarted(t *testing.T) {
	ts := setupTestServer(t)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	// The calendar fixture's Postseason entry starts 2026-12-13T08:00Z.
	before := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	started, err := client.HasPostseasonStarted(context.Background(), 2026, before)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if started {
		t.Error("postseason should not have started before 2026-12-13")
	}

	after := time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
	started, err = client.HasPostseasonStarted(context.Background(), 2026, after)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if !started {
		t.Error("postseason should have started after 2026-12-13")
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

	if year != 2026 {
		t.Errorf("year = %d, want 2026", year)
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
	t.Cleanup(client.SetURLs("", "", "", ts.URL+"/schedule", ""))

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
	t.Cleanup(client.SetURLs("", "", "", ts.URL+"/schedule", ""))

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
	t.Cleanup(client.SetURLs("", "", "", ts.URL+"/schedule", ""))

	// Empty response fails validation because the leagues list is missing.
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

	mux.HandleFunc(scoreboardPath+"/conferences", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(siteConferencesD1Fixture)); err != nil {
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
		serverURL+"/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard/conferences",
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

// TestGetGamesBySeason_CoversCalendarSpan verifies that the season fetch
// walks the site.api scoreboard one day at a time across the calendar's
// Regular and Postseason spans, carrying the division group on every day
// request and collecting the final games from each response.
func TestGetGamesBySeason_CoversCalendarSpan(t *testing.T) {
	var mu sync.Mutex
	var days []string
	var groups []string

	mux := http.NewServeMux()
	mux.HandleFunc("/apis/site/v2/sports/football/college-football/scoreboard",
		func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			dates := r.URL.Query().Get("dates")
			if len(dates) != 8 {
				// Calendar fetch (?dates=year) for getCalendarForYear.
				if _, err := w.Write([]byte(siteScoreboardCalendarFixture)); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			// Single-day request: record the day and serve the week-1
			// fixture, whose finalGames() yields exactly one game (401864494).
			days = append(days, dates)
			groups = append(groups, r.URL.Query().Get("groups"))
			if _, err := w.Write([]byte(siteScoreboardFixture)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	games, err := client.GetGamesBySeason(context.Background(), 2026, FBS)
	if err != nil {
		t.Fatalf("GetGamesBySeason: %v", err)
	}

	// Every day request must carry the division group.
	for i, group := range groups {
		if group != "80" {
			t.Errorf("day request %d groups = %q, want 80", i, group)
		}
	}

	// The calendar fixture's Regular Season span is 2026-08-22 (Saturday) /
	// 2026-12-13, the Postseason span 2026-12-13T08:00Z / 2027-01-28. The
	// requested days must cover both spans contiguously.
	want := map[string]bool{}
	addDays := func(from, to time.Time) {
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			want[d.Format("20060102")] = true
		}
	}
	addDays(time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 13, 0, 0, 0, 0, time.UTC))
	addDays(time.Date(2026, 12, 13, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 28, 0, 0, 0, 0, time.UTC))
	seen := map[string]bool{}
	for _, day := range days {
		if !want[day] {
			t.Errorf("unexpected day requested: %s", day)
			continue
		}
		seen[day] = true
	}
	// The spans share the 2026-12-13 boundary day, so it is legitimately
	// requested twice; every expected day must have been requested at least
	// once.
	for day := range want {
		if !seen[day] {
			t.Errorf("calendar-span day %s was never requested", day)
		}
	}

	// Each day response contributes exactly one final game.
	if len(games) != len(days) {
		t.Errorf("len(games) = %d, want %d (one final game per day response)",
			len(games), len(days))
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
	t.Cleanup(client.SetURLs("", "", "", ts.URL+"/schedule", ""))

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
		if err := json.NewEncoder(w).Encode(json.RawMessage(siteScoreboardFixture)); err != nil {
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
	t.Cleanup(client.SetURLs("", "", "", ts.URL+"/scoreboard", ""))

	games, err := client.GetCurrentWeekGames(context.Background(), FBS)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3 (two 202 challenges then success)", requests)
	}
	if len(games) != 1 {
		t.Fatalf("len(games) = %d, want 1", len(games))
	}
}
