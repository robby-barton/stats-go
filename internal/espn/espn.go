package espn

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"
)

// Sport identifies a college sport for ESPN API parameterization.
type Sport string

const (
	CollegeFootball   Sport = "college-football"
	CollegeBasketball Sport = "college-basketball"
)

// Short database identifiers for each sport.
const (
	SportDBFootball   = "cfb"
	SportDBBasketball = "cbb"
)

// SportDB returns the short database identifier for the sport.
func (s Sport) SportDB() string {
	switch s {
	case CollegeBasketball:
		return SportDBBasketball
	case CollegeFootball:
		return SportDBFootball
	}
	return SportDBFootball
}

type Group int64
type SeasonType int64

const (
	FBS  Group = 80
	FCS  Group = 81
	DII  Group = 57
	DIII Group = 58

	// Basketball D1 group on ESPN.
	D1Basketball Group = 50

	Regular    SeasonType = 2
	Postseason SeasonType = 3
)

// Groups returns the division groups used for schedule fetching for a sport.
func (s Sport) Groups() []Group {
	switch s {
	case CollegeBasketball:
		return []Group{D1Basketball}
	case CollegeFootball:
		return []Group{FBS, FCS}
	}
	return []Group{FBS, FCS}
}

// HasDivisionSplit returns true if the sport distinguishes divisions (e.g. FBS/FCS).
func (s Sport) HasDivisionSplit() bool {
	return s == CollegeFootball
}

func (c *Client) GetCurrentWeekGames(group Group) ([]Game, error) {
	var games []Game

	url := c.WeekURL() + fmt.Sprintf("&group=%d", group)

	var res GameScheduleESPN
	err := c.makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	for _, day := range res.Content.Schedule {
		for _, event := range day.Games {
			if event.Status.StatusType.Completed && event.Status.StatusType.Name == "STATUS_FINAL" {
				games = append(games, event)
			}
		}
	}

	return games, nil
}

func (c *Client) GetGamesByWeek(year int64, week int64, group Group, seasonType SeasonType) (*GameScheduleESPN, error) {
	url := c.WeekURL() +
		fmt.Sprintf("&year=%d&week=%d&group=%d&seasonType=%d", year, week, group, seasonType)

	var res GameScheduleESPN
	err := c.makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) GetCompletedGamesByWeek(year int64, week int64, group Group, seasonType SeasonType) ([]Game, error) {
	res, err := c.GetGamesByWeek(year, week, group, seasonType)
	if err != nil {
		return nil, err
	}

	var games []Game
	for _, day := range res.Content.Schedule {
		for _, event := range day.Games {
			if event.Status.StatusType.Completed && event.Status.StatusType.Name == "STATUS_FINAL" {
				games = append(games, event)
			}
		}
	}

	return games, nil
}

func (c *Client) GetWeeksInSeason(year int64) (int64, error) {
	url := c.WeekURL() + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := c.makeRequest(url, &res)
	if err != nil {
		return 0, err
	}

	return int64(len(res.Content.Calendar[0].Weeks)), nil
}

func (c *Client) HasPostseasonStarted(year int64, startTime time.Time) (bool, error) {
	url := c.WeekURL() + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := c.makeRequest(url, &res)
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

func (c *Client) GetGamesBySeason(year int64, group Group) ([]Game, error) {
	var gameIDs []Game

	numWeeks, err := c.GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for i := int64(1); i < numWeeks; i++ {
		games, err := c.GetCompletedGamesByWeek(year, i, group, Regular)
		if err != nil {
			return nil, err
		}

		gameIDs = append(gameIDs, games...)
	}

	games, err := c.GetCompletedGamesByWeek(year, int64(1), group, Postseason)
	if err != nil {
		return nil, err
	}

	gameIDs = append(gameIDs, games...)

	return gameIDs, nil
}

func (c *Client) GetGameStats(gameID int64) (*GameInfoESPN, error) {
	url := fmt.Sprintf(c.GameStatsURL(), gameID)

	var res GameInfoESPN
	err := c.makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) GetTeamInfo() (*TeamInfoESPN, error) {
	var res TeamInfoESPN
	err := c.makeRequest(c.TeamInfoURL(), &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) DefaultSeason() (int64, error) {
	var res GameScheduleESPN
	err := c.makeRequest(c.WeekURL(), &res)
	if err != nil {
		return 0, err
	}

	return res.Content.Defaults.Year, nil
}

func (c *Client) ConferenceMap() (map[Group]interface{}, error) {
	var res GameScheduleESPN
	err := c.makeRequest(c.WeekURL(), &res)
	if err != nil {
		return nil, err
	}

	conferences := res.Content.ConferenceAPI.Conferences

	if c.Sport == CollegeBasketball {
		d1 := map[int64]string{}
		for _, conference := range conferences {
			if conference.ParentGroupID == int64(D1Basketball) {
				d1[conference.GroupID] = conference.ShortName
			}
		}
		return map[Group]interface{}{ //nolint:exhaustive // basketball only has D1
			D1Basketball: d1,
		}, nil
	}

	fbs := map[int64]string{}
	fcs := map[int64]string{}
	dii := []int64{}
	diii := []int64{}

	for _, conference := range conferences {
		switch conference.ParentGroupID {
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

func (c *Client) TeamConferencesByYear(year int64) (map[int64]int64, error) {
	teamConfs := map[int64]int64{}

	numWeeks, err := c.GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for _, group := range c.Sport.Groups() {
		for i := int64(1); i < numWeeks; i++ {
			games, err := c.GetGamesByWeek(year, i, group, Regular)
			if err != nil {
				return nil, err
			}
			maps.Copy(teamConfs, extractTeamConfs(games))
		}

		games, err := c.GetGamesByWeek(year, int64(1), group, Postseason)
		if err != nil {
			return nil, err
		}
		maps.Copy(teamConfs, extractTeamConfs(games))
	}

	return teamConfs, nil
}

func extractTeamConfs(games *GameScheduleESPN) map[int64]int64 {
	teamConfs := map[int64]int64{}

	for _, day := range games.Content.Schedule {
		for _, event := range day.Games {
			if len(event.Competitions) == 0 {
				continue
			}
			for _, team := range event.Competitions[0].Competitors {
				teamConfs[team.Team.ID] = team.Team.ConferenceID
			}
		}
	}

	return teamConfs
}
