package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ---------------------------------------------------------------------------
// Strict backfill fixture — three seasons (2025, 2026, 2027), shared teams
// ---------------------------------------------------------------------------
//
// Team IDs (overlapping the men's fixture IDs on purpose):
//
//	11 = Huskies    (conference 300, bulk D-I listing)
//	12 = Gamecocks  (conference 400, bulk D-I listing)
//	13 = Eagles     (conference 400, bulk D-I listing, absent from bulk
//	                 listing — identity backfill path)
//	14 = Fill-in    (no conferenceId, not listed — lower-division opponent)
//
// Season 2026 (2025-26, completed): 3 regular finals across two weeks plus
// the championship final whose summary omits the postseason week.
// Season 2027 (2026-27, current): 2 regular finals.
// Season 2025 (2024-25): 1 regular final (lookback for an explicit 2026
// publication).

const (
	ncaawBFTeamHuskies   int64 = 11
	ncaawBFTeamGamecocks int64 = 12
	ncaawBFTeamEagles    int64 = 13
	ncaawBFTeamFiller    int64 = 14

	// ncaawBFFakeNow pins the observed clock so the 2027 walk (which ends
	// "today, Eastern") is hermetic and always covers the fixture days.
	ncaawBFFakeNow = "2026-11-27T17:00:00Z"
)

type ncaawBFTeam struct {
	id     int64
	confID int64 // 0 → the competitor carries no conferenceId
	bulkD1 bool  // listed by the bulk D-I /teams endpoint
}

type ncaawBFGame struct {
	id         int64
	seasonYear int64
	seasonType int64 // ESPN season type: 2 regular, 3 postseason (0 → 2)
	week       int64 // summary header week (0 = omitted, e.g. the Final)
	date       string
	day        string // ET walk day the scoreboard serves the event on
	home, away ncaawBFTeam
	homeScore  int64
	awayScore  int64
	confGame   bool
	// summaryScores, when non-nil, overrides the summary's scores so the
	// stored row disagrees with the advertised final (audit failure path).
	summaryScores *[2]int64
}

type ncaawBFSeason struct {
	year     int64
	calendar []string // flat ISO date list; nil = empty advertised calendar
}

// ncaawBFFixture drives every endpoint of the mock women's server. Tests
// clone the default fixture and mutate one field to force a specific
// failure.
type ncaawBFFixture struct {
	seasons       []ncaawBFSeason
	games         []ncaawBFGame
	conferences   []espn.SiteConference
	bulkD1        []espn.TeamInfo
	currentSeason int64
	calendarReady *atomic.Bool // non-nil: 2027's year query returns the prior year until ready

	dayStatus     map[string]int   // ET day → forced daily-walk HTTP status
	dailyYear     map[string]int64 // ET day → forced wrong season year
	summaryStatus map[int64]int    // game ID → forced summary HTTP status
	teamStatus    map[int64]int    // team ID → forced per-team HTTP status
}

func ncaawBFTeamHuskieDef() ncaawBFTeam {
	return ncaawBFTeam{id: ncaawBFTeamHuskies, confID: 300, bulkD1: true}
}
func ncaawBFTeamGamecockDef() ncaawBFTeam {
	return ncaawBFTeam{id: ncaawBFTeamGamecocks, confID: 400, bulkD1: true}
}
func ncaawBFTeamEagleDef() ncaawBFTeam {
	return ncaawBFTeam{id: ncaawBFTeamEagles, confID: 400, bulkD1: true}
}
func ncaawBFTeamFillerDef() ncaawBFTeam {
	return ncaawBFTeam{id: ncaawBFTeamFiller, confID: 0, bulkD1: false}
}

