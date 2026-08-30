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
)

// TestSiteScoreboardCalendarDecode pins the shape of the object-shaped
// scoreboard calendar (?dates=2026 capture) and the week-entry-to-week-number
// mapping, including the postseason handling (Bowls = week 1, CFP = week 999).
func TestSiteScoreboardCalendarDecode(t *testing.T) {
	var res SiteScoreboardESPN
	if err := json.Unmarshal([]byte(siteScoreboardCalendarFixture), &res); err != nil {
		t.Fatalf("unmarshal calendar fixture: %v", err)
	}
	if res.Leagues[0].Season.Year != 2026 {
		t.Errorf("season year = %d, want 2026", res.Leagues[0].Season.Year)
	}

	regular := res.calendarType(Regular)
	if regular == nil {
		t.Fatal("calendar fixture missing Regular Season entry")
	}
	if regular.Label != "Regular Season" {
		t.Errorf("regular label = %q", regular.Label)
	}
	if len(regular.Entries) != 15 {
		t.Fatalf("regular entries = %d, want 15", len(regular.Entries))
	}

	// Week entries are numbered 1..N in listed order — entry i carries week
	// number i+1.
	for i, week := range regular.Entries {
		if got, want := int64(week.Value), int64(i+1); got != want {
			t.Errorf("regular entry %d value = %d, want %d", i, got, want)
		}
		if !strings.HasPrefix(week.Label, "Week ") {
			t.Errorf("regular entry %d label = %q, want a week label", i, week.Label)
		}
	}
	if regular.Entries[0].StartDate != "2026-08-22T07:00Z" {
		t.Errorf("week 1 start = %q", regular.Entries[0].StartDate)
	}

	postseason := res.calendarType(Postseason)
	if postseason == nil {
		t.Fatal("calendar fixture missing Postseason entry")
	}
	if postseason.StartDate != "2026-12-13T08:00Z" {
		t.Errorf("postseason start = %q, want 2026-12-13T08:00Z", postseason.StartDate)
	}
	// Postseason entries are NOT week numbers of the regular season: Bowls is
	// week 1 of the postseason phase, the CFP a special 999.
	if len(postseason.Entries) != 2 ||
		int64(postseason.Entries[0].Value) != 1 || postseason.Entries[0].Label != "Bowls" ||
		int64(postseason.Entries[1].Value) != 999 || postseason.Entries[1].Label != "CFP" {
		t.Errorf("postseason entries = %+v, want Bowls(1) + CFP(999)", postseason.Entries)
	}

	// Off-season (value 4) must not be matched as either season type.
	if res.calendarType(SeasonType(4)) == nil {
		t.Error("calendar fixture missing Off Season entry")
	}

	// A calendar-less payload (plain scoreboard, JSON null calendar) has no
	// season-type entries at all.
	var plain SiteScoreboardESPN
	if err := json.Unmarshal([]byte(siteScoreboardFixture), &plain); err != nil {
		t.Fatalf("unmarshal plain fixture: %v", err)
	}
	if plain.calendarType(Regular) != nil || plain.calendarType(Postseason) != nil {
		t.Error("plain scoreboard fixture must not yield calendar entries")
	}
}

// TestFootballSeasonMetadata_SiteAPI exercises DefaultSeason, GetWeeksInSeason
// and HasPostseasonStarted end-to-end against the captured fixtures.
func TestFootballSeasonMetadata_SiteAPI(t *testing.T) {
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

	weeks, err := client.GetWeeksInSeason(context.Background(), 2026)
	if err != nil {
		t.Fatalf("GetWeeksInSeason: %v", err)
	}
	if weeks != 15 {
		t.Errorf("weeks = %d, want 15", weeks)
	}

	// Postseason starts 2026-12-13T08:00Z in the fixture.
	before := time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
	started, err := client.HasPostseasonStarted(context.Background(), 2026, before)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if started {
		t.Error("postseason should not have started before its start date")
	}

	after := time.Date(2026, 12, 14, 0, 0, 0, 0, time.UTC)
	started, err = client.HasPostseasonStarted(context.Background(), 2026, after)
	if err != nil {
		t.Fatalf("HasPostseasonStarted: %v", err)
	}
	if !started {
		t.Error("postseason should have started after its start date")
	}
}

