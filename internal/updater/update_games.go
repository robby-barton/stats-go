package updater

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/game"
)

func (u *Updater) checkGames(games []espn.Game) ([]espn.Game, error) {
	gameIDs := []int64{}
	for _, game := range games {
		gameIDs = append(gameIDs, game.ID)
	}
	var existing []database.Game
	if err := u.DB.Where("game_id in ? and sport = ?", gameIDs, u.sportDB()).Find(&existing).Error; err != nil {
		return nil, err
	}

	existsMap := map[int64]database.Game{}
	for _, x := range existing {
		existsMap[x.GameID] = x
	}

	var newGames []espn.Game
	for _, game := range games {
		existingGame, ok := existsMap[game.ID]
		if !ok {
			newGames = append(newGames, game)
			continue
		}

		// Malformed schedule entries cannot be score-compared; send them
		// through the full single-game path (which validates strictly) rather
		// than indexing blindly.
		if len(game.Competitions) == 0 || len(game.Competitions[0].Competitors) < 2 {
			newGames = append(newGames, game)
			continue
		}

		teams := game.Competitions[0]
		home := teams.Competitors[0]
		away := teams.Competitors[1]
		if home.HomeAway == "away" {
			home, away = away, home
		}
		if existingGame.Retry == 1 {
			// A previous fetch failed; always retry regardless of score match.
			newGames = append(newGames, game)
		} else if existingGame.HomeScore != home.Score || existingGame.AwayScore != away.Score {
			newGames = append(newGames, game)
		}
	}

	return newGames, nil
}

func (u *Updater) insertGameInfo(game *game.ParsedGameInfo) error {
	if game == nil {
		return errors.New("game nil")
	}

	return u.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				UpdateAll: true, // upsert
			}).
			Create(&game.GameInfo).Error; err != nil {
			return err
		}

		if len(game.TeamStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.TeamStats).Error; err != nil {
				return err
			}
		}

		if len(game.PassingStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.PassingStats).Error; err != nil {
				return err
			}
		}

		if len(game.RushingStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.RushingStats).Error; err != nil {
				return err
			}
		}

		if len(game.ReceivingStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.ReceivingStats).Error; err != nil {
				return err
			}
		}

		if len(game.FumbleStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.FumbleStats).Error; err != nil {
				return err
			}
		}

		if len(game.DefensiveStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.DefensiveStats).Error; err != nil {
				return err
			}
		}

		if len(game.InterceptionStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.InterceptionStats).Error; err != nil {
				return err
			}
		}

		if len(game.ReturnStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
					Columns: []clause.Column{
						{Name: "player_id"},
						{Name: "team_id"},
						{Name: "game_id"},
						{Name: "punt_kick"},
					},
				}).
				Create(&game.ReturnStats).Error; err != nil {
				return err
			}
		}

		if len(game.KickStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.KickStats).Error; err != nil {
				return err
			}
		}

		if len(game.PuntStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&game.PuntStats).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

const gamesBatchSize = 100

// FailedGame records a game whose ESPN fetch failed.
type FailedGame struct {
	GameID int64
	Err    error
}

// GameUpdateResult separates successfully persisted games from per-game
// failures so callers can report partial results instead of silently
// dropping them. Failed games are also persisted with retry=1 (see
// markGameRetry) so the next update cycle re-fetches them.
type GameUpdateResult struct {
	Processed []int64
	Failed    []FailedGame
}

// FailedIDs returns the IDs of games that failed to fetch.
func (r *GameUpdateResult) FailedIDs() []int64 {
	ids := make([]int64, len(r.Failed))
	for i, f := range r.Failed {
		ids[i] = f.GameID
	}
	return ids
}

// markGameRetry durably records a failed fetch using the games.retry column
// so the game is re-fetched on the next update cycle. If the game already has
// a row, only the retry flag is updated; otherwise a minimal row is seeded
// from the schedule entry. A successful later fetch clears the flag because
// insertGameInfo upserts the full row (retry defaults to 0).
func (u *Updater) markGameRetry(g espn.Game) error {
	retry := &database.Game{GameID: g.ID, Sport: u.sportDB(), Retry: 1}

	if len(g.Competitions) > 0 && len(g.Competitions[0].Competitors) >= 2 {
		comp := g.Competitions[0]
		home, away := comp.Competitors[0], comp.Competitors[1]
		if home.HomeAway == "away" {
			home, away = away, home
		}
		retry.HomeID = home.ID
		retry.AwayID = away.ID
		retry.HomeScore = home.Score
		retry.AwayScore = away.Score
	}

	return u.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "game_id"}},
		DoUpdates: clause.Assignments(map[string]any{"retry": 1}),
	}).Create(retry).Error
}

func (u *Updater) processGames(games []espn.Game) (*GameUpdateResult, error) {
	result := &GameUpdateResult{}

	for start := 0; start < len(games); start += gamesBatchSize {
		end := min(start+gamesBatchSize, len(games))
		batch := games[start:end]

		for _, g := range batch {
			stats, err := game.GetSingleGame(u.ESPN, g.ID)
			if err != nil {
				u.Logger.Warnf("skipping game %d: %v", g.ID, err)
				result.Failed = append(result.Failed, FailedGame{GameID: g.ID, Err: err})
				if markErr := u.markGameRetry(g); markErr != nil {
					u.Logger.Errorf("failed to mark game %d for retry: %v", g.ID, markErr)
				}
				continue
			}
			if err := u.insertGameInfo(stats); err != nil {
				return result, fmt.Errorf("persisting game %d: %w", g.ID, err)
			}
			result.Processed = append(result.Processed, stats.GameInfo.GameID)
			u.ESPN.Throttle()
		}

		u.Logger.Infof("processed %d/%d games", end, len(games))
	}

	return result, nil
}

func (u *Updater) UpdateCurrentWeek() (*GameUpdateResult, error) {
	games, err := game.GetCurrentWeekGames(u.ESPN)
	if err != nil {
		return nil, err
	}

	games, err = u.checkGames(games)
	if err != nil {
		return nil, err
	}

	return u.processGames(games)
}

func (u *Updater) UpdateGamesForYear(year int64) (*GameUpdateResult, error) {
	games, err := game.GetGamesForSeason(u.ESPN, year)
	if err != nil {
		return nil, err
	}

	games, err = u.checkGames(games)
	if err != nil {
		return nil, err
	}

	return u.processGames(games)
}

func (u *Updater) UpdateSingleGame(gameID int64) error {
	gameStats, err := game.GetSingleGame(u.ESPN, gameID)
	if err != nil {
		return err
	}

	return u.insertGameInfo(gameStats)
}
