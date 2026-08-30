package updater

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/sport"
)

// Updater orchestrates ESPN fetches, database persistence, and ranking
// computation for a single sport. Construct it with NewUpdater, which
// validates the dependencies; the fields are unexported so wiring always
// goes through that seam.
type Updater struct {
	db     *gorm.DB
	logger *zap.SugaredLogger
	espn   espn.SportClient
}

// NewUpdater validates its inputs and returns an Updater. It returns an
// error when the DB, logger, or ESPN client is nil, or when the client's
// sport is not a known persistence sport — mirroring ranking.NewRanker so
// bad wiring fails at construction instead of panicking mid-update.
func NewUpdater(db *gorm.DB, log *zap.SugaredLogger, client espn.SportClient) (*Updater, error) {
	if db == nil {
		return nil, errors.New("updater: nil DB")
	}
	if log == nil {
		return nil, errors.New("updater: nil logger")
	}
	if client == nil {
		return nil, errors.New("updater: nil ESPN client")
	}
	if err := client.SportInfo().DBSport().Validate(); err != nil {
		return nil, fmt.Errorf("updater: %w", err)
	}
	return &Updater{db: db, logger: log, espn: client}, nil
}

// sportDB returns the persistence identifier for the updater's sport.
func (u *Updater) sportDB() sport.Sport {
	return u.espn.SportInfo().DBSport()
}
