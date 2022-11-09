package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/robby-barton/stats-api/internal/ranking"
)

func main() {
	var year, week int64
	var fcs bool
	flag.Int64Var(&year, "y", 0, "ranking year")
	flag.Int64Var(&week, "w", 0, "ranking week")
	flag.BoolVar(&fcs, "f", false, "rank FCS")
	flag.Parse()

	r, err := ranking.NewRanker()
	if err != nil {
		panic(err)
	}
	sqlDB, _ := r.DB.DB()
	defer sqlDB.Close()

	start := time.Now()
	fbs, err := r.CalculateRanking(ranking.CalculateRankingParams{
		Year: year,
		Week: week,
		Fbs:  !fcs,
	})
	duration := time.Since(start)

	ranking.PrintRankings(fbs)
	fmt.Println(err)
	fmt.Println(duration)
}
