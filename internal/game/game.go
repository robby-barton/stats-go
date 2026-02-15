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

func GetGameStats(client espn.SportClient, games []espn.Game) ([]*ParsedGameInfo, error) {
	var parsedGameStats []*ParsedGameInfo

	for _, game := range games {
		gameStats, err := GetSingleGame(client, game.ID)
		if err != nil {
			return nil, err
		}

		parsedGameStats = append(parsedGameStats, gameStats)

		time.Sleep(client.RateLimitDuration())
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

// GetCurrentWeekGames fetches completed games for the current week across all
// groups defined for the client's sport.
func GetCurrentWeekGames(client espn.SportClient) ([]espn.Game, error) {
	var allGames [][]espn.Game
	for _, group := range client.SportInfo().Groups() {
		games, err := client.GetCurrentWeekGames(group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games)
	}

	return combineGames(allGames), nil
}

// GetGamesForSeason fetches all completed games for a season across all groups
// defined for the client's sport.
func GetGamesForSeason(client espn.SportClient, year int64) ([]espn.Game, error) {
	var allGames [][]espn.Game
	for _, group := range client.SportInfo().Groups() {
		games, err := client.GetGamesBySeason(year, group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games)
	}

	return combineGames(allGames), nil
}

func GetSingleGame(client espn.SportClient, gameID int64) (*ParsedGameInfo, error) {
	res, err := client.GetGameStats(gameID)
	if err != nil {
		return nil, err
	}

	parsedGame := &ParsedGameInfo{}
	parsedGame.parseGameInfo(res)
	parsedGame.GameInfo.Sport = client.SportInfo().SportDB()
	parsedGame.parseTeamInfo(res)
	if client.SportInfo() == espn.CollegeFootball {
		parsedGame.parsePlayerStats(res)
	}

	return parsedGame, nil
}
