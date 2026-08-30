package config

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
)

// clearPGVars unsets all PG_* environment variables for the test and restores
// them afterwards.
func clearPGVars(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PG_HOST", "PG_PORT", "PG_USER", "PG_PASSWORD", "PG_DBNAME", "PG_SSLMODE"} {
		t.Setenv(key, "")
	}
}

func TestSetupConfig_NoPGVarsYieldsSQLite(t *testing.T) {
	clearPGVars(t)

	cfg, err := SetupConfig()
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}
	if cfg.DBParams != nil {
		t.Errorf("DBParams = %+v, want nil (SQLite fallback)", cfg.DBParams)
	}
}

func TestSetupConfig_FullPGConfig(t *testing.T) {
	clearPGVars(t)
	t.Setenv("PG_HOST", "db.example.com")
	t.Setenv("PG_PORT", "5433")
	t.Setenv("PG_USER", "stats")
	t.Setenv("PG_PASSWORD", "secret")
	t.Setenv("PG_DBNAME", "statsdb")

	cfg, err := SetupConfig()
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}
	want := &database.DBParams{
		Host:     "db.example.com",
		Port:     5433,
		User:     "stats",
		Password: "secret",
		DBName:   "statsdb",
		SSLMode:  "",
	}
	if *cfg.DBParams != *want {
		t.Errorf("DBParams = %+v, want %+v", cfg.DBParams, want)
	}
}

func TestSetupConfig_PGPortDefaultsWhenUnset(t *testing.T) {
	clearPGVars(t)
	t.Setenv("PG_HOST", "db.example.com")
	t.Setenv("PG_USER", "stats")
	t.Setenv("PG_DBNAME", "statsdb")

	cfg, err := SetupConfig()
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}
	if cfg.DBParams == nil || cfg.DBParams.Port != defaultPGPort {
		t.Errorf("Port = %v, want default %d", cfg.DBParams, defaultPGPort)
	}
}

func TestSetupConfig_PartialPGConfigIsError(t *testing.T) {
	clearPGVars(t)
	t.Setenv("PG_HOST", "db.example.com")
	// PG_USER and PG_DBNAME missing

	_, err := SetupConfig()
	if err == nil {
		t.Fatal("expected error for partial PG config, got nil")
	}
}

func TestSetupConfig_MalformedPortIsError(t *testing.T) {
	clearPGVars(t)
	t.Setenv("PG_HOST", "db.example.com")
	t.Setenv("PG_PORT", "not-a-port")
	t.Setenv("PG_USER", "stats")
	t.Setenv("PG_DBNAME", "statsdb")

	_, err := SetupConfig()
	if err == nil {
		t.Fatal("expected error for malformed PG_PORT, got nil")
	}
}
