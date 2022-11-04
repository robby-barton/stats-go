package main

import (
	// "os"
	// "os/signal"
	// "syscall"
	// "time"

	"encoding/json"
	"fmt"

	"github.com/robby-barton/stats-api/internal/games"
)

func main() {
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

	weekGames, err := games.GetWeek(2022, 2, games.Regular, games.FBS)
	fmt.Println(weekGames)

	body, err := games.GetGameStats(401403869)

	jsonBody, _ := json.MarshalIndent(body, "", "    ")
	fmt.Println(string(jsonBody))
	fmt.Println(err)
}
