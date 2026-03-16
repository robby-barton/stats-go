package espn

import (
	"fmt"
	"maps"
	"sync"
	"time"
)

// BasketballClient wraps a shared *Client with basketball-specific season logic.
type BasketballClient struct {
	*Client
	cachedSeason     int64
	cachedSeasonErr  error
	cachedSeasonOnce sync.Once
}

// Compile-time interface check.
var _ SportClient = (*BasketballClient)(nil)

func (bc *BasketballClient) DefaultSeason() (int64, error) {
	bc.cachedSeasonOnce.Do(func() {
		sb, err := bc.GetScoreboard()
		if err != nil {
			bc.cachedSeasonErr = err
			return
		}
		bc.cachedSeason = sb.Leagues[0].Season.Year
	})
	return bc.cachedSeason, bc.cachedSeasonErr
}

// validateCurrentSeason returns an error if year does not match the current ESPN season.
// Used only for methods that have no historical equivalent (GetWeeksInSeason,
// HasPostseasonStarted) and are only called by the current-season scheduler.
func (bc *BasketballClient) validateCurrentSeason(year int64) error {
	current, err := bc.DefaultSeason()
	if err != nil {
		return err
	}
	if year != current {
		return fmt.Errorf("basketball only supports current season (%d), got year %d", current, year)
	}
	return nil
}

func (bc *BasketballClient) GetWeeksInSeason(year int64) (int64, error) {
	if err := bc.validateCurrentSeason(year); err != nil {
		return 0, err
	}
	return bc.getWeeksInSeasonFromScoreboard()
}

func (bc *BasketballClient) getWeeksInSeasonFromScoreboard() (int64, error) {
	sb, err := bc.GetScoreboard()
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

func (bc *BasketballClient) HasPostseasonStarted(year int64, _ time.Time) (bool, error) {
	if err := bc.validateCurrentSeason(year); err != nil {
		return false, err
	}
	sb, err := bc.GetScoreboard()
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
// For the current season it uses the scoreboard calendar (exact game dates only).
// For historical seasons it generates the full date range for the season window.
func (bc *BasketballClient) getSeasonDates(year int64) ([]string, error) {
	current, err := bc.DefaultSeason()
	if err != nil {
		return nil, err
	}
	if year == current {
		return bc.GetSeasonDates()
	}
	return bc.historicalSeasonDates(year), nil
}

func (bc *BasketballClient) GetGamesBySeason(year int64, group Group) ([]Game, error) {
	dates, err := bc.getSeasonDates(year)
	if err != nil {
		return nil, err
	}
	return bc.getGamesByDates(dates, group)
}

func (bc *BasketballClient) getGamesByDates(dates []string, group Group) ([]Game, error) {
	var allGames []Game
	for _, dateStr := range dates {
		date := dateToParam(dateStr)
		if date == "" {
			continue
		}
		games, err := bc.GetCompletedGamesByDate(date, group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games...)
		time.Sleep(bc.RateLimit)
	}
	return allGames, nil
}

func (bc *BasketballClient) TeamConferencesByYear(year int64) (map[int64]int64, error) {
	dates, err := bc.getSeasonDates(year)
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
			games, err := bc.GetGamesByDate(date, group)
			if err != nil {
				return nil, err
			}
			maps.Copy(teamConfs, extractTeamConfs(games))
			time.Sleep(bc.RateLimit)
		}
	}

	return teamConfs, nil
}

func (bc *BasketballClient) ConferenceMap() (ConferenceMapResult, error) {
	// Use a mid-season date to guarantee regular-season conference data.
	// During March Madness the default schedule page returns only tournament
	// groupings (NCAA Tournament, NIT, etc.) whose parentGroupId is nil,
	// causing the D1 conference list to come back empty.
	current, err := bc.DefaultSeason()
	if err != nil {
		return ConferenceMapResult{}, err
	}
	midSeasonDate := fmt.Sprintf("%d1215", current-1) // Dec 15 of prior calendar year

	var res GameScheduleESPN
	url := bc.WeekURL() + fmt.Sprintf("&date=%s", midSeasonDate)
	if err := bc.makeRequest(url, &res); err != nil {
		return ConferenceMapResult{}, err
	}

	conferences := res.Content.ConferenceAPI.Conferences

	d1 := map[int64]string{}
	for _, conference := range conferences {
		if int64(conference.ParentGroupID) == int64(D1Basketball) {
			d1[conference.GroupID] = conference.ShortName
		}
	}
	return ConferenceMapResult{
		Conferences: map[Group]map[int64]string{ //nolint:exhaustive // basketball only has D1
			D1Basketball: d1,
		},
	}, nil
}
