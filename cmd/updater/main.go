package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-co-op/gocron"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/logger"
	"github.com/robby-barton/stats-go/internal/updater"
	"github.com/robby-barton/stats-go/internal/writer"
)

func main() {
	logger := logger.NewLogger().Sugar()
	defer logger.Sync()

	var scheduled, games, rank, all, team, season, json bool
	var singleGame int64

	flag.BoolVar(&scheduled, "schedule", false, "run scheduler")
	flag.BoolVar(&games, "games", false, "one-time game update")
	flag.Int64Var(&singleGame, "single", 0, "force update one game")
	flag.BoolVar(&rank, "ranking", false, "one-time ranking update")
	flag.BoolVar(&all, "all", false, "update all rankings or games")
	flag.BoolVar(&team, "teams", false, "update team info")
	flag.BoolVar(&season, "season", false, "update season info")
	flag.BoolVar(&json, "json", false, "rewrite json")
	flag.Parse()

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
		Logger: logger,
		Writer: doWriter,
	}

	if scheduled {
		s := gocron.NewScheduler(time.Local)

		update := make(chan bool, 1)
		stop := make(chan bool, 1)
		go func() {
			for {
				select {
				case <-update:
					func() {
						defer func() {
							if r := recover(); r != nil {
								logger.Errorf("panic caught: %s", r)
							}
						}()

						if err := u.UpdateRecentRankings(); err != nil {
							logger.Error(err)
							return
						}
						logger.Info("rankings updated")

						if !cfg.Local {
							if err := u.UpdateRecentJSON(); err != nil {
								logger.Error(err)
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
		s.Cron("*/5 * * 1,8-12 *").Do(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("panic caught: %s", r)
				}
			}()

			var addedGames int
			addedGames, err = u.UpdateCurrentWeek()

			logger.Infof("Added %d games", addedGames)
			if err != nil {
				logger.Error(err)
			} else if addedGames > 0 {
				update <- true
			}
		})

		// Update team info
		// 5 am Sunday from August through January
		s.Cron("0 5 * 1,8-12 0").Do(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("panic caught: %s", r)
				}
			}()

			var addedTeams int
			addedTeams, err = u.UpdateTeamInfo()
			if err != nil {
				logger.Error(err)
				return
			}

			logger.Infof("Updated %d teams", addedTeams)
			if err := u.UpdateTeamsJSON(nil); err != nil {
				logger.Error(err)
			}
			if err := u.Writer.PurgeCache(context.Background()); err != nil {
				logger.Error(err)
			}
		})

		// Add new season
		// 6 am on August 10th
		s.Cron("0 6 10 8 *").Do(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("panic caught: %s", r)
				}
			}()

			var addedSeasons int
			addedSeasons, err = u.UpdateTeamSeasons(false)

			logger.Infof("Added %d seasons", addedSeasons)
			if err != nil {
				logger.Error(err)
			} else if addedSeasons > 0 {
				update <- true
			}
		})

		s.StartAsync()

		end := make(chan os.Signal, 1)
		signal.Notify(end, syscall.SIGINT, syscall.SIGTERM)

		<-end
		s.Stop()
		stop <- true
	} else {
		if games {
			var addedGames int
			if all {
				year, _, _ := time.Now().Date()
				addedGames, err = u.UpdateGamesForYear(int64(year))
			} else {
				addedGames, err = u.UpdateCurrentWeek()
			}
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Added %d games", addedGames)
			}
		}
		if singleGame > 0 {
			if err := u.UpdateSingleGame(singleGame); err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Game %d updated", singleGame)
			}
		}
		if rank {
			if all {
				err = u.UpdateAllRankings()
			} else {
				err = u.UpdateRecentRankings()
			}
			if err != nil {
				logger.Error(err)
			}
		}
		if team {
			var addedTeams int
			addedTeams, err = u.UpdateTeamInfo()
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Updated %d teams", addedTeams)
			}
		}
		if season {
			var addedSeasons int
			addedSeasons, err = u.UpdateTeamSeasons(true)
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Added %d seasons", addedSeasons)
			}
		}
		if json {
			if all {
				err = u.UpdateAllJSON()
			} else {
				err = u.UpdateRecentJSON()
			}
			if err != nil {
				logger.Error(err)
			}
		}
	}
}
