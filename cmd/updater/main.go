package main

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
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
	log := logger.NewLogger().Sugar()
	defer log.Sync()

	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		panic(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	rootCmd := &cobra.Command{
		Use:   "updater",
		Short: "College sports data updater",
	}
	rootCmd.SilenceUsage = true

	scheduleCmd := scheduleCommand(log, db, cfg.DeployScript)
	ncaafCmd := sportCommand(log, db, espn.CollegeFootball)
	ncaamCmd := sportCommand(log, db, espn.CollegeBasketball)

	rootCmd.AddCommand(scheduleCmd, ncaafCmd, ncaamCmd)

	rootCmd.Execute() //nolint:errcheck // cobra prints errors; exit code unused
}

func newUpdater(
	log *zap.SugaredLogger,
	db *gorm.DB,
	sport espn.Sport,
) updater.Updater {
	return updater.Updater{
		DB:     db,
		Logger: log,
		ESPN:   espn.NewClientForSport(sport),
	}
}

// deployer runs a deploy script in the background after rankings are updated.
// Calls to Trigger are coalesced: if a deploy is already queued, extra triggers
// are dropped so at most one deploy is pending at a time.
type deployer struct {
	script  string
	log     *zap.SugaredLogger
	trigger chan struct{}
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
	close(d.trigger)
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

// registerJobs adds the three cron jobs for a sport to the scheduler and
// returns a stop function that shuts down the ranking-update goroutine.
func (ss sportSchedule) registerJobs(
	s gocron.Scheduler,
	log *zap.SugaredLogger,
	u updater.Updater,
	d *deployer,
) func() {
	update := make(chan bool, 1)
	stop := make(chan bool, 1)

	go func() {
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
			case <-stop:
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

		addedGames, err := u.UpdateCurrentWeek()
		log.Infof("%s: added %d games: %v", ss.Name, len(addedGames), addedGames)
		if err != nil {
			log.Error(err)
		} else if len(addedGames) > 0 {
			update <- true
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

		addedTeams, err := u.UpdateTeamInfo()
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

		addedSeasons, err := u.UpdateTeamSeasons(false)
		log.Infof("%s: added %d seasons", ss.Name, addedSeasons)
		if err != nil {
			log.Error(err)
		} else if addedSeasons > 0 {
			update <- true
		}
	})); err != nil {
		panic(err)
	}

	return func() { stop <- true }
}

func scheduleCommand(
	log *zap.SugaredLogger,
	db *gorm.DB,
	deployScript string,
) *cobra.Command {
	return &cobra.Command{
		Use:   "schedule",
		Short: "Run the scheduled updater for all sports",
		RunE: func(_ *cobra.Command, _ []string) error {
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

			var stopFuncs []func()
			for _, sp := range sports {
				u := newUpdater(log, db, sp.sport)
				stopFn := sp.schedule.registerJobs(s, log, u, d)
				stopFuncs = append(stopFuncs, stopFn)
			}

			s.Start()

			end := make(chan os.Signal, 1)
			signal.Notify(end, syscall.SIGINT, syscall.SIGTERM)

			<-end
			if err := s.Shutdown(); err != nil {
				log.Error(err)
			}
			for _, fn := range stopFuncs {
				fn()
			}
			d.stop()

			return nil
		},
	}
}

func sportCommand(
	log *zap.SugaredLogger,
	db *gorm.DB,
	sport espn.Sport,
) *cobra.Command {
	u := newUpdater(log, db, sport)

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
	gamesCmd := &cobra.Command{
		Use:   "games",
		Short: "One-time game update",
		RunE: func(_ *cobra.Command, _ []string) error {
			if gamesSingle > 0 {
				if err := u.UpdateSingleGame(gamesSingle); err != nil {
					log.Error(err)
				} else {
					log.Infof("Game %d updated", gamesSingle)
				}
				return nil
			}

			var addedGames []int64
			var err error
			if gamesAll {
				year, _, _ := time.Now().Date()
				addedGames, err = u.UpdateGamesForYear(int64(year))
			} else {
				addedGames, err = u.UpdateCurrentWeek()
			}
			if err != nil {
				log.Error(err)
			} else {
				log.Infof("Added %d games: %v", len(addedGames), addedGames)
			}
			return nil
		},
	}
	gamesCmd.Flags().BoolVar(&gamesAll, "all", false, "update all games for the current year")
	gamesCmd.Flags().Int64Var(&gamesSingle, "single", 0, "force update one game by ID")
	gamesCmd.MarkFlagsMutuallyExclusive("all", "single")

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
			if err != nil {
				log.Error(err)
			}
			return nil
		},
	}
	rankingCmd.Flags().BoolVar(&rankingAll, "all", false, "update all rankings")

	teamsCmd := &cobra.Command{
		Use:   "teams",
		Short: "Update team info",
		RunE: func(_ *cobra.Command, _ []string) error {
			addedTeams, err := u.UpdateTeamInfo()
			if err != nil {
				log.Error(err)
			} else {
				log.Infof("Updated %d teams", addedTeams)
			}
			return nil
		},
	}

	seasonCmd := &cobra.Command{
		Use:   "season",
		Short: "Update season info",
		RunE: func(_ *cobra.Command, _ []string) error {
			addedSeasons, err := u.UpdateTeamSeasons(true)
			if err != nil {
				log.Error(err)
			} else {
				log.Infof("Added %d seasons", addedSeasons)
			}
			return nil
		},
	}

	cmd.AddCommand(gamesCmd, rankingCmd, teamsCmd, seasonCmd)

	return cmd
}
