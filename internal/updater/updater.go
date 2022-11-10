package updater

import (
	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"

	"gorm.io/gorm"
)

type Updater struct {
	DB  *gorm.DB
	CFG *config.Config
}

func NewUpdater() (*Updater, error) {
	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		return nil, err
	}

	return &Updater{
		DB:  db,
		CFG: cfg,
	}, nil
}
