package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/logger"
	"github.com/robby-barton/stats-go/internal/updater"
)

func main() {
	os.Exit(run())
}

func run() int {
	log := logger.NewLogger().Sugar()
	defer log.Sync()

	cfg, err := config.SetupConfig()
	if err != nil {
		panic(err)
	}

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		panic(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	rootCmd := &cobra.Command{
		Use:           "updater",
		Short:         "College sports data updater",
		SilenceErrors: true, // errors are logged once at the command boundary
	}
	rootCmd.SilenceUsage = true

	scheduleCmd, err := scheduleCommand(log, db, cfg.DeployScript)
	if err != nil {
		log.Error(err)
		return 1
	}
	ncaafCmd, err := sportCommand(log, db, espn.CollegeFootball)
	if err != nil {
		log.Error(err)
		return 1
	}
	ncaamCmd, err := sportCommand(log, db, espn.CollegeBasketball)
	if err != nil {
		log.Error(err)
		return 1
	}
	ncaawCmd, err := sportCommand(log, db, espn.CollegeWomensBasketball)
	if err != nil {
		log.Error(err)
		return 1
	}

	rootCmd.AddCommand(scheduleCmd, ncaafCmd, ncaamCmd, ncaawCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Error(err)
		return 1
	}
	return 0
}

// newUpdater builds a validated Updater for the given sport's ESPN client.
func newUpdater(
	log *zap.SugaredLogger,
	db *gorm.DB,
	sport espn.Sport,
) (*updater.Updater, error) {
	return updater.NewUpdater(db, log, espn.NewClientForSport(sport))
}

// deployTimeout is the hard limit for a single deploy script run. The script
// clones, installs, builds, and deploys stats-web, which normally takes a few
// minutes; a stuck run (hung network, orphaned child) is killed well before it
// can block the deployer goroutine indefinitely.
const deployTimeout = 30 * time.Minute

// deployer runs a deploy script in the background after rankings are updated.
// Calls to Trigger are coalesced: if a deploy is already queued, extra triggers
// are dropped so at most one deploy is pending at a time.
// stop() closes the trigger channel; it must only be called after all producers
// (ranking workers) have been joined, otherwise Trigger could send on a closed
// channel. stop is idempotent.
type deployer struct {
	script   string
	log      *zap.SugaredLogger
	al       *alerter
	timeout  time.Duration
	trigger  chan struct{}
	stopOnce sync.Once
}

func newDeployer(log *zap.SugaredLogger, script string, al *alerter) *deployer {
	d := &deployer{
		script:  script,
		log:     log,
		al:      al,
		timeout: deployTimeout,
		trigger: make(chan struct{}, 1),
	}
	go d.run()
	return d
}

// Trigger enqueues a deploy. If one is already pending, this is a no-op.
func (d *deployer) Trigger() {
	if d.script == "" {
		return
	}
	select {
	case d.trigger <- struct{}{}:
	default:
	}
}

func (d *deployer) stop() {
	d.stopOnce.Do(func() { close(d.trigger) })
}

func (d *deployer) run() {
	for range d.trigger {
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		//nolint:gosec // DEPLOY_SCRIPT is operator-supplied config, not user input
		cmd := exec.CommandContext(ctx, d.script)
		// Kill the whole process group on timeout, not just the sh process:
		// otherwise orphaned yarn/node children keep the output pipe open and
		// block Wait forever. WaitDelay bounds any remaining pipe read anyway.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			return nil
		}
		cmd.WaitDelay = 30 * time.Second
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				d.log.Errorf("deploy script timed out after %s: %v\n%s", d.timeout, err, out)
				d.al.failure("site deploy", fmt.Errorf("deploy script timed out after %s: %w", d.timeout, err))
			} else {
				d.log.Errorf("deploy script failed: %v\n%s", err, out)
				d.al.failure("site deploy", err)
			}
			continue
		}
		d.log.Infof("deploy script completed:\n%s", out)
		d.al.success("site deploy")
	}
}

// sportSchedule holds the cron expressions for a sport's scheduled jobs.
type sportSchedule struct {
	Name          string // human-readable label for log messages
	GamesCron     string // completed games poll
	TeamInfoCron  string // team metadata refresh
	NewSeasonCron string // season initialization
}

