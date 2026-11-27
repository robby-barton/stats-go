package updater

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/game"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ---------------------------------------------------------------------------
// Cross-sport game_id collision regression tests
//
// games.game_id is globally unique, and every dependent stat table keys on it.
// The write-side sport guard in insertGameInfo and markGameRetry must therefore
// refuse a game_id already stored under another sport atomically: the incumbent
// row (score, sport, retry) and its dependent stats stay untouched, and no
// incoming stat rows may be written. These tests run entirely against the
// hermetic in-memory SQLite fixtures.
// ---------------------------------------------------------------------------

const collisionGameID int64 = 401001

// incumbentNCAAFGame is a football game row that owns game_id 401001.
func incumbentNCAAFGame() database.Game {
	return database.Game{
		GameID:    collisionGameID,
		StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC),
		Sport:     sport.Football,
		ConfGame:  true,
		Season:    2023,
		Week:      1,
		HomeID:    1,
		HomeScore: 28,
		AwayID:    2,
		AwayScore: 14,
		Retry:     0,
	}
}

// incumbentTeamStats seeds a dependent stat row that belongs to the incumbent.
func incumbentTeamStats() database.TeamGameStats {
	return database.TeamGameStats{
		GameID:     collisionGameID,
		TeamID:     1,
		Drives:     11,
		FirstDowns: 22,
		PassYards:  250,
	}
}

// incomingBasketballParsed builds the parsed summary of a men's basketball
// game that reuses the incumbent's game_id with different teams and scores,
// plus team and player stat rows that must never reach the database.
func incomingBasketballParsed() *game.ParsedGameInfo {
	const (
		homeID  = 777
		awayID  = 888
		playerI = 999
	)
	return &game.ParsedGameInfo{
		Info: game.Info{
			GameID: collisionGameID, Season: 2024, Week: 5,
			StartTime: time.Date(2024, 1, 20, 1, 0, 0, 0, time.UTC),
			HomeID:    homeID, HomeScore: 100, AwayID: awayID, AwayScore: 98,
		},
		TeamStats: []game.TeamGameStats{
			{GameID: collisionGameID, TeamID: homeID, Drives: 1, FirstDowns: 9, PassYards: 88},
			{GameID: collisionGameID, TeamID: awayID, Drives: 1, FirstDowns: 8, PassYards: 70},
		},
		PassingStats: []game.PassingStats{
			{GameID: collisionGameID, PlayerID: playerI, TeamID: homeID, Completions: 7, Attempts: 12, Yards: 88},
		},
	}
}

func newBasketballUpdater(t *testing.T) *Updater {
	t.Helper()
	db := setupTestDB(t)
	u, err := NewUpdater(db, zap.NewNop().Sugar(), espn.NewClientForSport(espn.CollegeBasketball))
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	return u
}

func TestCrossSportCollision_NormalPathRefusedAtomically(t *testing.T) {
	u := newBasketballUpdater(t)
	incumbent := incumbentNCAAFGame()
	if err := u.db.Create(&incumbent).Error; err != nil {
		t.Fatalf("seed incumbent game: %v", err)
	}
	incStats := incumbentTeamStats()
	if err := u.db.Create(&incStats).Error; err != nil {
		t.Fatalf("seed incumbent stats: %v", err)
	}

	err := u.insertGameInfo(incomingBasketballParsed())
	if err == nil {
		t.Fatal("cross-sport insert must be refused")
	}
	if !strings.Contains(err.Error(), "refusing cross-sport game_id collision") {
		t.Fatalf("unexpected refusal error: %v", err)
	}

	var stored database.Game
	if err := u.db.First(&stored, "game_id = ?", collisionGameID).Error; err != nil {
		t.Fatalf("read incumbent game: %v", err)
	}
	if stored.Sport != sport.Football {
		t.Errorf("incumbent sport = %q, want ncaaf", stored.Sport)
	}
	if stored.HomeScore != 28 || stored.AwayScore != 14 {
		t.Errorf("incumbent scores = %d-%d, want 28-14", stored.HomeScore, stored.AwayScore)
	}
	if stored.HomeID != 1 || stored.AwayID != 2 {
		t.Errorf("incumbent teams = %d/%d, want 1/2", stored.HomeID, stored.AwayID)
	}
	if stored.Retry != 0 {
		t.Errorf("incumbent retry = %d, want 0", stored.Retry)
	}

	var teamStats []database.TeamGameStats
	if err := u.db.Find(&teamStats, "game_id = ?", collisionGameID).Error; err != nil {
		t.Fatalf("read team stats: %v", err)
	}
	if len(teamStats) != 1 {
		t.Fatalf("team stat rows = %d, want only the incumbent row", len(teamStats))
	}
	if teamStats[0].TeamID != 1 || teamStats[0].Drives != 11 || teamStats[0].FirstDowns != 22 {
		t.Errorf("incumbent team stats mutated: %+v", teamStats[0])
	}

	var passingCount int64
	if err := u.db.Model(&database.PassingStats{}).
		Where("game_id = ?", collisionGameID).Count(&passingCount).Error; err != nil {
		t.Fatalf("count passing stats: %v", err)
	}
	if passingCount != 0 {
		t.Errorf("incoming passing stats leaked: %d rows", passingCount)
	}
}

