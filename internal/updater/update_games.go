package updater

import (
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/game"
	"gorm.io/gorm"
)

func combineGames(gamesLists [][]int64) []int64 {
	found := make(map[int64]bool)
	var games []int64

	for _, gamesList := range gamesLists {
		for _, game := range gamesList {
			if !found[game] {
				found[game] = true
				games = append(games, game)
			}
		}
	}

	return games
}

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

func (u *Updater) insertGameInfo(game game.ParsedGameInfo) error {
	return u.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&game.GameInfo).Error; err != nil {
			return err
		}

		if len(game.TeamStats) > 0 {
			if err := tx.Create(&game.TeamStats).Error; err != nil {
				return err
			}
		}

		if len(game.PassingStats) > 0 {
			if err := tx.Create(&game.PassingStats).Error; err != nil {
				return err
			}
		}

		if len(game.RushingStats) > 0 {
			if err := tx.Create(&game.RushingStats).Error; err != nil {
				return err
			}
		}

		if len(game.ReceivingStats) > 0 {
			if err := tx.Create(&game.ReceivingStats).Error; err != nil {
				return err
			}
		}

		if len(game.FumbleStats) > 0 {
			if err := tx.Create(&game.FumbleStats).Error; err != nil {
				return err
			}
		}

		if len(game.DefensiveStats) > 0 {
			if err := tx.Create(&game.DefensiveStats).Error; err != nil {
				return err
			}
		}

		if len(game.InterceptionStats) > 0 {
			if err := tx.Create(&game.InterceptionStats).Error; err != nil {
				return err
			}
		}

		if len(game.ReturnStats) > 0 {
			if err := tx.Create(&game.ReturnStats).Error; err != nil {
				return err
			}
		}

		if len(game.KickStats) > 0 {
			if err := tx.Create(&game.KickStats).Error; err != nil {
				return err
			}
		}

		if len(game.PuntStats) > 0 {
			if err := tx.Create(&game.PuntStats).Error; err != nil {
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

	for _, game := range gameStats {
		if err := u.insertGameInfo(game); err != nil {
			return 0, err
		}
	}

	return len(gameStats), nil
}
