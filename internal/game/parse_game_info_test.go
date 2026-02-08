package game

import (
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/espn"
)

func TestParseGameInfo_Standard(t *testing.T) {
	gameInfo := &espn.GameInfoESPN{
		GamePackage: espn.GamePackage{
			Header: espn.Header{
				ID:   401234567,
				Week: 5,
				Season: espn.Season{
					Year: 2023,
					Type: int64(espn.Regular),
				},
				Competitions: []espn.Competitions{
					{
						Date:     "2023-10-07T16:00Z",
						ConfGame: true,
						Neutral:  false,
						Competitors: []espn.Competitors{
							{HomeAway: "home", ID: 100, Score: 35},
							{HomeAway: "away", ID: 200, Score: 21},
						},
					},
				},
			},
		},
	}

	var s ParsedGameInfo
	s.parseGameInfo(gameInfo)

	game := s.GameInfo
	if game.GameID != 401234567 {
		t.Errorf("GameID = %d, want 401234567", game.GameID)
	}
	if game.Week != 5 {
		t.Errorf("Week = %d, want 5", game.Week)
	}
	if game.Season != 2023 {
		t.Errorf("Season = %d, want 2023", game.Season)
	}
	if game.Postseason != 0 {
		t.Errorf("Postseason = %d, want 0 (regular season)", game.Postseason)
	}
	if !game.ConfGame {
		t.Error("ConfGame = false, want true")
	}
	if game.Neutral {
		t.Error("Neutral = true, want false")
	}
	if game.HomeID != 100 {
		t.Errorf("HomeID = %d, want 100", game.HomeID)
	}
	if game.HomeScore != 35 {
		t.Errorf("HomeScore = %d, want 35", game.HomeScore)
	}
	if game.AwayID != 200 {
		t.Errorf("AwayID = %d, want 200", game.AwayID)
	}
	if game.AwayScore != 21 {
		t.Errorf("AwayScore = %d, want 21", game.AwayScore)
	}

	expectedTime, _ := time.Parse("2006-01-02T15:04Z", "2023-10-07T16:00Z")
	if !game.StartTime.Equal(expectedTime) {
		t.Errorf("StartTime = %v, want %v", game.StartTime, expectedTime)
	}
}

func TestParseGameInfo_Postseason_NeutralSite(t *testing.T) {
	gameInfo := &espn.GameInfoESPN{
		GamePackage: espn.GamePackage{
			Header: espn.Header{
				ID:   401999999,
				Week: 1,
				Season: espn.Season{
					Year: 2023,
					Type: int64(espn.Postseason),
				},
				Competitions: []espn.Competitions{
					{
						Date:     "2024-01-01T17:00Z",
						ConfGame: false,
						Neutral:  true,
						Competitors: []espn.Competitors{
							{HomeAway: "away", ID: 300, Score: 28},
							{HomeAway: "home", ID: 400, Score: 31},
						},
					},
				},
			},
		},
	}

	var s ParsedGameInfo
	s.parseGameInfo(gameInfo)

	game := s.GameInfo
	if game.GameID != 401999999 {
		t.Errorf("GameID = %d, want 401999999", game.GameID)
	}
	if game.Postseason != 1 {
		t.Errorf("Postseason = %d, want 1 (Postseason - Regular = 3 - 2 = 1)", game.Postseason)
	}
	if !game.Neutral {
		t.Error("Neutral = false, want true")
	}
	if game.ConfGame {
		t.Error("ConfGame = true, want false")
	}
	if game.HomeID != 400 {
		t.Errorf("HomeID = %d, want 400", game.HomeID)
	}
	if game.HomeScore != 31 {
		t.Errorf("HomeScore = %d, want 31", game.HomeScore)
	}
	if game.AwayID != 300 {
		t.Errorf("AwayID = %d, want 300", game.AwayID)
	}
	if game.AwayScore != 28 {
		t.Errorf("AwayScore = %d, want 28", game.AwayScore)
	}
}
