package espn

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"
)

// BasketballClient wraps a shared *Client with basketball-specific season logic.
type BasketballClient struct {
	*Client
	seasonMu     sync.Mutex
	seasonCached bool
	cachedSeason int64
}

// Compile-time interface check.
var _ SportClient = (*BasketballClient)(nil)

// DefaultSeason returns the current ESPN season year. Only successful lookups
// are cached; a transient failure is retried on the next call.
func (bc *BasketballClient) DefaultSeason(ctx context.Context) (int64, error) {
	bc.seasonMu.Lock()
	defer bc.seasonMu.Unlock()

	if bc.seasonCached {
		return bc.cachedSeason, nil
	}

	sb, err := bc.GetScoreboard(ctx)
	if err != nil {
		return 0, err
	}
	bc.cachedSeason = sb.Leagues[0].Season.Year
	bc.seasonCached = true
	return bc.cachedSeason, nil
}

// validateCurrentSeason returns an error if year does not match the current ESPN season.
// Used only for methods that have no historical equivalent (GetWeeksInSeason,
// HasPostseasonStarted) and are only called by the current-season scheduler.
func (bc *BasketballClient) validateCurrentSeason(ctx context.Context, year int64) error {
	current, err := bc.DefaultSeason(ctx)
	if err != nil {
		return err
	}
	if year != current {
		return fmt.Errorf("basketball only supports current season (%d), got year %d", current, year)
	}
	return nil
}

func (bc *BasketballClient) GetWeeksInSeason(ctx context.Context, year int64) (int64, error) {
	if err := bc.validateCurrentSeason(ctx, year); err != nil {
		return 0, err
	}
	return bc.getWeeksInSeasonFromScoreboard(ctx)
}

func (bc *BasketballClient) getWeeksInSeasonFromScoreboard(ctx context.Context) (int64, error) {
	sb, err := bc.GetScoreboard(ctx)
	if err != nil {
		return 0, err
	}

	season := sb.Leagues[0].Season
	start, err := time.Parse("2006-01-02T15:04Z", season.StartDate)
	if err != nil {
		return 0, fmt.Errorf("parsing season start date: %w", err)
	}
	end, err := time.Parse("2006-01-02T15:04Z", season.EndDate)
	if err != nil {
		return 0, fmt.Errorf("parsing season end date: %w", err)
	}

	days := end.Sub(start).Hours() / 24
	weeks := int64(days/7) + 1
	return weeks, nil
}

func (bc *BasketballClient) HasPostseasonStarted(ctx context.Context, year int64, _ time.Time) (bool, error) {
	if err := bc.validateCurrentSeason(ctx, year); err != nil {
		return false, err
	}
	sb, err := bc.GetScoreboard(ctx)
	if err != nil {
		return false, err
	}
	return sb.Leagues[0].Season.Type.ID >= int64(Postseason), nil
}

// historicalSeasonDates generates all calendar dates for a basketball season.
// A basketball season ending in year Y runs from Nov 1 of Y-1 through Apr 10 of Y.
func (bc *BasketballClient) historicalSeasonDates(year int64) []string {
	start := time.Date(int(year)-1, time.November, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(int(year), time.April, 10, 0, 0, 0, 0, time.UTC)
	var dates []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d.Format("2006-01-02T00:00Z"))
	}
	return dates
}

// getSeasonDates returns game dates for the given year.
// For the current season it uses the site.api scoreboard calendar (exact game
// dates only), which requires a `dates` query parameter — the plain
// scoreboard response omits the calendar entirely (verified 2026-08-29).
// For historical seasons it generates the full date range for the season
// window.
func (bc *BasketballClient) getSeasonDates(ctx context.Context, year int64) ([]string, error) {
	current, err := bc.DefaultSeason(ctx)
	if err != nil {
		return nil, err
	}
	if year == current {
		return bc.GetSeasonDatesForYear(ctx, year)
	}
	return bc.historicalSeasonDates(year), nil
}

