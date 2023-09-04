package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/logger"
	"github.com/robby-barton/stats-go/internal/updater"

	"github.com/go-co-op/gocron"
)

func main() {
	logger := logger.NewLogger().Sugar()
	defer logger.Sync()

	var scheduled, games, rank, all, team, season bool

	flag.BoolVar(&scheduled, "s", false, "run scheduler")
	flag.BoolVar(&games, "g", false, "one-time game update")
	flag.BoolVar(&rank, "r", false, "one-time ranking update")
	flag.BoolVar(&all, "a", false, "update all rankings or games")
	flag.BoolVar(&team, "t", false, "update team info")
	flag.BoolVar(&season, "y", false, "update season info")
	flag.Parse()

	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		panic(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	u := updater.Updater{
		DB:     db,
		Logger: logger,
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

						err = u.UpdateRecentRankings()
						if err != nil {
							logger.Error(err)
						} else {
							logger.Info("rankings updated")
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

			addedGames, err := u.UpdateCurrentWeek()

			logger.Infof("Added %d games", addedGames)
			if addedGames > 0 {
				update <- true
			} else if err != nil {
				logger.Error(err)
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

			addedTeams, err := u.UpdateTeamInfo()
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Updated %d teams", addedTeams)
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

			addedSeasons, err := u.UpdateTeamSeasons(false)
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Added %d seasons", addedSeasons)
			}
		})

		s.StartAsync()

		end := make(chan os.Signal, 1)
		signal.Notify(end, syscall.SIGINT, syscall.SIGTERM)

		<-end
		s.Stop()
		stop <- true
	} else {
		var err error
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
			addedTeams, err := u.UpdateTeamInfo()
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Updated %d teams", addedTeams)
			}
		}
		if season {
			addedSeasons, err := u.UpdateTeamSeasons(true)
			if err != nil {
				logger.Error(err)
			} else {
				logger.Infof("Added %d seasons", addedSeasons)
			}
		}
	}
}