// registerJobs adds the three cron jobs for a sport to the scheduler and starts
// the ranking worker. The worker runs until ctx is canceled and must be joined
// before shutting down the deploy coalescer.
func (ss sportSchedule) registerJobs(
	ctx context.Context,
	s gocron.Scheduler,
	log *zap.SugaredLogger,
	u *updater.Updater,
	d *deployer,
	al *alerter,
	wg *sync.WaitGroup,
	fbsPriorEnabled bool,
) {
	update := make(chan bool, 1)
	womens := ss.Name == "ncaaw"
	requestRanking := func() {
		if womens {
			// A pending wake will already see all the games just written.
			select {
			case update <- true:
			default:
			}
			return
		}
		update <- true
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-update:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Errorf("%s panic caught: %s", ss.Name, r)
						}
					}()

					// The early-season prior is read fresh from its stored
					// row on every enabled football run, so a row frozen
					// after startup is usable without a restart.
					var updateErr error
					published := true
					switch {
					case womens:
						published, updateErr = u.UpdateScheduledNCAAWRankings()
					case fbsPriorEnabled:
						updateErr = u.UpdateRecentRankingsWithPrior()
					default:
						updateErr = u.UpdateRecentRankings()
					}
					if updateErr != nil {
						log.Error(updateErr)
						al.failure(ss.Name+" ranking", updateErr)
						return
					}
					al.success(ss.Name + " ranking")
					if published {
						log.Infof("%s rankings updated", ss.Name)
						d.Trigger()
					}
				}()
			case <-ctx.Done():
				return
			}
		}
	}()

	// A large women's slate can outlast a tick. Let the next tick catch up
	// rather than running two imports for the same sport at once.
	var gameOptions []gocron.JobOption
	if womens {
		gameOptions = append(gameOptions, gocron.WithSingletonMode(gocron.LimitModeReschedule))
	}
	if _, err := s.NewJob(gocron.CronJob(ss.GamesCron, false), gocron.NewTask(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("%s panic caught: %s", ss.Name, r)
			}
		}()

		result, err := u.UpdateCurrentWeek(ctx)
		if err != nil {
			log.Error(err)
			al.failure(ss.Name+" games", err)
		} else {
			al.success(ss.Name + " games")
			log.Infof("%s: added %d games: %v", ss.Name, len(result.Processed), result.Processed)
			if len(result.Failed) > 0 {
				log.Errorf("%s: failed to fetch %d games (marked for retry): %v",
					ss.Name, len(result.Failed), result.FailedIDs())
			}
			if len(result.Skipped) > 0 {
				log.Warnf("%s: protected %d games from reimport (not fetched, no retry): %v",
					ss.Name, len(result.Skipped), result.SkippedIDs())
			}
			if len(result.Processed) > 0 {
				requestRanking()
			}
		}
	}), gameOptions...); err != nil {
		panic(err)
	}

	// Team info refresh
	if _, err := s.NewJob(gocron.CronJob(ss.TeamInfoCron, false), gocron.NewTask(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("%s panic caught: %s", ss.Name, r)
			}
		}()

		addedTeams, err := u.UpdateTeamInfo(ctx)
		if err != nil {
			log.Error(err)
			al.failure(ss.Name+" team info", err)
			return
		}
		al.success(ss.Name + " team info")
		log.Infof("%s: updated %d teams", ss.Name, addedTeams)
	})); err != nil {
		panic(err)
	}

	// New season initialization
	if _, err := s.NewJob(gocron.CronJob(ss.NewSeasonCron, false), gocron.NewTask(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("%s panic caught: %s", ss.Name, r)
			}
		}()

		var addedSeasons int
		var err error
		if womens {
			addedSeasons, err = u.InitializeScheduledNCAAWSeason(ctx)
		} else {
			addedSeasons, err = u.UpdateTeamSeasons(ctx, false)
		}
		log.Infof("%s: added %d seasons", ss.Name, addedSeasons)
		if err != nil {
			log.Error(err)
			al.failure(ss.Name+" new season", err)
		} else {
			al.success(ss.Name + " new season")
		}
		// Successful annual football season initialization is the first
		// trigger for the opt-in automatic preseason freeze. The task
		// itself is a no-op outside the August 10-16 Eastern window.
		if fbsPriorEnabled && err == nil {
			if err := u.RunFBSPriorFreezeTask(ctx, time.Now()); err != nil {
				log.Warnf("fbs prior freeze after season init: %v", err)
			}
		}
		if addedSeasons > 0 {
			requestRanking()
		}
	})); err != nil {
		panic(err)
	}
}

