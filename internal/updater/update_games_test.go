package updater

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

// setupCheckGamesDB opens an in-memory SQLite DB with the games table migrated.
// This test lives outside the `integration`-tagged files so it runs in the
// default `go test ./...` suite.
func setupCheckGamesDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Game{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// finalScheduleGame builds an espn.Game with a FINAL status and the given
// competitors.
func finalScheduleGame(id int64, competitors []espn.Competitor) espn.Game {
	return espn.Game{
		ID: id,
		Status: espn.Status{
			StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true},
		},
		Competitions: []espn.Competition{{Competitors: competitors}},
	}
}

// TestCheckGames_MalformedScheduleEntries verifies that schedule games with
// missing competitions or competitors are still passed through for full
// single-game validation instead of panicking on out-of-range indexing.
func TestCheckGames_MalformedScheduleEntries(t *testing.T) {
	db := setupCheckGamesDB(t)

	// Existing game in the DB whose scores match the well-formed schedule
	// entry below (so it would be skipped if score comparison succeeded).
	existing := database.Game{
		GameID: 1001, Season: 2023, Week: 1,
		HomeID: 1, AwayID: 2, HomeScore: 28, AwayScore: 14,
		Sport:     "ncaaf",
		StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed game: %v", err)
	}

	u := &Updater{
		DB:     db,
		Logger: zap.NewNop().Sugar(),
		ESPN:   espn.NewClientForSport(espn.CollegeFootball),
	}

	wellFormed := finalScheduleGame(1001, []espn.Competitor{
		{ID: 1, Team: espn.ScheduleTeam{ID: 1}, Score: 28, HomeAway: "home"},
		{ID: 2, Team: espn.ScheduleTeam{ID: 2}, Score: 14, HomeAway: "away"},
	})
	noCompetitions := espn.Game{
		ID:     1002,
		Status: espn.Status{StatusType: espn.StatusType{Name: "STATUS_FINAL", Completed: true}},
	}
	noCompetitors := finalScheduleGame(1003, nil)

	games, err := u.checkGames([]espn.Game{wellFormed, noCompetitions, noCompetitors})
	if err != nil {
		t.Fatalf("checkGames: %v", err)
	}

	// The well-formed, score-matching game is skipped; both malformed entries
	// must be forwarded to the full single-game path (which validates
	// strictly) rather than being indexed blindly.
	ids := map[int64]bool{}
	for _, g := range games {
		ids[g.ID] = true
	}
	if ids[1001] {
		t.Error("well-formed game with matching scores should be skipped")
	}
	if !ids[1002] || !ids[1003] {
		t.Errorf("malformed games should be forwarded for validation, got %v", ids)
	}
}
