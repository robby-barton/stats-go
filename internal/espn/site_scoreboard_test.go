package espn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupSiteTestServer serves the captured site.api fixtures on the same
// paths the production client uses.
func setupSiteTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	scoreboardPath := "/apis/site/v2/sports/football/college-football/scoreboard"
	mux.HandleFunc(scoreboardPath, func(w http.ResponseWriter, r *http.Request) {
		// The group filter must be requested via the `groups` parameter —
		// site.api ignores cdn's `group` parameter entirely.
		if r.URL.Query().Get("groups") == "" {
			http.Error(w, "missing groups parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(json.RawMessage(siteScoreboardFixture)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	summaryPath := "/apis/site/v2/sports/football/college-football/summary"
	mux.HandleFunc(summaryPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(json.RawMessage(siteSummaryFixture)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestGetCurrentWeekGames_SiteAPI(t *testing.T) {
	ts := setupSiteTestServer(t)
	client := newTestClient()
	t.Cleanup(client.SetURLs("", "", ts.URL+"/apis/site/v2/sports/football/college-football/scoreboard", ""))

	games, err := client.GetCurrentWeekGames(context.Background(), FBS)
	if err != nil {
		t.Fatalf("GetCurrentWeekGames: %v", err)
	}

	// The fixture contains one game of each status (final, in progress,
	// scheduled); only the final one may survive.
	if len(games) != 1 {
		t.Fatalf("len(games) = %d, want 1", len(games))
	}

	g := games[0]
	if g.ID != 401864494 {
		t.Errorf("ID = %d, want 401864494", g.ID)
	}
	if !g.Status.StatusType.Completed || g.Status.StatusType.Name != "STATUS_FINAL" {
		t.Errorf("status = %+v, want completed STATUS_FINAL", g.Status.StatusType)
	}

	if len(g.Competitions) != 1 {
		t.Fatalf("len(Competitions) = %d, want 1", len(g.Competitions))
	}
	competitors := g.Competitions[0].Competitors
	if len(competitors) != 2 {
		t.Fatalf("len(Competitors) = %d, want 2", len(competitors))
	}

	home, away := competitors[0], competitors[1]
	if home.HomeAway != "home" || home.ID != 30 || home.Team.ID != 30 || home.Score != 42 {
		t.Errorf("home competitor = %+v", home)
	}
	if away.HomeAway != "away" || away.ID != 23 || away.Team.ID != 23 || away.Score != 26 {
		t.Errorf("away competitor = %+v", away)
	}
	// Conference IDs must survive the mapping for consumers like
	// extractTeamConfs (USC = Big Ten 5, San José State = Mountain West 17).
	if home.Team.ConferenceID != 5 || away.Team.ConferenceID != 17 {
		t.Errorf("conference IDs = %d/%d, want 5/17", home.Team.ConferenceID, away.Team.ConferenceID)
	}
}

// TestSiteScoreboardFinalGames checks the mapping/filter in isolation against
// the captured fixture, including that in-progress and scheduled games are
// dropped and that a non-numeric event ID is an error rather than a silent
// skip.
func TestSiteScoreboardFinalGames(t *testing.T) {
	var res SiteScoreboardESPN
	if err := json.Unmarshal([]byte(siteScoreboardFixture), &res); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if res.Season.Year != 2026 || res.Week.Number != 1 {
		t.Errorf("season/week = %d/%d, want 2026/1", res.Season.Year, res.Week.Number)
	}
	if len(res.Events) != 3 {
		t.Fatalf("len(Events) = %d, want 3", len(res.Events))
	}

	games, err := res.finalGames()
	if err != nil {
		t.Fatalf("finalGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("len(games) = %d, want 1 (only STATUS_FINAL survives)", len(games))
	}
	if games[0].ID != 401864494 {
		t.Errorf("game ID = %d, want 401864494", games[0].ID)
	}

	broken := SiteScoreboardESPN{Events: []SiteEvent{{ID: "not-a-number"}}}
	if _, err := broken.finalGames(); err == nil {
		t.Error("expected error for non-numeric event ID, got nil")
	}

	// ESPN occasionally emits a degenerate empty event object (verified live
	// 2026-08-30 in an FCS date-range response); it must be skipped, not
	// treated as a parse error.
	empty := SiteScoreboardESPN{Events: []SiteEvent{{}, {ID: "401864494", Status: res.Events[0].Status}}}
	emptyGames, err := empty.finalGames()
	if err != nil {
		t.Fatalf("finalGames with empty event: %v", err)
	}
	if len(emptyGames) != 1 || emptyGames[0].ID != 401864494 {
		t.Errorf("empty-event walk = %+v, want only game 401864494", emptyGames)
	}
}

func TestSiteScoreboardValidate(t *testing.T) {
	tests := []struct {
		name    string
		resp    SiteScoreboardESPN
		wantErr string
	}{
		{
			name:    "empty response",
			resp:    SiteScoreboardESPN{},
			wantErr: "missing leagues",
		},
		{
			name:    "no season year",
			resp:    SiteScoreboardESPN{Leagues: []SiteScoreboardLeague{{}}},
			wantErr: "missing season year",
		},
		{
			// The top-level season object is absent from ?dates= payloads; the
			// leagues[0].season year is what validate must require.
			name: "valid with no events and no top-level season (dates payload)",
			resp: SiteScoreboardESPN{
				Leagues: []SiteScoreboardLeague{{Season: SiteScoreboardLeagueSeason{Year: 2026}}},
			},
		},
		{
			name: "valid with top-level season (plain payload)",
			resp: SiteScoreboardESPN{
				Leagues: []SiteScoreboardLeague{{Season: SiteScoreboardLeagueSeason{Year: 2026}}},
				Season:  SiteSeason{Year: 2026, Type: 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.validate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validate() returned unexpected error: %v", err)
			}
		})
	}
}

// TestGetGameStats_SiteAPISummary verifies the summary endpoint feeds the
// same GameInfoESPN accessors the cdn playbyplay path did.
func TestGetGameStats_SiteAPISummary(t *testing.T) {
	ts := setupSiteTestServer(t)
	client := newTestClient()
	t.Cleanup(client.SetURLs(ts.URL+"/apis/site/v2/sports/football/college-football/summary?event=%d", "", "", ""))

	res, err := client.GetGameStats(context.Background(), 401864494)
	if err != nil {
		t.Fatalf("GetGameStats: %v", err)
	}

	header := res.GamePackage.Header
	if header.ID != 401864494 {
		t.Errorf("Header.ID = %d, want 401864494", header.ID)
	}
	if header.Season.Year != 2026 || header.Season.Type != 2 {
		t.Errorf("Season = %+v, want year 2026 type 2", header.Season)
	}
	if header.Week != 1 {
		t.Errorf("Week = %d, want 1", header.Week)
	}
	if len(header.Competitions) != 1 {
		t.Fatalf("len(Competitions) = %d, want 1", len(header.Competitions))
	}
	comp := header.Competitions[0]
	if comp.Date != "2026-08-29T19:00Z" {
		t.Errorf("Date = %q", comp.Date)
	}
	if comp.ConfGame || comp.Neutral {
		t.Error("expected non-conference, non-neutral site game")
	}
	if len(comp.Competitors) != 2 {
		t.Fatalf("len(Competitors) = %d, want 2", len(comp.Competitors))
	}
	if comp.Competitors[0].ID != 30 || comp.Competitors[0].Score != 42 {
		t.Errorf("competitor[0] = %+v", comp.Competitors[0])
	}
	if comp.Competitors[1].ID != 23 || comp.Competitors[1].Score != 26 {
		t.Errorf("competitor[1] = %+v", comp.Competitors[1])
	}

	// Box score: team statistics keyed by team ID (SJSU appears first).
	teams := res.GamePackage.Boxscore.Teams
	if len(teams) != 2 || teams[0].Team.ID != 23 || teams[1].Team.ID != 30 {
		t.Fatalf("boxscore teams = %+v", teams)
	}
	if teams[0].Statistics[0].Name != "firstDowns" || teams[0].Statistics[0].DisplayValue != "19" {
		t.Errorf("SJSU firstDowns stat = %+v", teams[0].Statistics[0])
	}

	// Box score: player statistics with labels/totals/athletes.
	players := res.GamePackage.Boxscore.Players
	if len(players) != 2 {
		t.Fatalf("len(Players) = %d, want 2", len(players))
	}
	passing := players[0].Statistics[0]
	if passing.Name != "passing" || len(passing.Labels) == 0 || len(passing.Totals) == 0 {
		t.Fatalf("passing stats = %+v", passing)
	}
	if len(passing.Athletes) == 0 || passing.Athletes[0].Athlete.ID != 5295238 {
		t.Errorf("passing athletes = %+v", passing.Athletes)
	}
}
