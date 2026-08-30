package updater

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/sport"
)

func TestUpdateSingleGame(t *testing.T) {
	u := newTestUpdater(t)

	if err := u.UpdateSingleGame(context.Background(), fixtureGameID1); err != nil {
		t.Fatalf("UpdateSingleGame: %v", err)
	}

	// Verify game row
	var game database.Game
	if err := u.db.Where("game_id = ?", fixtureGameID1).First(&game).Error; err != nil {
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
	if err := u.db.Where("game_id = ?", fixtureGameID1).Find(&teamStats).Error; err != nil {
		t.Fatalf("team stats query: %v", err)
	}
	if len(teamStats) != 2 {
		t.Errorf("len(teamStats) = %d, want 2", len(teamStats))
	}

	// Verify passing stats were inserted
	var passStats []database.PassingStats
	if err := u.db.Where("game_id = ?", fixtureGameID1).Find(&passStats).Error; err != nil {
		t.Fatalf("passing stats query: %v", err)
	}
	if len(passStats) == 0 {
		t.Error("expected passing stats, got none")
	}
}

func TestUpdateCurrentWeek(t *testing.T) {
	u := newTestUpdater(t)

	result, err := u.UpdateCurrentWeek(context.Background())
	if err != nil {
		t.Fatalf("UpdateCurrentWeek: %v", err)
	}

	// Mock server returns both FBS and FCS schedule with the same fixture.
	// GetCurrentWeekGames deduplicates via combineGames.
	// Fixture has 4 final games (IDs 401001, 401002, 401004, 401005)
	// and 2 in-progress (filtered out).
	if len(result.Processed) != 4 {
		t.Fatalf("len(Processed) = %d, want 4", len(result.Processed))
	}

	idSet := map[int64]bool{}
	for _, id := range result.Processed {
		idSet[id] = true
	}
	for _, expected := range []int64{fixtureGameID1, fixtureGameID2, fixtureGameID4, fixtureGameID5} {
		if !idSet[expected] {
			t.Errorf("expected game %d in results", expected)
		}
	}

	// Verify games in DB
	var count int64
	u.db.Model(&database.Game{}).Count(&count)
	if count != 4 {
		t.Errorf("game count = %d, want 4", count)
	}

	// Re-run should be a no-op (checkGames filters already-stored games with matching scores)
	result2, err := u.UpdateCurrentWeek(context.Background())
	if err != nil {
		t.Fatalf("UpdateCurrentWeek re-run: %v", err)
	}
	if len(result2.Processed) != 0 {
		t.Errorf("re-run returned %d games, want 0 (no-op)", len(result2.Processed))
	}
}

func TestUpdateCurrentWeek_ScoreChange(t *testing.T) {
	// First run: normal scores
	u := newTestUpdater(t)

	_, err := u.UpdateCurrentWeek(context.Background())
	if err != nil {
		t.Fatalf("initial UpdateCurrentWeek: %v", err)
	}

	// Verify initial score
	var game database.Game
	u.db.Where("game_id = ?", fixtureGameID1).First(&game)
	if game.HomeScore != 28 {
		t.Fatalf("initial home score = %d, want 28", game.HomeScore)
	}

	// Second run: override game 401001 scores to 31-14
	// We need a new server with score override, and re-point the client's
	// URLs at it.
	overrides := map[int64][2]int64{
		fixtureGameID1: {31, 14},
	}
	ts2 := setupTestServer(t, overrides)
	newTestURLs(t, u.espn, ts2.URL)

	result, err := u.UpdateCurrentWeek(context.Background())
	if err != nil {
		t.Fatalf("UpdateCurrentWeek with score change: %v", err)
	}

	// Only game 401001 should be re-fetched (score changed)
	found := false
	for _, id := range result.Processed {
		if id == fixtureGameID1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected game %d to be re-fetched due to score change", fixtureGameID1)
	}

	// Verify updated score in DB
	u.db.Where("game_id = ?", fixtureGameID1).First(&game)
	if game.HomeScore != 31 {
		t.Errorf("updated home score = %d, want 31", game.HomeScore)
	}
}

func TestUpdateTeamInfo(t *testing.T) {
	u := newTestUpdater(t)

	count, err := u.UpdateTeamInfo(context.Background())
	if err != nil {
		t.Fatalf("UpdateTeamInfo: %v", err)
	}
	if count != 4 {
		t.Errorf("team count = %d, want 4", count)
	}

	// Verify team names in DB
	var teams []database.TeamName
	if err := u.db.Find(&teams).Error; err != nil {
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
	u := newTestUpdater(t)

	count, err := u.UpdateTeamSeasons(context.Background(), true)
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
	if err := u.db.Find(&seasons).Error; err != nil {
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
	u := newTestUpdater(t)

	// Seed teams, seasons, and games directly
	seedTeamsAndSeasons(t, u.db)
	seedGames(t, u.db)

	// Run ranking
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}

	// Verify TeamWeekResult rows exist
	var results []database.TeamWeekResult
	if err := u.db.Find(&results).Error; err != nil {
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

// newTestURLs re-points a test updater's ESPN client at the given test server
// base URL for the remainder of the test.
func newTestURLs(t *testing.T, client espn.SportClient, serverURL string) {
	t.Helper()
	t.Cleanup(client.(*espn.FootballClient).SetURLs(
		serverURL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		serverURL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		serverURL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
		serverURL+"/apis/site/v2/sports/football/college-football/scoreboard",
		serverURL+"/apis/site/v2/sports/football/college-football/scoreboard/conferences",
	))
}

// TestNewUpdater_Validation verifies that NewUpdater rejects nil dependencies
// and invalid sports instead of panicking mid-update (mirrors NewRanker).
func TestNewUpdater_Validation(t *testing.T) {
	db := setupCheckGamesDB(t)
	log := zap.NewNop().Sugar()
	client := espn.NewClientForSport(espn.CollegeFootball)

	if _, err := NewUpdater(nil, log, client); err == nil {
		t.Error("nil DB: expected error, got nil")
	}
	if _, err := NewUpdater(db, nil, client); err == nil {
		t.Error("nil logger: expected error, got nil")
	}
	if _, err := NewUpdater(db, log, nil); err == nil {
		t.Error("nil ESPN client: expected error, got nil")
	}

	badSport := &espn.FootballClient{Client: &espn.Client{Sport: "bogus-sport"}}
	if _, err := NewUpdater(db, log, badSport); err == nil {
		t.Error("invalid sport: expected error, got nil")
	}

	u, err := NewUpdater(db, log, client)
	if err != nil {
		t.Fatalf("valid inputs: unexpected error: %v", err)
	}
	if u == nil {
		t.Fatal("valid inputs: updater is nil")
	}
	if u.sportDB() != sport.Football {
		t.Errorf("sportDB() = %q, want %q", u.sportDB(), sport.Football)
	}
}