// defaultNCAAWBFFixture returns the fully valid fixture every failure test
// starts from.
func defaultNCAAWBFFixture() *ncaawBFFixture {
	huskies, gamecocks, eagles, filler :=
		ncaawBFTeamHuskieDef(), ncaawBFTeamGamecockDef(), ncaawBFTeamEagleDef(), ncaawBFTeamFillerDef()

	fx := &ncaawBFFixture{
		seasons: []ncaawBFSeason{
			{year: 2025, calendar: []string{"2024-11-05T08:00Z"}},
			{year: 2026, calendar: []string{
				"2025-11-04T08:00Z", "2026-01-06T08:00Z", "2026-04-05T07:00Z",
			}},
			{year: 2027, calendar: []string{"2026-11-10T08:00Z", "2026-11-11T08:00Z"}},
		},
		conferences: []espn.SiteConference{
			{GroupID: 50, Name: "NCAA Division I", ShortName: "Division I"},
			{GroupID: 300, Name: "Big East Conference", ShortName: "Big East", ParentGroupID: espn.FlexInt64(50)},
			{GroupID: 400, Name: "Southeastern Conference", ShortName: "SEC", ParentGroupID: espn.FlexInt64(50)},
		},
		currentSeason: 2027,
		dayStatus:     map[string]int{},
		dailyYear:     map[string]int64{},
		summaryStatus: map[int64]int{},
		teamStatus:    map[int64]int{},
	}

	fx.games = []ncaawBFGame{
		{
			id: 61001, seasonYear: 2025, seasonType: 2, week: 1,
			date: "2024-11-05T23:00Z", day: "20241105",
			home: huskies, away: gamecocks, homeScore: 80, awayScore: 60, confGame: true,
		},
		{
			id: 61101, seasonYear: 2026, seasonType: 2, week: 1,
			date: "2025-11-04T23:00Z", day: "20251104",
			home: huskies, away: gamecocks, homeScore: 82, awayScore: 70, confGame: true,
		},
		{
			id: 61102, seasonYear: 2026, seasonType: 2, week: 1,
			date: "2025-11-05T01:00Z", day: "20251104",
			home: eagles, away: filler, homeScore: 95, awayScore: 40,
		},
		{
			id: 61103, seasonYear: 2026, seasonType: 2, week: 10,
			date: "2026-01-06T19:00Z", day: "20260106",
			home: huskies, away: eagles, homeScore: 70, awayScore: 65,
		},
		{
			id: 61104, seasonYear: 2026, seasonType: 3, week: 0, // Final: week omitted
			date: "2026-04-05T19:30Z", day: "20260405",
			home: huskies, away: gamecocks, homeScore: 79, awayScore: 51,
		},
		{
			id: 61201, seasonYear: 2027, seasonType: 2, week: 1,
			date: "2026-11-10T23:00Z", day: "20261110",
			home: huskies, away: gamecocks, homeScore: 75, awayScore: 72,
		},
		{
			id: 61202, seasonYear: 2027, seasonType: 2, week: 2,
			date: "2026-11-11T19:00Z", day: "20261111",
			home: eagles, away: gamecocks, homeScore: 60, awayScore: 58,
		},
	}

	for _, team := range []ncaawBFTeam{huskies, gamecocks, eagles} {
		if team.bulkD1 {
			fx.bulkD1 = append(fx.bulkD1, espn.TeamInfo{
				ID: team.id, Name: "Nickname", DisplayName: fmt.Sprintf("WBB Team %d", team.id),
				Abbreviation: "WBB", Location: "WBB", Slug: fmt.Sprintf("wbb-%d", team.id),
				IsActive: true,
			})
		}
	}
	return fx
}

// seasonYearForDay resolves the season-ending year a daily scoreboard
// response must carry for the given ET day.
func (fx *ncaawBFFixture) seasonYearForDay(day string) int64 {
	t, err := time.Parse("20060102", day)
	if err != nil {
		return fx.currentSeason
	}
	for _, s := range fx.seasons {
		start := time.Date(int(s.year)-1, time.November, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(int(s.year), time.April, 30, 0, 0, 0, 0, time.UTC)
		if !t.Before(start) && !t.After(end) {
			return s.year
		}
	}
	return fx.currentSeason
}

func (fx *ncaawBFFixture) gameByID(id int64) (ncaawBFGame, bool) {
	for _, g := range fx.games {
		if g.id == id {
			return g, true
		}
	}
	return ncaawBFGame{}, false
}

func ncaawBFStatus() espn.Status {
	return espn.Status{StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true}}
}

func ncaawBFEvent(g ncaawBFGame) espn.SiteEvent {
	return espn.SiteEvent{
		ID:     strconv.FormatInt(g.id, 10),
		Date:   g.date,
		Season: espn.SiteSeason{Year: g.seasonYear, Type: g.seasonType},
		Status: ncaawBFStatus(),
		Competitions: []espn.SiteCompetition{{
			Date:   g.date,
			Status: ncaawBFStatus(),
			Competitors: []espn.Competitor{
				{ID: g.home.id, Score: g.homeScore, HomeAway: "home",
					Team: espn.ScheduleTeam{ID: g.home.id, ConferenceID: espn.FlexInt64(g.home.confID)}},
				{ID: g.away.id, Score: g.awayScore, HomeAway: "away",
					Team: espn.ScheduleTeam{ID: g.away.id, ConferenceID: espn.FlexInt64(g.away.confID)}},
			},
		}},
	}
}

func ncaawBFSummary(g ncaawBFGame) espn.GameInfoESPN {
	homeScore, awayScore := g.homeScore, g.awayScore
	if g.summaryScores != nil {
		homeScore, awayScore = g.summaryScores[0], g.summaryScores[1]
	}
	seasonType := g.seasonType
	if seasonType == 0 {
		seasonType = 2
	}
	return espn.GameInfoESPN{
		GamePackage: espn.GamePackage{
			Header: espn.Header{
				ID: g.id,
				Competitions: []espn.Competitions{{
					ID:       g.id,
					Date:     g.date,
					ConfGame: g.confGame,
					Neutral:  false,
					Competitors: []espn.Competitors{
						{HomeAway: "home", ID: g.home.id, Score: homeScore},
						{HomeAway: "away", ID: g.away.id, Score: awayScore},
					},
					Status: ncaawBFStatus(),
				}},
				Season: espn.Season{Year: g.seasonYear, Type: seasonType},
				Week:   g.week,
			},
			Boxscore: espn.Boxscore{
				Teams: []espn.Teams{
					{Team: espn.Team{ID: g.home.id}, Statistics: []espn.TeamStatistics{
						{Name: "firstDowns", DisplayValue: "0"},
					}},
					{Team: espn.Team{ID: g.away.id}, Statistics: []espn.TeamStatistics{
						{Name: "firstDowns", DisplayValue: "0"},
					}},
				},
				Players: []espn.Players{},
			},
		},
	}
}

