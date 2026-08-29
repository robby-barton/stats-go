package game

import (
	"fmt"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

// parseGameInfo converts the ESPN game header into a database.Game. It
// validates cardinality before indexing so malformed upstream data produces a
// contextual error instead of a panic or zero-value corruption.
func (s *ParsedGameInfo) parseGameInfo(gameInfo *espn.GameInfoESPN) error {
	header := gameInfo.GamePackage.Header

	if len(header.Competitions) == 0 {
		return fmt.Errorf("game %d: response missing competitions", header.ID)
	}
	competition := header.Competitions[0]
	if len(competition.Competitors) < 2 {
		return fmt.Errorf("game %d: response has %d competitors, need at least 2",
			header.ID, len(competition.Competitors))
	}

	startTime, err := time.Parse("2006-01-02T15:04Z", competition.Date)
	if err != nil {
		return fmt.Errorf("game %d: parsing start time %q: %w", header.ID, competition.Date, err)
	}

	var game database.Game
	game.GameID = header.ID
	game.StartTime = startTime
	game.Week = header.Week
	game.Season = header.Season.Year
	game.Postseason = header.Season.Type - int64(espn.Regular)
	game.ConfGame = competition.ConfGame
	game.Neutral = competition.Neutral

	var homeID, awayID int64
	for _, team := range competition.Competitors {
		switch team.HomeAway {
		case "home":
			homeID = team.ID
			game.HomeScore = team.Score
		case "away":
			awayID = team.ID
			game.AwayScore = team.Score
		}
	}
	if homeID == 0 || awayID == 0 {
		return fmt.Errorf("game %d: missing home or away competitor", header.ID)
	}
	game.HomeID = homeID
	game.AwayID = awayID

	s.GameInfo = game
	return nil
}
