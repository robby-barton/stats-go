package main

import (
	"github.com/robby-barton/stats-api/internal/server"
)

func main() {
	s, err := server.NewServer()
	if err != nil {
		panic(err)
	}
	defer s.DB.Close()

	s.APIServer.Run()
}
