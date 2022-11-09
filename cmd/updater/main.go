package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/robby-barton/stats-api/internal/updater"

	"github.com/go-co-op/gocron"
)

func main() {
	var scheduled, games bool

	flag.BoolVar(&scheduled, "s", false, "run scheduler")
	flag.BoolVar(&games, "g", false, "one-time game update")
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
	}
}
