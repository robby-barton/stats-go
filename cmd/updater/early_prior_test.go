package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
)

// priorPinFixture writes a valid ncaaf-shadow-prior/1 artifact and returns
// the path and its exact SHA-256 pin.
func priorPinFixture(t *testing.T) (string, string) {
	t.Helper()
	file := map[string]any{
		"schema":          fbsprior.Schema,
		"season":          2026,
		"freeze_date":     "2026-08-24",
		"cutoff":          time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		"calendar_expiry": time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		"as_of":           time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		"captured_at":     time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		"field": []fbsprior.FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
		},
		"prior": map[string]any{
			"z_strength":      map[int64]float64{1: 1.0, 2: -1.0},
			"missing_history": []int64{},
			"degenerate":      false,
			"field_size":      2,
		},
		"input_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"limitations":  []string{"metadata only"},
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "prior.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

func observedLogger(t *testing.T) (*zap.SugaredLogger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	return zap.New(core).Sugar(), logs
}

func TestScheduleCommand_FlagsDefaultOff(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	cmd, err := scheduleCommand(zap.NewNop().Sugar(), db, "")
	if err != nil {
		t.Fatalf("scheduleCommand: %v", err)
	}

	enabled := cmd.Flags().Lookup("fbs-early-season-prior")
	if enabled == nil {
		t.Fatal("flag fbs-early-season-prior is not registered")
	}
	if enabled.DefValue != "false" {
		t.Errorf("fbs-early-season-prior default = %q, want false", enabled.DefValue)
	}
	for _, name := range []string{"fbs-early-season-prior-path", "fbs-early-season-prior-sha256"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("flag %s is not registered", name)
		}
		if flag.DefValue != "" {
			t.Errorf("%s default = %q, want empty", name, flag.DefValue)
		}
	}
}

func TestLoadFbsEarlySeasonPrior_OffNeverReadsFile(t *testing.T) {
	log, logs := observedLogger(t)

	// The path points at a fixture whose bytes do NOT match the pin: if the
	// file were read in off mode, loading would fail and warn.
	path, pin := priorPinFixture(t)
	wrongPin := "ffffffff" + pin[8:]

	got := loadFbsEarlySeasonPrior(fbsPriorFlags{enabled: false, path: path, sha256: wrongPin}, log)
	if got != nil {
		t.Fatalf("off mode returned a prior, want nil")
	}
	if logs.Len() != 0 {
		t.Errorf("off mode logged %d entries, want 0 (file must not be read)", logs.Len())
	}
}

func TestLoadFbsEarlySeasonPrior_EnabledValidLoads(t *testing.T) {
	log, logs := observedLogger(t)
	path, pin := priorPinFixture(t)

	got := loadFbsEarlySeasonPrior(fbsPriorFlags{enabled: true, path: path, sha256: pin}, log)
	if got == nil {
		t.Fatal("enabled mode with a valid artifact returned nil")
	}
	if got.Season != 2026 || got.SHA256 != pin {
		t.Errorf("prior = season %d sha %q, want 2026/%s", got.Season, got.SHA256, pin)
	}
	if logs.Len() != 0 {
		t.Errorf("valid load logged %d warnings, want 0", logs.Len())
	}
}

func TestLoadFbsEarlySeasonPrior_EnabledProblemsWarnAndFallBack(t *testing.T) {
	path, pin := priorPinFixture(t)
	validPin := pin

	tests := []struct {
		name  string
		flags fbsPriorFlags
	}{
		{
			name:  "missing path",
			flags: fbsPriorFlags{enabled: true},
		},
		{
			name:  "missing pin",
			flags: fbsPriorFlags{enabled: true, path: path},
		},
		{
			name:  "absent file",
			flags: fbsPriorFlags{enabled: true, path: filepath.Join(t.TempDir(), "absent.json"), sha256: validPin},
		},
		{
			name:  "digest mismatch",
			flags: fbsPriorFlags{enabled: true, path: path, sha256: "ffffffff" + pin[8:]},
		},
		{
			name:  "pin not hex64",
			flags: fbsPriorFlags{enabled: true, path: path, sha256: "nope"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logs := observedLogger(t)

			got := loadFbsEarlySeasonPrior(tt.flags, log)
			if got != nil {
				t.Fatalf("bad enabled configuration returned a prior, want nil (baseline fallback)")
			}
			if logs.Len() != 1 {
				t.Fatalf("bad enabled configuration logged %d entries, want exactly 1 warning", logs.Len())
			}
			if logs.All()[0].Level != zapcore.WarnLevel {
				t.Errorf("logged level = %s, want warn", logs.All()[0].Level)
			}
		})
	}
}

func TestLoadFbsEarlySeasonPrior_EnabledMalformedArtifactWarns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prior.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum := sha256.Sum256([]byte("{not json"))

	log, logs := observedLogger(t)
	got := loadFbsEarlySeasonPrior(fbsPriorFlags{
		enabled: true, path: path, sha256: hex.EncodeToString(sum[:]),
	}, log)
	if got != nil {
		t.Fatal("malformed artifact returned a prior, want nil")
	}
	if logs.Len() != 1 {
		t.Fatalf("malformed artifact logged %d entries, want 1 warning", logs.Len())
	}
}
