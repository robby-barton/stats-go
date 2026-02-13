package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/spf13/cobra"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
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
	u := updater.Updater{
		DB:     db,
		Logger: log,
		Writer: doWriter,
	}

	rootCmd := &cobra.Command{
		Use:   "updater",
		Short: "College football data updater",
	}
	rootCmd.SilenceUsage = true

	scheduleCmd := &cobra.Command{
		Use:   "schedule",
		Short: "Run the scheduled updater",
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := gocron.NewScheduler(gocron.WithLocation(time.Local))
			if err != nil {
				panic(err)
			}

			update := make(chan bool, 1)
			stop := make(chan bool, 1)
			go func() {
				for {
					select {
					case <-update:
						func() {
							defer func() {
								if r := recover(); r != nil {
									log.Errorf("panic caught: %s", r)
								}
							}()

							if err := u.UpdateRecentRankings(); err != nil {
								log.Error(err)
								return
							}
							log.Info("rankings updated")

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

			// Update completed games
			// every 5 minutes from August through January
			_, err = s.NewJob(gocron.CronJob("*/5 * * 1,8-12 *", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("panic caught: %s", r)
					}
				}()

				var addedGames []int64
				addedGames, err = u.UpdateCurrentWeek()

				log.Infof("Added %d games: %v", len(addedGames), addedGames)
				if err != nil {
					log.Error(err)
				} else if len(addedGames) > 0 {
					update <- true
				}
			}))
			if err != nil {
				panic(err)
			}

			// Update team info
			// 5 am Sunday from August through January
			_, err = s.NewJob(gocron.CronJob("0 5 * 1,8-12 0", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("panic caught: %s", r)
					}
				}()

				var addedTeams int
				addedTeams, err = u.UpdateTeamInfo()
				if err != nil {
					log.Error(err)
					return
				}

				log.Infof("Updated %d teams", addedTeams)
				if !cfg.Local {
					if err := u.UpdateTeamsJSON(nil); err != nil {
						log.Error(err)
					}
					if err := u.Writer.PurgeCache(context.Background()); err != nil {
						log.Error(err)
					}
				}
			}))
			if err != nil {
				panic(err)
			}

			// Add new season
			// 6 am on August 10th
			_, err = s.NewJob(gocron.CronJob("0 6 10 8 *", false), gocron.NewTask(func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("panic caught: %s", r)
					}
				}()

				var addedSeasons int
				addedSeasons, err = u.UpdateTeamSeasons(false)

				log.Infof("Added %d seasons", addedSeasons)
				if err != nil {
					log.Error(err)
				} else if addedSeasons > 0 {
					update <- true
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
			stop <- true

			return nil
		},
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

	rootCmd.AddCommand(scheduleCmd, gamesCmd, rankingCmd, teamsCmd, seasonCmd, jsonCmd)

	rootCmd.Execute() //nolint:errcheck // cobra prints errors; exit code unused
}
