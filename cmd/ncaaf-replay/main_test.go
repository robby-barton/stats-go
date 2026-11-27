package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ncaafreplay"
	"github.com/robby-barton/stats-go/internal/ranking/load"
)

func TestRunRequiresManifestAndOutputDir(t *testing.T) {
	if err := run([]string{}, os.Stderr); err == nil || !strings.Contains(err.Error(), "--manifest") {
		t.Fatalf("missing flags must fail: %v", err)
	}
	if err := run([]string{"--manifest", "x"}, os.Stderr); err == nil ||
		!strings.Contains(err.Error(), "--output-dir") {
		t.Fatalf("missing output dir must fail: %v", err)
	}
}

func TestRunRejectsMissingManifestFile(t *testing.T) {
	dir := t.TempDir()
	err := run([]string{"--manifest", filepath.Join(dir, "absent.json"),
		"--output-dir", filepath.Join(dir, "out")}, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "read replay manifest") {
		t.Fatalf("missing manifest must fail: %v", err)
	}
}

func TestRunRejectsSQLiteFallback(t *testing.T) {
	// Explicit environment only. A .env file is not a permitted fallback.
	for _, variable := range []string{"PG_HOST", "PG_PORT", "PG_USER", "PG_PASSWORD", "PG_DBNAME", "PG_SSLMODE"} {
		t.Setenv(variable, "")
		if err := os.Unsetenv(variable); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("API_ENV", "local")
	dir := t.TempDir()
	t.Chdir(dir)
	contents := []byte("PG_HOST=db.example.invalid\nPG_USER=ignored\nPG_DBNAME=ignored\n")
	if err := os.WriteFile(".env", contents, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"schema": "ncaaf-replay-manifest/1",
		"windows": [{"year": 2023, "week": 2}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"--manifest", manifest, "--output-dir", filepath.Join(dir, "out")}, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "never falls back to SQLite") {
		t.Fatalf("SQLite fallback must be refused: %v", err)
	}
}

func TestValidateParamsRequiresLocalHost(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		params := database.DBParams{Host: host, Port: 5432, User: "u", DBName: "d"}
		if err := validateParams(&params); err != nil {
			t.Fatalf("local host %s refused: %v", host, err)
		}
	}
	params := database.DBParams{Host: "db.example.com", Port: 5432, User: "u", DBName: "d"}
	if err := validateParams(&params); err == nil || !strings.Contains(err.Error(), "non-local") {
		t.Fatalf("non-local host must be refused: %v", err)
	}
}

func TestWriteReportUsesNewDirectoryOnly(t *testing.T) {
	parent := t.TempDir()
	existing := filepath.Join(parent, "existing")
	if err := os.Mkdir(existing, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeReport(existing, ncaafreplay.NewReport(load.ReplayLimits{MaxTeamRows: 512,
		MaxGameRows: 8192, MaxOlderRows: 6144})); err == nil {
		t.Fatal("existing output dir must be refused")
	}
	target := filepath.Join(parent, "fresh")
	if err := writeReport(target, ncaafreplay.NewReport(load.ReplayLimits{MaxTeamRows: 512,
		MaxGameRows: 8192, MaxOlderRows: 6144})); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "parity.json" {
		t.Fatalf("output dir must contain only parity.json: %v", entries)
	}
	data, err := os.ReadFile(filepath.Join(target, "parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"status": "blocked"`) {
		t.Fatalf("unexpected report content: %s", data)
	}
}
