// Package fbsprior implements the opt-in FBS early-season prior blend used by
// the scheduled football updater. It contains the ncaaf-shadow-prior/1
// artifact contract (a pinned, reviewed preseason prior produced by the
// research shadow-freeze tool), and the blend math fixed by the 2026
// retrospective research: rho 0.9, a 4-game (W+L+T, including FBS-FCS games)
// per-team expiry, a 56-Eastern-calendar-day full bypass measured from the
// verified preseason anchor, and an SD floor of 0.05.
//
// The package never reads AP inputs, never retrains at runtime, and performs
// no database I/O. The artifact is a plain local file; its SHA-256 pin
// establishes byte identity only, not provenance.
package fbsprior

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/robby-barton/stats-go/internal/ranking"
)

// The fixed research policy constants. None of these are operator-tunable.
const (
	// Schema is the only supported artifact schema.
	Schema = "ncaaf-shadow-prior/1"
	// Rho scales the frozen prior z-score. There is deliberately no
	// restandardization after blending, so the combined field's variance
	// shrinks below 1 by design.
	Rho = 0.9
	// ExpirationGames is the baseline game count (W+L+T, including FBS-FCS
	// games) at which a team's shrinkage weight reaches zero.
	ExpirationGames = int64(4)
	// CalendarDays is the Eastern calendar-day distance from the verified
	// preseason anchor to the full-bypass instant.
	CalendarDays = 56
	// SDFloor is the lower bound for the standard deviations used as
	// z-score denominators.
	SDFloor = 0.05
	// A season prior is a small FBS field and one value per team. Bound
	// startup reads so bad configuration cannot allocate arbitrary memory.
	maxArtifactBytes = 1 << 20
)

// FieldRow is one frozen FBS field identity.
type FieldRow struct {
	TeamID   int64  `json:"team_id"`
	TeamName string `json:"team_name"`
}

// Prior is a validated, pinned ncaaf-shadow-prior/1 artifact. Construct it
// only through Load, which enforces the full contract before returning.
type Prior struct {
	// Season is the target season the prior was frozen for.
	Season int64
	// FreezeDate is the predeclared preseason Eastern anchor date
	// (YYYY-MM-DD) the prior was frozen at.
	FreezeDate string
	// Cutoff is the preseason anchor: midnight Eastern on FreezeDate,
	// expressed in UTC.
	Cutoff time.Time
	// CalendarExpiry is Cutoff plus CalendarDays Eastern calendar days;
	// at or after this instant every team bypasses the blend.
	CalendarExpiry time.Time
	// AsOf is the observed-clock instant recorded inside the artifact.
	AsOf time.Time
	// CapturedAt is the post-capture clock instant recorded inside the
	// artifact. The blend requires the observed clock to be at or after it.
	CapturedAt time.Time
	// Field is the frozen FBS field: sorted, unique, positive team IDs with
	// nonempty names. The blend requires an exact ID and name match with
	// the current FBS field.
	Field []FieldRow
	// ZStrength maps every frozen-field team ID to its standardized prior
	// strength (population z-score over the full frozen field).
	ZStrength map[int64]float64
	// MissingHistory lists (ascending) the frozen-field team IDs with no
	// retained historical game against another frozen-field team.
	MissingHistory []int64
	// Degenerate reports that no usable strength signal existed; the
	// frozen prior is all-zero in that case.
	Degenerate bool
	// InputSHA256 is the embedded ranker-input fingerprint recorded inside
	// the artifact. It is metadata, not the external pin.
	InputSHA256 string
	// SHA256 is the verified digest of the exact artifact bytes.
	SHA256 string
}

// artifact file layout (mirrors the research shadow-freeze output).
type artifactFile struct {
	Schema         string        `json:"schema"`
	Season         int64         `json:"season"`
	FreezeDate     string        `json:"freeze_date"`
	Cutoff         time.Time     `json:"cutoff"`
	CalendarExpiry time.Time     `json:"calendar_expiry"`
	AsOf           time.Time     `json:"as_of"`
	CapturedAt     time.Time     `json:"captured_at"`
	Field          []FieldRow    `json:"field"`
	Prior          artifactPrior `json:"prior"`
	InputSHA256    string        `json:"input_sha256"`
	Limitations    []string      `json:"limitations"`
}

type artifactPrior struct {
	ZStrength      map[int64]float64 `json:"z_strength"`
	MissingHistory []int64           `json:"missing_history"`
	Degenerate     bool              `json:"degenerate"`
	FieldSize      int               `json:"field_size"`
}

