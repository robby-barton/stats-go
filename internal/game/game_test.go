package game

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func overrideGameURLs(t *testing.T, serverURL string) {
	t.Helper()
	restore := espn.SetTestURLs(
		serverURL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
	)
	t.Cleanup(restore)
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
	overrideGameURLs(t, ts.URL)

	client := espn.NewClient()
	parsed, err := GetSingleGame(client, 1001)
	if err != nil {
		t.Fatalf("GetSingleGame: %v", err)
	}

	if parsed.GameInfo.GameID != 1001 {
		t.Errorf("GameID = %d, want 1001", parsed.GameInfo.GameID)
	}
	if parsed.GameInfo.HomeID != 10 {
		t.Errorf("HomeID = %d, want 10", parsed.GameInfo.HomeID)
	}
	if parsed.GameInfo.AwayID != 20 {
		t.Errorf("AwayID = %d, want 20", parsed.GameInfo.AwayID)
	}
	if parsed.GameInfo.HomeScore != 28 {
		t.Errorf("HomeScore = %d, want 28", parsed.GameInfo.HomeScore)
	}
	if parsed.GameInfo.AwayScore != 14 {
		t.Errorf("AwayScore = %d, want 14", parsed.GameInfo.AwayScore)
	}
	if !parsed.GameInfo.ConfGame {
		t.Error("ConfGame = false, want true")
	}
	if parsed.GameInfo.Season != 2023 {
		t.Errorf("Season = %d, want 2023", parsed.GameInfo.Season)
	}
	if parsed.GameInfo.Week != 1 {
		t.Errorf("Week = %d, want 1", parsed.GameInfo.Week)
	}
}

func TestGetCurrentWeekGames(t *testing.T) {
	ts := setupGameTestServer(t)
	overrideGameURLs(t, ts.URL)

	client := espn.NewClient()
	games, err := GetCurrentWeekGames(client)
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
