package main

import (
	"github.com/robby-barton/stats-go/internal/server"
)

func main() {
	s, err := server.NewServer()
	if err != nil {
		panic(err)
	}

	// make sure to close underlying sql connection
	sqlDB, _ := s.DB.DB()
	defer sqlDB.Close()

	s.APIServer.Run()
}
