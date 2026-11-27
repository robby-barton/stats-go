package updater

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/sport"
	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
)

// The freeze loader's overflow bounds, following the research replay bounds:
// an FBS field is small (512 rows) and the two donor seasons of retained
// games are bounded (8192 rows). Exceeding either bound is treated as
// unusable data, not as something to truncate.
const (
	fbsPriorMaxFieldRows = 512
	fbsPriorMaxGameRows  = 8192

	// postgresDialect is gorm's dialect name for PostgreSQL, used to pick
	// the transaction isolation and locking behavior.
	postgresDialect = "postgres"
)

// fbsFreezeInput is the bounded, freeze-specific input for one season's
// prior: the sorted named FBS field for the target year, the retained donor
// games, and the deterministic fingerprint recorded inside the artifact.
type fbsFreezeInput struct {
	Season int64
	Cutoff time.Time
	Field  []fbsprior.FieldRow
	Games  []ranking.Game
	// InputSHA256 fingerprints the sorted field and retained games; it is
	// provenance metadata, distinct from the stored-byte checksum.
	InputSHA256 string
}

// RunFBSPriorFreezeTask is the automatic, idempotent preseason task behind
// the schedule's --fbs-early-season-prior flag. It requires a football updater
// and is a no-op outside August 10-16 Eastern or during another attempt.
// The target season is the Eastern calendar year; a season whose metadata is wholly
// absent is initialized first (the same annual path the scheduled job uses).
// Metadata that exists but fails coverage is never rewritten: the attempt
// warns and a later retry in the window picks it up.
func (u *Updater) RunFBSPriorFreezeTask(ctx context.Context, now time.Time) error {
	if u.espn.SportInfo() != espn.CollegeFootball {
		return errors.New("fbs prior freeze: football updater required")
	}
	season, _, inWindow := fbsprior.SeasonWindow(now)
	if !inWindow {
		return nil
	}
	if !u.priorFreezeMu.TryLock() {
		u.logger.Info("fbs prior freeze: attempt already running; skipping")
		return nil
	}
	defer u.priorFreezeMu.Unlock()

	db := u.db.WithContext(ctx)
	existing, err := fbsPriorRowExists(db, season)
	if err != nil {
		return fmt.Errorf("fbs prior freeze: checking stored prior for %d: %w", season, err)
	}
	if existing {
		return nil
	}

	exists, err := u.seasonsExist(season)
	if err != nil {
		return fmt.Errorf("fbs prior freeze: checking season %d metadata: %w", season, err)
	}
	if !exists {
		added, err := u.UpdateTeamSeasonsForYear(ctx, season, false)
		if err != nil {
			return fmt.Errorf("fbs prior freeze: initializing season %d metadata: %w", season, err)
		}
		u.logger.Infof("fbs prior freeze: initialized %d team seasons for %d", added, season)
	}

	return u.freezeFBSPrior(db, season, now)
}

// fbsPriorRowExists reports whether any row — valid or not — is stored for
// the season. A stored row is immutable: even a corrupt one is never
// recomputed or replaced, and rankings report the validation failure while
// staying on baseline.
func fbsPriorRowExists(db *gorm.DB, season int64) (bool, error) {
	var count int64
	err := db.Model(&database.FBSSeasonPrior{}).
		Where("season = ?", season).Count(&count).Error
	return count > 0, err
}

