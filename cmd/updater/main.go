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
		Sport:  sport,
	}
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
			cfb := newUpdater(log, db, w, espn.CollegeFootball)
			cbb := newUpdater(log, db, w, espn.CollegeBasketball)

			s, err := gocron.NewScheduler(gocron.WithLocation(time.Local))
			if err != nil {
				panic(err)
			}

			// --- Football update channel ---
			cfbUpdate := make(chan bool, 1)
			cfbStop := make(chan bool, 1)
			go func() {
				for {
					select {
					case <-cfbUpdate:
						func() {
							defer func() {
								if r := recover(); r != nil {
									log.Errorf("football panic caught: %s", r)
								}
							}()

							if err := cfb.UpdateRecentRankings(); err != nil {
								log.Error(err)
								return
							}
							log.Info("football rankings updated")

							if !cfg.Local {
								if err := cfb.UpdateRecentJSON(); err != nil {
									log.Error(err)
								}
							}
						}()
					case <-cfbStop:
						return
					}
				}
			}()

			// --- Basketball update channel ---
			cbbUpdate := make(chan bool, 1)
			cbbStop := make(chan bool, 1)
			go func() {
				for {
					select {
					case <-cbbUpdate:
						func() {
							defer func() {
								if r := recover(); r != nil {
									log.Errorf("basketball panic caught: %s", r)
								}
							}()

							if err := cbb.UpdateRecentRankings(); err != nil {
								log.Error(err)
								return
							}
							log.Info("basketball rankings updated")

							if !cfg.Local {
								if err := cbb.UpdateRecentJSON(); err != nil {
									log.Error(err)
								}
							}
						}()
					case <-cbbStop:
						return
					}
				}
			}()

			// Football: completed games every 5 min, Aug–Jan
			_, err = s.NewJob(gocron.CronJob("*/5 * * 1,8-12 *", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("football panic caught: %s", r)
					}
				}()

				addedGames, err := cfb.UpdateCurrentWeek()
				log.Infof("football: added %d games: %v", len(addedGames), addedGames)
				if err != nil {
					log.Error(err)
				} else if len(addedGames) > 0 {
					cfbUpdate <- true
				}
			}))
			if err != nil {
				panic(err)
			}

			// Football: team info, 5 am Sunday Aug–Jan
			_, err = s.NewJob(gocron.CronJob("0 5 * 1,8-12 0", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("football panic caught: %s", r)
					}
				}()

				addedTeams, err := cfb.UpdateTeamInfo()
				if err != nil {
					log.Error(err)
					return
				}

				log.Infof("football: updated %d teams", addedTeams)
				if !cfg.Local {
					if err := cfb.UpdateTeamsJSON(nil); err != nil {
						log.Error(err)
					}
					if err := cfb.Writer.PurgeCache(context.Background()); err != nil {
						log.Error(err)
					}
				}
			}))
			if err != nil {
				panic(err)
			}

			// Football: new season, 6 am Aug 10
			_, err = s.NewJob(gocron.CronJob("0 6 10 8 *", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("football panic caught: %s", r)
					}
				}()

				addedSeasons, err := cfb.UpdateTeamSeasons(false)
				log.Infof("football: added %d seasons", addedSeasons)
				if err != nil {
					log.Error(err)
				} else if addedSeasons > 0 {
					cfbUpdate <- true
				}
			}))
			if err != nil {
				panic(err)
			}

			// Basketball: completed games every 5 min, Nov–Apr
			_, err = s.NewJob(gocron.CronJob("*/5 * * 1-4,11-12 *", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("basketball panic caught: %s", r)
					}
				}()

				addedGames, err := cbb.UpdateCurrentWeek()
				log.Infof("basketball: added %d games: %v", len(addedGames), addedGames)
				if err != nil {
					log.Error(err)
				} else if len(addedGames) > 0 {
					cbbUpdate <- true
				}
			}))
			if err != nil {
				panic(err)
			}

			// Basketball: team info, 5 am Sunday Nov–Apr
			_, err = s.NewJob(gocron.CronJob("0 5 * 1-4,11-12 0", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("basketball panic caught: %s", r)
					}
				}()

				addedTeams, err := cbb.UpdateTeamInfo()
				if err != nil {
					log.Error(err)
					return
				}

				log.Infof("basketball: updated %d teams", addedTeams)
				if !cfg.Local {
					if err := cbb.UpdateTeamsJSON(nil); err != nil {
						log.Error(err)
					}
					if err := cbb.Writer.PurgeCache(context.Background()); err != nil {
						log.Error(err)
					}
				}
			}))
			if err != nil {
				panic(err)
			}

			// Basketball: new season, 6 am Nov 1
			_, err = s.NewJob(gocron.CronJob("0 6 1 11 *", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("basketball panic caught: %s", r)
					}
				}()

				addedSeasons, err := cbb.UpdateTeamSeasons(false)
				log.Infof("basketball: added %d seasons", addedSeasons)
				if err != nil {
					log.Error(err)
				} else if addedSeasons > 0 {
					cbbUpdate <- true
				}
			}))
			if err != nil {
				panic(err)
			}

			s.Start()

			end := make(chan os.Signal, 1)
			signal.Notify(end, syscall.SIGINT, syscall.SIGTERM)

			<-end
			if err := s.Shutdown(); err != nil {
				log.Error(err)
			}
			cfbStop <- true
			cbbStop <- true

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

	use := "football"
	short := "College football one-shot commands"
	if sport == espn.CollegeBasketball {
		use = "basketball"
		short = "College basketball one-shot commands"
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
