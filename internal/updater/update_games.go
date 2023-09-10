package updater

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/game"
)

func (u *Updater) checkGames(gameIds []int64) ([]int64, error) {
	var existing []int64
	err := u.DB.Model(database.Game{}).Select("game_id").Where("game_id in ?", gameIds).
		Find(&existing).Error
	if err != nil {
		return nil, err
	}
	exists := map[int64]bool{}
	for _, x := range existing {
		exists[x] = true
	}

	var newGames []int64
	for _, game := range gameIds {
		if !exists[game] {
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
					UpdateAll:    true, // upsert
					OnConstraint: "return_stats_pkey",
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

func (u *Updater) UpdateCurrentWeek() (int, error) {
	gameIds, err := game.GetCurrentWeekGames()
	if err != nil {
		return 0, err
	}

	gameIds, err = u.checkGames(gameIds)
	if err != nil {
		return 0, err
	}

	gameStats, err := game.GetGameStats(gameIds)
	if err != nil {
		return 0, err
	}

	for _, game := range gameStats {
		if err := u.insertGameInfo(game); err != nil {
			return 0, err
		}
	}

	return len(gameStats), nil
}

func (u *Updater) UpdateGamesForYear(year int64) (int, error) {
	gameIds, err := game.GetGamesForSeason(year)
	if err != nil {
		return 0, err
	}

	gameIds, err = u.checkGames(gameIds)
	if err != nil {
		return 0, err
	}

	gameStats, err := game.GetGameStats(gameIds)
	if err != nil {
		return 0, err
	}

	for _, game := range gameStats {
		if err := u.insertGameInfo(game); err != nil {
			return 0, err
		}
	}

	return len(gameStats), nil
}

func (u *Updater) UpdateSingleGame(gameID int64) error {
	gameStats, err := game.GetSingleGame(gameID)
	if err != nil {
		return err
	}

	return u.insertGameInfo(gameStats)
}