// Load reads the artifact at path, verifies its bytes against the operator's
// SHA-256 pin, and validates the full ncaaf-shadow-prior/1 contract. It
// returns an error on any missing file, digest mismatch, malformed payload,
// invalid clock, or inconsistent field: the caller must warn and stay on the
// exact baseline rather than block ordinary updater operation.
func Load(path, sha256Pin string) (*Prior, error) {
	if !isHex64(sha256Pin) {
		return nil, errors.New("fbsprior: sha256 pin must be 64 lowercase hex digits")
	}
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("fbsprior: read prior: %w", err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if digest != sha256Pin {
		return nil, fmt.Errorf("fbsprior: artifact digest %s does not match the pinned sha256", digest)
	}

	var file artifactFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("fbsprior: parse prior: %w", err)
	}
	if err := validate(file); err != nil {
		return nil, fmt.Errorf("fbsprior: invalid prior: %w", err)
	}

	return &Prior{
		Season:         file.Season,
		FreezeDate:     file.FreezeDate,
		Cutoff:         file.Cutoff,
		CalendarExpiry: file.CalendarExpiry,
		AsOf:           file.AsOf,
		CapturedAt:     file.CapturedAt,
		Field:          file.Field,
		ZStrength:      file.Prior.ZStrength,
		MissingHistory: file.Prior.MissingHistory,
		Degenerate:     file.Prior.Degenerate,
		InputSHA256:    file.InputSHA256,
		SHA256:         digest,
	}, nil
}

func readArtifact(path string) ([]byte, error) {
	// Check before opening so a configured pipe or device is not read.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("prior must be a regular file")
	}
	if info.Size() > maxArtifactBytes {
		return nil, errors.New("prior exceeds the 1 MiB size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArtifactBytes {
		return nil, errors.New("prior exceeds the 1 MiB size limit")
	}
	return data, nil
}

// CoversField reports whether the ranking's FBS teams match the prior's
// frozen field exactly, by team ID and name, with no team on either side.
func (p *Prior) CoversField(teams ranking.TeamList) bool {
	if len(teams) != len(p.Field) {
		return false
	}
	rows := make([]FieldRow, 0, len(teams))
	for id, team := range teams {
		if team == nil {
			return false
		}
		rows = append(rows, FieldRow{TeamID: id, TeamName: team.Name})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TeamID < rows[j].TeamID })
	for i := range rows {
		if rows[i] != p.Field[i] {
			return false
		}
	}
	return true
}

// validate enforces the full artifact contract: header, verified Eastern
// clocks, ordered timestamps, input-hash format, field identity, and
// strength-payload consistency.
func validate(file artifactFile) error {
	if file.Schema != Schema {
		return fmt.Errorf("unsupported schema %q", file.Schema)
	}
	if file.Season <= 0 || file.FreezeDate == "" {
		return errors.New("header is incomplete")
	}
	anchor, err := easternAnchor(file.FreezeDate)
	if err != nil {
		return fmt.Errorf("freeze date: %w", err)
	}
	if !file.Cutoff.Equal(anchor) || !file.CalendarExpiry.Equal(anchor.AddDate(0, 0, CalendarDays)) {
		return errors.New("cutoff/expiry is not the Eastern anchor and day-56 boundary")
	}
	if file.Cutoff.IsZero() || file.CalendarExpiry.IsZero() || file.AsOf.IsZero() ||
		file.CapturedAt.IsZero() {
		return errors.New("timestamps are incomplete")
	}
	if file.Cutoff.After(file.AsOf) || file.AsOf.After(file.CapturedAt) ||
		file.AsOf.In(anchor.Location()).Format(time.DateOnly) != file.FreezeDate ||
		int64(anchor.Year()) != file.Season {
		return errors.New("clock or freeze-date ordering violated")
	}
	if !isHex64(file.InputSHA256) {
		return errors.New("input hash is not 64 lowercase hex digits")
	}
	if err := validateFieldRows(file.Field); err != nil {
		return err
	}
	return validatePayload(file)
}

// easternAnchor parses a YYYY-MM-DD date as midnight Eastern (the verified
// preseason anchor convention shared with the research tooling).
func easternAnchor(date string) (time.Time, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, errors.New("eastern timezone data unavailable")
	}
	return time.ParseInLocation(time.DateOnly, date, loc)
}

func validateFieldRows(field []FieldRow) error {
	if len(field) == 0 {
		return errors.New("field is empty")
	}
	previous := int64(0)
	for _, row := range field {
		if row.TeamID <= 0 || row.TeamID <= previous {
			return fmt.Errorf("field team %d is invalid or unsorted", row.TeamID)
		}
		if row.TeamName == "" {
			return fmt.Errorf("field team %d has no name", row.TeamID)
		}
		previous = row.TeamID
	}
	return nil
}

func validatePayload(file artifactFile) error {
	if file.Prior.FieldSize != len(file.Field) || len(file.Prior.ZStrength) != len(file.Field) {
		return errors.New("strength map does not cover the frozen field exactly")
	}
	fieldIDs := make(map[int64]bool, len(file.Field))
	for _, row := range file.Field {
		fieldIDs[row.TeamID] = true
	}
	zeros := true
	for id, z := range file.Prior.ZStrength {
		if !fieldIDs[id] {
			return fmt.Errorf("strength covers unknown team %d", id)
		}
		if !finite(z) {
			return fmt.Errorf("strength for team %d is not finite", id)
		}
		if z != 0 {
			zeros = false
		}
	}
	previous := int64(0)
	for _, id := range file.Prior.MissingHistory {
		if id <= 0 || id <= previous || !fieldIDs[id] {
			return fmt.Errorf("missing history %d is invalid, unsorted or outside the field", id)
		}
		previous = id
	}
	if file.Prior.Degenerate && !zeros {
		return errors.New("degenerate prior claims nonzero strength")
	}
	return nil
}

func isHex64(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
