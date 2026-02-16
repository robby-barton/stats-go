package updater

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/writer"
)

type Updater struct {
	DB     *gorm.DB
	Logger *zap.SugaredLogger
	Writer writer.Writer
	ESPN   espn.SportClient
}

// sportDB returns the short database identifier for the updater's sport.
func (u *Updater) sportDB() string {
	return u.ESPN.SportInfo().SportDB()
}
