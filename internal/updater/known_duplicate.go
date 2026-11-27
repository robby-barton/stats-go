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

const (
	scheduleHome = "home"
	scheduleAway = "away"

	// knownEventLogMessage is the structured warning key for a protected import.
	knownEventLogMessage = "known-event import rejected"
)

type duplicateKey struct {
	sport   sport.Sport
	eventID int64
}

type duplicateRegistry map[duplicateKey]database.Game

// knownDuplicateHeaders lists reviewed retired ESPN event IDs keyed by sport.
// Values are the expected canonical game headers, used only for diagnostics.
// Each updater owns its registry; adding a case requires no import-flow edits.
func knownDuplicateHeaders() duplicateRegistry {
	return duplicateRegistry{
		// Army–Temple (2012): keep the canonical event repaired separately.
		{sport: sport.Football, eventID: 400368353}: {
			GameID: 323220349, Season: 2012, Week: 12,
			StartTime: time.Date(2012, 11, 17, 17, 0, 0, 0, time.UTC),
			HomeID:    349, HomeScore: 32, AwayID: 218, AwayScore: 63,
		},
	}
}

type duplicateRule struct {
	eventID   int64
	canonical database.Game
}

// duplicateRule looks up protection by exact sport and retired event ID.
// Missing or drifted canonical data cannot disable a registered refusal.
func (u *Updater) duplicateRule(id int64) (duplicateRule, bool) {
	sp := u.sportDB()
	canonical, ok := u.duplicateHeaders[duplicateKey{sport: sp, eventID: id}]
	canonical.Sport = sp
	return duplicateRule{eventID: id, canonical: canonical}, ok
}

// matchesHeader compares the reviewed identity without regard to event ID:
// the canonical row is queried by its own ID, and parsed duplicate headers
// legitimately carry the retired ID. Kickoff equality compares instants.
func (r duplicateRule) matchesHeader(actual database.Game) bool {
	want := r.canonical
	return actual.Sport == want.Sport && actual.Season == want.Season && actual.Week == want.Week &&
		actual.StartTime.Equal(want.StartTime) && actual.HomeID == want.HomeID && actual.HomeScore == want.HomeScore &&
		actual.AwayID == want.AwayID && actual.AwayScore == want.AwayScore && actual.Postseason == want.Postseason &&
		actual.Neutral == want.Neutral && actual.ConfGame == want.ConfGame && actual.Retry == want.Retry
}

// scheduleSourceDetail checks the competition fields that the scoreboard DTO
// actually retains. Missing full-header fields are not inferred.
func (r duplicateRule) scheduleSourceDetail(g espn.Game) string {
	if len(g.Competitions) != 1 || len(g.Competitions[0].Competitors) != 2 {
		return "source schedule identity malformed"
	}

	home, away := g.Competitions[0].Competitors[0], g.Competitions[0].Competitors[1]
	if home.HomeAway == scheduleAway {
		home, away = away, home
	}
	want := r.canonical
	if home.HomeAway != scheduleHome || away.HomeAway != scheduleAway ||
		home.ID != want.HomeID || home.Team.ID != want.HomeID || home.Score != want.HomeScore ||
		away.ID != want.AwayID || away.Team.ID != want.AwayID || away.Score != want.AwayScore {
		return "source schedule identity mismatch"
	}
	return ""
}

// ProtectedGame is a game intentionally refused by the known-event guard. It is
// reported in GameUpdateResult.Skipped and is never fetched, retried, or written.
type ProtectedGame struct {
	GameID          int64
	CanonicalGameID int64
	Sport           sport.Sport
	Reason          string
}

func (p ProtectedGame) refusal() error {
	return fmt.Errorf(
		"known-event protection: %s game %d rejected against canonical %d: %s",
		p.Sport, p.GameID, p.CanonicalGameID, p.Reason,
	)
}

// protectedDuplicate logs a registered refusal. A canonical read failure
// cannot change the refusal or block unrelated entries in a batch.
func (u *Updater) protectedDuplicate(rule duplicateRule, sourceDetail string) ProtectedGame {
	reason, err := u.protectedDuplicateReason(rule, sourceDetail)
	if err != nil {
		reason = "canonical game could not be verified"
	}
	p := ProtectedGame{
		GameID: rule.eventID, CanonicalGameID: rule.canonical.GameID,
		Sport: rule.canonical.Sport, Reason: reason,
	}
	fields := []any{"sport", p.Sport, "game_id", p.GameID, "canonical_game_id", p.CanonicalGameID, "reason", reason}
	if err != nil {
		fields = append(fields, "error", err)
	}
	u.logger.Warnw(knownEventLogMessage, fields...)
	return p
}

// protectedDuplicateReason uses canonical state for diagnostics only.
func (u *Updater) protectedDuplicateReason(rule duplicateRule, sourceDetail string) (string, error) {
	want := rule.canonical
	var canonical database.Game
	err := u.db.Where("game_id = ? and sport = ?", want.GameID, want.Sport).First(&canonical).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "canonical game missing", nil
	case err != nil:
		return "", fmt.Errorf("known-event protection: reading canonical %d: %w", want.GameID, err)
	case !rule.matchesHeader(canonical):
		return "canonical identity mismatch", nil
	case sourceDetail != "":
		return sourceDetail, nil
	default:
		return "protected duplicate; canonical stats preserved (no redirect or retry)", nil
	}
}
