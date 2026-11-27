package main

import (
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/updater"
)

func TestScheduleCommand_PriorFlagDefaultOffAndSole(t *testing.T) {
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

	// The former manual file/hash companion flags must be gone: the
	// scheduled updater creates and stores the prior itself.
	for _, name := range []string{"fbs-early-season-prior-path", "fbs-early-season-prior-sha256"} {
		if flag := cmd.Flags().Lookup(name); flag != nil {
			t.Errorf("flag %s still registered; the manual artifact contract was removed", name)
		}
	}
}

// TestRegisterFBSPriorRetryJob verifies the daily retry job wiring: exactly
// one job is registered for the enabled updater, and none when not called.
func TestRegisterFBSPriorRetryJob(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	u, err := updater.NewUpdater(db, zap.NewNop().Sugar(), espn.NewClientForSport(espn.CollegeFootball))
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}

	s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}

	if len(s.Jobs()) != 0 {
		t.Fatalf("fresh scheduler has %d jobs", len(s.Jobs()))
	}
	if err := registerFBSPriorRetryJob(t.Context(), s, zap.NewNop().Sugar(), u); err != nil {
		t.Fatalf("registerFBSPriorRetryJob: %v", err)
	}
	if len(s.Jobs()) != 1 {
		t.Fatalf("jobs = %d, want exactly the one daily retry job", len(s.Jobs()))
	}
}
