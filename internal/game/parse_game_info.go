package game

import (
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func (s *ParsedGameInfo) parseGameInfo(gameInfo *espn.GameInfoESPN) {
	var game database.Game

	game.GameID = gameInfo.GamePackage.Header.ID
	game.StartTime, _ = time.Parse("2006-01-02T15:04Z",
		gameInfo.GamePackage.Header.Competitions[0].Date)
	game.Week = gameInfo.GamePackage.Header.Week
	game.Season = gameInfo.GamePackage.Header.Season.Year
	game.Postseason = gameInfo.GamePackage.Header.Season.Type - int64(espn.Regular)
	game.ConfGame = gameInfo.GamePackage.Header.Competitions[0].ConfGame
	game.Neutral = gameInfo.GamePackage.Header.Competitions[0].Neutral

	for _, team := range gameInfo.GamePackage.Header.Competitions[0].Competitors {
		switch team.HomeAway {
		case "home":
			game.HomeID = team.ID
			game.HomeScore = team.Score
		case "away":
			game.AwayID = team.ID
			game.AwayScore = team.Score
		}
	}

	s.GameInfo = game
}
