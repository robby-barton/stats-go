package updater

import (
	"context"
	"errors"
	"time"

	"github.com/robby-barton/stats-go/internal/ranking/load"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ncaawScheduleYear returns the season-ending year. Scheduled calls are
// confined to November–April Eastern; November starts the next season even
// when ESPN's default scoreboard still advertises the previous one.
func ncaawScheduleYear(now time.Time) int64 {
	today := now.In(ncaawETLoc())
	year := int64(today.Year())
	if today.Month() >= time.November {
		year++
	}
	return year
}

// InitializeScheduledNCAAWSeason imports the expected season if not yet
// present. Once initialized, subsequent startup-window ticks do no API work.
func (u *Updater) InitializeScheduledNCAAWSeason(ctx context.Context) (int, error) {
	if u.sportDB() != sport.WomensBasketball {
		return 0, errors.New("scheduled NCAAW season initialization requires a women's basketball updater")
	}
	return u.UpdateTeamSeasonsForYear(ctx, ncaawScheduleYear(nowFunc()), false)
}

// UpdateScheduledNCAAWRankings publishes the current season's latest snapshot.
// Neither games without membership nor membership without games can write a
// snapshot or trigger a deploy. Never fall back to the previous season.
func (u *Updater) UpdateScheduledNCAAWRankings() (bool, error) {
	if u.sportDB() != sport.WomensBasketball {
		return false, errors.New("scheduled NCAAW ranking requires a women's basketball updater")
	}
	year := ncaawScheduleYear(nowFunc())
	exists, err := u.seasonsExist(year)
	if err != nil || !exists {
		return false, err
	}
	rows, err := u.rankingForWeek(year, 0, nil)
	if errors.Is(err, load.ErrNoRankableGames) {
		return false, nil // normal before the first completed game
	}
	if err != nil || len(rows) == 0 {
		return false, err
	}
	if err := u.insertRankingsToDB(rows); err != nil {
		return false, err
	}
	return true, nil
}