// freezeFBSPrior computes and persists one immutable prior for the season.
// The game guard, donor snapshot, and conflict-do-nothing insert run in a
// single transaction. The freeze window is rechecked just before insertion.
func (u *Updater) freezeFBSPrior(db *gorm.DB, season int64, now time.Time) error {
	inWindowSeason, _, inWindow := fbsprior.SeasonWindow(now)
	if !inWindow || inWindowSeason != season {
		return fmt.Errorf("fbs prior freeze: outside the August 10-16 Eastern window (season %d)", season)
	}

	cutoff, err := fbsprior.AnchorInstantFor(season)
	if err != nil {
		return fmt.Errorf("fbs prior freeze: %w", err)
	}
	var frozen *fbsprior.Prior
	err = freezeTransaction(db, func(tx *gorm.DB) error {
		existing, err := fbsPriorRowExists(tx, season)
		if err != nil {
			return fmt.Errorf("fbs prior freeze: recheck stored row: %w", err)
		}
		if existing {
			return nil
		}

		// Never auto-freeze midseason: any persisted football game for the
		// target season — any phase, either division — or an unclassified
		// retry placeholder (the retry writer can persist season=0 rows)
		// refuses the freeze.
		var games int64
		if err := tx.Model(&database.Game{}).
			Where("sport = ? and (season = ? or season = 0)", sport.Football, season).
			Count(&games).Error; err != nil {
			return fmt.Errorf("fbs prior freeze: game guard for season %d: %w", season, err)
		}
		if games > 0 {
			return fmt.Errorf("fbs prior freeze: refusing to freeze season %d: %d football game rows are already persisted",
				season, games)
		}

		in, err := loadFBSFreezeInput(tx, season, cutoff)
		if err != nil {
			return fmt.Errorf("fbs prior freeze: season %d input unusable: %w", season, err)
		}
		strength, err := freezeStrength(in)
		if err != nil {
			return fmt.Errorf("fbs prior freeze: season %d strength unusable: %w", season, err)
		}

		captured := nowFunc()
		capturedSeason, freezeDate, capturedInWindow := fbsprior.SeasonWindow(captured)
		if !capturedInWindow || capturedSeason != season || captured.Before(now) {
			return fmt.Errorf("fbs prior freeze: invalid capture clock %s for attempt started at %s",
				captured.Format(time.RFC3339), now.Format(time.RFC3339))
		}

		data, prior, err := fbsprior.Build(fbsprior.FreezeOutput{
			Season:         season,
			FreezeDate:     freezeDate,
			AsOf:           now,
			CapturedAt:     captured,
			Field:          in.Field,
			ZStrength:      strength.Z,
			MissingHistory: strength.MissingHistory,
			Degenerate:     false,
			InputSHA256:    in.InputSHA256,
		})
		if err != nil {
			return fmt.Errorf("fbs prior freeze: season %d artifact: %w", season, err)
		}

		row := database.FBSSeasonPrior{
			Season:   season,
			Artifact: string(data),
			Checksum: prior.SHA256,
		}
		insert := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "season"}},
			DoNothing: true,
		}).Create(&row)
		if insert.Error != nil {
			return fmt.Errorf("fbs prior freeze: insert season %d: %w", season, insert.Error)
		}
		if insert.RowsAffected == 1 {
			frozen = prior
		}
		return nil
	})
	if err == nil && frozen != nil {
		u.logger.Infof(
			"fbs prior freeze: frozen season %d on %s (captured %s, anchor %s, expiry %s, "+
				"field %d, missing history %d, sha256 %s)",
			season, frozen.FreezeDate, frozen.CapturedAt.Format(time.RFC3339),
			frozen.AnchorInstant.Format(time.RFC3339), frozen.CalendarExpiry.Format(time.RFC3339),
			len(frozen.Field), len(frozen.MissingHistory), frozen.SHA256)
	}
	return err
}

// freezeTransaction locks before taking a repeatable snapshot, serializing
// freezes and game writes. The SQLite no-op update acquires its write lock
// without creating or deleting any rows.
func freezeTransaction(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	var opts *sql.TxOptions
	if db.Name() == postgresDialect {
		opts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		switch tx.Name() {
		case postgresDialect:
			if err := tx.Exec("SET LOCAL lock_timeout = '10s'").Error; err != nil {
				return fmt.Errorf("fbs prior freeze: bounding lock wait: %w", err)
			}
			for _, table := range []string{"fbs_season_priors", "games"} {
				if err := tx.Exec("LOCK TABLE " + table + " IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
					return fmt.Errorf("fbs prior freeze: locking %s: %w", table, err)
				}
			}
		case "sqlite":
			if err := tx.Exec("UPDATE fbs_season_priors SET season = season WHERE 1 = 0").Error; err != nil {
				return fmt.Errorf("fbs prior freeze: acquiring the write lock: %w", err)
			}
		default:
			return fmt.Errorf("fbs prior freeze: unsupported dialect %q", tx.Name())
		}
		return fn(tx)
	}, opts)
}

