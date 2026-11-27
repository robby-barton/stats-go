package updater

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/ranking/load"
	"github.com/robby-barton/stats-go/internal/sport"
	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
)

// ---------------------------------------------------------------------------
// Per-run prior state
// ---------------------------------------------------------------------------

// priorRun carries the per-run state of the opt-in FBS early-season prior:
// the observation clock, the current Eastern season, whether the run
// reconstructs priors in memory (the inline `ranking --all` rebuild only),
// and the memoized prior loads/reconstructions and earliest-kickoff lookups,
// including misses and errors. A fresh priorRun is created for every run, so
// memoization never crosses runs and the scheduled worker still reads the
// stored row fresh on every run.
type priorRun struct {
	now           time.Time
	currentSeason int64
	reconstruct   bool
	priors        map[int64]*fbsprior.Prior
	earliest      map[int64]earliestKickoff
}

type earliestKickoff struct {
	at  time.Time
	err error
}

// newPriorRun starts one prior run. reconstruct enables in-memory prior
// reconstruction for the inline `ranking --all` rebuild; the scheduled
// worker passes false and only ever reads stored rows.
func newPriorRun(now time.Time, reconstruct bool) *priorRun {
	currentSeason, _, _ := fbsprior.SeasonWindow(now)
	return &priorRun{
		now:           now,
		currentSeason: currentSeason,
		reconstruct:   reconstruct,
		priors:        make(map[int64]*fbsprior.Prior),
		earliest:      make(map[int64]earliestKickoff),
	}
}

// priorForWindow resolves the same-season prior for one FBS window: the
// shared window gate (persisted regular-phase game of the loaded season,
// before the fixed day-56 calendar expiry) followed by the memoized
// per-season load or reconstruction. It returns nil for the normal silent
// skips (no persisted game, a non-regular stored phase, a window from
// another season than the loaded one, or the calendar expiry) — the prior
// pipeline is not even entered for those windows.
func (u *Updater) priorForWindow(
	in ranking.Input,
	window load.Window,
	clock time.Time,
	latest bool,
	run *priorRun,
) *fbsprior.Prior {
	if !window.Found || window.Postseason != 0 || window.Season != in.Year {
		return nil
	}
	expiry, err := fbsprior.CalendarExpiryForSeason(window.Season)
	if err != nil {
		return nil
	}
	if !clock.Before(expiry) {
		return nil
	}
	if !latest {
		// For an explicit week, Kickoff belongs to the upcoming week;
		// only games through the historical cutoff count for eligibility.
		earliest, err := u.earliestRegularKickoff(run, in.Year)
		if err != nil {
			u.logger.Warnf("fbs early-season prior: checking first regular kickoff for %d: %v; staying on baseline rankings",
				in.Year, err)
			return nil
		}
		if earliest.IsZero() || earliest.After(clock) {
			return nil
		}
	}
	return u.priorForSeason(run, window.Season)
}

// priorForSeason returns the same-season prior for one season, memoized once
// per season per run (including misses and errors). The scheduled worker
// reads the stored row; the inline rebuild always reconstructs in memory.
func (u *Updater) priorForSeason(run *priorRun, season int64) *fbsprior.Prior {
	if prior, ok := run.priors[season]; ok {
		return prior
	}
	prior := u.priorForSeasonOnce(run, season)
	run.priors[season] = prior
	return prior
}

func (u *Updater) priorForSeasonOnce(run *priorRun, season int64) *fbsprior.Prior {
	if run.reconstruct {
		// The inline rebuild's purpose is to remake priors as it goes: it
		// never reads, validates, or reuses fbs_season_priors — not even a
		// valid stored row — and never writes one. This includes the current
		// Eastern season, so a manual rebuild can blend even when the
		// scheduler has not frozen (or could not freeze) a prior.
		prior, err := u.reconstructPrior(season)
		if err != nil {
			u.logger.Warnf("fbs early-season prior: reconstructing the prior for season %d: %v; staying on baseline rankings",
				season, err)
			return nil
		}
		u.logger.Infof(
			"fbs early-season prior: reconstructed the prior for season %d in memory (anchor %s, expiry %s, "+
				"field %d, missing history %d, input sha256 %s)",
			season, prior.AnchorInstant.Format(time.RFC3339), prior.CalendarExpiry.Format(time.RFC3339),
			len(prior.Field), len(prior.MissingHistory), prior.InputSHA256)
		return prior
	}

	prior, err := u.loadStoredPrior(season)
	switch {
	case err == nil:
		return prior
	case errors.Is(err, gorm.ErrRecordNotFound):
		// The scheduler owns freezing: a missing row means the freeze has
		// not happened (or failed) — never synthesize one here.
		u.logger.Warnf("fbs early-season prior: no frozen prior for season %d; staying on baseline rankings", season)
		return nil
	default:
		u.logger.Warnf("fbs early-season prior: reading the stored prior: %v; staying on baseline rankings", err)
		return nil
	}
}

