package espn

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"
)

// FootballClient wraps a shared *Client with football-specific season logic.
type FootballClient struct{ *Client }

// Compile-time interface check.
var _ SportClient = (*FootballClient)(nil)

func (fc *FootballClient) DefaultSeason() (int64, error) {
	var res GameScheduleESPN
	err := fc.makeRequest(fc.WeekURL(), &res)
	if err != nil {
		return 0, err
	}

	return res.Content.Defaults.Year, nil
}

func (fc *FootballClient) GetWeeksInSeason(year int64) (int64, error) {
	url := fc.WeekURL() + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := fc.makeRequest(url, &res)
	if err != nil {
		return 0, err
	}

	if len(res.Content.Calendar) == 0 || len(res.Content.Calendar[0].Weeks) == 0 {
		return 0, fmt.Errorf("schedule response missing calendar/weeks for year %d", year)
	}

	return int64(len(res.Content.Calendar[0].Weeks)), nil
}

func (fc *FootballClient) HasPostseasonStarted(year int64, startTime time.Time) (bool, error) {
	url := fc.WeekURL() + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := fc.makeRequest(url, &res)
	if err != nil {
		return false, err
	}

	if len(res.Content.Calendar) < 2 {
		return false, fmt.Errorf("schedule response has %d calendar entries, need at least 2 for postseason",
			len(res.Content.Calendar))
	}

	postSeasonStart, _ := time.Parse("2006-01-02T15:04Z",
		res.Content.Calendar[1].StartDate)
	return postSeasonStart.Before(startTime), nil
}

func (fc *FootballClient) GetGamesBySeason(year int64, group Group) ([]Game, error) {
	var allGames []Game

	numWeeks, err := fc.GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for i := int64(1); i < numWeeks; i++ {
		games, err := fc.GetCompletedGamesByWeek(year, i, group, Regular)
		if err != nil {
			return nil, err
		}

		allGames = append(allGames, games...)
	}

	games, err := fc.GetCompletedGamesByWeek(year, int64(1), group, Postseason)
	if err != nil {
		return nil, err
	}

	allGames = append(allGames, games...)

	return allGames, nil
}

func (fc *FootballClient) TeamConferencesByYear(year int64) (map[int64]int64, error) {
	teamConfs := map[int64]int64{}

	numWeeks, err := fc.GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for _, group := range fc.Sport.Groups() {
		for i := int64(1); i < numWeeks; i++ {
			games, err := fc.GetGamesByWeek(year, i, group, Regular)
			if err != nil {
				return nil, err
			}
			maps.Copy(teamConfs, extractTeamConfs(games))
		}

		games, err := fc.GetGamesByWeek(year, int64(1), group, Postseason)
		if err != nil {
			return nil, err
		}
		maps.Copy(teamConfs, extractTeamConfs(games))
	}

	return teamConfs, nil
}

func (fc *FootballClient) ConferenceMap() (map[Group]interface{}, error) {
	var res GameScheduleESPN
	err := fc.makeRequest(fc.WeekURL(), &res)
	if err != nil {
		return nil, err
	}

	conferences := res.Content.ConferenceAPI.Conferences

	fbs := map[int64]string{}
	fcs := map[int64]string{}
	dii := []int64{}
	diii := []int64{}

	for _, conference := range conferences {
		switch int64(conference.ParentGroupID) {
		case int64(FBS):
			fbs[conference.GroupID] = conference.ShortName
		case int64(FCS):
			fcs[conference.GroupID] = conference.ShortName
		default:
			if slices.Contains([]int64{int64(DII), int64(DIII)}, conference.GroupID) {
				for _, conf := range conference.SubGroups {
					group, _ := strconv.ParseInt(conf, 10, 64)
					switch conference.GroupID {
					case int64(DII):
						dii = append(dii, group)
					case int64(DIII):
						diii = append(diii, group)
					}
				}
			}
		}
	}

	return map[Group]interface{}{ //nolint:exhaustive // football doesn't have D1Basketball
		FBS:  fbs,
		FCS:  fcs,
		DII:  dii,
		DIII: diii,
	}, nil
}