// Retry at 06:07 Eastern each day of the preseason window, regardless
// of the scheduler's timezone. The task also checks the Eastern date.
const fbsPriorRetryCron = "CRON_TZ=America/New_York 7 6 10-16 8 *"

// registerFBSPriorRetryJob registers the daily retry job for the automatic
// preseason freeze. It is only called for the enabled football updater.
func registerFBSPriorRetryJob(
	ctx context.Context,
	s gocron.Scheduler,
	log *zap.SugaredLogger,
	u *updater.Updater,
) error {
	_, err := s.NewJob(gocron.CronJob(fbsPriorRetryCron, false), gocron.NewTask(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("fbs prior freeze retry panic caught: %s", r)
			}
		}()

		if err := u.RunFBSPriorFreezeTask(ctx, time.Now()); err != nil {
			log.Warnf("fbs prior freeze: %v", err)
		}
	}), gocron.WithStartAt(gocron.WithStartImmediately()))
	return err
}

// scheduledSport pairs a sport's cron schedule with its ESPN client sport.
type scheduledSport struct {
	schedule sportSchedule
	sport    espn.Sport
}

// scheduledSports returns the registered sport schedules. With women's polling
// disabled (the default) the list is exactly the two baseline entries, so no
// women's updater, cron job, or ranking worker is constructed and the existing
// football and men's basketball behavior is bit-for-bit unchanged.
//
// The women's cron expressions pin America/New_York with a CRON_TZ prefix so
// the Eastern Nov-Apr season window holds regardless of the process timezone,
// and they sit next to — not on top of — the shared men's basketball ticks:
// the games poll runs two minutes off the men's 5-minute grid, the weekly team
// refresh runs an hour after the men's Sunday refresh. Women's season
// initialization retries daily November 1–14 if ESPN is not ready. Rankings
// use the same worker pattern and shared deploy coalescer as the other sports.
func scheduledSports(ncaawPolling bool) []scheduledSport {
	sports := []scheduledSport{
		{
			schedule: sportSchedule{
				Name:          "ncaaf",
				GamesCron:     "*/5 * * 1,8-12 *",
				TeamInfoCron:  "0 5 * 1,8-12 0",
				NewSeasonCron: "0 6 10 8 *",
			},
			sport: espn.CollegeFootball,
		},
		{
			schedule: sportSchedule{
				Name:          "ncaam",
				GamesCron:     "*/5 * * 1-4,11-12 *",
				TeamInfoCron:  "0 5 * 1-4,11-12 0",
				NewSeasonCron: "0 6 1 11 *",
			},
			sport: espn.CollegeBasketball,
		},
	}
	if ncaawPolling {
		sports = append(sports, scheduledSport{
			schedule: sportSchedule{
				Name:          "ncaaw",
				GamesCron:     "CRON_TZ=America/New_York 2-59/5 * * 1-4,11-12 *",
				TeamInfoCron:  "CRON_TZ=America/New_York 0 6 * 1-4,11-12 0",
				NewSeasonCron: "CRON_TZ=America/New_York 0 7 1-14 11 *",
			},
			sport: espn.CollegeWomensBasketball,
		})
	}
	return sports
}