// ---------------------------------------------------------------------------
// Bounded freeze input
// ---------------------------------------------------------------------------

type fbsMetaRow struct {
	TeamID int64
	Name   string
}

// loadFBSFreezeInput builds the bounded freeze input: named FBS metadata for
// the target and prior years, and the retained donor games (seasons
// year-2..year-1, through the fixed anchor, between target-field teams).
// Every refusal is terminal for the attempt: the caller logs it and no
// candidate is published.
func loadFBSFreezeInput(tx *gorm.DB, season int64, cutoff time.Time) (*fbsFreezeInput, error) {
	field, err := loadFBSField(tx, season, "target")
	if err != nil {
		return nil, err
	}
	priorField, err := loadFBSField(tx, season-1, "prior-year")
	if err != nil {
		return nil, err
	}

	fieldIDs := make(map[int64]bool, len(field))
	for _, row := range field {
		fieldIDs[row.TeamID] = true
	}

	// A prior-year FBS team missing from the target field is a departure
	// the loader cannot distinguish from truncated target-year metadata:
	// refuse conservatively and let a later retry (or a repair) decide.
	var departed []int64
	for _, row := range priorField {
		if !fieldIDs[row.TeamID] {
			departed = append(departed, row.TeamID)
		}
	}
	if len(departed) > 0 {
		return nil, fmt.Errorf(
			"%d prior-year FBS team(s) absent from the target field (e.g. %v): refusing an unsupported departure",
			len(departed), sampleIDs(departed, 5))
	}

	games, perSeason, err := loadRetainedGames(tx, season, cutoff, fieldIDs)
	if err != nil {
		return nil, err
	}
	if perSeason[0] == 0 {
		return nil, fmt.Errorf("no usable retained games in donor season %d", season-2)
	}
	if perSeason[1] == 0 {
		return nil, fmt.Errorf("no usable retained games in donor season %d", season-1)
	}

	// A continuing FBS member (in both fields) without a single usable game
	// across the two-year window is indistinguishable from truncated
	// history: refuse. New FBS members without history are fine — the
	// strength step imputes them at the observed-field mean.
	observed := make(map[int64]bool, len(games)*2)
	for _, g := range games {
		observed[g.HomeID] = true
		observed[g.AwayID] = true
	}
	priorIDs := make(map[int64]bool, len(priorField))
	for _, row := range priorField {
		priorIDs[row.TeamID] = true
	}
	for _, row := range field {
		if priorIDs[row.TeamID] && !observed[row.TeamID] {
			return nil, fmt.Errorf("continuing FBS team %d has no usable game across %d-%d",
				row.TeamID, season-2, season-1)
		}
	}

	return &fbsFreezeInput{
		Season:      season,
		Cutoff:      cutoff,
		Field:       field,
		Games:       games,
		InputSHA256: freezeInputFingerprint(field, games),
	}, nil
}

// loadFBSField loads the named FBS field for one year, bounded at
// fbsPriorMaxFieldRows with an overflow-detecting limit.
func loadFBSField(tx *gorm.DB, year int64, label string) ([]fbsprior.FieldRow, error) {
	var rows []fbsMetaRow
	err := tx.Model(&database.TeamSeason{}).
		Select("team_seasons.team_id as team_id, coalesce(team_names.name, '') as name").
		Joins("left join team_names on team_seasons.team_id = team_names.team_id and team_seasons.sport = team_names.sport").
		Where("team_seasons.year = ? and team_seasons.sport = ? and team_seasons.fbs = 1",
			year, sport.Football).
		Order("team_seasons.team_id asc").
		Limit(fbsPriorMaxFieldRows + 1).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("loading %s FBS metadata for %d: %w", label, year, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no %s FBS metadata for %d", label, year)
	}
	if len(rows) > fbsPriorMaxFieldRows {
		return nil, fmt.Errorf("%s FBS field for %d exceeds the %d-row bound", label, year, fbsPriorMaxFieldRows)
	}
	field := make([]fbsprior.FieldRow, 0, len(rows))
	seen := make(map[int64]bool, len(rows))
	previous := int64(0)
	for _, row := range rows {
		if row.TeamID <= 0 || row.TeamID <= previous || seen[row.TeamID] {
			return nil, fmt.Errorf("%s FBS metadata for %d: team %d is invalid or duplicated", label, year, row.TeamID)
		}
		if row.Name == "" {
			return nil, fmt.Errorf("%s FBS metadata for %d: team %d has no name", label, year, row.TeamID)
		}
		seen[row.TeamID] = true
		previous = row.TeamID
		field = append(field, fbsprior.FieldRow{TeamID: row.TeamID, TeamName: row.Name})
	}
	return field, nil
}

