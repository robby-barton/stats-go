package updater

import (
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ---------------------------------------------------------------------------
// Women's basketball weekly/Final snapshot fixtures
// ---------------------------------------------------------------------------
//
// These tests exercise the ranking publication path only (seeded games, no
// ESPN fetches): the stable women's Final key, its atomic replacement of
// stale Final variants, the retained weekly backfill seam, and the existing
// regular-window label convention.

const wbSeason int64 = 2027

// seedWomensTeamsAndSeasons seeds three D-I women's teams for the 2027
// season. The team IDs overlap the men's fixture deliberately so every
// assertion also proves sport isolation.
func seedWomensTeamsAndSeasons(t *testing.T, u *Updater) {
	t.Helper()

	teamNames := []database.TeamName{
		{TeamID: 11, Name: "WAlpha", DisplayName: "WBall Alpha", Abbreviation: "WBA",
			Location: "WBall Alpha", Slug: "wball-alpha", IsActive: true, Sport: sport.WomensBasketball},
		{TeamID: 12, Name: "WBeta", DisplayName: "WBall Beta", Abbreviation: "WBB",
			Location: "WBall Beta", Slug: "wball-beta", IsActive: true, Sport: sport.WomensBasketball},
		{TeamID: 13, Name: "WGamma", DisplayName: "WBall Gamma", Abbreviation: "WBG",
			Location: "WBall Gamma", Slug: "wball-gamma", IsActive: true, Sport: sport.WomensBasketball},
	}
	if err := u.db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed women's team_names: %v", err)
	}
	teamSeasons := []database.TeamSeason{
		{TeamID: 11, Year: wbSeason, FBS: 1, Conf: "Big East", Sport: sport.WomensBasketball},
		{TeamID: 12, Year: wbSeason, FBS: 1, Conf: "Big East", Sport: sport.WomensBasketball},
		{TeamID: 13, Year: wbSeason, FBS: 1, Conf: "SEC", Sport: sport.WomensBasketball},
	}
	if err := u.db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed women's team_seasons: %v", err)
	}
}

