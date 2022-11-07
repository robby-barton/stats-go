package main

import (
	// "os"
	// "os/signal"
	// "syscall"

	"fmt"
	"time"

	"github.com/robby-barton/stats-api/internal/updater"

	"github.com/go-co-op/gocron"
)

func main() {
	u, err := updater.NewUpdater()
	if err != nil {
		panic(err)
	}
	sqlDB, _ := u.DB.DB()
	defer sqlDB.Close()

	s := gocron.NewScheduler(time.Local)

	// update games at 5:30 AM every day
	s.Every(1).Day().At("05:30").Do(func(){
		start := time.Now()
		err = u.UpdateGamesForYear(2022)
		fmt.Println(err)

		duration := time.Since(start)
		fmt.Println(duration)
	})

	s.StartBlocking()
}