func scheduleCommand(
	log *zap.SugaredLogger,
	db *gorm.DB,
	deployScript string,
) (*cobra.Command, error) {
	// newUpdater validates its inputs; a nil DB or logger is a wiring bug, so
	// surface it as an error instead of a runtime panic inside a cron job.
	if db == nil || log == nil {
		return nil, errors.New("schedule: nil DB or logger")
	}
	var fbsPriorEnabled bool
	var ncaawPolling bool
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Run the scheduled updater for all sports",
		RunE: func(_ *cobra.Command, _ []string) error {
			// Process-level context: cancelled on shutdown so ranking workers
			// stop before the deployer's trigger channel is closed.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			s, err := gocron.NewScheduler(gocron.WithLocation(time.Local))
			if err != nil {
				panic(err)
			}

			al := newAlerter(log)
			d := newDeployer(log, deployScript, al)

			sports := scheduledSports(ncaawPolling)

			var wg sync.WaitGroup
			// The early-season prior is football-only: enable it for the ncaaf
			// worker and freeze task, and leave basketball and backfill entirely
			// baseline. One-shot commands stay baseline unless the ranking
			// command's own --fbs-early-season-prior flag is used with --all.
			// With the schedule flag off no prior job is registered and the
			// scheduled worker never touches the stored table.
			var footballUpdater *updater.Updater
			for _, sp := range sports {
				u, err := newUpdater(log, db, sp.sport)
				if err != nil {
					return err
				}
				enabled := fbsPriorEnabled && sp.sport == espn.CollegeFootball
				sp.schedule.registerJobs(ctx, s, log, u, d, al, &wg, enabled)
				if sp.sport == espn.CollegeFootball {
					footballUpdater = u
				}
			}
			if fbsPriorEnabled {
				log.Info("fbs early-season prior enabled: automatic preseason freeze on")
				if err := registerFBSPriorRetryJob(ctx, s, log, footballUpdater); err != nil {
					return err
				}
			}
			if ncaawPolling {
				log.Info("ncaaw polling enabled: women's basketball scheduled jobs registered")
			}

			end := make(chan os.Signal, 1)
			signal.Notify(end, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(end)
			s.Start()
			<-end
			// Cancel the process context before shutting down the scheduler so
			// in-flight ESPN requests, retries, and rate-limit sleeps are
			// interrupted promptly instead of blocking s.Shutdown().
			cancel()
			if err := s.Shutdown(); err != nil {
				log.Error(err)
			}
			// Join the ranking workers so no goroutine can call d.Trigger()
			// after the trigger channel is closed below.
			wg.Wait()
			d.stop()

			return nil
		},
	}
	// The sole opt-in FBS early-season prior flag. Default off: with the
	// flag unset no prior job is registered, no stored prior row is ever
	// read or written, and every ranking path stays at the exact baseline.
	cmd.Flags().BoolVar(&fbsPriorEnabled, "fbs-early-season-prior", false,
		"automatically freeze and blend a stored FBS preseason prior into scheduled football rankings (default off)")
	// The opt-in women's polling flag. Default off: with the flag unset (and
	// NCAAW_POLLING unset) no women's job is registered and every existing
	// path stays at the exact baseline. The Compose NCAAW_POLLING variable
	// supplies the default so an environment can opt in without touching the
	// command line; the explicit flag still overrides it.
	cmd.Flags().BoolVar(&ncaawPolling, "ncaaw-polling", ncaawPollingDefault(os.Getenv("NCAAW_POLLING")),
		"register scheduled women's basketball jobs (games, team info, season init with retry, rankings) "+
			"(default from NCAAW_POLLING, default off)")
	return cmd, nil
}

