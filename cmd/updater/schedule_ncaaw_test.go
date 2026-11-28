package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"go.uber.org/zap"

	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/updater"
)

// TestScheduleCommand_NCAAWPollingFlagDefaultOff verifies the opt-in women's
// polling flag exists on schedule, defaults to false with the Compose
// environment variable unset, and is independent of the FBS prior flag.
func TestScheduleCommand_NCAAWPollingFlagDefaultOff(t *testing.T) {
	t.Setenv("NCAAW_POLLING", "")

	cmd, err := scheduleCommand(zap.NewNop().Sugar(), testDB(t), "")
	if err != nil {
		t.Fatalf("scheduleCommand: %v", err)
	}

	flag := cmd.Flags().Lookup("ncaaw-polling")
	if flag == nil {
		t.Fatal("flag ncaaw-polling is not registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("ncaaw-polling default = %q, want false", flag.DefValue)
	}
}

// TestNCAAWPollingDefault covers the NCAAW_POLLING env-to-flag-default
// mapping: only unambiguous truthy spellings opt an environment in.
func TestNCAAWPollingDefault(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{env: "", want: false},
		{env: "false", want: false},
		{env: "0", want: false},
		{env: "no", want: false},
		{env: "garbage", want: false},
		{env: "true", want: true},
		{env: "TRUE", want: true},
		{env: " True ", want: true},
		{env: "1", want: true},
		{env: "yes", want: true},
		{env: "on", want: true},
	} {
		if got := ncaawPollingDefault(tc.env); got != tc.want {
			t.Errorf("ncaawPollingDefault(%q) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

// TestScheduledSports_BaselineGuardAndOptIn pins the wiring contract: with
// women's polling off the returned list is byte-identical to the historical
// two-entry baseline (no ncaaw updater, job, or worker is ever built), and
// with it on exactly one ncaaw entry is appended with Eastern-pinned crons
// covering the Nov–Apr season.
func TestScheduledSports_BaselineGuardAndOptIn(t *testing.T) {
	baseline := scheduledSports(false)
	if len(baseline) != 2 {
		t.Fatalf("disabled schedule has %d sports, want exactly ncaaf and ncaam", len(baseline))
	}
	want := []sportSchedule{
		{
			Name:          "ncaaf",
			GamesCron:     "*/5 * * 1,8-12 *",
			TeamInfoCron:  "0 5 * 1,8-12 0",
			NewSeasonCron: "0 6 10 8 *",
		},
		{
			Name:          "ncaam",
			GamesCron:     "*/5 * * 1-4,11-12 *",
			TeamInfoCron:  "0 5 * 1-4,11-12 0",
			NewSeasonCron: "0 6 1 11 *",
		},
	}
	for i, ss := range want {
		if baseline[i].schedule != ss {
			t.Errorf("baseline entry %d = %+v, want %+v (existing behavior must be unchanged)", i, baseline[i].schedule, ss)
		}
	}
	if baseline[0].sport != espn.CollegeFootball || baseline[1].sport != espn.CollegeBasketball {
		t.Errorf("baseline sports = %v, %v; want ncaaf, ncaam", baseline[0].sport, baseline[1].sport)
	}

	enabled := scheduledSports(true)
	if len(enabled) != 3 {
		t.Fatalf("enabled schedule has %d sports, want 3", len(enabled))
	}
	w := enabled[2]
	if w.sport != espn.CollegeWomensBasketball {
		t.Fatalf("third sport = %v, want women's basketball", w.sport)
	}
	ws := w.schedule
	if ws.Name != "ncaaw" {
		t.Errorf("women's schedule name = %q, want ncaaw", ws.Name)
	}
	for field, expr := range map[string]string{
		"GamesCron":     ws.GamesCron,
		"TeamInfoCron":  ws.TeamInfoCron,
		"NewSeasonCron": ws.NewSeasonCron,
	} {
		if !strings.HasPrefix(expr, "CRON_TZ=America/New_York ") {
			t.Errorf("women's %s = %q, want an America/New_York CRON_TZ prefix", field, expr)
		}
	}
	if !strings.Contains(ws.GamesCron, "1-4,11-12") {
		t.Errorf("women's games cron %q must cover the Nov–Apr season", ws.GamesCron)
	}
	// Off the men's 5-minute grid so the two basketball polls never fire on
	// the same tick.
	if !strings.HasPrefix(ws.GamesCron[len("CRON_TZ=America/New_York "):], "2-59/5 ") {
		t.Errorf("women's games cron = %q, want a 5-minute cadence offset from the men's */5 grid", ws.GamesCron)
	}
	if ws.TeamInfoCron[len("CRON_TZ=America/New_York "):] != "0 6 * 1-4,11-12 0" {
		t.Errorf("women's team info cron = %q, want the Sunday 06:00 Eastern refresh", ws.TeamInfoCron)
	}
	if ws.NewSeasonCron[len("CRON_TZ=America/New_York "):] != "0 7 1-14 11 *" {
		t.Errorf("women's season cron = %q, want November 1–14 07:00 Eastern", ws.NewSeasonCron)
	}
}

func TestRegisterNCAAWSchedule(t *testing.T) {
	log := zap.NewNop().Sugar()
	u, err := updater.NewUpdater(testDB(t), log, espn.NewClientForSport(espn.CollegeWomensBasketball))
	if err != nil {
		t.Fatalf("NewUpdater: %v", err)
	}
	s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	al := newAlerter(log)
	d := newDeployer(log, "", al)
	var wg sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		if err := s.Shutdown(); err != nil {
			t.Error(err)
		}
		wg.Wait()
		d.stop()
	})
	ncaaw := scheduledSports(true)[2]
	ncaaw.schedule.registerJobs(ctx, s, log, u, d, al, &wg, false)
	if len(s.Jobs()) != 3 {
		t.Fatalf("women's schedule jobs = %d, want 3 (games, team info, season init with retry)", len(s.Jobs()))
	}
}
