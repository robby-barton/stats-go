package updater

import (
	"errors"
	"time"

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
		} else {
			teams := game.Competitions[0]
			home := teams.Competitors[0]
			away := teams.Competitors[1]
			if home.HomeAway == "away" {
				home, away = away, home
			}
			if existingGame.HomeScore != home.Score || existingGame.AwayScore != away.Score {
				newGames = append(newGames, game)
			}
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

func (u *Updater) processGames(games []espn.Game) ([]int64, error) {
	var allGameIDs []int64

	for start := 0; start < len(games); start += gamesBatchSize {
		end := min(start+gamesBatchSize, len(games))
		batch := games[start:end]

		for _, g := range batch {
			stats, err := game.GetSingleGame(u.ESPN, g.ID)
			if err != nil {
				u.Logger.Warnf("skipping game %d: %v", g.ID, err)
				continue
			}
			if err := u.insertGameInfo(stats); err != nil {
				return allGameIDs, err
			}
			allGameIDs = append(allGameIDs, stats.GameInfo.GameID)
			time.Sleep(u.ESPN.RateLimitDuration())
		}

		u.Logger.Infof("processed %d/%d games", end, len(games))
	}

	return allGameIDs, nil
}

func (u *Updater) UpdateCurrentWeek() ([]int64, error) {
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

func (u *Updater) UpdateGamesForYear(year int64) ([]int64, error) {
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
