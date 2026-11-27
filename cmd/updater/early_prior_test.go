package main

import (
	"strings"
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

// TestRankingCommand_PriorFlagScope verifies the opt-in CLI scope: the
// ranking command's --fbs-early-season-prior flag exists on the football
// command only, defaults off, and requires --all — plain ranking runs and
// basketball never touch the prior.
func TestRankingCommand_PriorFlagScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	for _, tc := range []struct {
		name     string
		sport    espn.Sport
		wantFlag bool
	}{
		{name: "ncaaf", sport: espn.CollegeFootball, wantFlag: true},
		{name: "ncaam", sport: espn.CollegeBasketball, wantFlag: false},
		{name: "ncaaw", sport: espn.CollegeWomensBasketball, wantFlag: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := sportCommand(zap.NewNop().Sugar(), db, tc.sport)
			if err != nil {
				t.Fatalf("sportCommand: %v", err)
			}
			if cmd.Name() != tc.name {
				t.Fatalf("command name = %q, want %q", cmd.Name(), tc.name)
			}
			rankingCmd, _, err := cmd.Find([]string{"ranking"})
			if err != nil {
				t.Fatalf("find ranking: %v", err)
			}
			flag := rankingCmd.Flags().Lookup("fbs-early-season-prior")
			if tc.wantFlag {
				if flag == nil {
					t.Fatal("flag fbs-early-season-prior is not registered on ncaaf ranking")
				}
				if flag.DefValue != "false" {
					t.Errorf("fbs-early-season-prior default = %q, want false", flag.DefValue)
				}
			} else if flag != nil {
				t.Errorf("flag fbs-early-season-prior must not be registered on %s ranking", tc.name)
			}
		})
	}

	// The flag requires --all: a plain ranking run must refuse it.
	cmd, err := sportCommand(zap.NewNop().Sugar(), db, espn.CollegeFootball)
	if err != nil {
		t.Fatalf("sportCommand: %v", err)
	}
	rankingCmd, _, err := cmd.Find([]string{"ranking"})
	if err != nil {
		t.Fatalf("find ranking: %v", err)
	}
	if err := rankingCmd.Flags().Set("fbs-early-season-prior", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := rankingCmd.RunE(rankingCmd, nil); err == nil {
		t.Fatal("ranking --fbs-early-season-prior without --all succeeded; want an error")
	} else if !strings.Contains(err.Error(), "requires --all") {
		t.Errorf("error = %v, want 'requires --all'", err)
	}
}
