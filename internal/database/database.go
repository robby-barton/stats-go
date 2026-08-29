package database

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type DBParams struct {
	Host     string
	Port     int64
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func NewDatabase(params *DBParams) (*gorm.DB, error) {
	if params != nil {
		return postgresDB(params)
	}
	return sqliteDB()
}

func postgresDB(params *DBParams) (*gorm.DB, error) {
	connInfo := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		params.Host,
		params.Port,
		params.User,
		params.Password,
		params.DBName,
		params.SSLMode,
	)

	return gorm.Open(postgres.Open(connInfo), &gorm.Config{
		SkipDefaultTransaction: true, // handle my own transactions
	})
}

func sqliteDB() (*gorm.DB, error) {
	// _foreign_keys=on enables SQLite FK enforcement on every pooled
	// connection (the PRAGMA is per-connection, so the DSN is the only
	// reliable way to set it).
	return gorm.Open(sqlite.Open("db/stats.db?_foreign_keys=on"), &gorm.Config{
		SkipDefaultTransaction: true, // handle my own transactions
	})
}

// MigrateSchema applies the GORM schema (all tables) to the database.
// It is additive only — creates missing tables, columns, and indexes, and
// never drops existing data — so it is safe to run on every deployment.
// It is wired into docker-compose as a one-shot service that must succeed
// before the updater starts.
func MigrateSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&TeamName{},
		&TeamSeason{},
		&TeamWeekResult{},
		&Game{},
		&TeamGameStats{},
		&Composite{},
		&Recruiting{},
		&Roster{},
		&Player{},
		&PassingStats{},
		&RushingStats{},
		&ReceivingStats{},
		&ReturnStats{},
		&KickStats{},
		&PuntStats{},
		&InterceptionStats{},
		&FumbleStats{},
		&DefensiveStats{},
	)
}