// loadStoredPrior reads and validates the stored prior row for one season
// (scheduled worker only). It returns gorm.ErrRecordNotFound when no row is
// stored, and a validation error for read failures, checksum mismatches,
// unparsable payloads, or a payload keyed by another season. A stored row is
// immutable: even a corrupt one is never repaired, replaced, or regenerated.
func (u *Updater) loadStoredPrior(season int64) (*fbsprior.Prior, error) {
	var row database.FBSSeasonPrior
	if err := u.db.Where("season = ?", season).First(&row).Error; err != nil {
		return nil, err
	}
	data := []byte(row.Artifact)
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != row.Checksum {
		return nil, fmt.Errorf("stored prior for season %d fails its checksum", season)
	}
	prior, err := fbsprior.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("stored prior for season %d is invalid: %w", season, err)
	}
	if prior.Season != season {
		return nil, fmt.Errorf("stored prior keyed by season %d contains season %d", season, prior.Season)
	}
	return prior, nil
}

// ---------------------------------------------------------------------------
// In-memory reconstruction (inline rebuild only)
// ---------------------------------------------------------------------------

// reconstructPrior uses the same bounded donor inputs, field validation and
// SRS strength calculation as the scheduled freeze. It reads only season
// metadata and donor games in a consistent snapshot. It has no capture
// timestamp and never consults prior storage. Unlike scheduled freezing, it
// can run after the season begins: current-season games are never inputs.
func (u *Updater) reconstructPrior(season int64) (*fbsprior.Prior, error) {
	anchor, err := fbsprior.AnchorInstantFor(season)
	if err != nil {
		return nil, err
	}
	var prior *fbsprior.Prior
	err = priorSnapshot(u.db, func(tx *gorm.DB) error {
		in, err := loadFBSFreezeInput(tx, season, anchor)
		if err != nil {
			return err
		}
		if err := verifyNoTargetOrFutureGames(in, season, anchor); err != nil {
			return err
		}
		strength, err := freezeStrength(in)
		if err != nil {
			return err
		}
		prior, err = fbsprior.NewReconstructed(season, in.Field, strength.Z, strength.MissingHistory, in.InputSHA256)
		return err
	})
	if err != nil {
		return nil, err
	}
	return prior, nil
}

// verifyNoTargetOrFutureGames enforces, on the loaded reconstruction input,
// what the scheduled freeze's midseason guard covers for its own run: no
// target-season game and no game after the anchor may leak into the prior's
// donor window. The loader's query already bounds both; this is the cheap
// defensive recheck on the loaded rows.
func verifyNoTargetOrFutureGames(in *fbsFreezeInput, season int64, anchor time.Time) error {
	for _, g := range in.Games {
		if g.Season >= season {
			return fmt.Errorf("target-season game %d (season %d) leaked into the prior input", g.GameID, g.Season)
		}
		if g.StartTime.After(anchor) {
			return fmt.Errorf("game %d (start %s) leaked past the anchor cutoff %s",
				g.GameID, g.StartTime.UTC().Format(time.RFC3339), anchor.Format(time.RFC3339))
		}
	}
	return nil
}

// priorSnapshot reads reconstruction inputs from one consistent read-only
// snapshot: repeatable read on Postgres (declared read-only so no
// reconstruction step can write); a plain transaction elsewhere (SQLite
// serializes writers, giving the same consistency). Reconstruction never
// writes.
func priorSnapshot(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	var opts *sql.TxOptions
	if db.Name() == postgresDialect {
		opts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	}
	return db.Transaction(fn, opts)
}

// earliestRegularKickoff returns the earliest persisted regular-season
// kickoff for one football season, memoized once per season per run
// (including the zero-time miss when the season has no regular games yet).
// The inline rebuild uses it to require at least one same-season regular
// game through the window cutoff, so a preseason or week-1 snapshot before
// any regular games stays at the exact baseline.
func (u *Updater) earliestRegularKickoff(run *priorRun, season int64) (time.Time, error) {
	if entry, ok := run.earliest[season]; ok {
		return entry.at, entry.err
	}
	var game database.Game
	err := u.db.
		Where("sport = ? and season = ? and postseason = 0", sport.Football, season).
		Order("start_time asc").
		Limit(1).
		Take(&game).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// No regular game yet for the season: the zero time is the miss.
		run.earliest[season] = earliestKickoff{}
		return time.Time{}, nil
	}
	if err != nil {
		run.earliest[season] = earliestKickoff{err: err}
		return time.Time{}, err
	}
	run.earliest[season] = earliestKickoff{at: game.StartTime}
	return game.StartTime, nil
}