// loadRetainedGames loads the bounded donor window: seasons year-2..year-1,
// started at or before the cutoff, between two target-field teams, in the
// stable descending order the ranker's game selection expects.
func loadRetainedGames(
	tx *gorm.DB,
	season int64,
	cutoff time.Time,
	fieldIDs map[int64]bool,
) ([]ranking.Game, [2]int, error) {
	ids := make([]int64, 0, len(fieldIDs))
	for id := range fieldIDs {
		ids = append(ids, id)
	}
	var rows []database.Game
	err := tx.Model(&database.Game{}).
		Where("sport = ? and season >= ? and season <= ? and start_time <= ? and home_id in ? and away_id in ?",
			sport.Football, season-2, season-1, cutoff, ids, ids).
		Order("start_time desc, game_id asc").
		Limit(fbsPriorMaxGameRows + 1).
		Find(&rows).Error
	if err != nil {
		return nil, [2]int{}, fmt.Errorf("loading retained games for %d: %w", season, err)
	}
	if len(rows) > fbsPriorMaxGameRows {
		return nil, [2]int{}, fmt.Errorf("retained games for %d exceed the %d-row bound", season, fbsPriorMaxGameRows)
	}

	games := make([]ranking.Game, 0, len(rows))
	seen := make(map[int64]bool, len(rows))
	perSeason := [2]int{}
	for _, row := range rows {
		if row.GameID <= 0 || seen[row.GameID] {
			return nil, [2]int{}, fmt.Errorf("retained game %d for %d is invalid or duplicated", row.GameID, season)
		}
		if row.HomeID == row.AwayID {
			return nil, [2]int{}, fmt.Errorf("retained game %d for %d has identical home and away teams", row.GameID, season)
		}
		seen[row.GameID] = true
		games = append(games, ranking.Game{
			GameID:    row.GameID,
			Season:    row.Season,
			Week:      row.Week,
			StartTime: row.StartTime,
			HomeID:    row.HomeID,
			HomeScore: row.HomeScore,
			AwayID:    row.AwayID,
			AwayScore: row.AwayScore,
		})
		if row.Season == season-2 {
			perSeason[0]++
		} else {
			perSeason[1]++
		}
	}
	return games, perSeason, nil
}