// GetCurrentWeekGames fetches completed games for today and yesterday off the
// site.api scoreboard (dates=YYYYMMDD&groups=N, one request per ET day —
// verified live 2026-08-30: the basketball scoreboard honors the groups
// filter, and without it the plain scoreboard returns only a subset of the
// day's games). If a late-night game finishes after ESPN rolls to the next
// day, a single-day fetch would miss it permanently. Fetching two days
// ensures the 5-minute cron has a full day of retries to catch it.
func (bc *BasketballClient) GetCurrentWeekGames(ctx context.Context, group Group) ([]Game, error) {
	now := time.Now()
	var allGames []Game
	seen := make(map[int64]bool)

	for daysBack := 0; daysBack <= 1; daysBack++ {
		date := now.AddDate(0, 0, -daysBack).Format("20060102")
		games, err := bc.getScoreboardDay(ctx, date, group)
		if err != nil {
			return nil, err
		}
		for _, g := range games {
			if !seen[g.ID] {
				seen[g.ID] = true
				allGames = append(allGames, g)
			}
		}
		bc.Throttle(ctx)
	}

	return allGames, nil
}

// GetGamesBySeason fetches every completed game of a season for the D1 group
// off the site.api scoreboard, walking the season's game dates one ET day at
// a time (dates=YYYYMMDD&groups=50, games collected via finalGames — the same
// per-day walk the football backfill uses, whose single-day fetches are the
// only verified-complete form; see docs/espn-api.md). The date list comes
// from the scoreboard calendar for the current season and from the
// synthesized Nov 1 – Apr 10 window for historical seasons.
//
// A basketball season spans ~150 game dates (plus off-days in the
// synthesized historical window, which the scoreboard answers with empty
// event lists), so with the default 500ms throttle a full-season backfill
// takes ~80 seconds — acceptable for the `updater games --year` path.
func (bc *BasketballClient) GetGamesBySeason(ctx context.Context, year int64, group Group) ([]Game, error) {
	dates, err := bc.getSeasonDates(ctx, year)
	if err != nil {
		return nil, err
	}
	return bc.getGamesByDates(ctx, dates, group)
}

func (bc *BasketballClient) getGamesByDates(ctx context.Context, dates []string, group Group) ([]Game, error) {
	var allGames []Game
	for _, dateStr := range dates {
		date := dateToParam(dateStr)
		if date == "" {
			continue
		}
		games, err := bc.getScoreboardDay(ctx, date, group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games...)
		bc.Throttle(ctx)
	}
	return allGames, nil
}

// TeamConferencesByYear extracts team → conference ID mappings from the site.api
// scoreboard for every game date of the given year: the date list from
// getSeasonDates is walked one ET day at a time with
// scoreboard?dates=YYYYMMDD&groups=50, and each competitor's
// team.conferenceId is collected (verified live 2026-08-30: basketball
// scoreboard competitors carry conferenceId, except rare non-D1 fill-ins
// like Bethesda Flames, which decode to 0 and are skipped). Competitors of
// all statuses are collected — conference IDs are identical in every phase.
// This replaces the cdn schedule walk, retired with the rest of the
// basketball cdn migration (2026-08-30).
func (bc *BasketballClient) TeamConferencesByYear(ctx context.Context, year int64) (map[int64]int64, error) {
	dates, err := bc.getSeasonDates(ctx, year)
	if err != nil {
		return nil, err
	}

	teamConfs := map[int64]int64{}
	for _, group := range bc.Sport.Groups() {
		for _, dateStr := range dates {
			date := dateToParam(dateStr)
			if date == "" {
				continue
			}
			url := bc.ScoreboardURL() + fmt.Sprintf("?dates=%s&groups=%d", date, group)
			var res SiteScoreboardESPN
			if err := makeRequest(ctx, bc.Client, url, &res); err != nil {
				return nil, err
			}
			maps.Copy(teamConfs, extractTeamConfs(res.Events))
			bc.Throttle(ctx)
		}
	}

	if len(teamConfs) == 0 {
		return nil, fmt.Errorf("no teams found for the %d season", year)
	}
	return teamConfs, nil
}

// ConferenceMap fetches D1 conference metadata off the site.api
// scoreboard/conferences endpoint (groups=50 returns all 32 D1 conferences —
// verified live 2026-08-29). This replaces the old cdn-schedule mid-season-
// date workaround, which existed because March Madness schedule pages only
// exposed tournament groupings; the dedicated conferences endpoint is immune
// to that problem. ConferenceMap has no year parameter, so like the cdn path
// it always reflects the current season's conferences.
func (bc *BasketballClient) ConferenceMap(ctx context.Context) (ConferenceMapResult, error) {
	res, err := bc.GetConferences(ctx, D1Basketball)
	if err != nil {
		return ConferenceMapResult{}, err
	}

	return ConferenceMapResult{
		Conferences: map[Group]map[int64]string{ //nolint:exhaustive // basketball only has D1
			D1Basketball: res.conferenceShortNames(D1Basketball),
		},
	}, nil
}
