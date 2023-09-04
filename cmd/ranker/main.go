package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
)

func main() {
	// args
	var year, week int64
	var top int
	var fcs, rating bool
	flag.Int64Var(&year, "y", 0, "ranking year")
	flag.Int64Var(&week, "w", 0, "ranking week")
	flag.IntVar(&top, "t", 0, "print top N teams")
	flag.BoolVar(&fcs, "f", false, "rank FCS")
	flag.BoolVar(&rating, "r", false, "print rating")
	flag.Parse()

	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		panic(err)
	}

	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	r := ranking.Ranker{
		DB:   db,
		Year: year,
		Week: week,
		Fcs:  fcs,
	}

	start := time.Now()
	div, err := r.CalculateRanking()
	duration := time.Since(start)

	// sanitize input
	if top <= 0 || top > len(div) {
		top = len(div)
	}

	if rating {
		r.PrintSRS(div, top)
	} else {
		r.PrintRankings(div, top)
	}
	fmt.Println(err)      //nolint:forbidigo // allow
	fmt.Println(duration) //nolint:forbidigo // allow
}
