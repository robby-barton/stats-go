package updater

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/sport"
)

// This exact pair is the reviewed 2012 Army–Temple repair, not a general dedup
// rule. The canonical repair was applied separately; this guard only prevents
// the retired duplicate from being reimported. Keep the identity in sync with
// docs/ncaaf-reimport-guard.md.
const (
	canonicalArmyTempleID int64 = 323220349
	duplicateArmyTempleID int64 = 400368353
	scheduleHome                = "home"
	scheduleAway                = "away"

	// knownEventLogMessage is the structured warning key for a protected import.
	knownEventLogMessage = "known-event import rejected"
)

// armyTempleIdentity reports whether g is the full, reviewed canonical header.
// It is used for diagnostics only: the duplicate is refused even when this
// returns false.
func armyTempleIdentity(g database.Game) bool {
	return g.Sport == sport.Football && g.Season == 2012 && g.Week == 12 &&
		g.StartTime.Equal(time.Date(2012, 11, 17, 17, 0, 0, 0, time.UTC)) &&
		g.HomeID == 349 && g.HomeScore == 32 && g.AwayID == 218 && g.AwayScore == 63 &&
		g.Postseason == 0 && !g.Neutral && !g.ConfGame && g.Retry == 0
}

// isProtectedDuplicate reports whether id is the retired Army–Temple duplicate
// for this updater's sport. The decision is unconditional; absent or drifted
// canonical metadata cannot disable it.
func (u *Updater) isProtectedDuplicate(id int64) bool {
	return u.sportDB() == sport.Football && id == duplicateArmyTempleID
}

// ProtectedGame is a game intentionally refused by the known-event guard. It is
// reported in GameUpdateResult.Skipped and is never fetched, retried, or
// written.
type ProtectedGame struct {
	GameID int64
	Reason string
}

// refusal returns the direct-call refusal error for a protected game.
func (p ProtectedGame) refusal() error {
	return fmt.Errorf(
		"known-event protection: ncaaf game %d rejected against canonical %d: %s",
		p.GameID, canonicalArmyTempleID, p.Reason,
	)
}

// protectedDuplicate records a refusal and logs the diagnostic. A canonical
// read failure cannot change the refusal or block unrelated entries in a batch.
func (u *Updater) protectedDuplicate(id int64, sourceDetail string) ProtectedGame {
	reason, err := u.protectedDuplicateReason(sourceDetail)
	if err != nil {
		reason = "canonical game could not be verified"
		u.logger.Warnw(knownEventLogMessage,
			"game_id", id, "canonical_game_id", canonicalArmyTempleID, "reason", reason, "error", err)
	} else {
		u.logger.Warnw(knownEventLogMessage,
			"game_id", id, "canonical_game_id", canonicalArmyTempleID, "reason", reason)
	}
	return ProtectedGame{GameID: id, Reason: reason}
}

// protectedDuplicateReason explains why the protected duplicate is refused.
// Canonical state only changes the diagnostic, never the outcome. sourceDetail
// carries an optional source-identity mismatch found before the canonical read;
// it is a diagnostic string, not a control-flow error.
func (u *Updater) protectedDuplicateReason(sourceDetail string) (string, error) {
	var canonical database.Game
	err := u.db.Where("game_id = ? and sport = ?", canonicalArmyTempleID, sport.Football).
		First(&canonical).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "canonical game missing", nil
	case err != nil:
		return "", fmt.Errorf("known-event protection: reading canonical %d: %w", canonicalArmyTempleID, err)
	case !armyTempleIdentity(canonical):
		return "canonical identity mismatch", nil
	case sourceDetail != "":
		return sourceDetail, nil
	default:
		return "protected duplicate; canonical stats preserved (no redirect or retry)", nil
	}
}

// armyTempleScheduleSourceDetail validates the schedule DTO fields that
// site.api retains. Scores and competitor identity are checked; missing
// full-header fields are not invented. It returns an empty string when the
// source matches and is diagnostic only.
func armyTempleScheduleSourceDetail(g espn.Game) string {
	if len(g.Competitions) != 1 || len(g.Competitions[0].Competitors) != 2 {
		return "source schedule identity malformed"
	}

	home, away := g.Competitions[0].Competitors[0], g.Competitions[0].Competitors[1]
	if home.HomeAway == scheduleAway {
		home, away = away, home
	}
	if home.HomeAway != scheduleHome || away.HomeAway != scheduleAway ||
		home.ID != 349 || home.Team.ID != 349 || home.Score != 32 ||
		away.ID != 218 || away.Team.ID != 218 || away.Score != 63 {
		return "source schedule identity mismatch"
	}
	return ""
}
