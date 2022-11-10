package main

import (
	"flag"
	"fmt"
	"time"

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

	r, err := ranking.NewRanker(nil)
	if err != nil {
		panic(err)
	}
	sqlDB, _ := r.DB.DB()
	defer sqlDB.Close()

	start := time.Now()
	div, err := r.CalculateRanking(ranking.CalculateRankingParams{
		Year: year,
		Week: week,
		Fcs:  fcs,
	})
	duration := time.Since(start)

	// sanitize input
	if top <= 0 || top > len(div) {
		top = len(div)
	}

	if rating {
		ranking.PrintSRS(div, top)
	} else {
		ranking.PrintRankings(div, top)
	}
	fmt.Println(err)
	fmt.Println(duration)
}
