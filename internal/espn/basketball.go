package espn

import (
	"fmt"
	"maps"
	"time"
)

// BasketballClient wraps a shared *Client with basketball-specific season logic.
type BasketballClient struct{ *Client }

// Compile-time interface check.
var _ SportClient = (*BasketballClient)(nil)

func (bc *BasketballClient) DefaultSeason() (int64, error) {
	sb, err := bc.GetScoreboard()
	if err != nil {
		return 0, err
	}
	return sb.Leagues[0].Season.Year, nil
}

func (bc *BasketballClient) GetWeeksInSeason(_ int64) (int64, error) {
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

func (bc *BasketballClient) HasPostseasonStarted(_ int64, _ time.Time) (bool, error) {
	sb, err := bc.GetScoreboard()
	if err != nil {
		return false, err
	}
	return sb.Leagues[0].Season.Type.ID >= int64(Postseason), nil
}

func (bc *BasketballClient) GetGamesBySeason(_ int64, group Group) ([]Game, error) {
	return bc.getGamesBySeasonDates(group)
}

func (bc *BasketballClient) getGamesBySeasonDates(group Group) ([]Game, error) {
	dates, err := bc.GetSeasonDates()
	if err != nil {
		return nil, err
	}

	var allGames []Game
	for _, dateStr := range dates {
		date := dateToParam(dateStr)
		games, err := bc.GetCompletedGamesByDate(date, group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games...)
		time.Sleep(bc.RateLimit)
	}

	return allGames, nil
}

func (bc *BasketballClient) TeamConferencesByYear(_ int64) (map[int64]int64, error) {
	return bc.teamConferencesByDates()
}

func (bc *BasketballClient) teamConferencesByDates() (map[int64]int64, error) {
	dates, err := bc.GetSeasonDates()
	if err != nil {
		return nil, err
	}

	teamConfs := map[int64]int64{}
	for _, group := range bc.Sport.Groups() {
		for _, dateStr := range dates {
			date := dateToParam(dateStr)
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

func (bc *BasketballClient) ConferenceMap() (map[Group]interface{}, error) {
	var res GameScheduleESPN
	err := bc.makeRequest(bc.WeekURL(), &res)
	if err != nil {
		return nil, err
	}

	conferences := res.Content.ConferenceAPI.Conferences

	d1 := map[int64]string{}
	for _, conference := range conferences {
		if int64(conference.ParentGroupID) == int64(D1Basketball) {
			d1[conference.GroupID] = conference.ShortName
		}
	}
	return map[Group]interface{}{ //nolint:exhaustive // basketball only has D1
		D1Basketball: d1,
	}, nil
}
