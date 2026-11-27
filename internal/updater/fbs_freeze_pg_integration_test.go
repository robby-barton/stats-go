//go:build integration

package updater

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
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

func openPriorIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("STATS_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("STATS_TEST_PG_DSN not set; skipping the PostgreSQL prior test")
	}
	open := func(dsn string) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger:                 logger.Default.LogMode(logger.Silent),
			SkipDefaultTransaction: true,
		})
		if err != nil {
			t.Fatalf("open postgres: %v", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("unwrap sql db: %v", err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	root := open(dsn)
	schema := fmt.Sprintf("fbs_prior_fixture_%x", time.Now().UnixNano())
	if err := root.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create fixture schema: %v", err)
	}
	t.Cleanup(func() {
		if err := root.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop fixture schema: %v", err)
		}
	})
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse postgres URL: %v", err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
	} else {
		dsn += " search_path=" + schema
	}
	return open(dsn)
}

// TestIntegration_FreezeOnPostgres exercises the freeze transaction's
// PostgreSQL lock path (bounded lock waits, table locks before the
// snapshot, conflict-do-nothing immutability, and the game guard) on an
// explicitly provisioned disposable local instance. It is skipped unless
// STATS_TEST_PG_DSN holds a Postgres conninfo, e.g.:
//
//	STATS_TEST_PG_DSN='host=localhost port=55432 user=stats password=stats dbname=stats sslmode=disable' \
//	  go test -tags=integration -count=1 ./internal/updater/
func TestIntegration_FreezeOnPostgres(t *testing.T) {
	db := openPriorIntegrationDB(t)

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
	stored, err := fbsprior.Parse([]byte(row.Artifact))
	if err != nil {
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

	// The inline path works after the season starts and without prior
	// storage. Its input fingerprint must still match the preseason freeze.
	if err := db.Migrator().DropTable(&database.FBSSeasonPrior{}); err != nil {
		t.Fatalf("drop prior table in fixture: %v", err)
	}
	inMemory, err := u.reconstructPrior(2023)
	if err != nil {
		t.Fatalf("reconstruct on postgres: %v", err)
	}
	if inMemory.InputSHA256 != stored.InputSHA256 {
		t.Fatal("the target-season game changed the rebuild's donor inputs")
	}
	if !inMemory.CapturedAt.IsZero() || inMemory.SHA256 != "" {
		t.Fatal("reconstructed prior unexpectedly has frozen capture metadata")
	}
}

// TestIntegration_RebuildPublicationOnPostgres exercises the explicit
// rebuild's publication on PostgreSQL: the advisory-lock serialization
// against the opted-in scheduled read/calc/write, the authoritative
// replacement of the frozen row by the freshly computed rebuilt artifact,
// and the whole-publication rollback when the prior store is unavailable.
func TestIntegration_RebuildPublicationOnPostgres(t *testing.T) {
	db := openPriorIntegrationDB(t)
	if err := database.MigrateSchema(db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

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
	if err := u.RunFBSPriorFreezeTask(context.Background(), freezeClock()); err != nil {
		t.Fatalf("freeze on postgres: %v", err)
	}
	frozen := storedPriorRow(t, u.db, 2023)

	seedGames(t, u.db)
	esUniqueStartTimes(t, u.db)
	stubNow(t, esObservedClock())
	if err := u.UpdateRecentRankings(); err != nil {
		t.Fatalf("UpdateRecentRankings: %v", err)
	}
	baseline := storedRows(t, u.db)

	// A concurrent transaction holding the publication advisory lock must
	// delay the rebuild's write phase; once it commits, the rebuild
	// publishes.
	holder := u.db.WithContext(t.Context()).Begin()
	if holder.Error != nil {
		t.Fatal(holder.Error)
	}
	if err := holder.Exec("SELECT pg_advisory_xact_lock(?)",
		fbsPriorPublicationLockKey).Error; err != nil {
		t.Fatalf("hold publication lock: %v", err)
	}
	result := make(chan error, 1)
	go func() {
		result <- u.UpdateAllRankingsWithPrior()
	}()
	select {
	case err := <-result:
		t.Fatalf("rebuild published while the publication lock was held: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatalf("release publication lock: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("rebuild publication after the lock release: %v", err)
	}

	// The frozen row was replaced by the freshly computed rebuilt artifact.
	published := storedPriorRow(t, u.db, 2023)
	if published.Artifact == frozen.Artifact {
		t.Error("the rebuild did not replace the frozen row")
	}
	rebuilt, err := fbsprior.Parse([]byte(published.Artifact))
	if err != nil {
		t.Fatalf("published artifact: %v", err)
	}
	if rebuilt.RebuiltAt.IsZero() || !rebuilt.CapturedAt.Equal(rebuilt.RebuiltAt) {
		t.Errorf("rebuilt artifact timestamps = rebuilt %s captured %s",
			rebuilt.RebuiltAt, rebuilt.CapturedAt)
	}

	// The opted-in scheduled run reads the published prior and blends.
	if err := u.db.Where("1 = 1").Delete(&database.TeamWeekResult{}).Error; err != nil {
		t.Fatalf("clear rows: %v", err)
	}
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("UpdateRecentRankingsWithPrior: %v", err)
	}
	if esBlendedCount(baseline, storedRows(t, u.db), 2023) == 0 {
		t.Error("the scheduled run did not blend the published rebuilt prior")
	}

	// Without the prior store the whole publication aborts and the rebuilt
	// rankings roll back.
	if err := db.Migrator().DropTable(&database.FBSSeasonPrior{}); err != nil {
		t.Fatalf("drop fbs_season_priors: %v", err)
	}
	before := storedRows(t, u.db)
	if err := u.UpdateAllRankingsWithPrior(); err == nil {
		t.Fatal("rebuild succeeded without the prior store")
	}
	esAssertSameRows(t, before, storedRows(t, u.db), "store-unavailable rollback")

	// A scheduled run must still publish baseline: a failed prior SELECT
	// must not abort its outer PostgreSQL publication transaction.
	if err := u.UpdateRecentRankingsWithPrior(); err != nil {
		t.Fatalf("scheduled baseline without the prior table: %v", err)
	}
	esAssertSameRows(t, baseline, storedRows(t, u.db), "scheduled baseline without prior store")
}
