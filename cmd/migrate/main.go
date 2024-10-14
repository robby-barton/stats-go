package main

import (
	"reflect"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/logger"
)

func main() {
	logger := logger.NewLogger().Sugar()
	defer logger.Sync()

	logger.Info("Start")

	cfg := config.SetupConfig()
	postgres, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		panic(err)
	}
	postgresDB, _ := postgres.DB()
	defer postgresDB.Close()
	sqlite, err := database.NewDatabase(nil)
	if err != nil {
		panic(err)
	}
	sqliteDB, _ := sqlite.DB()
	defer sqliteDB.Close()

	models := []any{
		[]database.TeamName{},
		[]database.TeamSeason{},
		[]database.TeamWeekResult{},
		[]database.Game{},
		[]database.TeamGameStats{},
		[]database.PassingStats{},
		[]database.RushingStats{},
		[]database.ReceivingStats{},
		[]database.FumbleStats{},
		[]database.DefensiveStats{},
		[]database.InterceptionStats{},
		[]database.ReturnStats{},
		[]database.KickStats{},
		[]database.PuntStats{},
	}
	for _, model := range models {
		err := migrate(postgres, sqlite, model, logger)
		if err != nil {
			panic(err)
		}
	}

	logger.Info("End")
}

func migrate(source *gorm.DB, destination *gorm.DB, object any, logger *zap.SugaredLogger) error {
	result := source.Find(&object)
	if result.Error != nil {
		return result.Error
	}
	logger.Infof("Migrating %d %s objects\n", result.RowsAffected, reflect.TypeOf(object).String())
	err := destination.Clauses((clause.OnConflict{UpdateAll: true})).CreateInBatches(object, 1000).Error
	if err != nil {
		return err
	}

	return nil
}
