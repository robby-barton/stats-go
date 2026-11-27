// Command ncaaf-replay runs a narrow, read-only NCAAF parity check: for each
// explicitly listed manifest window it compares the production ranking
// baseline (the unblended, default path) with an independently loaded
// explicit-cutoff replay, inside a verified read-only repeatable-read
// PostgreSQL transaction that never commits. It writes exactly one artifact
// (parity.json) into a new output directory and exits non-zero on any parity
// failure. It never contacts ESPN or AP, performs no updater work, and
// issues no database writes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ncaafreplay"
	"github.com/robby-barton/stats-go/internal/ranking/load"
)

const commandTimeout = 5 * time.Minute

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ncaaf-replay:", err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("ncaaf-replay", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "path to the replay manifest JSON (required)")
	outputDir := flags.String("output-dir", "", "new directory that will receive parity.json (required)")
	maxTeamRows := flags.Int("max-team-rows", 512, "replay team row bound (<= 512)")
	maxGameRows := flags.Int("max-game-rows", 8192, "replay base game row bound (<= 8192)")
	maxOlderRows := flags.Int("max-older-rows", 6144, "replay aggregate older row bound (<= 6144)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *manifestPath == "" || *outputDir == "" {
		return errors.New("--manifest and --output-dir are required")
	}
	limits := load.ReplayLimits{MaxTeamRows: *maxTeamRows, MaxGameRows: *maxGameRows, MaxOlderRows: *maxOlderRows}

	manifest, err := ncaafreplay.LoadManifest(*manifestPath)
	if err != nil {
		return err
	}
	params, err := config.DBParamsFromEnv()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	if params == nil {
		return errors.New("PostgreSQL is not configured; the read-only replay requires explicit " +
			"PG_* settings and never falls back to SQLite")
	}
	if err := validateParams(params); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	report, _ := ncaafreplay.Run(ctx, params, manifest, limits)
	if err := writeReport(*outputDir, report); err != nil {
		return err
	}
	if report.Status != ncaafreplay.StatusPassed {
		fmt.Fprintf(stderr, "ncaaf-replay: parity %s; see %s/parity.json\n", report.Status, *outputDir)
		return errors.New("parity failed")
	}
	fmt.Fprintf(stderr, "ncaaf-replay: parity passed over %d window(s); see %s/parity.json\n",
		report.Counts.Windows, *outputDir)
	return nil
}

// validateParams rejects unsafe PostgreSQL targets before any connection:
// a replay must be pointed at an explicitly configured, disposable database,
// never a default deployment.
func validateParams(params *database.DBParams) error {
	if params.Host == "localhost" || params.Host == "127.0.0.1" || params.Host == "::1" {
		return nil
	}
	return fmt.Errorf("refusing non-local PostgreSQL host %q; point PG_* at a local disposable database", params.Host)
}

// writeReport stages parity.json and renames it into place; the output
// directory must not already exist.
func writeReport(dir string, report ncaafreplay.Report) error {
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		return errors.New("output path already exists")
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".ncaaf-replay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "parity.json"), append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(stage, dir)
}
