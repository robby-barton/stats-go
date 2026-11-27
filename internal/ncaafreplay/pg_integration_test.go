//go:build integration

package ncaafreplay

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
)

// Optional disposable-PostgreSQL coverage: exercises the verified read-only
// transaction and the full capture/verify cycle against a real server. It
// never runs in default CI — it is skipped unless STATS_TEST_PG_DSN is set,
// and the DSN must point at a disposable database (never production).
func TestReadOnlyReplayAgainstDisposablePostgreSQL(t *testing.T) {
	dsn := os.Getenv("STATS_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("STATS_TEST_PG_DSN not set; skipping disposable PostgreSQL coverage")
	}
	params := dsnParams(t, dsn)
	if params.Host != "localhost" && params.Host != "127.0.0.1" && params.Host != "::1" {
		t.Fatal("integration test requires a disposable loopback database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlog.Default.LogMode(gormlog.Silent),
	})
	if err != nil {
		t.Fatalf("open disposable postgres: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	models := []any{&database.Game{}, &database.TeamSeason{}, &database.TeamName{}}
	for _, model := range models {
		if db.Migrator().HasTable(model) {
			t.Fatal("integration test requires a fresh database; fixture tables already exist")
		}
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seed, err := os.ReadFile("testdata/seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(seed)).Error; err != nil {
		t.Fatalf("seed documented fixture: %v", err)
	}
	var gamesBefore, seasonsBefore, namesBefore int64
	counts := map[string]*int64{"games": &gamesBefore, "team_seasons": &seasonsBefore, "team_names": &namesBefore}
	for table, target := range counts {
		if err := db.Raw("SELECT count(*) FROM " + table).Scan(target).Error; err != nil {
			t.Fatal(err)
		}
	}

	manifest, err := LoadManifest("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), params, manifest, testLimits())
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if report.Status != StatusPassed {
		t.Fatalf("parity %s: %+v", report.Status, report.Failures)
	}
	if !report.Transaction.ReadOnly || report.Transaction.Isolation != "repeatable read" ||
		!report.Transaction.Rollback {
		t.Fatalf("transaction proof incomplete: %+v", report.Transaction)
	}

	// Nothing may have been written by the read-only transaction.
	for table, before := range counts {
		var after int64
		if err := db.Raw("SELECT count(*) FROM " + table).Scan(&after).Error; err != nil {
			t.Fatal(err)
		}
		if after != *before {
			t.Fatalf("read-only transaction wrote rows in %s: %d -> %d", table, *before, after)
		}
	}
}

// dsnParams extracts connection parameters from a keyword/value or URL DSN so
// the transaction path opens its own verified read-only connection to the
// same disposable server.
func dsnParams(t *testing.T, dsn string) *database.DBParams {
	t.Helper()
	params := &database.DBParams{Port: 5432}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse DSN: %v", err)
		}
		params.Host = parsed.Hostname()
		if port, err := strconv.ParseInt(parsed.Port(), 10, 64); err == nil {
			params.Port = port
		}
		params.User = parsed.User.Username()
		params.Password, _ = parsed.User.Password()
		params.DBName = strings.TrimPrefix(parsed.Path, "/")
		query := parsed.Query()
		params.SSLMode = query.Get("sslmode")
		return params
	}
	values := map[string]string{}
	for _, field := range strings.Fields(dsn) {
		key, value, found := strings.Cut(field, "=")
		if found {
			values[key] = value
		}
	}
	params.Host = values["host"]
	params.User = values["user"]
	params.Password = values["password"]
	params.DBName = values["dbname"]
	params.SSLMode = values["sslmode"]
	if port, err := strconv.ParseInt(values["port"], 10, 64); err == nil && port > 0 {
		params.Port = port
	}
	return params
}
