package main

import (
	"context"
	"os"
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
	"github.com/robby-barton/stats-go/internal/writer"
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

	doWriter, err := writer.NewDigitalOceanWriter(cfg.DOConfig)
	if err != nil {
		panic(err)
	}

	rootCmd := &cobra.Command{
		Use:   "updater",
		Short: "College sports data updater",
	}
	rootCmd.SilenceUsage = true

	scheduleCmd := scheduleCommand(log, cfg, db, doWriter)
	footballCmd := sportCommand(log, db, doWriter, espn.CollegeFootball)
	basketballCmd := sportCommand(log, db, doWriter, espn.CollegeBasketball)

	rootCmd.AddCommand(scheduleCmd, footballCmd, basketballCmd)

	rootCmd.Execute() //nolint:errcheck // cobra prints errors; exit code unused
}

func newUpdater(
	log *zap.SugaredLogger,
	db *gorm.DB,
	w writer.Writer,
	sport espn.Sport,
) updater.Updater {
	return updater.Updater{
		DB:     db,
		Logger: log,
		Writer: w,
		ESPN:   espn.NewClientForSport(sport),
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
	cfg *config.Config,
	u updater.Updater,
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

					if !cfg.Local {
						if err := u.UpdateRecentJSON(); err != nil {
							log.Error(err)
						}
					}
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
		if !cfg.Local {
			if err := u.UpdateTeamsJSON(nil); err != nil {
				log.Error(err)
			}
			if err := u.Writer.PurgeCache(context.Background()); err != nil {
				log.Error(err)
			}
		}
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
	cfg *config.Config,
	db *gorm.DB,
	w writer.Writer,
) *cobra.Command {
	return &cobra.Command{
		Use:   "schedule",
		Short: "Run the scheduled updater for all sports",
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := gocron.NewScheduler(gocron.WithLocation(time.Local))
			if err != nil {
				panic(err)
			}

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
						Name:          "ncaambb",
						GamesCron:     "*/5 * * 1-4,11-12 *",
						TeamInfoCron:  "0 5 * 1-4,11-12 0",
						NewSeasonCron: "0 6 1 11 *",
					},
					sport: espn.CollegeBasketball,
				},
			}

			var stopFuncs []func()
			for _, sp := range sports {
				u := newUpdater(log, db, w, sp.sport)
				stopFn := sp.schedule.registerJobs(s, log, cfg, u)
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

			return nil
		},
	}
}

func sportCommand(
	log *zap.SugaredLogger,
	db *gorm.DB,
	w writer.Writer,
	sport espn.Sport,
) *cobra.Command {
	u := newUpdater(log, db, w, sport)

	use := "ncaaf"
	short := "NCAA football one-shot commands"
	if sport == espn.CollegeBasketball {
		use = "ncaambb"
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

	var jsonAll bool
	jsonCmd := &cobra.Command{
		Use:   "json",
		Short: "Rewrite JSON output",
		RunE: func(_ *cobra.Command, _ []string) error {
			var err error
			if jsonAll {
				err = u.UpdateAllJSON()
			} else {
				err = u.UpdateRecentJSON()
			}
			if err != nil {
				log.Error(err)
			}
			return nil
		},
	}
	jsonCmd.Flags().BoolVar(&jsonAll, "all", false, "rewrite all JSON")

	cmd.AddCommand(gamesCmd, rankingCmd, teamsCmd, seasonCmd, jsonCmd)

	return cmd
}
