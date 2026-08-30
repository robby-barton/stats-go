package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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

	rootCmd.AddCommand(scheduleCmd, ncaafCmd, ncaamCmd)

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

// deployer runs a deploy script in the background after rankings are updated.
// Calls to Trigger are coalesced: if a deploy is already queued, extra triggers
// are dropped so at most one deploy is pending at a time.
// stop() closes the trigger channel; it must only be called after all producers
// (ranking workers) have been joined, otherwise Trigger could send on a closed
// channel. stop is idempotent.
type deployer struct {
	script   string
	log      *zap.SugaredLogger
	trigger  chan struct{}
	stopOnce sync.Once
}

func newDeployer(log *zap.SugaredLogger, script string) *deployer {
	d := &deployer{
		script:  script,
		log:     log,
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
		//nolint:gosec // DEPLOY_SCRIPT is operator-supplied config, not user input
		cmd := exec.CommandContext(context.Background(), d.script)
		out, err := cmd.CombinedOutput()
		if err != nil {
			d.log.Errorf("deploy script failed: %v\n%s", err, out)
			continue
		}
		d.log.Infof("deploy script completed:\n%s", out)
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
// the ranking-update worker goroutine. The worker runs until ctx is cancelled;
// wg is released once the worker has exited so callers can join it before
// shutting down the deployer (whose only Trigger producer is this worker).
func (ss sportSchedule) registerJobs(
	ctx context.Context,
	s gocron.Scheduler,
	log *zap.SugaredLogger,
	u *updater.Updater,
	d *deployer,
	wg *sync.WaitGroup,
) {
	update := make(chan bool, 1)

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

					if err := u.UpdateRecentRankings(); err != nil {
						log.Error(err)
						return
					}
					log.Infof("%s rankings updated", ss.Name)
					d.Trigger()
				}()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Completed games poll
	if _, err := s.NewJob(gocron.CronJob(ss.GamesCron, false), gocron.NewTask(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("%s panic caught: %s", ss.Name, r)
			}
		}()

		result, err := u.UpdateCurrentWeek(ctx)
		if err != nil {
			log.Error(err)
		} else {
			log.Infof("%s: added %d games: %v", ss.Name, len(result.Processed), result.Processed)
			if len(result.Failed) > 0 {
				log.Errorf("%s: failed to fetch %d games (marked for retry): %v",
					ss.Name, len(result.Failed), result.FailedIDs())
			}
			if len(result.Processed) > 0 {
				update <- true
			}
		}
	})); err != nil {
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
			return
		}

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

		addedSeasons, err := u.UpdateTeamSeasons(ctx, false)
		log.Infof("%s: added %d seasons", ss.Name, addedSeasons)
		if err != nil {
			log.Error(err)
		} else if addedSeasons > 0 {
			update <- true
		}
	})); err != nil {
		panic(err)
	}
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

			d := newDeployer(log, deployScript)

			sports := []struct {
				schedule sportSchedule
				sport    espn.Sport
			}{
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

			var wg sync.WaitGroup
			for _, sp := range sports {
				u, err := newUpdater(log, db, sp.sport)
				if err != nil {
					return err
				}
				sp.schedule.registerJobs(ctx, s, log, u, d, &wg)
			}

			s.Start()

			end := make(chan os.Signal, 1)
			signal.Notify(end, syscall.SIGINT, syscall.SIGTERM)

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
	return cmd, nil
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

	use := "ncaaf"
	short := "NCAA football one-shot commands"
	if sport == espn.CollegeBasketball {
		use = "ncaam"
		short = "NCAA men's basketball one-shot commands"
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
			return nil
		},
	}
	gamesCmd.Flags().BoolVar(&gamesAll, "all", false, "update all games for the current year")
	gamesCmd.Flags().Int64Var(&gamesSingle, "single", 0, "force update one game by ID")
	gamesCmd.Flags().Int64Var(&gamesYear, "year", 0, "update all games for a specific year")
	gamesCmd.MarkFlagsMutuallyExclusive("all", "single", "year")

	var rankingAll bool
	rankingCmd := &cobra.Command{
		Use:   "ranking",
		Short: "One-time ranking update",
		RunE: func(_ *cobra.Command, _ []string) error {
			var err error
			if rankingAll {
				err = u.UpdateAllRankings()
			} else {
				err = u.UpdateRecentRankings()
			}
			return err
		},
	}
	rankingCmd.Flags().BoolVar(&rankingAll, "all", false, "update all rankings")

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

	cmd.AddCommand(gamesCmd, rankingCmd, teamsCmd, seasonCmd, backfillCmd)

	return cmd, nil
}
