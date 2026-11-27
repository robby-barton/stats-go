//go:build integration

package updater

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
)

// TestIntegration_FreezeOnPostgres exercises the freeze transaction's
// PostgreSQL lock path (bounded lock waits, table locks before the
// snapshot, conflict-do-nothing immutability, and the game guard) on an
// explicitly provisioned disposable local instance. It is skipped unless
// STATS_TEST_PG_DSN holds a Postgres conninfo, e.g.:
//
//	STATS_TEST_PG_DSN='host=localhost port=55432 user=stats password=stats dbname=stats sslmode=disable' \
//	  go test -tags=integration -count=1 ./internal/updater/
func TestIntegration_FreezeOnPostgres(t *testing.T) {
	dsn := os.Getenv("STATS_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("STATS_TEST_PG_DSN not set; skipping the PostgreSQL freeze test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if got := db.Dialector.Name(); got != "postgres" {
		t.Fatalf("dialector %q, want postgres: the lock-path branch must be exercised", got)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// Schema initialization must be idempotent (the migrate-schema
	// contract): apply twice, including on an already-populated schema
	// after the first freeze below.
	if err := database.MigrateSchema(db); err != nil {
		t.Fatalf("first MigrateSchema: %v", err)
	}

	// The freeze path performs no ESPN calls; point the client at a closed
	// local test server so any accidental fetch fails locally instead of
	// reaching a real endpoint.
	closed := httptest.NewServer(nil)
	closed.Close()
	client := &espn.FootballClient{Client: &espn.Client{
		MaxAttempts:    1,
		InitialBackoff: time.Millisecond,
		RequestTimeout: time.Second,
		RateLimit:      0,
		Sport:          espn.CollegeFootball,
	}}
	client.SetURLs(
		closed.URL+"/summary?event=%d",
		closed.URL+"/teams",
		closed.URL+"/scoreboard",
		closed.URL+"/scoreboard/conferences",
	)
	u, err := NewUpdater(db, zap.NewNop().Sugar(), client)
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}

	seedTeamsAndSeasons(t, u.db)
	seedDonorSeasons(t, u.db)
	seedDonorGames(t, u.db)
	stubNow(t, freezeClock())

	// Freeze: the transaction takes the table locks before any read.
	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err != nil {
		t.Fatalf("freeze on postgres: %v", err)
	}
	row := storedPriorRow(t, u.db, 2023)
	if _, err := fbsprior.Parse([]byte(row.Artifact)); err != nil {
		t.Fatalf("stored artifact: %v", err)
	}

	// Repeat: the stored row must be immutable.
	later := time.Date(2023, 8, 16, 15, 0, 0, 0, time.UTC)
	stubNow(t, later)
	if err := u.RunFBSPriorFreezeTask(context.Background(), later); err != nil {
		t.Fatalf("repeat freeze on postgres: %v", err)
	}
	if again := storedPriorRow(t, u.db, 2023); again.Artifact != row.Artifact {
		t.Error("repeat attempt replaced the stored row")
	}

	// Schema re-initialization after the table has rows.
	if err := database.MigrateSchema(db); err != nil {
		t.Fatalf("second MigrateSchema: %v", err)
	}

	// Hold a concurrent target-season game write open. The freezer must
	// take its snapshot only after the writer commits and refuse the prior.
	if err := u.db.Where("season = 2023").Delete(&database.FBSSeasonPrior{}).Error; err != nil {
		t.Fatalf("clear stored prior fixture: %v", err)
	}
	writer := u.db.WithContext(t.Context()).Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	defer func() { _ = writer.Rollback().Error }()
	if err := writer.Create(&database.Game{
		GameID: 401501, Season: 2023, Week: 1, HomeID: 1, AwayID: 2,
		HomeScore: 28, AwayScore: 14, Sport: "ncaaf",
		StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatalf("seed game: %v", err)
	}
	stubNow(t, freezeClock())
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	result := make(chan error, 1)
	finished := make(chan struct{})
	defer func() { cancel(); <-finished }()
	go func() {
		defer close(finished)
		result <- u.freezeFBSPrior(u.db.WithContext(ctx), 2023, freezeClock())
	}()

	for {
		var waiting bool
		if err := u.db.WithContext(ctx).Raw(`SELECT EXISTS (
			SELECT 1 FROM pg_locks WHERE relation = 'games'::regclass
			AND mode = 'ShareRowExclusiveLock' AND NOT granted
		)`).Scan(&waiting).Error; err != nil {
			t.Fatalf("waiting for freeze to request the games lock: %v", err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("freeze did not wait for the game writer: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := writer.Commit().Error; err != nil {
		t.Fatalf("commit game writer: %v", err)
	}
	if err := <-result; err == nil || !strings.Contains(err.Error(), "already persisted") {
		t.Fatalf("freeze did not observe the committed target-season game: %v", err)
	}
}
