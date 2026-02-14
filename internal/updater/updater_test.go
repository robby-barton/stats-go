//go:build integration

package updater

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func TestUpdateSingleGame(t *testing.T) {
	u, _ := newTestUpdater(t, nil)

	if err := u.UpdateSingleGame(fixtureGameID1); err != nil {
		t.Fatalf("UpdateSingleGame: %v", err)
	}

	// Verify game row
	var game database.Game
	if err := u.DB.Where("game_id = ?", fixtureGameID1).First(&game).Error; err != nil {
		t.Fatalf("game not found: %v", err)
	}
	if game.HomeScore != 28 || game.AwayScore != 14 {
		t.Errorf("scores = %d-%d, want 28-14", game.HomeScore, game.AwayScore)
	}
	if game.HomeID != 1 || game.AwayID != 2 {
		t.Errorf("teams = %d vs %d, want 1 vs 2", game.HomeID, game.AwayID)
	}
	if game.Season != 2023 || game.Week != 1 {
		t.Errorf("season/week = %d/%d, want 2023/1", game.Season, game.Week)
	}

	// Verify team stats were inserted
	var teamStats []database.TeamGameStats
	if err := u.DB.Where("game_id = ?", fixtureGameID1).Find(&teamStats).Error; err != nil {
		t.Fatalf("team stats query: %v", err)
	}
	if len(teamStats) != 2 {
		t.Errorf("len(teamStats) = %d, want 2", len(teamStats))
	}

	// Verify passing stats were inserted
	var passStats []database.PassingStats
	if err := u.DB.Where("game_id = ?", fixtureGameID1).Find(&passStats).Error; err != nil {
		t.Fatalf("passing stats query: %v", err)
	}
	if len(passStats) == 0 {
		t.Error("expected passing stats, got none")
	}
}

func TestUpdateCurrentWeek(t *testing.T) {
	u, _ := newTestUpdater(t, nil)

	gameIDs, err := u.UpdateCurrentWeek()
	if err != nil {
		t.Fatalf("UpdateCurrentWeek: %v", err)
	}

	// Mock server returns both FBS and FCS schedule with the same fixture.
	// GetCurrentWeekGames deduplicates via combineGames.
	// Fixture has 4 final games (IDs 401001, 401002, 401004, 401005)
	// and 2 in-progress (filtered out).
	if len(gameIDs) != 4 {
		t.Fatalf("len(gameIDs) = %d, want 4", len(gameIDs))
	}

	idSet := map[int64]bool{}
	for _, id := range gameIDs {
		idSet[id] = true
	}
	for _, expected := range []int64{fixtureGameID1, fixtureGameID2, fixtureGameID4, fixtureGameID5} {
		if !idSet[expected] {
			t.Errorf("expected game %d in results", expected)
		}
	}

	// Verify games in DB
	var count int64
	u.DB.Model(&database.Game{}).Count(&count)
	if count != 4 {
		t.Errorf("game count = %d, want 4", count)
	}

	// Re-run should be a no-op (checkGames filters already-stored games with matching scores)
	gameIDs2, err := u.UpdateCurrentWeek()
	if err != nil {
		t.Fatalf("UpdateCurrentWeek re-run: %v", err)
	}
	if len(gameIDs2) != 0 {
		t.Errorf("re-run returned %d games, want 0 (no-op)", len(gameIDs2))
	}
}

func TestUpdateCurrentWeek_ScoreChange(t *testing.T) {
	// First run: normal scores
	u, _ := newTestUpdater(t, nil)

	_, err := u.UpdateCurrentWeek()
	if err != nil {
		t.Fatalf("initial UpdateCurrentWeek: %v", err)
	}

	// Verify initial score
	var game database.Game
	u.DB.Where("game_id = ?", fixtureGameID1).First(&game)
	if game.HomeScore != 28 {
		t.Fatalf("initial home score = %d, want 28", game.HomeScore)
	}

	// Second run: override game 401001 scores to 31-14
	// We need a new server with score override, and re-override the URLs
	overrides := map[int64][2]int64{
		fixtureGameID1: {31, 14},
	}
	ts2 := setupTestServer(t, overrides)
	restore := newTestURLs(t, ts2.URL)
	defer restore()

	gameIDs, err := u.UpdateCurrentWeek()
	if err != nil {
		t.Fatalf("UpdateCurrentWeek with score change: %v", err)
	}

	// Only game 401001 should be re-fetched (score changed)
	found := false
	for _, id := range gameIDs {
		if id == fixtureGameID1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected game %d to be re-fetched due to score change", fixtureGameID1)
	}

	// Verify updated score in DB
	u.DB.Where("game_id = ?", fixtureGameID1).First(&game)
	if game.HomeScore != 31 {
		t.Errorf("updated home score = %d, want 31", game.HomeScore)
	}
}

