package updater

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/sport"
)

func TestNCAAWScheduledSeasonYearUsesEasternDate(t *testing.T) {
	opening := time.Date(2026, time.November, 1, 4, 0, 0, 0, time.UTC)
	if got := ncaawScheduleYear(opening.Add(-time.Minute)); got != 2026 {
		t.Errorf("October 31 Eastern selected %d, want 2026", got)
	}
	if got := ncaawScheduleYear(opening); got != 2027 {
		t.Errorf("November 1 Eastern selected %d, want 2027", got)
	}
	if got := ncaawScheduleYear(time.Date(2027, time.April, 3, 22, 0, 0, 0, time.UTC)); got != 2027 {
		t.Errorf("April 2027 selected %d, want 2027", got)
	}
}

func TestNCAAWScheduledSeasonRetriesPastStaleESPNMetadata(t *testing.T) {
	fx := defaultNCAAWBFFixture()
	fx.currentSeason = 2026 // The plain scoreboard still reports last season.
	fx.calendarReady = &atomic.Bool{}
	u := newNCAAWBackfillUpdater(t, fx)
	stubNow(t, time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC))
	ctx := t.Context()

	if year, err := u.espn.DefaultSeason(ctx); err != nil || year != 2026 {
		t.Fatalf("seeding stale season cache: year %d error %v", year, err)
	}
	old := database.TeamSeason{TeamID: 11, Sport: sport.WomensBasketball, Year: 2026, FBS: 1, Conf: "Big East"}
	if err := u.db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := u.UpdateSingleGame(ctx, 61104); err != nil {
		t.Fatal(err)
	}
	if added, err := u.InitializeScheduledNCAAWSeason(ctx); added != 0 || err == nil ||
		!strings.Contains(err.Error(), "requested for season 2027, got season 2026") {
		t.Fatalf("unready calendar initialized %d rows, error = %v", added, err)
	}
	if wrote, err := u.UpdateScheduledNCAAWRankings(); err != nil || wrote {
		t.Fatalf("ranking before current season membership: wrote %v, error %v", wrote, err)
	}

	fx.calendarReady.Store(true)
	if added, err := u.InitializeScheduledNCAAWSeason(ctx); added != 3 || err != nil {
		t.Fatalf("ready calendar initialized %d rows, want 3: %v", added, err)
	}
	if wrote, err := u.UpdateScheduledNCAAWRankings(); err != nil || wrote {
		t.Fatalf("ranking before first current season game: wrote %v, error %v", wrote, err)
	}
	if rows := ncaawBFRankings(t, u.db, "sport = ?", sport.WomensBasketball); len(rows) != 0 {
		t.Fatalf("initialization unexpectedly published %d ranking rows", len(rows))
	}
	// If a later cron tick requested the calendar again it would now fail.
	fx.calendarReady.Store(false)
	if added, err := u.InitializeScheduledNCAAWSeason(ctx); added != 0 || err != nil {
		t.Fatalf("initialized season should short-circuit: added %d, error %v", added, err)
	}
}

func TestNCAAWScheduledPollPublishesLatestWeek(t *testing.T) {
	u := newWomensBasketballTestUpdater(t)
	stubNow(t, time.Date(2027, time.January, 11, 16, 0, 0, 0, time.UTC))
	seedWomensCrossSportContext(t, u.db)
	if _, err := u.UpdateTeamInfo(t.Context()); err != nil {
		t.Fatal(err)
	}
	if added, err := u.InitializeScheduledNCAAWSeason(t.Context()); added != 3 || err != nil {
		t.Fatalf("season init added %d teams, want 3: %v", added, err)
	}
	polled, err := u.UpdateCurrentWeek(t.Context())
	if err != nil || len(polled.Processed) != 3 {
		t.Fatalf("poll result = %+v, error = %v, want 3 imported games", polled, err)
	}
	if wrote, err := u.UpdateScheduledNCAAWRankings(); err != nil || !wrote {
		t.Fatalf("rank after poll: wrote %v, error %v", wrote, err)
	}
	rows := ncaawBFRankings(t, u.db, "sport = ?", sport.WomensBasketball)
	if len(rows) != 3 {
		t.Fatalf("published %d rows, want 3", len(rows))
	}
	for _, row := range rows {
		if row.Year != 2027 || row.Week != 11 || row.Postseason != 0 || !row.Fbs {
			t.Errorf("invalid scheduled ranking row: %+v", row)
		}
	}
	if rows := ncaawBFRankings(t, u.db, "sport in (?, ?)", sport.Football, sport.Basketball); len(rows) != 0 {
		t.Fatalf("scheduled NCAAW poll published %d ranking rows for other sports", len(rows))
	}
	polled, err = u.UpdateCurrentWeek(t.Context())
	if err != nil || len(polled.Processed) != 0 {
		t.Fatalf("unchanged poll result = %+v, error = %v", polled, err)
	}
}