// ncaawBFBase is the mock server's women's basketball URL prefix.
const ncaawBFBase = "/apis/site/v2/sports/basketball/womens-college-basketball"

func setupNCAAWBackfillServer(t *testing.T, fx *ncaawBFFixture) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	encode := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

	mux.HandleFunc(ncaawBFBase+"/summary", func(w http.ResponseWriter, r *http.Request) {
		var gameID int64
		fmt.Sscanf(r.URL.Query().Get("event"), "%d", &gameID) //nolint:errcheck // test helper
		if status, ok := fx.summaryStatus[gameID]; ok {
			http.Error(w, "forced summary failure", status)
			return
		}
		g, ok := fx.gameByID(gameID)
		if !ok {
			http.Error(w, "unknown event", http.StatusNotFound)
			return
		}
		encode(w, ncaawBFSummary(g))
	})

	mux.HandleFunc(ncaawBFBase+"/teams", func(w http.ResponseWriter, _ *http.Request) {
		encode(w, espn.TeamInfoESPN{Sports: []espn.TeamInfoSport{{
			ID: 600, Name: "Basketball", Slug: "basketball",
			Leagues: []espn.League{{
				ID: 41, Name: "NCAA Women's Basketball", Abbreviation: "NCAAW",
				ShortName: "NCAAW", Slug: "womens-college-basketball",
				Year:  fx.currentSeason,
				Teams: fx.bulkD1Wraps(),
			}},
		}}})
	})

	mux.HandleFunc(ncaawBFBase+"/teams/", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:], 10, 64)
		if err != nil {
			http.Error(w, "bad team id", http.StatusBadRequest)
			return
		}
		if status, ok := fx.teamStatus[id]; ok {
			http.Error(w, "forced team failure", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"team": {"id": "%d", "slug": "ncaaw-backfilled-%d",`+
			` "location": "WBB", "name": "Nickname", "abbreviation": "WBB",`+
			` "displayName": "WBB Team %d", "shortDisplayName": "W %d",`+
			` "nickname": "Nickname", "isActive": true}}`, id, id, id, id)
	})

	mux.HandleFunc(ncaawBFBase+"/scoreboard/conferences", func(w http.ResponseWriter, _ *http.Request) {
		encode(w, espn.ConferencesESPN{Conferences: fx.conferences})
	})

	mux.HandleFunc(ncaawBFBase+"/scoreboard", func(w http.ResponseWriter, r *http.Request) {
		dates := r.URL.Query().Get("dates")
		switch {
		case dates == "":
			// Plain scoreboard: season metadata for DefaultSeason.
			encode(w, espn.ScoreboardESPN{Leagues: []espn.ScoreboardLeague{{
				Season: espn.ScoreboardSeason{
					Year:      fx.currentSeason,
					StartDate: "2026-07-13T07:00Z",
					EndDate:   "2027-04-07T06:59Z",
					Type:      espn.ScoreboardSeasonType{ID: 2, Name: "Regular Season"},
				},
			}}})
		case len(dates) == 4:
			// Season calendar query: the flat women's date list.
			year, _ := strconv.ParseInt(dates, 10, 64)
			var calendar []string
			for _, s := range fx.seasons {
				if s.year == year {
					calendar = s.calendar
				}
			}
			encode(w, espn.ScoreboardESPN{Leagues: []espn.ScoreboardLeague{{
				Season: espn.ScoreboardSeason{
					Year:      fx.advertisedCalendarYear(year),
					StartDate: "2026-07-13T07:00Z",
					EndDate:   "2027-04-07T06:59Z",
					Type:      espn.ScoreboardSeasonType{ID: 2, Name: "Regular Season"},
				},
				Calendar: calendar,
			}}})
		case len(dates) == 8:
			// One ET day of the D1 group-50 daily walk.
			if status, ok := fx.dayStatus[dates]; ok {
				http.Error(w, "forced daily failure", status)
				return
			}
			year := fx.dailyYear[dates]
			if year == 0 {
				year = fx.seasonYearForDay(dates)
			}
			events := []espn.SiteEvent{}
			for _, g := range fx.games {
				if g.day == dates {
					events = append(events, ncaawBFEvent(g))
				}
			}
			encode(w, espn.SiteScoreboardESPN{
				Leagues: []espn.SiteScoreboardLeague{{
					Season: espn.SiteScoreboardLeagueSeason{Year: year},
				}},
				Events: events,
			})
		default:
			http.Error(w, "unsupported dates parameter", http.StatusBadRequest)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func (fx *ncaawBFFixture) advertisedCalendarYear(year int64) int64 {
	if year == 2027 && fx.calendarReady != nil && !fx.calendarReady.Load() {
		return year - 1
	}
	return year
}

// bulkD1Wraps wraps the bulk-listed teams for the /teams response.
func (fx *ncaawBFFixture) bulkD1Wraps() []espn.TeamWrap {
	wraps := make([]espn.TeamWrap, 0, len(fx.bulkD1))
	for _, team := range fx.bulkD1 {
		wraps = append(wraps, espn.TeamWrap{Team: team})
	}
	return wraps
}

// newNCAAWBackfillUpdater builds the women's updater against the fixture
// server and pins the observed clock so the current-season walk is hermetic.
func newNCAAWBackfillUpdater(t *testing.T, fx *ncaawBFFixture) *Updater {
	t.Helper()
	return ncaawBackfillFixtureUpdater(t, setupTestDB(t), fx)
}

func ncaawBackfillFixtureUpdater(t *testing.T, db *gorm.DB, fx *ncaawBFFixture) *Updater {
	t.Helper()
	fakeNow, err := time.Parse(time.RFC3339, ncaawBFFakeNow)
	if err != nil {
		t.Fatalf("parsing fixture clock: %v", err)
	}
	origNow := nowFunc
	nowFunc = func() time.Time { return fakeNow }
	t.Cleanup(func() { nowFunc = origNow })

	ts := setupNCAAWBackfillServer(t, fx)

	client := &espn.BasketballClient{Client: &espn.Client{
		MaxAttempts:    2,
		InitialBackoff: time.Millisecond,
		RequestTimeout: 5 * time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeWomensBasketball,
	}}
	t.Cleanup(client.SetURLs(
		ts.URL+ncaawBFBase+"/summary?event=%d",
		ts.URL+ncaawBFBase+"/teams?limit=1000",
		ts.URL+ncaawBFBase+"/scoreboard",
		ts.URL+ncaawBFBase+"/scoreboard/conferences",
	))

	u, err := NewUpdater(db, zap.NewNop().Sugar(), client)
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	return u
}

// ---------------------------------------------------------------------------
// Assertions helpers
// ---------------------------------------------------------------------------

func ncaawBFCount(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := db.Model(database.Game{}).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return count
}

func ncaawBFRankings(t *testing.T, db *gorm.DB, query string, args ...any) []database.TeamWeekResult {
	t.Helper()
	var rows []database.TeamWeekResult
	if err := db.Where(query, args...).Find(&rows).Error; err != nil {
		t.Fatalf("ranking query %q: %v", query, err)
	}
	return rows
}

// ---------------------------------------------------------------------------
// Success paths
// ---------------------------------------------------------------------------

// TestNCAAWBackfill_SuccessScopedPublication covers the default scoped run:
// 2026 and 2027 are imported and fully audited, but only 2027 is published —
// 2026 stays lookback-only and never reaches team_week_results.
func TestNCAAWBackfill_SuccessScopedPublication(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())
	ctx := context.Background()

	if err := u.BackfillNCAAW(ctx, 2026, 2027, 0); err != nil {
		t.Fatalf("BackfillNCAAW: %v", err)
	}

	// Games: 4 finals for 2026 (including the Final whose week is omitted)
	// and 2 for 2027, all retry=0, ncaaw only.
	if got := ncaawBFCount(t, u.db, "sport = ? and season = ?", "ncaaw", 2026); got != 4 {
		t.Errorf("2026 ncaaw games = %d, want 4", got)
	}
	if got := ncaawBFCount(t, u.db, "sport = ? and season = ?", "ncaaw", 2027); got != 2 {
		t.Errorf("2027 ncaaw games = %d, want 2", got)
	}
	if got := ncaawBFCount(t, u.db, "sport = ?", "ncaaw"); got != 6 {
		t.Errorf("ncaaw games = %d, want 6", got)
	}
	if got := ncaawBFCount(t, u.db, "sport = ? and retry = 1", "ncaaw"); got != 0 {
		t.Errorf("retry-marked ncaaw games = %d, want 0", got)
	}

	var finalGame database.Game
	if err := u.db.Where("game_id = ? and sport = ?", int64(61104), "ncaaw").
		First(&finalGame).Error; err != nil {
		t.Fatalf("loading the 2026 Final: %v", err)
	}
	if finalGame.Postseason != 1 || finalGame.Week != 0 {
		t.Errorf("Final game postseason/week = %d/%d, want 1/0 (missing week tolerated)",
			finalGame.Postseason, finalGame.Week)
	}

	// Non-D-I filler: stored in the game record, no membership, never ranked.
	var fillerGame database.Game
	if err := u.db.Where("game_id = ? and sport = ?", int64(61102), "ncaaw").
		First(&fillerGame).Error; err != nil {
		t.Fatalf("loading the filler game: %v", err)
	}
	if fillerGame.AwayID != ncaawBFTeamFiller {
		t.Errorf("filler game away = %d, want %d", fillerGame.AwayID, ncaawBFTeamFiller)
	}
	var fillerSeasons int64
	if err := u.db.Model(database.TeamSeason{}).
		Where("sport = ? and team_id = ?", "ncaaw", ncaawBFTeamFiller).
		Count(&fillerSeasons).Error; err != nil {
		t.Fatalf("counting filler team_seasons: %v", err)
	}
	if fillerSeasons != 0 {
		t.Errorf("filler team_seasons rows = %d, want 0 (stored but not ranked)", fillerSeasons)
	}

	// Membership: 3 D-I teams per season, fbs=1, identity rows present.
	var seasons []database.TeamSeason
	if err := u.db.Where("sport = ?", "ncaaw").Find(&seasons).Error; err != nil {
		t.Fatalf("querying ncaaw team_seasons: %v", err)
	}
	if len(seasons) != 6 {
		t.Fatalf("ncaaw team_seasons rows = %d, want 6 (3 teams x 2 seasons)", len(seasons))
	}
	for _, s := range seasons {
		if s.FBS != 1 || (s.Year != 2026 && s.Year != 2027) {
			t.Errorf("team %d season (%d, fbs=%d): want D-I in 2026/2027", s.TeamID, s.Year, s.FBS)
		}
		if s.Conf != "Big East" && s.Conf != "SEC" {
			t.Errorf("team %d conf = %q, want a mapped D-I conference", s.TeamID, s.Conf)
		}
	}
	for _, teamID := range []int64{ncaawBFTeamHuskies, ncaawBFTeamGamecocks, ncaawBFTeamEagles} {
		var count int64
		if err := u.db.Model(database.TeamName{}).
			Where("team_id = ? and sport = ?", teamID, "ncaaw").Count(&count).Error; err != nil {
			t.Fatalf("counting team_names for %d: %v", teamID, err)
		}
		if count != 1 {
			t.Errorf("team_names rows for %d = %d, want 1", teamID, count)
		}
	}
	var backfilled database.TeamName
	if err := u.db.Where("team_id = ? and sport = ?", ncaawBFTeamEagles, "ncaaw").
		First(&backfilled).Error; err != nil {
		t.Fatalf("identity backfill for team 13: %v", err)
	}
	if backfilled.Slug != "ncaaw-backfilled-13" {
		t.Errorf("backfilled slug = %q, want ncaaw-backfilled-13", backfilled.Slug)
	}

	// Publication: only 2027, ESPN weeks 1 and 2, D-I teams only.
	rows := ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")
	if len(rows) != 6 {
		t.Fatalf("published ncaaw ranking rows = %d, want 6 (3 teams x 2 weeks)", len(rows))
	}
	phases := map[[2]int64]bool{}
	ranks := map[int64]bool{}
	for _, row := range rows {
		if row.Year != 2027 {
			t.Errorf("published ranking year = %d, want 2027 only (2026 stays lookback-only)", row.Year)
		}
		if !row.Fbs {
			t.Errorf("team %d ranking row fbs=false, want true", row.TeamID)
		}
		if row.TeamID == ncaawBFTeamFiller {
			t.Error("the lower-division filler must never be ranked")
		}
		phases[[2]int64{row.Week, row.Postseason}] = true
		if row.Postseason == 0 {
			ranks[row.FinalRank] = true
		}
	}
	for _, phase := range [][2]int64{{1, 0}, {2, 0}} {
		if !phases[phase] {
			t.Errorf("missing published phase (week=%d, postseason=%d)", phase[0], phase[1])
		}
	}
	for rank := int64(1); rank <= 3; rank++ {
		if !ranks[rank] {
			t.Errorf("regular rank %d missing from %v", rank, ranks)
		}
	}

	// No other sport was touched or written.
	if got := ncaawBFCount(t, u.db, "sport in (?)", []string{"ncaaf", "ncaam"}); got != 0 {
		t.Errorf("cross-sport rows = %d, want 0", got)
	}
}

// TestNCAAWBackfill_ExplicitPublishFromExposesOlderYear covers the explicit
// --publish-from 2026 run: 2025 is imported and audited as the lookback
// dataset, 2026 is fully validated and published, and nothing else appears.
func TestNCAAWBackfill_ExplicitPublishFromExposesOlderYear(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())
	ctx := context.Background()

	if err := u.BackfillNCAAW(ctx, 2025, 2026, 2026); err != nil {
		t.Fatalf("BackfillNCAAW: %v", err)
	}

	if got := ncaawBFCount(t, u.db, "sport = ? and season = ?", "ncaaw", 2025); got != 1 {
		t.Errorf("2025 ncaaw games = %d, want 1 (lookback dataset)", got)
	}
	rows := ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")
	if len(rows) != 9 {
		t.Fatalf("published rows = %d, want 9 (3 teams x 3 phases for 2026)", len(rows))
	}
	phases := map[[2]int64]bool{}
	for _, row := range rows {
		if row.Year != 2026 {
			t.Errorf("published year = %d, want 2026 only", row.Year)
		}
		phases[[2]int64{row.Week, row.Postseason}] = true
	}
	for _, phase := range [][2]int64{{1, 0}, {10, 0}, {0, 1}} {
		if !phases[phase] {
			t.Errorf("missing published phase (week=%d, postseason=%d)", phase[0], phase[1])
		}
	}
}

// TestNCAAWBackfill_RerunIsIdempotent covers rerunning the same scoped
// backfill: no duplicate imports, retry rows stay clear, the publication is
// stable, stale ranking phases of published years are cleaned, and other
// sports' ranking rows are untouched.
func TestNCAAWBackfill_RerunIsIdempotent(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())
	ctx := context.Background()

	// Stale ranking phases from an earlier (hypothetical) publication: a
	// ncaaw current-week snapshot that the fresh run must replace, and a
	// ncaam row that must survive untouched.
	stale := []database.TeamWeekResult{
		{TeamID: 11, Name: "WBB Team 11", Conf: "Big East", Year: 2027, Week: 42,
			Postseason: 0, Sport: "ncaaw", FinalRank: 1, Fbs: true},
		{TeamID: 11, Name: "Men's Huskies", Conf: "Big East", Year: 2027, Week: 5,
			Postseason: 0, Sport: "ncaam", FinalRank: 1, Fbs: true},
	}
	if err := u.db.Create(&stale).Error; err != nil {
		t.Fatalf("seeding stale ranking rows: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := u.BackfillNCAAW(ctx, 2026, 2027, 0); err != nil {
			t.Fatalf("BackfillNCAAW run %d: %v", run, err)
		}
	}

	if got := ncaawBFCount(t, u.db, "sport = ?", "ncaaw"); got != 6 {
		t.Errorf("ncaaw games after rerun = %d, want 6", got)
	}
	rows := ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")
	if len(rows) != 6 {
		t.Fatalf("published rows after rerun = %d, want 6", len(rows))
	}
	for _, row := range rows {
		if row.Week == 42 {
			t.Errorf("stale week-42 ncaaw phase survived: %+v", row)
		}
	}
	mens := ncaawBFRankings(t, u.db, "sport = ? and team_id = ?", "ncaam", int64(11))
	if len(mens) != 1 || mens[0].Week != 5 {
		t.Errorf("ncaam ranking rows changed by the ncaaw rerun: %+v", mens)
	}
}

// TestNCAAWBackfill_CrossSportIsolation proves the strict backfill writes
// only ncaaw rows while school IDs overlap ncaam/ncaaf rows.
func TestNCAAWBackfill_CrossSportIsolation(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())
	seedWomensCrossSportContext(t, u.db)
	ctx := context.Background()

	if err := u.BackfillNCAAW(ctx, 2026, 2027, 0); err != nil {
		t.Fatalf("BackfillNCAAW: %v", err)
	}

	var mensGame database.Game
	if err := u.db.Where("game_id = ? and sport = ?", int64(501001), "ncaam").
		First(&mensGame).Error; err != nil {
		t.Fatalf("seeded ncaam game missing: %v", err)
	}
	if mensGame.HomeScore != 71 || mensGame.AwayScore != 66 {
		t.Errorf("ncaam game changed: %d-%d", mensGame.HomeScore, mensGame.AwayScore)
	}
	var count int64
	if err := u.db.Model(database.TeamSeason{}).Where("sport = ?", "ncaam").Count(&count).Error; err != nil {
		t.Fatalf("counting ncaam team_seasons: %v", err)
	}
	if count != 2 {
		t.Errorf("ncaam team_seasons = %d, want 2 (untouched)", count)
	}
	if err := u.db.Model(database.Game{}).
		Where("game_id in (?) and sport in (?)", []int64{61101, 61201}, []string{"ncaaf", "ncaam"}).
		Count(&count).Error; err != nil {
		t.Fatalf("counting leaked women's event IDs: %v", err)
	}
	if count != 0 {
		t.Errorf("women's event IDs leaked into other sports: %d rows", count)
	}
	if err := u.db.Model(database.TeamWeekResult{}).
		Where("sport in (?)", []string{"ncaaf", "ncaam"}).Count(&count).Error; err != nil {
		t.Fatalf("counting cross-sport ranking rows: %v", err)
	}
	if count != 0 {
		t.Errorf("cross-sport ranking rows = %d, want 0", count)
	}
}

// ---------------------------------------------------------------------------
// Fail-closed paths
// ---------------------------------------------------------------------------

// TestNCAAWBackfill_LookbackRequired covers the --publish-from gate:
// publishing a year without its lookback season inside the audited import
// range is refused before anything is written.
func TestNCAAWBackfill_LookbackRequired(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())

	err := u.BackfillNCAAW(context.Background(), 2027, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW(2027, 2027) must refuse to publish 2027 without the 2026 lookback")
	}
	if !strings.Contains(err.Error(), "lookback") {
		t.Errorf("error = %v, want a lookback-range explanation", err)
	}
	var count int64
	for _, model := range []any{&database.Game{}, &database.TeamSeason{}, &database.TeamWeekResult{}} {
		if err := u.db.Model(model).Count(&count).Error; err != nil {
			t.Fatalf("counting %T: %v", model, err)
		}
		if count != 0 {
			t.Errorf("%T rows = %d, want 0 (refused before any write)", model, count)
		}
	}
}

// TestNCAAWBackfill_IncompleteSummaryFailsClosed covers an unresolved game
// summary failure: the run aborts before publication, the partial scoped raw
// import is retained (retry=1 for the failed game), and nothing is ranked.
func TestNCAAWBackfill_IncompleteSummaryFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.summaryStatus[61201] = http.StatusInternalServerError
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on an unresolved summary failure")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error = %v, want a retry marker mention", err)
	}

	// Partial scoped raw import retained for rerun: the failed game is
	// retry-marked, the healthy games are stored, nothing is published.
	var retryGame database.Game
	if err := u.db.Where("game_id = ? and sport = ?", int64(61201), "ncaaw").
		First(&retryGame).Error; err != nil {
		t.Fatalf("retry-marked game missing: %v", err)
	}
	if retryGame.Retry != 1 {
		t.Errorf("retry flag = %d, want 1", retryGame.Retry)
	}
	if got := ncaawBFCount(t, u.db, "sport = ? and retry = 0", "ncaaw"); got != 5 {
		t.Errorf("healthy imported games = %d, want 5", got)
	}
	if got := len(ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")); got != 0 {
		t.Errorf("ranking rows = %d, want 0", got)
	}
}

// TestNCAAWBackfill_MissingIdentityFailsClosed covers a D-I team whose
// identity backfill failed: the metadata audit refuses to publish.
func TestNCAAWBackfill_MissingIdentityFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.teamStatus[13] = http.StatusInternalServerError
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on missing D-I identity metadata")
	}
	if !strings.Contains(err.Error(), "team_names") {
		t.Errorf("error = %v, want a team_names mention", err)
	}
	if got := len(ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")); got != 0 {
		t.Errorf("ranking rows = %d, want 0", got)
	}
}

// TestNCAAWBackfill_UnclassifiedLikelyD1FailsClosed covers a team that
// appears in games without a D-I conference on the scoreboard but IS listed
// as current D-I: it must not be silently accepted or dropped.
func TestNCAAWBackfill_UnclassifiedLikelyD1FailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.bulkD1 = append(fx.bulkD1, espn.TeamInfo{
		ID: ncaawBFTeamFiller, Name: "Filler", DisplayName: "WBB Team 14",
		Abbreviation: "W14", Location: "WBB", Slug: "wbb-14", IsActive: true,
	})
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on an unclassified likely D-I team")
	}
	if !strings.Contains(err.Error(), "unclassified") {
		t.Errorf("error = %v, want an unclassified-team mention", err)
	}
	if got := len(ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")); got != 0 {
		t.Errorf("ranking rows = %d, want 0", got)
	}
}

// TestNCAAWBackfill_EmptyCalendarFailsClosed covers the empty advertised
// calendar.
func TestNCAAWBackfill_EmptyCalendarFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.seasons[1].calendar = nil // season 2026
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on an empty calendar")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want an empty-calendar mention", err)
	}
}

// TestNCAAWBackfill_WrongYearCalendarFailsClosed covers a calendar carrying
// dates outside the requested season-ending year's window.
func TestNCAAWBackfill_WrongYearCalendarFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.seasons[1].calendar = append(fx.seasons[1].calendar, "2024-11-05T08:00Z") // 2025 season date
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on a wrong-year calendar date")
	}
	if !strings.Contains(err.Error(), "season window") {
		t.Errorf("error = %v, want a season-window mention", err)
	}
}

// TestNCAAWBackfill_UnusableDailyFeedFailsClosed covers a daily walk
// response that cannot be fetched.
func TestNCAAWBackfill_UnusableDailyFeedFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.dayStatus["20260106"] = http.StatusInternalServerError
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on an unusable daily feed")
	}
	if !strings.Contains(err.Error(), "unusable") {
		t.Errorf("error = %v, want an unusable-feed mention", err)
	}
	if got := len(ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")); got != 0 {
		t.Errorf("ranking rows = %d, want 0", got)
	}
}

// TestNCAAWBackfill_WrongYearDailyFeedFailsClosed covers a daily walk
// response that answers with the wrong season year.
func TestNCAAWBackfill_WrongYearDailyFeedFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.dailyYear["20260106"] = 2025
	u := newNCAAWBackfillUpdater(t, fx)

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on a wrong-year daily response")
	}
	if !strings.Contains(err.Error(), "season year 2025") {
		t.Errorf("error = %v, want a wrong-season-year mention", err)
	}
}

// TestNCAAWBackfill_GameDayBeyondCalendarIsStillImported mirrors the live
// 2025-26 finding (122 advertised calendar dates vs 146 game days, verified
// 2026-11-27): a game on an ET day the flat calendar omits must still be
// imported and audited — the daily walk, not the calendar, is the discovery
// authority. The same fixture geometry (a game day missing from the
// advertised calendar) therefore expects success, not failure.
func TestNCAAWBackfill_GameDayBeyondCalendarIsStillImported(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.games = append(fx.games, ncaawBFGame{
		id: 61105, seasonYear: 2026, seasonType: 2, week: 12,
		date: "2026-02-15T19:00Z", day: "20260215",
		home: ncaawBFTeamHuskieDef(), away: ncaawBFTeamGamecockDef(),
		homeScore: 66, awayScore: 64,
	})
	u := newNCAAWBackfillUpdater(t, fx)

	if err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0); err != nil {
		t.Fatalf("BackfillNCAAW: %v", err)
	}
	if got := ncaawBFCount(t, u.db, "game_id = ? and sport = ?", int64(61105), "ncaaw"); got != 1 {
		t.Errorf("game beyond the advertised calendar stored %d times, want 1", got)
	}
	// The week-12 game joins the published regular weeks for 2027's
	// lookback without widening the publication scope.
	if got := len(ncaawBFRankings(t, u.db, "sport = ? and year = ?", "ncaaw", int64(2026))); got != 0 {
		t.Errorf("2026 ranking rows = %d, want 0 (lookback-only)", got)
	}
}

// TestNCAAWBackfill_ScoreMismatchAuditFailsClosed covers the game audit:
// a stored row whose scores disagree with the advertised final blocks the
// publication and leaves previously published ranking rows untouched.
func TestNCAAWBackfill_ScoreMismatchAuditFailsClosed(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.games[1].summaryScores = &[2]int64{83, 69} // scoreboard says 82-70
	u := newNCAAWBackfillUpdater(t, fx)

	// Previously published 2027 rows from an earlier run: the failed audit
	// must not change them.
	published := []database.TeamWeekResult{
		{TeamID: 11, Name: "WBB Team 11", Conf: "Big East", Year: 2027, Week: 5,
			Postseason: 0, Sport: "ncaaw", FinalRank: 2, FinalRaw: 0.5, Fbs: true},
		{TeamID: 12, Name: "WBB Team 12", Conf: "SEC", Year: 2027, Week: 5,
			Postseason: 0, Sport: "ncaaw", FinalRank: 3, FinalRaw: 0.25, Fbs: true},
	}
	if err := u.db.Create(&published).Error; err != nil {
		t.Fatalf("seeding published rows: %v", err)
	}

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on a score mismatch")
	}
	if !strings.Contains(err.Error(), "do not match the advertised final") {
		t.Errorf("error = %v, want an advertised-final mismatch mention", err)
	}
	rows := ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")
	if len(rows) != len(published) {
		t.Fatalf("ranking rows after failed audit = %d, want %d (unchanged)", len(rows), len(published))
	}
	for _, row := range rows {
		if row.FinalRank != published[row.TeamID-11].FinalRank ||
			row.FinalRaw != published[row.TeamID-11].FinalRaw {
			t.Errorf("team %d ranking row changed on audit failure: %+v", row.TeamID, row)
		}
	}
}

// TestNCAAWBackfill_StaleStoredRowFailsClosed covers a stored games row for
// the requested season that the feed no longer advertises.
func TestNCAAWBackfill_StaleStoredRowFailsClosed(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())

	stale := database.Game{
		GameID: 699999, Season: 2027, Week: 5,
		HomeID: ncaawBFTeamHuskies, AwayID: ncaawBFTeamGamecocks,
		HomeScore: 50, AwayScore: 49, Sport: "ncaaw",
		StartTime: time.Date(2026, 12, 20, 19, 0, 0, 0, time.UTC),
	}
	if err := u.db.Create(&stale).Error; err != nil {
		t.Fatalf("seeding the stale row: %v", err)
	}

	err := u.BackfillNCAAW(context.Background(), 2026, 2027, 0)
	if err == nil {
		t.Fatal("BackfillNCAAW must fail closed on a stale stored row")
	}
	if !strings.Contains(err.Error(), "stale or orphan") {
		t.Errorf("error = %v, want a stale-row mention", err)
	}
	if got := len(ncaawBFRankings(t, u.db, "sport = ?", "ncaaw")); got != 0 {
		t.Errorf("ranking rows = %d, want 0", got)
	}
}

// TestNCAAWBackfill_ArgumentValidation covers the run-level argument gates.
func TestNCAAWBackfill_PublicationWriteFailureRestoresOldRows(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())
	old := database.TeamWeekResult{
		TeamID: 11, Sport: sport.WomensBasketball, Year: 2027, Week: 42,
		Name: "prior publication", FinalRank: 1, Fbs: true,
	}
	if err := u.db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := u.db.Exec(`create trigger reject_ranking before insert on team_week_results
		when new.sport = 'ncaaw' begin select raise(abort, 'forced ranking write failure'); end`).Error; err != nil {
		t.Fatal(err)
	}
	err := u.BackfillNCAAW(t.Context(), 2026, 2027, 0)
	if err == nil || !strings.Contains(err.Error(), "forced ranking write failure") {
		t.Fatalf("backfill error = %v, want the injected ranking write failure", err)
	}
	if got := ncaawBFRankings(t, u.db, "sport = ?", sport.WomensBasketball); len(got) != 1 || got[0] != old {
		t.Fatalf("publication rollback left %v, want the original ranking", got)
	}
}

func TestNCAAWBackfill_ArgumentValidation(t *testing.T) {
	u := newNCAAWBackfillUpdater(t, defaultNCAAWBFFixture())
	ctx := context.Background()

	if err := u.BackfillNCAAW(ctx, 2027, 2026, 0); err == nil {
		t.Error("from > to must be refused")
	}
	if err := u.BackfillNCAAW(ctx, 0, 2027, 0); err == nil {
		t.Error("from = 0 must be refused")
	}
	if err := u.BackfillNCAAW(ctx, 2026, 2027, 2028); err == nil {
		t.Error("--publish-from outside the import range must be refused")
	}
	if err := u.BackfillNCAAW(ctx, 2026, 2027, 2025); err == nil {
		t.Error("--publish-from below the import range must be refused")
	}
}