func TestUpdateTeamInfo(t *testing.T) {
	u, _ := newTestUpdater(t, nil)

	count, err := u.UpdateTeamInfo()
	if err != nil {
		t.Fatalf("UpdateTeamInfo: %v", err)
	}
	if count != 4 {
		t.Errorf("team count = %d, want 4", count)
	}

	// Verify team names in DB
	var teams []database.TeamName
	if err := u.DB.Find(&teams).Error; err != nil {
		t.Fatalf("query teams: %v", err)
	}
	if len(teams) != 4 {
		t.Errorf("len(teams) = %d, want 4", len(teams))
	}

	// Verify name parsing (display name minus nickname = school name)
	teamMap := map[int64]string{}
	for _, team := range teams {
		teamMap[team.TeamID] = team.Name
	}
	if teamMap[1] != "Alpha" {
		t.Errorf("team 1 name = %q, want %q", teamMap[1], "Alpha")
	}
}

func TestUpdateTeamSeasons(t *testing.T) {
	u, _ := newTestUpdater(t, nil)

	count, err := u.UpdateTeamSeasons(true)
	if err != nil {
		t.Fatalf("UpdateTeamSeasons: %v", err)
	}

	// The mock conference map has 2 FBS conferences (conf IDs 100, 200).
	// TeamConferencesByYear iterates weeks x groups and collects team->conf mappings.
	// All fixture games use teams 1-6 with conf IDs 100, 200.
	if count == 0 {
		t.Error("expected at least some team seasons inserted")
	}

	var seasons []database.TeamSeason
	if err := u.DB.Find(&seasons).Error; err != nil {
		t.Fatalf("query seasons: %v", err)
	}
	if len(seasons) == 0 {
		t.Error("no team_season rows found")
	}

	// Verify FBS assignment
	for _, s := range seasons {
		if s.TeamID >= 1 && s.TeamID <= 4 {
			if s.FBS != 1 {
				t.Errorf("team %d FBS = %d, want 1", s.TeamID, s.FBS)
			}
		}
	}
}

func TestRankingForWeek(t *testing.T) {
	u, _ := newTestUpdater(t, nil)

	// Seed teams, seasons, and games directly
	seedTeamsAndSeasons(t, u.DB)
	seedGames(t, u.DB)

	// Run ranking
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}

	// Verify TeamWeekResult rows exist
	var results []database.TeamWeekResult
	if err := u.DB.Find(&results).Error; err != nil {
		t.Fatalf("query results: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no ranking results found")
	}

	// Check that FBS teams got ranked
	fbsResults := 0
	for _, r := range results {
		if r.Fbs {
			fbsResults++
			if r.FinalRank == 0 {
				t.Errorf("team %d has FinalRank 0", r.TeamID)
			}
		}
	}
	if fbsResults != 4 {
		t.Errorf("FBS results = %d, want 4", fbsResults)
	}
}

func TestUpdateRecentJSON(t *testing.T) {
	u, cw := newTestUpdater(t, nil)

	// Seed the full pipeline: teams, seasons, games, rankings
	seedTeamsAndSeasons(t, u.DB)
	seedGames(t, u.DB)

	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}

	// Run JSON export
	if err := u.UpdateRecentJSON(); err != nil {
		t.Fatalf("UpdateRecentJSON: %v", err)
	}

	// Verify expected files were written
	expectedFiles := []string{
		"cfb/availRanks.json",
		"cfb/gameCount.json",
		"cfb/latest.json",
	}
	for _, f := range expectedFiles {
		if !cw.hasFile(f) {
			t.Errorf("expected file %q not written", f)
		}
	}

	// Verify ranking files were written (pattern: ranking/YEAR/DIVISION/WEEK.json)
	if cw.fileCount() < len(expectedFiles) {
		t.Errorf("total files = %d, want at least %d", cw.fileCount(), len(expectedFiles))
	}

	// Verify PurgeCache was called
	if cw.purgeCount != 1 {
		t.Errorf("PurgeCache count = %d, want 1", cw.purgeCount)
	}
}

// newTestURLs is a helper that overrides ESPN URLs for a given test server base URL.
func newTestURLs(t *testing.T, serverURL string) func() {
	t.Helper()
	return espn.SetTestURLs(
		serverURL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
	)
}