// TestFootballSeasonMetadata_MissingCalendar verifies the error paths when the
// scoreboard payload carries no calendar (e.g. ESPN drops the dates parameter
// behavior).
func TestFootballSeasonMetadata_MissingCalendar(t *testing.T) {
	var mu sync.Mutex
	var requests int
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"leagues": [{"season": {"year": 2026}}]}`))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := newTestClient()
	t.Cleanup(client.SetURLs("", "", ts.URL, ""))

	if _, err := client.GetWeeksInSeason(context.Background(), 2026); err == nil {
		t.Error("expected error for missing calendar, got nil")
	}
	if _, err := client.HasPostseasonStarted(context.Background(), 2026, time.Now()); err == nil {
		t.Error("expected error for missing calendar, got nil")
	}
}

// TestFootballConferenceMap_SiteAPI verifies ConferenceMap against the
// captured scoreboard/conferences fixtures for all four football groups.
func TestFootballConferenceMap_SiteAPI(t *testing.T) {
	ts := setupTestServer(t)
	client := newTestClient()
	overrideURLs(t, client.Client, ts.URL)

	res, err := client.ConferenceMap(context.Background())
	if err != nil {
		t.Fatalf("ConferenceMap: %v", err)
	}

	fbs := res.Conferences[FBS]
	if len(fbs) != 11 {
		t.Fatalf("FBS conferences = %d, want 11", len(fbs))
	}
	if fbs[8] != "SEC" || fbs[1] != "ACC" || fbs[4] != "Big 12" || fbs[37] != "Sun Belt" {
		t.Errorf("FBS short names = %v", fbs)
	}
	if _, ok := fbs[80]; ok {
		t.Error("FBS root entry must not appear as a conference")
	}

	fcs := res.Conferences[FCS]
	if len(fcs) != 14 {
		t.Fatalf("FCS conferences = %d, want 14", len(fcs))
	}
	if fcs[21] != "MVFC" || fcs[22] != "Ivy" || fcs[20] != "Big Sky" {
		t.Errorf("FCS short names = %v", fcs)
	}

	if got, want := len(res.SubGroups[DII]), 17; got != want {
		t.Errorf("DII sub-groups = %d, want %d", got, want)
	}
	if got, want := len(res.SubGroups[DIII]), 30; got != want {
		t.Errorf("DIII sub-groups = %d, want %d", got, want)
	}
	for _, id := range []int64{104, 187, 139} {
		if !containsID(res.SubGroups[DII], id) {
			t.Errorf("DII sub-groups missing %d", id)
		}
	}
}

// TestBasketballConferenceMap_SiteAPI verifies the D1 conference map against
// the captured basketball conferences fixture.
func TestBasketballConferenceMap_SiteAPI(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := newBasketballTestClient(t, ts.URL)

	res, err := client.ConferenceMap(context.Background())
	if err != nil {
		t.Fatalf("ConferenceMap: %v", err)
	}

	d1 := res.Conferences[D1Basketball]
	if len(d1) != 32 {
		t.Fatalf("D1 conferences = %d, want 32", len(d1))
	}
	if d1[4] != "Big East" || d1[23] != "SEC" || d1[21] != "Pac-12" || d1[49] != "Summit" {
		t.Errorf("D1 short names = %v", d1)
	}
	if _, ok := d1[50]; ok {
		t.Error("D1 root entry must not appear as a conference")
	}
}

// TestBasketball_GetSeasonDatesForYear verifies the dates-parameterized
// scoreboard fetch used for current-season date navigation.
func TestBasketball_GetSeasonDatesForYear(t *testing.T) {
	ts := setupBasketballTestServer(t)
	client := newBasketballTestClient(t, ts.URL)

	dates, err := client.GetSeasonDatesForYear(context.Background(), 2024)
	if err != nil {
		t.Fatalf("GetSeasonDatesForYear: %v", err)
	}
	if len(dates) != 2 || dates[0] != "2024-01-06T08:00Z" || dates[1] != "2024-01-13T08:00Z" {
		t.Errorf("dates = %v, want the fixture's two game dates", dates)
	}
}

// TestSiteBasketballScoreboardFixtureDecode pins the live basketball
// scoreboard shape: the flat date-string calendar decodes to nil inside
// SiteScoreboardLeague, competitors carry team.conferenceId, and a real
// STATUS_SCHEDULED event (separate capture, 2026-27 season preview) is
// dropped by finalGames.
func TestSiteBasketballScoreboardFixtureDecode(t *testing.T) {
	var res SiteScoreboardESPN
	if err := json.Unmarshal([]byte(siteBasketballScoreboardFixture), &res); err != nil {
		t.Fatalf("unmarshal basketball scoreboard fixture: %v", err)
	}
	if err := res.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// The flat basketball calendar must decode to nil (not an error).
	if res.Leagues[0].Calendar != nil {
		t.Errorf("flat calendar decoded to %+v, want nil", res.Leagues[0].Calendar)
	}

	// The captured day (2026-02-14, groups=50) has three finals with
	// conference IDs on every competitor.
	for _, ev := range res.Events {
		for _, comp := range ev.Competitions {
			for _, c := range comp.Competitors {
				if c.Team.ConferenceID == 0 {
					t.Errorf("event %s competitor %d has no conferenceId", ev.ID, c.ID)
				}
			}
		}
	}

	games, err := res.finalGames()
	if err != nil {
		t.Fatalf("finalGames: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("len(games) = %d, want 3", len(games))
	}
	// Texas Tech 78 at Arizona 75 (OT) — scores must survive the mapping.
	if games[0].ID != 401827679 {
		t.Errorf("game[0].ID = %d, want 401827679", games[0].ID)
	}
	competitors := games[0].Competitions[0].Competitors
	if competitors[0].Team.ID != 12 || competitors[0].Score != 75 ||
		competitors[1].Team.ID != 2641 || competitors[1].Score != 78 {
		t.Errorf("competitors = %+v", competitors)
	}

	// A real scheduled event (separate capture) must be filtered out.
	var sched SiteScoreboardESPN
	if err := json.Unmarshal(
		[]byte(`{"leagues":[{"season":{"year":2027}}],"events":[`+siteBasketballScheduledEventJSON+`]}`),
		&sched); err != nil {
		t.Fatalf("unmarshal scheduled event: %v", err)
	}
	schedGames, err := sched.finalGames()
	if err != nil {
		t.Fatalf("finalGames(scheduled): %v", err)
	}
	if len(schedGames) != 0 {
		t.Errorf("scheduled event survived finalGames: %+v", schedGames)
	}
}

// TestSiteBasketballPlainFixtureDecode documents the plain (no dates)
// basketball scoreboard: the flat calendar IS present without a dates
// parameter (unlike football), and ESPN had already flipped the current
// season to 2027 at capture time (2026-08-30).
func TestSiteBasketballPlainFixtureDecode(t *testing.T) {
	var sb ScoreboardESPN
	if err := json.Unmarshal([]byte(siteBasketballPlainFixture), &sb); err != nil {
		t.Fatalf("unmarshal plain fixture into ScoreboardESPN: %v", err)
	}
	if err := sb.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if sb.Leagues[0].Season.Year != 2027 {
		t.Errorf("season year = %d, want 2027 (ESPN flipped to the 2026-27 preview)", sb.Leagues[0].Season.Year)
	}
	if len(sb.Leagues[0].Calendar) != 2 {
		t.Errorf("calendar entries = %d, want 2 (trimmed)", len(sb.Leagues[0].Calendar))
	}

	// The same payload decodes into SiteScoreboardESPN without error (the
	// flat calendar is ignored there).
	var site SiteScoreboardESPN
	if err := json.Unmarshal([]byte(siteBasketballPlainFixture), &site); err != nil {
		t.Fatalf("unmarshal plain fixture into SiteScoreboardESPN: %v", err)
	}
	if site.Leagues[0].Calendar != nil {
		t.Errorf("flat calendar decoded to %+v, want nil", site.Leagues[0].Calendar)
	}
}

// TestSiteBasketballSummaryFixturesDecode verifies both captured basketball
// summary responses (current-season 2025-26 and a historical 2024-25 game)
// decode through GameInfoESPN with the fields the game parser consumes.
func TestSiteBasketballSummaryFixturesDecode(t *testing.T) {
	var cur GameInfoESPN
	if err := json.Unmarshal([]byte(siteBasketballSummaryFixture), &cur); err != nil {
		t.Fatalf("unmarshal summary fixture: %v", err)
	}
	if err := cur.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cur.GamePackage.Header.ID != 401827679 || cur.GamePackage.Header.Week != 15 {
		t.Errorf("header = %+v", cur.GamePackage.Header)
	}
	if cur.GamePackage.Header.Competitions[0].Date != "2026-02-14T23:30Z" {
		t.Errorf("date = %q", cur.GamePackage.Header.Competitions[0].Date)
	}
	if len(cur.GamePackage.Boxscore.Teams) != 2 || len(cur.GamePackage.Boxscore.Players) != 2 {
		t.Errorf("boxscore = %+v", cur.GamePackage.Boxscore)
	}

	var hist GameInfoESPN
	if err := json.Unmarshal([]byte(siteBasketballSummaryHistoricalFixture), &hist); err != nil {
		t.Fatalf("unmarshal historical summary fixture: %v", err)
	}
	if err := hist.validate(); err != nil {
		t.Fatalf("validate historical: %v", err)
	}
	// header.season.year must be the game's own season (2024-25 = 2025),
	// not the current season — historical backfills depend on it.
	if hist.GamePackage.Header.Season.Year != 2025 || hist.GamePackage.Header.Week != 11 {
		t.Errorf("historical header = %+v", hist.GamePackage.Header)
	}
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
