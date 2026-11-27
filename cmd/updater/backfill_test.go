package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/espn"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	return db
}

func findSubcommand(t *testing.T, cmd *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("subcommand %q not found", name)
	return nil
}

// TestNCAAWBackfillCommand_StrictWiring verifies the women's basketball
// backfill is the strict variant: it carries --publish-from and refuses to
// publish a season without its lookback year inside the import range.
func TestNCAAWBackfillCommand_StrictWiring(t *testing.T) {
	log := zap.NewNop().Sugar()
	cmd, err := sportCommand(log, testDB(t), espn.CollegeWomensBasketball)
	if err != nil {
		t.Fatalf("sportCommand: %v", err)
	}

	backfill := findSubcommand(t, cmd, "backfill")
	for _, flag := range []string{"from", "to", "publish-from"} {
		if backfill.Flags().Lookup(flag) == nil {
			t.Errorf("ncaaw backfill is missing the --%s flag", flag)
		}
	}

	// Publishing 2027 with only 2027 imported is refused before any write.
	cmd.SetArgs([]string{"backfill", "--from", "2027", "--to", "2027"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "lookback") {
		t.Errorf("publishing 2027 without the 2026 lookback: err = %v, want a lookback refusal", err)
	}

	// An out-of-range --publish-from is refused.
	cmd.SetArgs([]string{"backfill", "--from", "2026", "--to", "2027", "--publish-from", "2028"})
	if err := cmd.Execute(); err == nil {
		t.Error("--publish-from beyond --to must be refused")
	}
}

// TestNCAAWBackfillCommand_GenericWiringUnchanged verifies football keeps the
// generic backfill (required --from/--to, no --publish-from flag).
func TestNCAAWBackfillCommand_GenericWiringUnchanged(t *testing.T) {
	log := zap.NewNop().Sugar()
	for _, sport := range []espn.Sport{espn.CollegeFootball, espn.CollegeBasketball} {
		cmd, err := sportCommand(log, testDB(t), sport)
		if err != nil {
			t.Fatalf("sportCommand(%s): %v", sport, err)
		}
		backfill := findSubcommand(t, cmd, "backfill")
		if backfill.Flags().Lookup("publish-from") != nil {
			t.Errorf("%s backfill must not expose --publish-from", sport)
		}
		for _, flag := range []string{"from", "to"} {
			if backfill.Flags().Lookup(flag) == nil {
				t.Errorf("%s backfill is missing --%s", sport, flag)
			}
		}
	}
}