// freezeInputFingerprint hashes a deterministic serialization of the sorted
// field and the retained games (canonical RFC3339 instants, stable order).
func freezeInputFingerprint(field []fbsprior.FieldRow, games []ranking.Game) string {
	h := sha256.New()
	for _, row := range field {
		fmt.Fprintf(h, "t %d %s\n", row.TeamID, row.TeamName)
	}
	for _, g := range games {
		fmt.Fprintf(h, "g %d %d %d %s %d %d %d %d\n",
			g.GameID, g.Season, g.Week, g.StartTime.UTC().Format(time.RFC3339Nano),
			g.HomeID, g.HomeScore, g.AwayID, g.AwayScore)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sampleIDs(ids []int64, n int) []int64 {
	if len(ids) > n {
		return ids[:n]
	}
	return ids
}

// ---------------------------------------------------------------------------
// Strength (ported research formula)
// ---------------------------------------------------------------------------

type frozenStrength struct {
	Z              map[int64]float64
	MissingHistory []int64
}

// freezeStrength ports the researched freeze formula: recompute the ranking
// over the retained window with the ranker treating season-1 as its current
// season and season-2 as its back season (never executing any backfill
// callback), then standardize the observed SRSNorm values over the full
// target field. No-history newcomers are imputed at the observed-field mean
// before standardizing, so their prior z-score is exactly zero and they are
// listed in MissingHistory. Degenerate or nonfinite results are refused.
func freezeStrength(in *fbsFreezeInput) (*frozenStrength, error) {
	observed := make(map[int64]bool, len(in.Games)*2)
	for _, g := range in.Games {
		observed[g.HomeID] = true
		observed[g.AwayID] = true
	}
	if len(observed) == 0 {
		return nil, errors.New("no observed history")
	}

	obsTeams := make([]ranking.TeamInfo, 0, len(observed))
	for _, row := range in.Field {
		if observed[row.TeamID] {
			obsTeams = append(obsTeams, ranking.TeamInfo{ID: row.TeamID, Name: row.TeamName, FBS: true})
		}
	}

	ranker, err := ranking.NewRanker(ranking.Input{
		Year:      in.Season - 1,
		Week:      0,
		Sport:     sport.Football,
		StartTime: in.Cutoff,
		Teams:     obsTeams,
		Games:     in.Games,
		Backfill:  nil, // never execute caller-supplied callbacks
	})
	if err != nil {
		return nil, fmt.Errorf("historical ranking rejected: %w", err)
	}
	list, err := ranker.CalculateRanking()
	if err != nil {
		return nil, fmt.Errorf("historical ranking failed: %w", err)
	}

	observedNorm := make(map[int64]float64, len(obsTeams))
	for id, team := range list {
		if !isFinite(team.SRSNorm) {
			return nil, fmt.Errorf("team %d SRSNorm is not finite", id)
		}
		observedNorm[id] = team.SRSNorm
	}

	var sum float64
	for _, v := range observedNorm {
		sum += v
	}
	mean := sum / float64(len(observedNorm))

	// Impute no-history teams at the observed-field mean before
	// standardizing: their prior z-score becomes exactly zero.
	values := make(map[int64]float64, len(in.Field))
	var missing []int64
	for _, row := range in.Field {
		if v, ok := observedNorm[row.TeamID]; ok {
			values[row.TeamID] = v
		} else {
			values[row.TeamID] = mean
			missing = append(missing, row.TeamID)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })

	fullValues := make([]float64, 0, len(in.Field))
	for _, row := range in.Field {
		fullValues = append(fullValues, values[row.TeamID])
	}
	_, sd := meanAndPopSD(fullValues)
	if sd == 0 {
		return nil, errors.New("zero variance across the field (degenerate strength)")
	}

	denom := math.Max(sd, fbsprior.SDFloor)
	z := make(map[int64]float64, len(in.Field))
	for _, row := range in.Field {
		value := (values[row.TeamID] - mean) / denom
		if !isFinite(value) {
			return nil, fmt.Errorf("team %d prior z-score is not finite", row.TeamID)
		}
		z[row.TeamID] = roundStable(value)
	}
	return &frozenStrength{Z: z, MissingHistory: missing}, nil
}

// roundStable collapses the ranker's ~1 ulp of map-ordering noise so that
// freezing the same retained inputs always encodes the same stored bytes
// (immutable repeats and equivalent restarts produce identical artifacts).
// Rounding at 12 decimals is far below any meaningful strength difference
// and never affects the blend's own arithmetic.
func roundStable(v float64) float64 {
	rounded := math.Round(v*1e12) / 1e12
	if rounded == 0 {
		return 0 // normalize a possible -0 to +0 for stable encoding
	}
	return rounded
}

// meanAndPopSD returns the arithmetic mean and the population standard
// deviation of the given values (the research helper).
func meanAndPopSD(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var sq float64
	for _, v := range values {
		dev := v - mean
		sq += dev * dev
	}
	return mean, math.Sqrt(sq / float64(len(values)))
}

// isFinite reports whether v is a usable (non-NaN, non-infinite) float.
func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
