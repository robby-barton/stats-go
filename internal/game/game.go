package game

import (
	"context"

	"go.uber.org/zap"

	"github.com/robby-barton/stats-go/internal/espn"
)

// ParsedGameInfo is a fully-parsed single game: header, team stats, and (for
// football) player stats. It uses the game package's domain types; mapping to
// the GORM persistence models happens in internal/updater.
type ParsedGameInfo struct {
	Info              Info
	TeamStats         []TeamGameStats
	PassingStats      []PassingStats
	RushingStats      []RushingStats
	ReceivingStats    []ReceivingStats
	FumbleStats       []FumbleStats
	DefensiveStats    []DefensiveStats
	InterceptionStats []InterceptionStats
	ReturnStats       []ReturnStats
	KickStats         []KickStats
	PuntStats         []PuntStats
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
func GetCurrentWeekGames(ctx context.Context, client espn.SportClient) ([]espn.Game, error) {
	var allGames [][]espn.Game
	for _, group := range client.SportInfo().Groups() {
		games, err := client.GetCurrentWeekGames(ctx, group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games)
		client.Throttle(ctx)
	}

	return combineGames(allGames), nil
}

// GetGamesForSeason fetches all completed games for a season across all groups
// defined for the client's sport.
func GetGamesForSeason(ctx context.Context, client espn.SportClient, year int64) ([]espn.Game, error) {
	var allGames [][]espn.Game
	for _, group := range client.SportInfo().Groups() {
		games, err := client.GetGamesBySeason(ctx, year, group)
		if err != nil {
			return nil, err
		}
		allGames = append(allGames, games)
		client.Throttle(ctx)
	}

	return combineGames(allGames), nil
}

// GetSingleGame fetches and parses one game's box score. Unknown stat names
// are reported through the logger instead of stdout.
func GetSingleGame(
	ctx context.Context,
	client espn.SportClient,
	log *zap.SugaredLogger,
	gameID int64,
) (*ParsedGameInfo, error) {
	res, err := client.GetGameStats(ctx, gameID)
	if err != nil {
		return nil, err
	}

	parsedGame := &ParsedGameInfo{}
	if err := parsedGame.parseGameInfo(res); err != nil {
		return nil, err
	}
	parsedGame.parseTeamInfo(res, log)
	if client.SportInfo() == espn.CollegeFootball {
		parsedGame.parsePlayerStats(res, log)
	}

	return parsedGame, nil
}
