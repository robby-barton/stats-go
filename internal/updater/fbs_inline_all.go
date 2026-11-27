package updater

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
// reconstructs priors in memory (the inline `ranking --all` rebuild only)
// and publishes them, and the memoized prior loads/reconstructions and
// earliest-kickoff lookups, including misses and errors. A fresh priorRun is
// created for every run, so memoization never crosses runs and the scheduled
// worker still reads the stored row fresh on every run.
type priorRun struct {
	now           time.Time
	currentSeason int64
	reconstruct   bool
	publish       bool
	priors        map[int64]*fbsprior.Prior
	failed        map[int64]bool
	readErr       error // datastore failure, not proof of an unusable prior
	earliest      map[int64]earliestKickoff
	// tx, when set, overrides the updater's database handle so the whole
	// scheduled read/calc/write runs inside the publication-serialized
	// transaction. The inline rebuild never sets it: its reads happen
	// before the publication transaction opens.
	tx *gorm.DB
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
		failed:        make(map[int64]bool),
		earliest:      make(map[int64]earliestKickoff),
	}
}

// runDB returns the database handle a run must read through: the serialized
// publication transaction when the scheduled worker runs inside one, and the
// updater's handle otherwise.
func runDB(u *Updater, run *priorRun) *gorm.DB {
	if run != nil && run.tx != nil {
		return run.tx
	}
	return u.db
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
	if run.publish {
		return run.priors[window.Season] // only seasons staged for publication
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
		// never reads, validates, or reuses fbs_season_priors as an input —
		// not even a valid stored row. This includes the current Eastern
		// season, so a manual rebuild can blend even when the scheduler has
		// not frozen (or could not freeze) a prior. The freshly computed
		// prior is later published over the stored row.
		prior, err := u.reconstructPrior(season)
		if err != nil {
			if run.publish && errors.Is(err, errFBSPriorRead) {
				run.readErr = fmt.Errorf("fbs early-season prior: reading inputs for season %d: %w", season, err)
				return nil
			}
			run.failed[season] = true
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

	prior, err := u.loadStoredPrior(runDB(u, run), season)
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
// unparsable payloads, or a payload keyed by another season. The scheduled
// worker never repairs, replaces, or regenerates a stored row — only the
// explicit rebuild publication does.
func (u *Updater) loadStoredPrior(db *gorm.DB, season int64) (*fbsprior.Prior, error) {
	var row database.FBSSeasonPrior
	// Use a savepoint within the scheduled publication transaction: on
	// PostgreSQL a failed SELECT otherwise poisons the baseline write too.
	if err := db.Transaction(func(query *gorm.DB) error {
		return query.Where("season = ?", season).First(&row).Error
	}); err != nil {
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
	var inputErr error
	err = priorSnapshot(u.db, func(tx *gorm.DB) error {
		prior, inputErr = reconstructFromSnapshot(tx, season, anchor)
		return inputErr
	})
	if err != nil && inputErr == nil {
		// Begin/commit/read-transaction errors are not evidence of bad data.
		return nil, fmt.Errorf("%w: snapshot for season %d: %w", errFBSPriorRead, season, err)
	}
	return prior, err
}

func reconstructFromSnapshot(tx *gorm.DB, season int64, anchor time.Time) (*fbsprior.Prior, error) {
	in, err := loadFBSFreezeInput(tx, season, anchor)
	if err != nil {
		return nil, err
	}
	if err := verifyNoTargetOrFutureGames(in, season, anchor); err != nil {
		return nil, err
	}
	strength, err := freezeStrength(in)
	if err != nil {
		return nil, err
	}
	return fbsprior.NewReconstructed(season, in.Field, strength.Z, strength.MissingHistory, in.InputSHA256)
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
	err := runDB(u, run).
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

// ---------------------------------------------------------------------------
// Publication of rebuilt priors (inline rebuild only)
// ---------------------------------------------------------------------------

// fbsPriorPublicationLockKey is the fixed PostgreSQL advisory-lock key that
// serializes the explicit rebuild's publication write phase with the
// opted-in scheduled read/calc/write, so a scheduled run can never read the
// old prior and commit after a corrected rebuild publication. The key is an
// arbitrary constant reserved for this feature.
const fbsPriorPublicationLockKey = int64(0x46627350) // "FbsP"

// publicationTransaction runs fn inside the one transaction that serializes
// the manual rebuild's publication write phase and the opted-in scheduled
// read/calc/write: a single PostgreSQL advisory xact lock, or the SQLite
// write lock acquired up front (a deferred transaction that only upgraded
// after reading could fail and then overwrite). The rebuild takes the lock
// only over its writes — its reads happen before the transaction opens —
// and nothing takes it when the feature is off. A lock or serialization
// failure aborts the transaction; nothing is half-published.
func publicationTransaction(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	var opts *sql.TxOptions
	if db.Name() == postgresDialect {
		// Acquire the advisory lock before the ranking/lookup statements get
		// their snapshots, even if the server default is repeatable read.
		opts = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		switch tx.Name() {
		case postgresDialect:
			if err := tx.Exec("SET LOCAL lock_timeout = '10s'").Error; err != nil {
				return fmt.Errorf("fbs prior publication: bounding lock wait: %w", err)
			}
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)",
				fbsPriorPublicationLockKey).Error; err != nil {
				return fmt.Errorf("fbs prior publication: acquiring the publication lock: %w", err)
			}
		case "sqlite":
			if err := tx.Exec("UPDATE team_week_results SET team_id = team_id WHERE 1 = 0").Error; err != nil {
				return fmt.Errorf("fbs prior publication: acquiring the write lock: %w", err)
			}
		default:
			return fmt.Errorf("fbs prior publication: unsupported dialect %q", tx.Name())
		}
		return fn(tx)
	}, opts)
}

// rebuildPriorSeasons resolves the seasons whose priors the explicit rebuild
// publication stages: every season with persisted ranking windows whose
// fixed August 10 Eastern anchor has been reached, plus the current Eastern
// season when its football metadata exists even before its first game.
// Future years and seasons whose anchor is not yet reached are never staged.
// Each staged season gets exactly one reconstruction attempt (memoized on
// the run): it either yields a publishable prior or invalidates the
// season's stale persisted prior.
func (u *Updater) rebuildPriorSeasons(run *priorRun, years []yearInfo) ([]int64, error) {
	seen := make(map[int64]bool, len(years)+1)
	seasons := make([]int64, 0, len(years)+1)
	add := func(season int64) {
		if season <= 0 || seen[season] {
			return
		}
		anchor, err := fbsprior.AnchorInstantFor(season)
		if err != nil {
			return // impossible for a positive season; skip defensively
		}
		if run.now.Before(anchor) {
			return // the fixed anchor is not reached yet
		}
		seen[season] = true
		seasons = append(seasons, season)
	}
	for _, year := range years {
		// Never stage a future year.
		if year.Year <= run.currentSeason {
			add(year.Year)
		}
	}
	if !seen[run.currentSeason] {
		exists, err := u.seasonsExist(run.currentSeason)
		if err != nil {
			return nil, fmt.Errorf("fbs early-season prior: checking season %d metadata: %w",
				run.currentSeason, err)
		}
		if exists {
			add(run.currentSeason)
		}
	}
	return seasons, nil
}

// publishRebuiltRankings publishes the rebuilt priors and the rebuilt
// rankings in one publication-serialized transaction, so the explicit
// rebuild is never half-applied. If the prior table is unavailable or any
// write fails, the whole publication — priors and rankings alike — rolls
// back.
func (u *Updater) publishRebuiltRankings(
	run *priorRun,
	seasons []int64,
	rankings []database.TeamWeekResult,
) error {
	err := publicationTransaction(u.db, func(tx *gorm.DB) error {
		if err := publishRebuiltPriors(tx, run, seasons); err != nil {
			return err
		}
		return upsertTeamWeekResults(tx, rankings)
	})
	if err != nil {
		return fmt.Errorf("fbs early-season prior: publishing the rebuilt priors and rankings: %w", err)
	}
	for _, season := range seasons {
		if prior := run.priors[season]; prior != nil {
			u.logger.Infof(
				"fbs early-season prior: published the rebuilt prior for season %d (rebuilt %s, source %q, "+
					"anchor %s, expiry %s, field %d, missing history %d, input sha256 %s)",
				season, run.now.UTC().Format(time.RFC3339), fbsprior.RebuildSource,
				prior.AnchorInstant.Format(time.RFC3339), prior.CalendarExpiry.Format(time.RFC3339),
				len(prior.Field), len(prior.MissingHistory), prior.InputSHA256)
		} else {
			u.logger.Infof(
				"fbs early-season prior: no prior remains published for season %d "+
					"(reconstruction failed; the season stays on baseline)", season)
		}
	}
	return nil
}

// publishRebuiltPriors stages the per-season publication decisions inside
// the publication transaction: a successfully reconstructed season replaces
// its persisted prior — including the current season's existing row, which
// the explicit rebuild is allowed to replace midseason — and a season whose
// reconstruction was attempted but could not yield a usable prior has its
// stale persisted prior invalidated, that season's row only, never a broad
// delete. Any failure aborts the whole publication.
func publishRebuiltPriors(tx *gorm.DB, run *priorRun, seasons []int64) error {
	for _, season := range seasons {
		if prior := run.priors[season]; prior != nil {
			data, published, err := fbsprior.BuildRebuilt(season, run.now, prior)
			if err != nil {
				return fmt.Errorf("fbs early-season prior: rebuilding the season %d artifact: %w", season, err)
			}
			row := database.FBSSeasonPrior{
				Season:   season,
				Artifact: string(data),
				Checksum: published.SHA256,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "season"}},
				UpdateAll: true,
			}).Create(&row).Error; err != nil {
				return fmt.Errorf("fbs early-season prior: publishing the rebuilt season %d prior: %w", season, err)
			}
			continue
		}
		if !run.failed[season] {
			return fmt.Errorf("fbs early-season prior: season %d was never attempted; refusing to invalidate", season)
		}
		if err := tx.Where("season = ?", season).Delete(&database.FBSSeasonPrior{}).Error; err != nil {
			return fmt.Errorf("fbs early-season prior: invalidating the stale season %d prior: %w", season, err)
		}
	}
	return nil
}
