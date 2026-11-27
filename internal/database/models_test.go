package database

import (
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm/schema"
)

func TestRankingScoreUsesPostgresFloat(t *testing.T) {
	model, err := schema.Parse(&TeamWeekResult{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	field := model.LookUpField("final_raw")
	if field == nil {
		t.Fatal("team_week_results is missing final_raw")
	}
	// The default GORM float mapping is numeric, which postgres.js returns
	// as a string. Pin the PostgreSQL storage type to the site contract.
	if got := (postgres.Dialector{}).DataTypeOf(field); got != "double precision" {
		t.Errorf("final_raw PostgreSQL type = %q, want double precision", got)
	}
}