// seedWomensRegularGames seeds two regular weeks: week 10 (11 beats 12) and
// week 11 (13 beats 11).
func seedWomensRegularGames(t *testing.T, u *Updater) {
	t.Helper()

	games := []database.Game{
		{
			GameID: 601001, Season: wbSeason, Week: 10,
			HomeID: 11, HomeScore: 82, AwayID: 12, AwayScore: 70,
			Sport: sport.WomensBasketball, StartTime: time.Date(2027, 1, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			GameID: 601002, Season: wbSeason, Week: 11,
			HomeID: 13, HomeScore: 70, AwayID: 11, AwayScore: 65,
			Sport: sport.WomensBasketball, StartTime: time.Date(2027, 1, 13, 0, 0, 0, 0, time.UTC),
		},
	}
	if err := u.db.Create(&games).Error; err != nil {
		t.Fatalf("seed women's regular games: %v", err)
	}
}

// seedWomensSemifinal seeds one numbered postseason game (11 beats 12).
func seedWomensSemifinal(t *testing.T, u *Updater) {
	t.Helper()

	game := database.Game{
		GameID: 601003, Season: wbSeason, Week: 4, Postseason: 1,
		HomeID: 11, HomeScore: 75, AwayID: 12, AwayScore: 74,
		Sport: sport.WomensBasketball, StartTime: time.Date(2027, 4, 2, 0, 0, 0, 0, time.UTC),
	}
	if err := u.db.Create(&game).Error; err != nil {
		t.Fatalf("seed women's semifinal: %v", err)
	}
}

// seedWomensChampionship seeds the championship in the observed ESPN shape:
// postseason, with header.week omitted so the stored week is 0 (13 beats 11).
func seedWomensChampionship(t *testing.T, u *Updater) {
	t.Helper()

	game := database.Game{
		GameID: 601004, Season: wbSeason, Week: 0, Postseason: 1,
		HomeID: 13, HomeScore: 78, AwayID: 11, AwayScore: 77,
		Sport: sport.WomensBasketball, StartTime: time.Date(2027, 4, 5, 23, 0, 0, 0, time.UTC),
	}
	if err := u.db.Create(&game).Error; err != nil {
		t.Fatalf("seed women's championship: %v", err)
	}
}

func womensRankingRows(t *testing.T, u *Updater) []database.TeamWeekResult {
	t.Helper()

	var rows []database.TeamWeekResult
	if err := u.db.Where("sport = ?", sport.WomensBasketball).Find(&rows).Error; err != nil {
		t.Fatalf("query women's team_week_results: %v", err)
	}
	return rows
}

func mensRankingRows(t *testing.T, u *Updater) []database.TeamWeekResult {
	t.Helper()

	var rows []database.TeamWeekResult
	if err := u.db.Where("sport = ?", sport.Basketball).Find(&rows).Error; err != nil {
		t.Fatalf("query men's team_week_results: %v", err)
	}
	return rows
}

// TestWomensFinal_StableKeyAcrossPostseasonUpdates publishes the women's
// Final twice — once with a numbered semifinal as the latest game, once after
// the championship whose ESPN week is missing — and verifies both updates
// land on the one stable (postseason=1, week=0) key per season instead of
// accumulating Final variants.
func TestWomensFinal_StableKeyAcrossPostseasonUpdates(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	seedWomensTeamsAndSeasons(t, u)
	seedWomensRegularGames(t, u)
	seedWomensSemifinal(t, u)

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("first Final publication: %v", err)
	}
	first := womensRankingRows(t, u)
	if len(first) != 3 {
		t.Fatalf("first Final rows = %d, want 3", len(first))
	}
	for _, r := range first {
		if r.Postseason != 1 || r.Week != 0 {
			t.Errorf("team %d Final key = (week %d, postseason %d), want (0, 1)",
				r.TeamID, r.Week, r.Postseason)
		}
		if r.Year != wbSeason || r.Sport != sport.WomensBasketball {
			t.Errorf("team %d Final identity = (%d, %s)", r.TeamID, r.Year, r.Sport)
		}
	}

	// The championship arrives with a missing week.
	seedWomensChampionship(t, u)
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("second Final publication: %v", err)
	}

	second := womensRankingRows(t, u)
	if len(second) != 3 {
		t.Fatalf("second Final rows = %d, want exactly 3 (no accumulated variants)", len(second))
	}
	byID := map[int64]database.TeamWeekResult{}
	for _, r := range second {
		if r.Postseason != 1 || r.Week != 0 {
			t.Errorf("team %d Final key = (week %d, postseason %d), want (0, 1)",
				r.TeamID, r.Week, r.Postseason)
		}
		byID[r.TeamID] = r
	}
	// The second publication computed through the championship: team 11's
	// record moved from 2-1 to 2-2 and team 13's from 1-0 to 2-0.
	if byID[11].Wins != 2 || byID[11].Losses != 2 {
		t.Errorf("team 11 Final record = %d-%d, want 2-2", byID[11].Wins, byID[11].Losses)
	}
	if byID[13].Wins != 2 || byID[13].Losses != 0 {
		t.Errorf("team 13 Final record = %d-%d, want 2-0", byID[13].Wins, byID[13].Losses)
	}
	if byID[12].Losses != 2 {
		t.Errorf("team 12 Final record = %d-%d, want 0-2", byID[12].Wins, byID[12].Losses)
	}
}

