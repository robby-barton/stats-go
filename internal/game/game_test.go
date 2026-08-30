package game

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/robby-barton/stats-go/internal/espn"
)

func setupGameTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/core/college-football/schedule", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(espn.GameScheduleESPN{
			Content: espn.Content{
				Schedule: map[string]espn.Day{
					"2023-09-02": {
						Games: []espn.Game{
							{
								ID: 1001,
								Status: espn.Status{
									StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true},
								},
							},
							{
								ID: 1002,
								Status: espn.Status{
									StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true},
								},
							},
						},
					},
				},
				Calendar: []espn.Calendar{
					{Weeks: []espn.Week{{Num: 1}}},
				},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/core/college-football/playbyplay", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(espn.GameInfoESPN{
			GamePackage: espn.GamePackage{
				Header: espn.Header{
					ID: 1001,
					Competitions: []espn.Competitions{
						{
							ID:       1001,
							Date:     "2023-09-02T23:00Z",
							ConfGame: true,
							Competitors: []espn.Competitors{
								{HomeAway: "home", ID: 10, Score: 28},
								{HomeAway: "away", ID: 20, Score: 14},
							},
						},
					},
					Season: espn.Season{Year: 2023, Type: 2},
					Week:   1,
				},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// site.api scoreboard endpoint for the current-week games fetch
	const scoreboardPath = "/apis/site/v2/sports/football/college-football/scoreboard"
	mux.HandleFunc(scoreboardPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(espn.SiteScoreboardESPN{
			Leagues: []espn.SiteScoreboardLeague{{}},
			Season:  espn.SiteSeason{Year: 2023, Type: 2},
			Week:    espn.SiteWeek{Number: 1},
			Events: []espn.SiteEvent{
				{
					ID:     "1001",
					Status: espn.Status{StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true}},
					Competitions: []espn.SiteCompetition{{
						Status: espn.Status{StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true}},
						Competitors: []espn.Competitor{
							{ID: 10, Team: espn.ScheduleTeam{ID: 10, ConferenceID: 100}, Score: 28, HomeAway: "home"},
							{ID: 20, Team: espn.ScheduleTeam{ID: 20, ConferenceID: 100}, Score: 14, HomeAway: "away"},
						},
					}},
				},
				{
					ID:     "1002",
					Status: espn.Status{StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true}},
					Competitions: []espn.SiteCompetition{{
						Status: espn.Status{StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true}},
						Competitors: []espn.Competitor{
							{ID: 30, Team: espn.ScheduleTeam{ID: 30, ConferenceID: 200}, Score: 21, HomeAway: "home"},
							{ID: 40, Team: espn.ScheduleTeam{ID: 40, ConferenceID: 200}, Score: 10, HomeAway: "away"},
						},
					}},
				},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func overrideGameURLs(t *testing.T, client *espn.Client, serverURL string) {
	t.Helper()
	t.Cleanup(client.SetURLs(
		serverURL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
		serverURL+"/apis/site/v2/sports/football/college-football/scoreboard",
	))
}

// newTestClient returns a football client with fast retry settings for tests.
func newTestClient() *espn.FootballClient {
	return &espn.FootballClient{Client: &espn.Client{
		MaxAttempts:    2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeFootball,
	}}
}

func setupBasketballTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/core/mens-college-basketball/schedule", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(espn.GameScheduleESPN{
			Content: espn.Content{
				Schedule: map[string]espn.Day{
					"2024-01-06": {
						Games: []espn.Game{
							{
								ID: 2001,
								Status: espn.Status{
									StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true},
								},
							},
							{
								ID: 2002,
								Status: espn.Status{
									StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true},
								},
							},
						},
					},
				},
				Calendar: []espn.Calendar{
					{Weeks: []espn.Week{{Num: 1}}},
				},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/core/mens-college-basketball/playbyplay", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(espn.GameInfoESPN{
			GamePackage: espn.GamePackage{
				Header: espn.Header{
					ID: 2001,
					Competitions: []espn.Competitions{
						{
							ID:       2001,
							Date:     "2024-01-06T19:00Z",
							ConfGame: true,
							Competitors: []espn.Competitors{
								{HomeAway: "home", ID: 30, Score: 78},
								{HomeAway: "away", ID: 40, Score: 65},
							},
						},
					},
					Season: espn.Season{Year: 2024, Type: 2},
					Week:   10,
				},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestGetSingleGame_Basketball(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := &espn.BasketballClient{Client: &espn.Client{
		MaxAttempts:    2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeBasketball,
	}}
	t.Cleanup(client.SetURLs(
		ts.URL+"/core/mens-college-basketball/schedule?xhr=1&render=false&userab=18",
		ts.URL+"/core/mens-college-basketball/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		ts.URL+"/apis/site/v2/sports/basketball/mens-college-basketball/teams?limit=1000",
		"",
	))

	parsed, err := GetSingleGame(context.Background(), client, zap.NewNop().Sugar(), 2001)
	if err != nil {
		t.Fatalf("GetSingleGame: %v", err)
	}

	if parsed.Info.GameID != 2001 {
		t.Errorf("GameID = %d, want 2001", parsed.Info.GameID)
	}
	if parsed.Info.HomeID != 30 {
		t.Errorf("HomeID = %d, want 30", parsed.Info.HomeID)
	}
	if parsed.Info.HomeScore != 78 {
		t.Errorf("HomeScore = %d, want 78", parsed.Info.HomeScore)
	}
	if parsed.Info.AwayScore != 65 {
		t.Errorf("AwayScore = %d, want 65", parsed.Info.AwayScore)
	}

	// Basketball should not have player stats
	if len(parsed.PassingStats) != 0 {
		t.Errorf("len(PassingStats) = %d, want 0", len(parsed.PassingStats))
	}
	if len(parsed.RushingStats) != 0 {
		t.Errorf("len(RushingStats) = %d, want 0", len(parsed.RushingStats))
	}
	if len(parsed.ReceivingStats) != 0 {
		t.Errorf("len(ReceivingStats) = %d, want 0", len(parsed.ReceivingStats))
	}
}

func TestGetCurrentWeekGames_Basketball(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := &espn.BasketballClient{Client: &espn.Client{
		MaxAttempts:    2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 1 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeBasketball,
	}}
	t.Cleanup(client.SetURLs(
		ts.URL+"/core/mens-college-basketball/schedule?xhr=1&render=false&userab=18",
		ts.URL+"/core/mens-college-basketball/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		ts.URL+"/apis/site/v2/sports/basketball/mens-college-basketball/teams?limit=1000",
		"",
	))

	games, err := GetCurrentWeekGames(context.Background(), client)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}

	// Basketball has only 1 group (D1Basketball), so no dedup needed.
	// Fixture returns 2 final games.
	if len(games) != 2 {
		t.Fatalf("len(games) = %d, want 2", len(games))
	}

	ids := map[int64]bool{}
	for _, g := range games {
		ids[g.ID] = true
	}
	if !ids[2001] || !ids[2002] {
		t.Errorf("expected game IDs 2001 and 2002, got %v", ids)
	}
}

