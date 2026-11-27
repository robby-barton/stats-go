package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

// ---------------------------------------------------------------------------
// Women's basketball fixture — 3 D-I teams, 1 non-D-I filler, overlapping IDs
// ---------------------------------------------------------------------------
//
// The women's school IDs intentionally overlap the men's basketball fixture
// (IDs 11-14) to prove per-sport isolation: ESPN reuses the same team ID for
// the same school across sports, and the shared tables key everything by
// sport. Team 14 appears on the scoreboard without a conferenceId (the live
// non-D1 fill-in pattern) so it must show up in games but never in D-I
// membership or ranking rows.

const (
	wbFixtureGameID1 int64 = 601001
	wbFixtureGameID2 int64 = 601002 // vs non-D-I filler (team 14)
	wbFixtureGameID4 int64 = 601004
)

func wbFixtureScoreboardResponse() espn.ScoreboardESPN {
	return espn.ScoreboardESPN{
		Leagues: []espn.ScoreboardLeague{{
			Season: espn.ScoreboardSeason{
				Year:      2027,
				StartDate: "2026-11-09T08:00Z",
				EndDate:   "2027-04-06T06:59Z",
				Type:      espn.ScoreboardSeasonType{ID: 2, Name: "Regular Season"},
			},
			Calendar: []string{"2027-01-05T08:00Z", "2027-01-12T08:00Z"},
		}},
	}
}

func wbFixtureGameInfoResponse(gameID int64) espn.GameInfoESPN {
	games := map[int64]espn.GameInfoESPN{
		wbFixtureGameID1: wbNewGameInfo(wbFixtureGameID1, 11, 82, 12, 70, 10, true),
		wbFixtureGameID2: wbNewGameInfo(wbFixtureGameID2, 13, 95, 14, 40, 10, false),
		wbFixtureGameID4: wbNewGameInfo(wbFixtureGameID4, 11, 78, 13, 72, 11, false),
	}
	if g, ok := games[gameID]; ok {
		return g
	}
	return wbNewGameInfo(gameID, 11, 0, 12, 0, 10, false)
}

func wbNewGameInfo(
	gameID, homeID, homeScore, awayID, awayScore, week int64,
	confGame bool,
) espn.GameInfoESPN {
	const year = 2027
	dateStr := "2027-01-05T19:00Z"
	if week == 11 {
		dateStr = "2027-01-12T19:00Z"
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
					{Team: espn.Team{ID: homeID}, Statistics: []espn.TeamStatistics{
						{Name: "firstDowns", DisplayValue: "0"},
					}},
					{Team: espn.Team{ID: awayID}, Statistics: []espn.TeamStatistics{
						{Name: "firstDowns", DisplayValue: "0"},
					}},
				},
				Players: []espn.Players{},
			},
		},
	}
}

// wbFixtureTeamInfoResponse omits team 13: the bulk /teams endpoint misses it,
// so the season update must backfill its identity via the per-team endpoint.
func wbFixtureTeamInfoResponse() espn.TeamInfoESPN {
	return espn.TeamInfoESPN{
		Sports: []espn.TeamInfoSport{{
			ID:   600,
			Name: "Basketball",
			Slug: "basketball",
			Leagues: []espn.League{{
				ID:           41,
				Name:         "NCAA Women's Basketball",
				Abbreviation: "NCAAW",
				ShortName:    "NCAAW",
				Slug:         "womens-college-basketball",
				Year:         2027,
				Teams: []espn.TeamWrap{
					{Team: espn.TeamInfo{
						ID: 11, Name: "Huskies", DisplayName: "WBall Alpha Huskies",
						Abbreviation: "WBA", Location: "WBall Alpha", Slug: "wball-alpha",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 12, Name: "Gamecocks", DisplayName: "WBall Beta Gamecocks",
						Abbreviation: "WBB", Location: "WBall Beta", Slug: "wball-beta",
						IsActive: true,
					}},
					// Team 13 intentionally absent from the bulk listing.
				},
			}},
		}},
	}
}

// ---------------------------------------------------------------------------
// Mock HTTP server for the women's basketball paths
// ---------------------------------------------------------------------------

func setupWomensBasketballTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	const base = "/apis/site/v2/sports/basketball/womens-college-basketball"
	mux := http.NewServeMux()

	mux.HandleFunc(base+"/summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var gameID int64
		fmt.Sscanf(r.URL.Query().Get("event"), "%d", &gameID) //nolint:errcheck // test helper
		if err := json.NewEncoder(w).Encode(wbFixtureGameInfoResponse(gameID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc(base+"/teams", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(wbFixtureTeamInfoResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Per-team endpoint (identity backfill): echoes the requested ID.
	mux.HandleFunc(base+"/teams/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"team": {"id": "%s", "slug": "wball-backfilled", "location": "WBall Gamma",`+
			` "name": "Eagles", "abbreviation": "WBG", "displayName": "WBall Gamma Eagles",`+
			` "shortDisplayName": "W Gamma", "nickname": "Eagles", "isActive": true}}`, id)
	})

	// site.api scoreboard — mirrors the men's mock's query-dependent shapes.
	mux.HandleFunc(base+"/scoreboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dates := r.URL.Query().Get("dates")
		switch {
		case dates == "" || len(dates) == 4:
			if err := json.NewEncoder(w).Encode(wbFixtureScoreboardResponse()); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case dates == time.Now().Format("20060102"):
			resp := espn.SiteScoreboardESPN{
				Leagues: []espn.SiteScoreboardLeague{{Season: espn.SiteScoreboardLeagueSeason{Year: 2027}}},
				Events: []espn.SiteEvent{
					wbSiteEvent(wbFixtureGameID1, 11, 300, 82, 12, 300, 70),
					wbSiteEvent(wbFixtureGameID2, 13, 400, 95, 14, 0, 40),
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case dates == time.Now().AddDate(0, 0, -1).Format("20060102"):
			resp := espn.SiteScoreboardESPN{
				Leagues: []espn.SiteScoreboardLeague{{Season: espn.SiteScoreboardLeagueSeason{Year: 2027}}},
				Events: []espn.SiteEvent{
					wbSiteEvent(wbFixtureGameID1, 11, 300, 82, 12, 300, 70),
					wbSiteEvent(wbFixtureGameID4, 11, 300, 78, 13, 400, 72),
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		default:
			// Season walk: the same events carry team.conferenceId for the
			// TeamConferencesByYear pass. Team 14 carries no conferenceId.
			resp := espn.SiteScoreboardESPN{
				Leagues: []espn.SiteScoreboardLeague{{Season: espn.SiteScoreboardLeagueSeason{Year: 2027}}},
				Events: []espn.SiteEvent{
					wbSiteEvent(wbFixtureGameID1, 11, 300, 82, 12, 300, 70),
					wbSiteEvent(wbFixtureGameID2, 13, 400, 95, 14, 0, 40),
					wbSiteEvent(wbFixtureGameID4, 11, 300, 78, 13, 400, 72),
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	})

	mux.HandleFunc(base+"/scoreboard/conferences", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := espn.ConferencesESPN{Conferences: []espn.SiteConference{
			{GroupID: 50, Name: "NCAA Division I", ShortName: "Division I"},
			{GroupID: 300, Name: "Big East Conference", ShortName: "Big East", ParentGroupID: espn.FlexInt64(50)},
			{GroupID: 400, Name: "Southeastern Conference", ShortName: "SEC", ParentGroupID: espn.FlexInt64(50)},
		}}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// wbSiteEvent builds one completed women's scoreboard event. A conference
// ID of 0 is emitted as an omitted conferenceId (the non-D1 fill-in shape).
func wbSiteEvent(id, homeID, homeConf, homeScore, awayID, awayConf, awayScore int64) espn.SiteEvent {
	ev := espn.SiteEvent{
		ID:     fmt.Sprintf("%d", id),
		Season: espn.SiteSeason{Year: 2027, Type: 2},
		Week:   espn.SiteWeek{Number: 10},
		Status: espn.Status{StatusType: espn.StatusType{
			Name: "STATUS_FINAL", Completed: true,
		}},
		Competitions: []espn.SiteCompetition{{
			Competitors: []espn.Competitor{
				{
					ID: homeID, Score: homeScore, HomeAway: "home",
					Team: espn.ScheduleTeam{ID: homeID, ConferenceID: espn.FlexInt64(homeConf)},
				},
				{
					ID: awayID, Score: awayScore, HomeAway: "away",
					Team: espn.ScheduleTeam{ID: awayID, ConferenceID: espn.FlexInt64(awayConf)},
				},
			},
		}},
	}
	ev.Competitions[0].Status = ev.Status
	return ev
}

// ---------------------------------------------------------------------------
// Women's test Updater constructor
// ---------------------------------------------------------------------------

func newWomensBasketballTestUpdater(t *testing.T) *Updater {
	t.Helper()

	db := setupTestDB(t)
	ts := setupWomensBasketballTestServer(t)

	client := &espn.BasketballClient{Client: &espn.Client{
		MaxAttempts:    2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 5 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeWomensBasketball,
	}}
	t.Cleanup(client.SetURLs(
		ts.URL+"/apis/site/v2/sports/basketball/womens-college-basketball/summary?event=%d",
		ts.URL+"/apis/site/v2/sports/basketball/womens-college-basketball/teams?limit=1000",
		ts.URL+"/apis/site/v2/sports/basketball/womens-college-basketball/scoreboard",
		ts.URL+"/apis/site/v2/sports/basketball/womens-college-basketball/scoreboard/conferences",
	))

	u, err := NewUpdater(db, zap.NewNop().Sugar(), client)
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	return u
}

// seedWomensCrossSportContext seeds the same school IDs under ncaam and ncaaf
// so every ncaaw write must be isolated by sport.
func seedWomensCrossSportContext(t *testing.T, db *gorm.DB) {
	t.Helper()

	teamNames := []database.TeamName{
		{TeamID: 11, Name: "BBall Alpha", DisplayName: "Men's Huskies",
			Abbreviation: "MBA", Location: "BBall Alpha", Slug: "bball-alpha", IsActive: true, Sport: "ncaam"},
		{TeamID: 12, Name: "BBall Beta", DisplayName: "Men's Gamecocks",
			Abbreviation: "MBB", Location: "BBall Beta", Slug: "bball-beta", IsActive: true, Sport: "ncaam"},
		{TeamID: 11, Name: "Alpha", DisplayName: "Football Alpha",
			Abbreviation: "FAL", Location: "Football Alpha", Slug: "football-alpha", IsActive: true, Sport: "ncaaf"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed cross-sport team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 11, Year: 2027, FBS: 1, Conf: "Big East", Sport: "ncaam"},
		{TeamID: 12, Year: 2027, FBS: 1, Conf: "Big East", Sport: "ncaam"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed cross-sport team_seasons: %v", err)
	}

	games := []database.Game{
		{
			GameID: 501001, Season: 2027, Week: 10,
			HomeID: 11, AwayID: 12, HomeScore: 71, AwayScore: 66,
			Sport:     "ncaam",
			StartTime: time.Date(2027, 1, 5, 19, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed cross-sport games: %v", err)
	}
}

// ---------------------------------------------------------------------------
// End-to-end test
// ---------------------------------------------------------------------------

// TestWomensBasketball_EndToEnd exercises the hermetic one-shot pipeline
// ESPN mock -> games -> team seasons -> D-I ranking for ncaaw, with school
// IDs overlapping the men's fixture and explicit sport isolation. The
// ncaam/ncaaf rows seeded before the run must be byte-for-byte untouched.
func TestWomensBasketball_EndToEnd(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	seedWomensCrossSportContext(t, u.db)
	ctx := context.Background()

	// 1. Season membership: conference walk + conference map over the
	// women's endpoints. The non-D-I filler (14) must never get a row, and
	// team 13 (absent from the bulk listing) must be backfilled.
	added, err := u.UpdateTeamSeasons(ctx, true)
	if err != nil {
		t.Fatalf("UpdateTeamSeasons: %v", err)
	}
	if added != 3 {
		t.Fatalf("added seasons = %d, want 3 (D-I teams 11, 12, 13 only)", added)
	}

	var seasons []database.TeamSeason
	if err := u.db.Where("sport = ?", "ncaaw").Find(&seasons).Error; err != nil {
		t.Fatalf("query ncaaw team_seasons: %v", err)
	}
	if len(seasons) != 3 {
		t.Fatalf("len(ncaaw team_seasons) = %d, want 3", len(seasons))
	}
	for _, s := range seasons {
		if s.Year != 2027 || s.FBS != 1 {
			t.Errorf("ncaaw team_seasons (%d, %d, fbs=%d): want 2027/D-I", s.TeamID, s.Year, s.FBS)
		}
		if s.Conf != "Big East" && s.Conf != "SEC" {
			t.Errorf("ncaaw team %d conf = %q, want a mapped D-I conference", s.TeamID, s.Conf)
		}
	}
	var count int64
	if err := u.db.Model(database.TeamSeason{}).Where("sport = ?", "ncaam").Count(&count).Error; err != nil {
		t.Fatalf("count ncaam team_seasons: %v", err)
	}
	if count != 2 {
		t.Errorf("ncaam team_seasons rows = %d, want 2 (untouched)", count)
	}

	// Identity backfill: team 13 was absent from the bulk /teams listing.
	var backfilled database.TeamName
	if err := u.db.Where("team_id = ? and sport = ?", 13, "ncaaw").First(&backfilled).Error; err != nil {
		t.Fatalf("team_names backfill for women's team 13: %v", err)
	}
	if backfilled.Slug != "wball-backfilled" {
		t.Errorf("backfilled slug = %q, want wball-backfilled", backfilled.Slug)
	}

	// 2. Games: the women's updater ingests its own events on its own path.
	result, err := u.UpdateCurrentWeek(ctx)
	if err != nil {
		t.Fatalf("UpdateCurrentWeek: %v", err)
	}
	if len(result.Processed) != 3 {
		t.Fatalf("processed games = %v, want all 3 finals", result.Processed)
	}

	var wbGames []database.Game
	if err := u.db.Where("sport = ?", "ncaaw").Find(&wbGames).Error; err != nil {
		t.Fatalf("query ncaaw games: %v", err)
	}
	if len(wbGames) != 3 {
		t.Fatalf("ncaaw games = %d, want 3", len(wbGames))
	}
	byID := map[int64]database.Game{}
	for _, g := range wbGames {
		byID[g.GameID] = g
	}
	if byID[wbFixtureGameID1].HomeScore != 82 || byID[wbFixtureGameID1].AwayScore != 70 {
		t.Errorf("game %d scores = %d-%d, want 82-70",
			wbFixtureGameID1, byID[wbFixtureGameID1].HomeScore, byID[wbFixtureGameID1].AwayScore)
	}
	// The game against the non-D-I filler is retained for the record.
	if byID[wbFixtureGameID2].AwayID != 14 || byID[wbFixtureGameID2].AwayScore != 40 {
		t.Errorf("game %d must keep the non-D-I opponent", wbFixtureGameID2)
	}

	// Cross-sport isolation: the seeded men's game is untouched, and no
	// ncaam/ncaaf rows reference the women's event IDs.
	var mbGame database.Game
	if err := u.db.Where("game_id = ? and sport = ?", 501001, "ncaam").First(&mbGame).Error; err != nil {
		t.Fatalf("seeded ncaam game missing: %v", err)
	}
	if mbGame.HomeScore != 71 || mbGame.AwayScore != 66 {
		t.Errorf("ncaam game scores changed: %d-%d", mbGame.HomeScore, mbGame.AwayScore)
	}
	if err := u.db.Model(database.Game{}).
		Where("game_id = ? and sport in (?)", wbFixtureGameID1, []string{"ncaaf", "ncaam"}).
		Count(&count).Error; err != nil {
		t.Fatalf("count cross-sport women's event ID: %v", err)
	}
	if count != 0 {
		t.Errorf("women's event %d leaked into another sport", wbFixtureGameID1)
	}

	// 3. D-I ranking: only the three D-I women's teams, only ncaaw rows,
	// Fbs=true per the single-division basketball contract.
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}

	var rankings []database.TeamWeekResult
	if err := u.db.Where("sport = ?", "ncaaw").Find(&rankings).Error; err != nil {
		t.Fatalf("query ncaaw rankings: %v", err)
	}
	if len(rankings) != 3 {
		t.Fatalf("ncaaw ranking rows = %d, want 3 D-I teams", len(rankings))
	}
	ranked := map[int64]database.TeamWeekResult{}
	for _, r := range rankings {
		if r.Year != 2027 {
			t.Errorf("ranking year = %d, want 2027", r.Year)
		}
		if !r.Fbs {
			t.Errorf("ranking row fbs=false, want true (single D-I division)")
		}
		if r.Postseason != 0 {
			t.Errorf("regular-week ranking has postseason=%d, want 0", r.Postseason)
		}
		ranked[r.TeamID] = r
	}
	if _, ok := ranked[14]; ok {
		t.Error("non-D-I filler 14 must never be ranked")
	}
	if ranked[11].FinalRank != 1 {
		t.Errorf("team 11 FinalRank = %d, want 1 (2-0 with best margins)", ranked[11].FinalRank)
	}
	if ranked[11].Wins != 2 || ranked[11].Losses != 0 {
		t.Errorf("team 11 record = %d-%d, want 2-0",
			ranked[11].Wins, ranked[11].Losses)
	}
	if ranked[12].Wins != 0 || ranked[12].Losses != 1 {
		t.Errorf("team 12 record = %d-%d, want 0-1",
			ranked[12].Wins, ranked[12].Losses)
	}

	// No ranking rows for the men's sport — the women's run must not write
	// cross-sport snapshots.
	if err := u.db.Model(database.TeamWeekResult{}).
		Where("sport in (?)", []string{"ncaaf", "ncaam"}).Count(&count).Error; err != nil {
		t.Fatalf("count cross-sport ranking rows: %v", err)
	}
	if count != 0 {
		t.Errorf("cross-sport ranking rows = %d, want 0", count)
	}
}