// TestWomensFinal_ReplacesLegacyStaleVariants seeds Final rows left behind by
// older week-numbered variants of the contract plus a regular archive row and
// men's rows, publishes the women's Final, and verifies the stale variants
// are replaced atomically while the regular row and the other sport's rows
// survive untouched.
func TestWomensFinal_ReplacesLegacyStaleVariants(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	seedWomensTeamsAndSeasons(t, u)
	seedWomensRegularGames(t, u)
	seedWomensSemifinal(t, u)
	seedWomensChampionship(t, u)

	legacy := []database.TeamWeekResult{
		{TeamID: 11, Year: wbSeason, Week: 5, Postseason: 1, Sport: sport.WomensBasketball,
			FinalRank: 7, FinalRaw: 9.9},
		{TeamID: 12, Year: wbSeason, Week: 2, Postseason: 1, Sport: sport.WomensBasketball,
			FinalRank: 8, FinalRaw: 8.8},
		// The season's regular archive must survive the Final publication.
		{TeamID: 11, Year: wbSeason, Week: 12, Postseason: 0, Sport: sport.WomensBasketball,
			FinalRank: 1, FinalRaw: 1.5},
		// Another sport's Final and regular rows are out of scope entirely.
		{TeamID: 11, Year: 2024, Week: 0, Postseason: 1, Sport: sport.Basketball,
			FinalRank: 3, FinalRaw: 3.3},
		{TeamID: 11, Year: 2024, Week: 5, Postseason: 0, Sport: sport.Basketball,
			FinalRank: 2, FinalRaw: 2.2},
	}
	if err := u.db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("Final publication: %v", err)
	}

	womens := womensRankingRows(t, u)
	var finals, regular []database.TeamWeekResult
	for _, r := range womens {
		if r.Postseason == 1 {
			finals = append(finals, r)
		} else {
			regular = append(regular, r)
		}
	}
	if len(finals) != 3 {
		t.Fatalf("women's Final rows = %d, want 3", len(finals))
	}
	for _, r := range finals {
		if r.Week != 0 {
			t.Errorf("team %d Final week = %d, want 0 (stale variant survived)", r.TeamID, r.Week)
		}
		if r.FinalRank < 1 || r.FinalRank > 3 {
			t.Errorf("team %d Final rank = %d, want a freshly computed 1-3", r.TeamID, r.FinalRank)
		}
	}
	if len(regular) != 1 || regular[0].TeamID != 11 || regular[0].Week != 12 ||
		regular[0].FinalRank != 1 || regular[0].FinalRaw != 1.5 {
		t.Errorf("women's regular archive rows = %+v, want the untouched week-12 row", regular)
	}

	mens := mensRankingRows(t, u)
	if len(mens) != 2 {
		t.Fatalf("men's rows = %d, want 2 untouched", len(mens))
	}
	for _, r := range mens {
		if r.Postseason == 1 && (r.FinalRank != 3 || r.FinalRaw != 3.3) {
			t.Errorf("men's Final row changed: %+v", r)
		}
		if r.Postseason == 0 && (r.FinalRank != 2 || r.FinalRaw != 2.2) {
			t.Errorf("men's regular row changed: %+v", r)
		}
	}
}

// TestWomensRegularPublicationKeepsStaleFinal pins the delete gating: a
// publication whose batch carries no women's Final rows must not delete
// anything, so a stale Final variant survives until a Final is actually
// published for its season.
func TestWomensRegularPublicationKeepsStaleFinal(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	seedWomensTeamsAndSeasons(t, u)

	stale := database.TeamWeekResult{
		TeamID: 11, Year: wbSeason, Week: 3, Postseason: 1, Sport: sport.WomensBasketball,
		FinalRank: 9, FinalRaw: 7.7,
	}
	if err := u.db.Create(&stale).Error; err != nil {
		t.Fatalf("seed stale Final variant: %v", err)
	}

	regular := []database.TeamWeekResult{
		{TeamID: 11, Year: wbSeason, Week: 12, Postseason: 0, Sport: sport.WomensBasketball,
			FinalRank: 1, FinalRaw: 1.1},
		{TeamID: 12, Year: wbSeason, Week: 12, Postseason: 0, Sport: sport.WomensBasketball,
			FinalRank: 2, FinalRaw: 1.05},
	}
	if err := u.insertRankingsToDB(regular); err != nil {
		t.Fatalf("regular publication: %v", err)
	}

	rows := womensRankingRows(t, u)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want the 2 regular rows plus the stale Final variant", len(rows))
	}
	var staleSurvived bool
	for _, r := range rows {
		if r.Postseason == 1 && r.Week == 3 && r.FinalRaw == 7.7 {
			staleSurvived = true
		}
	}
	if !staleSurvived {
		t.Error("stale Final variant was deleted by a regular-only publication")
	}
}