func TestCrossSportCollision_RetryPathRefusedAtomically(t *testing.T) {
	u := newBasketballUpdater(t)
	incumbent := incumbentNCAAFGame()
	if err := u.db.Create(&incumbent).Error; err != nil {
		t.Fatalf("seed incumbent game: %v", err)
	}

	schedule := finalScheduleGame(collisionGameID, []espn.Competitor{
		{ID: 777, Team: espn.ScheduleTeam{ID: 777}, Score: 100, HomeAway: "home"},
		{ID: 888, Team: espn.ScheduleTeam{ID: 888}, Score: 98, HomeAway: "away"},
	})

	err := u.markGameRetry(schedule)
	if err == nil {
		t.Fatal("cross-sport retry flag must be refused")
	}
	if !strings.Contains(err.Error(), "refusing cross-sport game_id collision") {
		t.Fatalf("unexpected refusal error: %v", err)
	}

	var stored database.Game
	if err := u.db.First(&stored, "game_id = ?", collisionGameID).Error; err != nil {
		t.Fatalf("read incumbent game: %v", err)
	}
	if stored.Sport != sport.Football {
		t.Errorf("incumbent sport = %q, want ncaaf", stored.Sport)
	}
	if stored.Retry != 0 {
		t.Errorf("incumbent retry = %d, want 0 (retry flag must not cross sports)", stored.Retry)
	}
	if stored.HomeScore != 28 || stored.AwayScore != 14 {
		t.Errorf("incumbent scores mutated to %d-%d", stored.HomeScore, stored.AwayScore)
	}

	// The refusal must not have left a second row behind either.
	var gameCount int64
	if err := u.db.Model(&database.Game{}).Where("game_id = ?", collisionGameID).Count(&gameCount).Error; err != nil {
		t.Fatalf("count games: %v", err)
	}
	if gameCount != 1 {
		t.Errorf("game rows for id %d = %d, want 1", collisionGameID, gameCount)
	}
}

func TestSameSportRetryFlagSetThenClearedOnCorrection(t *testing.T) {
	u := newTestUpdater(t)
	incumbent := incumbentNCAAFGame()
	if err := u.db.Create(&incumbent).Error; err != nil {
		t.Fatalf("seed incumbent game: %v", err)
	}

	// A failed fetch on the same sport only flips the retry flag; the
	// incumbent's stored scores and teams stay as they are.
	schedule := finalScheduleGame(collisionGameID, []espn.Competitor{
		{ID: 1, Team: espn.ScheduleTeam{ID: 1}, Score: 99, HomeAway: "home"},
		{ID: 2, Team: espn.ScheduleTeam{ID: 2}, Score: 42, HomeAway: "away"},
	})
	if err := u.markGameRetry(schedule); err != nil {
		t.Fatalf("markGameRetry: %v", err)
	}

	var flagged database.Game
	if err := u.db.First(&flagged, "game_id = ?", collisionGameID).Error; err != nil {
		t.Fatalf("read flagged game: %v", err)
	}
	if flagged.Retry != 1 {
		t.Errorf("retry = %d, want 1", flagged.Retry)
	}
	if flagged.HomeScore != 28 || flagged.AwayScore != 14 || flagged.HomeID != 1 || flagged.AwayID != 2 {
		t.Errorf("retry flag overwrote incumbent row: %+v", flagged)
	}

	// A successful later fetch upserts the full row: corrected scores land
	// and the retry flag is cleared.
	parsed := &game.ParsedGameInfo{
		Info: game.Info{
			GameID: collisionGameID, Season: 2023, Week: 1,
			StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC),
			HomeID:    1, HomeScore: 35, AwayID: 2, AwayScore: 21, ConfGame: true,
		},
		TeamStats: []game.TeamGameStats{
			{GameID: collisionGameID, TeamID: 1, Drives: 12, FirstDowns: 24},
			{GameID: collisionGameID, TeamID: 2, Drives: 9, FirstDowns: 15},
		},
	}
	if err := u.insertGameInfo(parsed); err != nil {
		t.Fatalf("insertGameInfo: %v", err)
	}

	var corrected database.Game
	if err := u.db.First(&corrected, "game_id = ?", collisionGameID).Error; err != nil {
		t.Fatalf("read corrected game: %v", err)
	}
	if corrected.HomeScore != 35 || corrected.AwayScore != 21 {
		t.Errorf("scores = %d-%d, want corrected 35-21", corrected.HomeScore, corrected.AwayScore)
	}
	if corrected.Retry != 0 {
		t.Errorf("retry = %d, want 0 after successful fetch", corrected.Retry)
	}
	if corrected.Sport != sport.Football {
		t.Errorf("sport = %q, want ncaaf", corrected.Sport)
	}

	var teamStats []database.TeamGameStats
	if err := u.db.Order("team_id").Find(&teamStats, "game_id = ?", collisionGameID).Error; err != nil {
		t.Fatalf("read team stats: %v", err)
	}
	if len(teamStats) != 2 || teamStats[0].FirstDowns != 24 || teamStats[1].FirstDowns != 15 {
		t.Errorf("team stats = %+v, want two upserted rows", teamStats)
	}
}