func TestCombineGames(t *testing.T) {
	list1 := []espn.Game{
		{ID: 100},
		{ID: 200},
		{ID: 300},
	}
	list2 := []espn.Game{
		{ID: 200},
		{ID: 400},
	}

	result := combineGames([][]espn.Game{list1, list2})

	if len(result) != 4 {
		t.Fatalf("len(result) = %d, want 4", len(result))
	}

	ids := map[int64]bool{}
	for _, g := range result {
		ids[g.ID] = true
	}
	for _, want := range []int64{100, 200, 300, 400} {
		if !ids[want] {
			t.Errorf("missing game ID %d", want)
		}
	}
}

func TestGetSingleGame(t *testing.T) {
	ts := setupGameTestServer(t)
	client := newTestClient()
	overrideGameURLs(t, client.Client, ts.URL)
	parsed, err := GetSingleGame(context.Background(), client, zap.NewNop().Sugar(), 1001)
	if err != nil {
		t.Fatalf("GetSingleGame: %v", err)
	}

	if parsed.Info.GameID != 1001 {
		t.Errorf("GameID = %d, want 1001", parsed.Info.GameID)
	}
	if parsed.Info.HomeID != 10 {
		t.Errorf("HomeID = %d, want 10", parsed.Info.HomeID)
	}
	if parsed.Info.AwayID != 20 {
		t.Errorf("AwayID = %d, want 20", parsed.Info.AwayID)
	}
	if parsed.Info.HomeScore != 28 {
		t.Errorf("HomeScore = %d, want 28", parsed.Info.HomeScore)
	}
	if parsed.Info.AwayScore != 14 {
		t.Errorf("AwayScore = %d, want 14", parsed.Info.AwayScore)
	}
	if !parsed.Info.ConfGame {
		t.Error("ConfGame = false, want true")
	}
	if parsed.Info.Season != 2023 {
		t.Errorf("Season = %d, want 2023", parsed.Info.Season)
	}
	if parsed.Info.Week != 1 {
		t.Errorf("Week = %d, want 1", parsed.Info.Week)
	}
}

func TestGetCurrentWeekGames(t *testing.T) {
	ts := setupGameTestServer(t)
	client := newTestClient()
	overrideGameURLs(t, client.Client, ts.URL)
	games, err := GetCurrentWeekGames(context.Background(), client)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}

	// Both FBS and FCS calls return the same 2 games from our fixture,
	// but combineGames deduplicates them
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
}