// ncaawPollingDefault resolves the --ncaaw-polling flag default from the
// Compose NCAAW_POLLING environment variable so an environment can opt in via
// its .env override; only unambiguous truthy spellings enable polling, and
// anything else (including unset) stays off.
func ncaawPollingDefault(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func sportCommand(
	log *zap.SugaredLogger,
	db *gorm.DB,
	sport espn.Sport,
) (*cobra.Command, error) {
	u, err := newUpdater(log, db, sport)
	if err != nil {
		return nil, err
	}

	use := string(sport.DBSport())
	var short string
	switch sport {
	case espn.CollegeFootball:
		short = "NCAA football one-shot commands"
	case espn.CollegeBasketball:
		short = "NCAA men's basketball one-shot commands"
	case espn.CollegeWomensBasketball:
		short = "NCAA women's basketball one-shot commands"
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
	}

	var gamesAll bool
	var gamesSingle int64
	var gamesYear int64
	gamesCmd := &cobra.Command{
		Use:   "games",
		Short: "One-time game update",
		RunE: func(_ *cobra.Command, _ []string) error {
			if gamesSingle > 0 {
				if err := u.UpdateSingleGame(context.Background(), gamesSingle); err != nil {
					return fmt.Errorf("game %d: %w", gamesSingle, err)
				}
				log.Infof("Game %d updated", gamesSingle)
				return nil
			}

			var result *updater.GameUpdateResult
			var err error
			switch {
			case gamesYear > 0:
				result, err = u.UpdateGamesForYear(context.Background(), gamesYear)
			case gamesAll:
				year, _, _ := time.Now().Date()
				result, err = u.UpdateGamesForYear(context.Background(), int64(year))
			default:
				result, err = u.UpdateCurrentWeek(context.Background())
			}
			if err != nil {
				return err
			}
			log.Infof("Added %d games: %v", len(result.Processed), result.Processed)
			if len(result.Failed) > 0 {
				log.Errorf("Failed to fetch %d games (marked for retry): %v",
					len(result.Failed), result.FailedIDs())
			}
			if len(result.Skipped) > 0 {
				log.Warnf("Protected %d games from reimport (not fetched, no retry): %v",
					len(result.Skipped), result.SkippedIDs())
			}
			return nil
		},
	}
	gamesCmd.Flags().BoolVar(&gamesAll, "all", false, "update all games for the current year")
	gamesCmd.Flags().Int64Var(&gamesSingle, "single", 0, "force update one game by ID")
	gamesCmd.Flags().Int64Var(&gamesYear, "year", 0, "update all games for a specific year")
	gamesCmd.MarkFlagsMutuallyExclusive("all", "single", "year")

	var rankingAll bool
	var rankingPrior bool
	rankingCmd := &cobra.Command{
		Use:   "ranking",
		Short: "One-time ranking update",
		RunE: func(_ *cobra.Command, _ []string) error {
			if rankingPrior && !rankingAll {
				return fmt.Errorf("--fbs-early-season-prior requires --all")
			}
			if rankingAll {
				if rankingPrior {
					return u.UpdateAllRankingsWithPrior()
				}
				return u.UpdateAllRankings()
			}
			return u.UpdateRecentRankings()
		},
	}
	rankingCmd.Flags().BoolVar(&rankingAll, "all", false, "update all rankings")
	if sport == espn.CollegeFootball {
		// The sole opt-in FBS early-season prior flag, shared with `schedule`.
		// It is football-only and only meaningful with --all: plain ranking
		// runs and backfill recomputation stay at the exact baseline.
		rankingCmd.Flags().BoolVar(&rankingPrior, "fbs-early-season-prior", false,
			"blend the freshly recomputed FBS early-season prior into early-season rankings "+
				"and republish the rebuilt priors while rebuilding all rankings (requires --all; default off)")
	}

	teamsCmd := &cobra.Command{
		Use:   "teams",
		Short: "Update team info",
		RunE: func(_ *cobra.Command, _ []string) error {
			addedTeams, err := u.UpdateTeamInfo(context.Background())
			if err != nil {
				return err
			}
			log.Infof("Updated %d teams", addedTeams)
			return nil
		},
	}

	var seasonYear int64
	seasonCmd := &cobra.Command{
		Use:   "season",
		Short: "Update season info",
		RunE: func(_ *cobra.Command, _ []string) error {
			var (
				addedSeasons int
				err          error
			)
			if seasonYear > 0 {
				addedSeasons, err = u.UpdateTeamSeasonsForYear(context.Background(), seasonYear, true)
			} else {
				addedSeasons, err = u.UpdateTeamSeasons(context.Background(), true)
			}
			if err != nil {
				return err
			}
			log.Infof("Added %d seasons", addedSeasons)
			return nil
		},
	}
	seasonCmd.Flags().Int64Var(&seasonYear, "year", 0, "update seasons for a specific year (default: current season)")

	var backfillCmd *cobra.Command
	if sport == espn.CollegeWomensBasketball {
		backfillCmd = ncaawBackfillCommand(u)
	} else {
		backfillCmd = genericBackfillCommand(log, u, use)
	}

	cmd.AddCommand(gamesCmd, rankingCmd, teamsCmd, seasonCmd, backfillCmd)

	return cmd, nil
}

// genericBackfillCommand is the original backfill for ncaaf and ncaam: it
// walks the requested years and republishes every stored season. Women's
// basketball uses the strict ncaawBackfillCommand instead.
func genericBackfillCommand(
	log *zap.SugaredLogger,
	u *updater.Updater,
	use string,
) *cobra.Command {
	var backfillFrom, backfillTo int64
	backfillCmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill games, seasons, and rankings for a range of years",
		Long: `Fetches team seasons and games from ESPN for each year in [from, to],
then recomputes all rankings. Existing records are skipped unless already absent.

Example:
  updater ncaam backfill --from 2021 --to 2025`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if backfillFrom <= 0 || backfillTo <= 0 || backfillFrom > backfillTo {
				return fmt.Errorf("--from and --to must be positive and from <= to")
			}
			for year := backfillFrom; year <= backfillTo; year++ {
				log.Infof("Backfilling %s year %d...", use, year)

				n, err := u.UpdateTeamSeasonsForYear(context.Background(), year, false)
				if err != nil {
					return fmt.Errorf("team seasons %d: %w", year, err)
				}
				log.Infof("  seasons: %d teams", n)

				result, err := u.UpdateGamesForYear(context.Background(), year)
				if err != nil {
					return fmt.Errorf("games %d: %w", year, err)
				}
				log.Infof("  games: %d added", len(result.Processed))
				if len(result.Failed) > 0 {
					log.Errorf("  games: %d failed (marked for retry): %v",
						len(result.Failed), result.FailedIDs())
				}
				if len(result.Skipped) > 0 {
					log.Warnf("  games: %d protected from reimport (no retry): %v",
						len(result.Skipped), result.SkippedIDs())
				}
			}

			log.Infof("Recomputing all %s rankings...", use)
			if err := u.UpdateAllRankings(); err != nil {
				return fmt.Errorf("rankings: %w", err)
			}
			log.Infof("Backfill complete (%s %d–%d)", use, backfillFrom, backfillTo)
			return nil
		},
	}
	backfillCmd.Flags().Int64VarP(&backfillFrom, "from", "f", 0, "first year to backfill (inclusive)")
	backfillCmd.Flags().Int64VarP(&backfillTo, "to", "t", 0, "last year to backfill (inclusive)")
	if err := backfillCmd.MarkFlagRequired("from"); err != nil {
		panic(err)
	}
	if err := backfillCmd.MarkFlagRequired("to"); err != nil {
		panic(err)
	}
	return backfillCmd
}

