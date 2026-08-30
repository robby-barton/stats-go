//go:build livecheck

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/robby-barton/stats-go/internal/espn"
)

func main() {
	ctx := context.Background()

	// Football season metadata (site.api scoreboard)
	fc := espn.NewClientForSport(espn.CollegeFootball)
	year, err := fc.DefaultSeason(ctx)
	fmt.Println("ncaaf DefaultSeason:", year, "err:", err)
	weeks, err := fc.GetWeeksInSeason(ctx, 2026)
	fmt.Println("ncaaf GetWeeksInSeason(2026):", weeks, "err:", err)
	started, err := fc.HasPostseasonStarted(ctx, 2026, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	fmt.Println("ncaaf HasPostseasonStarted(2027-01-01):", started, "err:", err)
	cm, err := fc.ConferenceMap(ctx)
	if err != nil {
		fmt.Println("ncaaf ConferenceMap err:", err)
	} else {
		fmt.Println("ncaaf ConferenceMap FBS confs:", len(cm.Conferences[espn.FBS]),
			"FCS confs:", len(cm.Conferences[espn.FCS]),
			"DII subgroups:", len(cm.SubGroups[espn.DII]),
			"DIII subgroups:", len(cm.SubGroups[espn.DIII]))
	}

	// Basketball season metadata (site.api scoreboard)
	bb := &espn.BasketballClient{Client: espn.NewClientForSport(espn.CollegeBasketball).(*espn.BasketballClient).Client}
	bc := bb
	byear, err := bc.DefaultSeason(ctx)
	fmt.Println("ncaam DefaultSeason:", byear, "err:", err)
	bweeks, err := bc.GetWeeksInSeason(ctx, byear)
	fmt.Println("ncaam GetWeeksInSeason:", bweeks, "err:", err)
	dates, err := bc.GetSeasonDatesForYear(ctx, byear)
	if err != nil {
		fmt.Println("ncaam GetSeasonDatesForYear err:", err)
	} else {
		span := "none"
		if len(dates) > 0 {
			span = dates[0] + " .. " + dates[len(dates)-1]
		}
		fmt.Println("ncaam GetSeasonDatesForYear:", len(dates), "dates", span)
	}
	bcm, err := bc.ConferenceMap(ctx)
	if err != nil {
		fmt.Println("ncaam ConferenceMap err:", err)
	} else {
		fmt.Println("ncaam ConferenceMap D1 confs:", len(bcm.Conferences[espn.D1Basketball]))
	}
	os.Exit(0)
}