// TestWomensRegularLatestUsesESPNWeek verifies the latest regular snapshot
// updates the week of the latest stored game and publishes no Final rows.
func TestWomensRegularLatestUsesESPNWeek(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	seedWomensTeamsAndSeasons(t, u)
	seedWomensRegularGames(t, u)

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("regular latest publication: %v", err)
	}

	rows := womensRankingRows(t, u)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for _, r := range rows {
		if r.Postseason != 0 || r.Week != 11 {
			t.Errorf("team %d row = (week %d, postseason %d), want (11, 0)",
				r.TeamID, r.Week, r.Postseason)
		}
	}
}

// TestWomensBackfillSeam exercises the retained integration seam the strict
// backfill uses — u.regularSeasonWeeks, u.rankingForWeek(year, week, nil),
// and u.insertRankingsToDB — against women's data with a missing-week
// championship stored: the enumerated regular weeks exclude postseason games,
// every explicit week ranks under its own label, the week-0 slot resolves the
// stable Final, and one atomic publication replaces a seeded stale variant.
func TestWomensBackfillSeam(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	seedWomensTeamsAndSeasons(t, u)
	seedWomensRegularGames(t, u)
	seedWomensSemifinal(t, u)
	seedWomensChampionship(t, u)

	// A stale Final variant left by an older publication.
	stale := database.TeamWeekResult{
		TeamID: 11, Year: wbSeason, Week: 6, Postseason: 1, Sport: sport.WomensBasketball,
		FinalRank: 5, FinalRaw: 6.6,
	}
	if err := u.db.Create(&stale).Error; err != nil {
		t.Fatalf("seed stale Final variant: %v", err)
	}

	weeks, err := u.regularSeasonWeeks(wbSeason)
	if err != nil {
		t.Fatalf("regularSeasonWeeks: %v", err)
	}
	if len(weeks) != 2 || weeks[0] != 10 || weeks[1] != 11 {
		t.Fatalf("regularSeasonWeeks = %v, want [10 11] (postseason games excluded)", weeks)
	}

	var batch []database.TeamWeekResult
	for _, week := range weeks {
		rows, err := u.rankingForWeek(wbSeason, week, nil)
		if err != nil {
			t.Fatalf("rankingForWeek(%d, %d, nil): %v", wbSeason, week, err)
		}
		if len(rows) != 3 {
			t.Fatalf("week %d rows = %d, want 3", week, len(rows))
		}
		for _, r := range rows {
			if r.Week != week || r.Postseason != 0 {
				t.Errorf("week %d row = (week %d, postseason %d), want (%d, 0)",
					week, r.Week, r.Postseason, week)
			}
		}
		batch = append(batch, rows...)
	}

	finalRows, err := u.rankingForWeek(wbSeason, 0, nil)
	if err != nil {
		t.Fatalf("rankingForWeek Final slot: %v", err)
	}
	if len(finalRows) != 3 {
		t.Fatalf("Final rows = %d, want 3", len(finalRows))
	}
	for _, r := range finalRows {
		if r.Week != 0 || r.Postseason != 1 {
			t.Errorf("Final row = (week %d, postseason %d), want (0, 1)", r.Week, r.Postseason)
		}
	}
	batch = append(batch, finalRows...)

	if err := u.insertRankingsToDB(batch); err != nil {
		t.Fatalf("insertRankingsToDB: %v", err)
	}

	rows := womensRankingRows(t, u)
	// 2 regular weeks x 3 teams + 3 Final rows; the stale variant is gone.
	if len(rows) != 9 {
		t.Fatalf("persisted rows = %d, want 9", len(rows))
	}
	for _, r := range rows {
		if r.Postseason == 1 && r.Week != 0 {
			t.Errorf("team %d Final week = %d, want 0 (stale variant replaced)", r.TeamID, r.Week)
		}
	}
}