// ncaawBackfillCommand is the strict women's basketball backfill. It imports
// and audits every requested season-ending year against one validated daily
// ESPN D1 group-50 walk, then publishes rankings only for the verified
// publication window; any audit failure leaves existing ranking rows
// untouched. See updater.BackfillNCAAW for the full contract.
func ncaawBackfillCommand(u *updater.Updater) *cobra.Command {
	var backfillFrom, backfillTo, publishFrom int64
	backfillCmd := &cobra.Command{
		Use:   "backfill",
		Short: "Strict backfill: import, audit, then publish verified women's seasons",
		Long: `Imports and audits women's basketball seasons for every year in [from, to],
then publishes rankings for [publish-from, to] only.

Each season-ending year is validated against ESPN's year-specific flat
calendar and walked one ET day at a time on the D1 group-50 scoreboard;
invalid, empty, or wrong-year calendars, unusable daily responses, summary
failures, score or opponent mismatches, stale or orphan rows, and
unclassified likely D-I teams all fail the run before any ranking write.

--publish-from (default: --to) picks the earliest published year. Earlier
years in the range are imported and audited as lookback input only and stay
out of team_week_results. Publishing a year requires its prior season inside
the audited import range (the ranking's one-season lookback), so exposing an
older year means importing its lookback too.

Examples:
  updater ncaaw backfill --from 2026 --to 2027
      imports and audits 2026 and 2027, publishes 2027 only
  updater ncaaw backfill --from 2025 --to 2026 --publish-from 2026
      publishes 2026 after validating the 2025 lookback`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return u.BackfillNCAAW(context.Background(), backfillFrom, backfillTo, publishFrom)
		},
	}
	backfillCmd.Flags().Int64VarP(&backfillFrom, "from", "f", 0,
		"first season-ending year to import and audit (inclusive)")
	backfillCmd.Flags().Int64VarP(&backfillTo, "to", "t", 0,
		"last season-ending year to import, audit, and publish (inclusive)")
	backfillCmd.Flags().Int64Var(&publishFrom, "publish-from", 0,
		"earliest season-ending year to publish rankings for (default: --to; earlier years stay lookback-only)")
	if err := backfillCmd.MarkFlagRequired("from"); err != nil {
		panic(err)
	}
	if err := backfillCmd.MarkFlagRequired("to"); err != nil {
		panic(err)
	}
	return backfillCmd
}
