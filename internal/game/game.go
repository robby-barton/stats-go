package game

import (
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

func GetGameStats(client *espn.Client, games []espn.Game) ([]*ParsedGameInfo, error) {
	var parsedGameStats []*ParsedGameInfo

	for _, game := range games {
		gameStats, err := GetSingleGame(client, game.ID)
		if err != nil {
			return nil, err
		}

		parsedGameStats = append(parsedGameStats, gameStats)

		time.Sleep(client.RateLimit)
	}

	return parsedGameStats, nil
}

func combineGames(gamesLists [][]espn.Game) []espn.Game {
	found := make(map[int64]bool)
	var games []espn.Game

	for _, gamesList := range gamesLists {
		for _, game := range gamesList {
			if !found[game.ID] {
				found[game.ID] = true
				games = append(games, game)
			}
		}
	}

	return games
}

func GetCurrentWeekGames(client *espn.Client) ([]espn.Game, error) {
	fbsGames, err := client.GetCurrentWeekGames(espn.FBS)
	if err != nil {
		return nil, err
	}

	fcsGames, err := client.GetCurrentWeekGames(espn.FCS)
	if err != nil {
		return nil, err
	}

	games := combineGames([][]espn.Game{fbsGames, fcsGames})

	return games, nil
}

func GetGamesForSeason(client *espn.Client, year int64) ([]espn.Game, error) {
	fbsGames, err := client.GetGamesBySeason(year, espn.FBS)
	if err != nil {
		return nil, err
	}

	fcsGames, err := client.GetGamesBySeason(year, espn.FCS)
	if err != nil {
		return nil, err
	}

	games := combineGames([][]espn.Game{fbsGames, fcsGames})

	return games, nil
}

func GetSingleGame(client *espn.Client, gameID int64) (*ParsedGameInfo, error) {
	res, err := client.GetGameStats(gameID)
	if err != nil {
		return nil, err
	}

	parsedGame := &ParsedGameInfo{}
	parsedGame.parseGameInfo(res)
	parsedGame.parseTeamInfo(res)
	parsedGame.parsePlayerStats(res)

	return parsedGame, nil
}
