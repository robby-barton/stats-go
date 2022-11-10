package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/robby-barton/stats-go/internal/updater"

	"github.com/go-co-op/gocron"
)

func main() {
	var scheduled, games, rank, rankAll bool

	flag.BoolVar(&scheduled, "s", false, "run scheduler")
	flag.BoolVar(&games, "g", false, "one-time game update")
	flag.BoolVar(&rank, "r", false, "one-time ranking update")
	flag.BoolVar(&rankAll, "a", false, "one-time update of all rankings")
	flag.Parse()

	u, err := updater.NewUpdater()
	if err != nil {
		panic(err)
	}
	sqlDB, _ := u.DB.DB()
	defer sqlDB.Close()

	if scheduled {
		s := gocron.NewScheduler(time.Local)

		// update games at 5:30 AM every day
		s.Every(1).Day().At("05:30").Do(func() {
			start := time.Now()
			err = u.UpdateGamesForYear(2022)
			fmt.Println(err)

			duration := time.Since(start)
			fmt.Println(duration)
		})

		s.StartBlocking()
	} else {
		if games {
			err = u.UpdateGamesForYear(2022)
			fmt.Println(err)
		}
		if rank {
			err = u.UpdateRecentRankings()
			fmt.Println(err)
		}
		if rankAll {
			err = u.UpdateAllRankings()
			fmt.Println(err)
		}
	}
}
