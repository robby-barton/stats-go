package espn

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"
)

type Group int64
type SeasonType int64

const (
	FBS  Group = 80
	FCS  Group = 81
	DII  Group = 57
	DIII Group = 58

	Regular    SeasonType = 2
	Postseason SeasonType = 3
)

func GetCurrentWeekGames(group Group) ([]Game, error) {
	var games []Game

	url := weekURL + fmt.Sprintf("&group=%d", group)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
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

func GetGamesByWeek(year int64, week int64, group Group, seasonType SeasonType) (*GameScheduleESPN, error) {
	url := weekURL +
		fmt.Sprintf("&year=%d&week=%d&group=%d&seasonType=%d", year, week, group, seasonType)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func GetCompletedGamesByWeek(year int64, week int64, group Group, seasonType SeasonType) ([]Game, error) {
	res, err := GetGamesByWeek(year, week, group, seasonType)
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

func GetWeeksInSeason(year int64) (int64, error) {
	url := weekURL + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return 0, err
	}

	return int64(len(res.Content.Calendar[0].Weeks)), nil
}

func HasPostseasonStarted(year int64, startTime time.Time) (bool, error) {
	url := weekURL + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return false, err
	}

	postSeasonStart, _ := time.Parse("2006-01-02T15:04Z",
		res.Content.Calendar[1].StartDate)
	return postSeasonStart.Before(startTime), nil
}

func GetGamesBySeason(year int64, group Group) ([]Game, error) {
	var gameIDs []Game

	numWeeks, err := GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for i := int64(1); i < numWeeks; i++ {
		games, err := GetCompletedGamesByWeek(year, i, group, Regular)
		if err != nil {
			return nil, err
		}

		gameIDs = append(gameIDs, games...)
	}

	games, err := GetCompletedGamesByWeek(year, int64(1), group, Postseason)
	if err != nil {
		return nil, err
	}

	gameIDs = append(gameIDs, games...)

	return gameIDs, nil
}

func GetGameStats(gameID int64) (*GameInfoESPN, error) {
	url := fmt.Sprintf(gameStatsURL, gameID)

	var res GameInfoESPN
	err := makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func GetTeamInfo() (*TeamInfoESPN, error) {
	var res TeamInfoESPN
	err := makeRequest(teamInfoURL, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func DefaultSeason() (int64, error) {
	var res GameScheduleESPN
	err := makeRequest(weekURL, &res)
	if err != nil {
		return 0, err
	}

	return res.Content.Defaults.Year, nil
}

func ConferenceMap() (map[Group]interface{}, error) {
	var res GameScheduleESPN
	err := makeRequest(weekURL, &res)
	if err != nil {
		return nil, err
	}

	fbs := map[int64]string{}
	fcs := map[int64]string{}
	dii := []int64{}
	diii := []int64{}

	conferences := res.Content.ConferenceAPI.Conferences

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

	return map[Group]interface{}{
		FBS:  fbs,
		FCS:  fcs,
		DII:  dii,
		DIII: diii,
	}, nil
}

func TeamConferencesByYear(year int64) (map[int64]int64, error) {
	teamConfs := map[int64]int64{}

	numWeeks, err := GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for _, group := range []Group{FBS, FCS} {
		for i := int64(1); i < numWeeks; i++ {
			games, err := GetGamesByWeek(year, i, group, Regular)
			if err != nil {
				return nil, err
			}
			maps.Copy(teamConfs, extractTeamConfs(games))
		}

		games, err := GetGamesByWeek(year, int64(1), group, Postseason)
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
			for _, team := range event.Competitions[0].Competitors {
				teamConfs[team.Team.ID] = team.Team.ConferenceID
			}
		}
	}

	return teamConfs
}
