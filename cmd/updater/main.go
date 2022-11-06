package main

import (
	// "os"
	// "os/signal"
	// "syscall"

	"fmt"
	"time"

	"github.com/robby-barton/stats-api/internal/updater"
)

func main() {
	start := time.Now()

	// ticker := time.NewTicker(24 * time.Hour)
	// quit := make(chan bool)
	// go func() {
	// 	for {
	// 		select {
	// 		case <- ticker.C:
	// 			println("Test")
	// 		case <- quit:
	// 			ticker.Stop()
	// 			return
	// 		}
	// 	}
	// }()

	// sigc := make(chan os.Signal, 1)
	// signal.Notify(
	// 	sigc,
	// 	syscall.SIGINT,
	// 	syscall.SIGTERM,
	// 	syscall.SIGQUIT,
	// )
	// <- sigc
	// quit <- true

	u, err := updater.NewUpdater()
	if err != nil {
		panic(err)
	}

	err = u.UpdateGamesForYear(2022)
	fmt.Println(err)

	duration := time.Since(start)
	fmt.Println(duration)
}
