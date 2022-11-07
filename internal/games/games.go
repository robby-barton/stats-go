package games

import (
	"fmt"
	"time"

	"github.com/robby-barton/stats-api/internal/database"
)

type group int64
type seasonType int64

const (
	FBS group = 80
	FCS group = 81

	Regular    seasonType = 2
	Postseason seasonType = 3
)

type ParsedGameInfo struct {
	GameInfo          database.Game
	TeamStats         []database.TeamGameStats
	PassingStats      []database.PassingStats
	RushingStats      []database.RushingStats
	ReceivingStats    []database.ReceivingStats
	FumbleStats       []database.FumbleStats
	DefensiveStats    []database.DefensiveStats
	InterceptionStats []database.InterceptionStats
	ReturnStats       []database.ReturnStats
	KickStats         []database.KickStats
	PuntStats         []database.PuntStats
}

func GetGamesByWeek(
	year int64,
	week int64,
	group group,
	seasonType seasonType,
) ([]int64, error) {
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

func getSeasonWeeksInYear(year int64) (int64, error) {
	url := fmt.Sprintf(weekUrl, year, int64(1), Regular, FBS)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return 0, err
	}

	return int64(len(res.Content.Calendar[0].Weeks)), nil
}

func GetGamesByYear(
	year int64,
	group group,
) ([]int64, error) {
	var gameIds []int64

	numWeeks, err := getSeasonWeeksInYear(year)
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
) (*ParsedGameInfo, error) {
	var parsedGameStats ParsedGameInfo

	url := fmt.Sprintf(gameStatsUrl, gameId)

	var res GameInfoESPN
	err := makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	parsedGameStats.parseGameInfo(res)
	parsedGameStats.parseTeamInfo(res)
	parsedGameStats.parsePlayerStats(res)

	return &parsedGameStats, nil
}

func combineGames(gamesLists [][]int64) []int64 {
	keys := make(map[int64]bool)
	var games []int64

	for _, gamesList := range gamesLists {
		for _, game := range gamesList {
			if _, value := keys[game]; !value {
				keys[game] = true
				games = append(games, game)
			}
		}
	}

	return games
}

func UpdateGamesForYear(year int64) ([]ParsedGameInfo, error) {
	fbsGames, err := GetGamesByYear(year, FBS)
	if err != nil {
		return nil, err
	}

	fcsGames, err := GetGamesByYear(year, FCS)
	if err != nil {
		return nil, err
	}

	gameIds := combineGames([][]int64{fbsGames, fcsGames})

	var parsedGameInfo []ParsedGameInfo
	for i, gameId := range gameIds {
		fmt.Printf("%d/%d\n", i+1, len(gameIds))
		gameInfo, err := GetGameStats(gameId)
		if err != nil {
			fmt.Println(gameId)
			return nil, err
		}

		parsedGameInfo = append(parsedGameInfo, *gameInfo)

		time.Sleep(100 * time.Millisecond)
	}

	return parsedGameInfo, nil
}
