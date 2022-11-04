package games

import (
	"fmt"

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

func GetWeek(
	year int64,
	week int64,
	seasonType seasonType,
	group group,
) ([]int64, error) {
	var games []int64

	url := fmt.Sprintf(weekUrl, year, week, seasonType, group)

	var res GameScheduleESPN
	err := makeRequest(url, &res)
	if err != nil {
		return nil, err
	}

	for _, event := range res.Events {
		if event.Status.StatusType.Name == "STATUS_FINAL" {
			games = append(games, event.Id)
		}
	}

	return games, nil
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
