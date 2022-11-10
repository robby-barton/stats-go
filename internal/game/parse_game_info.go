package game

import (
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func (s *ParsedGameInfo) parseGameInfo(gameInfo *espn.GameInfoESPN) {
	var game database.Game

	game.GameId = gameInfo.GamePackage.Header.Id
	game.StartTime, _ = time.Parse("2006-01-02T15:04Z",
		gameInfo.GamePackage.Header.Competitions[0].Date)
	game.Week = gameInfo.GamePackage.Header.Week
	game.Season = gameInfo.GamePackage.Header.Season.Year
	game.Postseason = gameInfo.GamePackage.Header.Season.Type - int64(espn.Regular)
	if gameInfo.GamePackage.Header.Competitions[0].ConfGame {
		game.ConfGame = 1
	}
	if gameInfo.GamePackage.Header.Competitions[0].Neutral {
		game.Neutral = 1
	}

	for _, team := range gameInfo.GamePackage.Header.Competitions[0].Competitors {
		if team.HomeAway == "home" {
			game.HomeId = team.Id
		} else if team.HomeAway == "away" {
			game.AwayId = team.Id
		}
	}

	s.GameInfo = game
}
