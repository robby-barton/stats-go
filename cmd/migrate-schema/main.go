// Command migrate-schema applies the database schema (all GORM models) to the
// configured database. It is additive only and idempotent, so it is safe to
// run on every deployment. Docker Compose wires it as a one-shot service that
// must complete successfully before the updater starts.
package main

import (
	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/logger"
)

func main() {
	log := logger.NewLogger().Sugar()

	cfg, err := config.SetupConfig()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	sqlDB, _ := db.DB()

	log.Info("Applying database schema")
	if err := database.MigrateSchema(db); err != nil {
		log.Fatalf("schema migration failed: %v", err)
	}

	if err := sqlDB.Close(); err != nil {
		log.Errorf("closing database connection: %v", err)
	}
	log.Info("Schema migration complete")
	_ = log.Sync()
}
