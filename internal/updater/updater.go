package updater

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/sport"
)

type Updater struct {
	DB     *gorm.DB
	Logger *zap.SugaredLogger
	ESPN   espn.SportClient
}

// sportDB returns the persistence identifier for the updater's sport.
func (u *Updater) sportDB() sport.Sport {
	return u.ESPN.SportInfo().DBSport()
}
