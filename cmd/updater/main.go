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
	logger := logger.NewLogger()
	defer logger.Sync()
	sugar := logger.Sugar()

	var scheduled, games, rank, all, team bool

	flag.BoolVar(&scheduled, "s", false, "run scheduler")
	flag.BoolVar(&games, "g", false, "one-time game update")
	flag.BoolVar(&rank, "r", false, "one-time ranking update")
	flag.BoolVar(&all, "a", false, "update all rankings or games")
	flag.BoolVar(&team, "t", false, "update team info")
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
		Logger: sugar,
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
								sugar.Errorf("panic caught: %s", r)
							}
						}()

						err = u.UpdateRecentRankings()
						if err != nil {
							sugar.Error(err)
						} else {
							sugar.Info("rankings updated")
						}
					}()
				case <-stop:
					return
				}
			}
		}()

		s.Cron("*/5 * * 1,8-12 *").Do(func() {
			defer func() {
				if r := recover(); r != nil {
					sugar.Errorf("panic caught: %s", r)
				}
			}()

			addedGames, err := u.UpdateCurrentWeek()

			sugar.Infof("Added %d games\n", addedGames)
			if addedGames > 0 {
				update <- true
			} else if err != nil {
				sugar.Error(err)
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
				sugar.Error(err)
			} else {
				sugar.Infof("Added %d games\n", addedGames)
			}
		}
		if rank {
			if all {
				err = u.UpdateAllRankings()
			} else {
				err = u.UpdateRecentRankings()
			}
			if err != nil {
				sugar.Error(err)
			}
		}
		if team {
			addedTeams, err := u.UpdateTeamInfo()
			if err != nil {
				sugar.Error(err)
			} else {
				sugar.Infof("Updated %d teams\n", addedTeams)
			}
		}
	}
}
