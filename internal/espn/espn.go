package espn

import (
	"fmt"
	"time"
)

type group int64
type seasonType int64

const (
	FBS group = 80
	FCS group = 81

	Regular    seasonType = 2
	Postseason seasonType = 3
)

type ESPNResponses interface {
	GameInfoESPN | GameScheduleESPN
}

func GetGamesByWeek(year int64, week int64, group group, seasonType seasonType) ([]int64, error) {
	var games []int64

	url := fmt.Sprintf(weekUrl, year, week, seasonType, group)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	for _, day := range res.Content.Schedule {
		for _, event := range day.Games {

			if event.Status.StatusType.Name == "STATUS_FINAL" {
				games = append(games, event.Id)
			}
		}
	}

	return games, nil
}

func GetWeeksInSeason(year int64) (int64, error) {
	url := fmt.Sprintf(weekUrl, year, int64(1), Regular, FBS)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return 0, err
	}

	return int64(len(res.Content.Calendar[0].Weeks)), nil
}

func HasPostseasonStarted(year int64, startTime time.Time) (bool, error) {
	url := fmt.Sprintf(weekUrl, year, int64(1), Regular, FBS)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return false, err
	}

	postSeasonStart, _ := time.Parse("2006-01-02T15:04Z",
		res.Content.Calendar[1].StartDate)
	return postSeasonStart.Before(startTime), nil
}

func GetGamesBySeason(year int64, group group) ([]int64, error) {
	var gameIds []int64

	numWeeks, err := GetWeeksInSeason(year)
	if err != nil {
		return nil, err
	}

	for i := int64(1); i < numWeeks; i++ {
		games, err := GetGamesByWeek(year, i, group, Regular)
		if err != nil {
			return nil, err
		}

		gameIds = append(gameIds, games...)

	}

	games, err := GetGamesByWeek(year, int64(1), group, Postseason)
	if err != nil {
		return nil, err
	}

	gameIds = append(gameIds, games...)

	return gameIds, nil
}

func GetGameStats(
	gameId int64,
) (*GameInfoESPN, error) {
	url := fmt.Sprintf(gameStatsUrl, gameId)

	var res GameInfoESPN
	err := makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}
