package game

import (
	"fmt"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
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

func GetGameStats(gameIds []int64) ([]ParsedGameInfo, error) {
	var parsedGameStats []ParsedGameInfo

	for i, gameId := range gameIds {
		fmt.Printf("%d/%d\n", i+1, len(gameIds))
		res, err := espn.GetGameStats(gameId)
		if err != nil {
			return nil, err
		}

		var parsedGame ParsedGameInfo
		parsedGame.parseGameInfo(res)
		parsedGame.parseTeamInfo(res)
		parsedGame.parsePlayerStats(res)

		parsedGameStats = append(parsedGameStats, parsedGame)

		time.Sleep(100 * time.Millisecond)
	}

	return parsedGameStats, nil
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

func GetGamesForSeason(year int64) ([]int64, error) {
	fbsGames, err := espn.GetGamesBySeason(year, espn.FBS)
	if err != nil {
		return nil, err
	}

	fcsGames, err := espn.GetGamesBySeason(year, espn.FCS)
	if err != nil {
		return nil, err
	}

	gameIds := combineGames([][]int64{fbsGames, fcsGames})

	return gameIds, nil
}