func TestSameSportRetrySeedsNewGameRow(t *testing.T) {
	u := newTestUpdater(t)

	// A game with no stored row gets a minimal seed row from the schedule
	// entry, flagged for retry.
	schedule := finalScheduleGame(401006, []espn.Competitor{
		{ID: 1, Team: espn.ScheduleTeam{ID: 1}, Score: 7, HomeAway: "home"},
		{ID: 2, Team: espn.ScheduleTeam{ID: 2}, Score: 3, HomeAway: "away"},
	})
	if err := u.markGameRetry(schedule); err != nil {
		t.Fatalf("markGameRetry: %v", err)
	}

	var seeded database.Game
	if err := u.db.First(&seeded, "game_id = ?", 401006).Error; err != nil {
		t.Fatalf("read seeded game: %v", err)
	}
	if seeded.Retry != 1 || seeded.Sport != sport.Football {
		t.Errorf("seeded row = retry %d sport %q, want retry 1 sport ncaaf", seeded.Retry, seeded.Sport)
	}
	if seeded.HomeID != 1 || seeded.AwayID != 2 || seeded.HomeScore != 7 || seeded.AwayScore != 3 {
		t.Errorf("seeded row missing schedule identity: %+v", seeded)
	}
}

// ---------------------------------------------------------------------------
// Dialect review: the guard must build identically for Postgres and SQLite —
// ON CONFLICT (game_id) DO UPDATE SET ... WHERE games.sport = '<sport>' —
// where the table-qualified games.sport reference resolves to the incumbent row
// in both dialects. DryRun sessions emit the SQL without touching PostgreSQL.
// ---------------------------------------------------------------------------

func assertGuardSQL(t *testing.T, db *gorm.DB, dialect, conflictTarget string) {
	t.Helper()

	gameRow := database.Game{
		GameID: collisionGameID, Sport: sport.Basketball,
		Season: 2024, Week: 5, HomeID: 777, AwayID: 888,
	}
	stmt := db.Session(&gorm.Session{DryRun: true}).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "game_id"}},
			UpdateAll: true,
			Where:     gameConflictGuard(sport.Basketball),
		}).Create(&gameRow).Statement
	if stmt.Error != nil {
		t.Fatalf("%s: build statement: %v", dialect, stmt.Error)
	}
	sql := stmt.SQL.String()

	// The guard must appear after the DO UPDATE SET list (a clause-level
	// WHERE on the upsert, not a statement-level WHERE) and reference the
	// incumbent games row, not the incoming excluded row. Bind values stay as
	// placeholders under DryRun.
	for _, want := range []string{
		"ON CONFLICT " + conflictTarget + " DO UPDATE SET",
		"WHERE games.sport =",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("%s SQL missing %q:\n%s", dialect, want, sql)
		}
	}
	if guard := strings.Index(sql, "WHERE games.sport ="); guard < strings.Index(sql, "DO UPDATE SET") {
		t.Errorf("%s SQL: guard must follow the DO UPDATE SET list, got:\n%s", dialect, sql)
	}
}

func TestGameConflictGuardBuildsForBothDialects(t *testing.T) {
	sqliteDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	assertGuardSQL(t, sqliteDB, "sqlite", "(`game_id`)")

	// Postgres dialect with a lazily opened *sql.DB and no automatic ping:
	// the DSN is parsed and the statement built, but no connection is ever
	// established.
	conn, err := sql.Open("pgx", "host=localhost user=none password=none dbname=none port=1 sslmode=disable")
	if err != nil {
		t.Fatalf("open pgx driver: %v", err)
	}
	pgDB, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres dialect: %v", err)
	}
	assertGuardSQL(t, pgDB, "postgres", `("game_id")`)
}
