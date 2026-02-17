package updater

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/espn"
)

type Updater struct {
	DB     *gorm.DB
	Logger *zap.SugaredLogger
	ESPN   espn.SportClient
}

// sportDB returns the short database identifier for the updater's sport.
func (u *Updater) sportDB() string {
	return u.ESPN.SportInfo().SportDB()
}
