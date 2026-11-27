// Package fbsprior implements the opt-in FBS early-season prior blend used by
// the scheduled football updater. It contains the ncaaf-season-prior/1
// artifact contract (a versioned, checksum-verified preseason prior that the
// scheduled updater itself freezes and persists) and the blend math fixed by
// the 2026 retrospective research: rho 0.9, a 4-game (W+L+T, including
// FBS-FCS games) per-team expiry, a 56-Eastern-calendar-day full bypass
// measured from the fixed August 10 midnight Eastern anchor, and an SD floor
// of 0.05.
//
// The package never reads AP inputs, never retrains at runtime, and performs
// no database I/O. The artifact is an opaque versioned text payload: Build
// encodes a validated artifact, Parse verifies the exact stored bytes and
// enforces the full contract on every read.
package fbsprior

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/robby-barton/stats-go/internal/ranking"
)

// The fixed research policy constants. None of these are operator-tunable.
const (
	// Schema is the only supported artifact schema. The old
	// ncaaf-shadow-prior/1 schema (operator-supplied files pinned by a
	// manual SHA-256 flag) is deliberately rejected.
	Schema = "ncaaf-season-prior/1"
	// Rho scales the frozen prior z-score. There is deliberately no
	// restandardization after blending, so the combined field's variance
	// shrinks below 1 by design.
	Rho = 0.9
	// ExpirationGames is the baseline game count (W+L+T, including FBS-FCS
	// games) at which a team's shrinkage weight reaches zero.
	ExpirationGames = int64(4)
	// CalendarDays is the Eastern calendar-day distance from the fixed
	// preseason anchor to the full-bypass instant.
	CalendarDays = 56
	// SDFloor is the lower bound for the standard deviations used as
	// z-score denominators.
	SDFloor = 0.05
	// A season prior is a small FBS field and one value per team. Bound
	// reads so bad stored rows cannot allocate arbitrary memory.
	maxArtifactBytes = 1 << 20
)

// FieldRow is one frozen FBS field identity.
type FieldRow struct {
	TeamID   int64  `json:"team_id"`
	TeamName string `json:"team_name"`
}

// Prior is a validated FBS strength prior. Build and Parse handle the stored
// artifact; NewReconstructed handles the in-memory rebuild without capture
// metadata or a stored-byte digest.
type Prior struct {
	// Season is the target season the prior was frozen for.
	Season int64
	// Anchor is the fixed preseason Eastern anchor date (YYYY-MM-DD,
	// always August 10 of Season). The anchor never moves with the actual
	// freeze date.
	Anchor string
	// AnchorInstant is midnight Eastern on Anchor, expressed in UTC.
	AnchorInstant time.Time
	// CalendarExpiry is AnchorInstant plus CalendarDays Eastern calendar
	// days; at or after this instant every team bypasses the blend.
	CalendarExpiry time.Time
	// FreezeDate is the observed Eastern capture date (YYYY-MM-DD,
	// August 10-16 of Season). It is recorded separately from the anchor so
	// retry dates are visible.
	FreezeDate string
	// AsOf is the observed-clock instant at the start of the freezing
	// attempt, recorded inside the artifact.
	AsOf time.Time
	// CapturedAt is the observed-clock instant at capture, recorded inside
	// the artifact. The blend requires the observed clock to be at or
	// after it.
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
	// InputSHA256 is the deterministic fingerprint of the sorted field and
	// retained ranker inputs recorded inside the artifact. It is provenance
	// metadata, distinct from the checksum of the exact stored bytes.
	InputSHA256 string
	// SHA256 is the verified digest of the exact artifact bytes.
	SHA256 string
}

// artifact is the JSON layout of the stored payload.
type artifact struct {
	Schema         string        `json:"schema"`
	Season         int64         `json:"season"`
	Anchor         string        `json:"anchor"`
	AnchorInstant  time.Time     `json:"anchor_instant"`
	CalendarExpiry time.Time     `json:"calendar_expiry"`
	FreezeDate     string        `json:"freeze_date"`
	AsOf           time.Time     `json:"as_of"`
	CapturedAt     time.Time     `json:"captured_at"`
	Field          []FieldRow    `json:"field"`
	Prior          artifactPrior `json:"prior"`
	InputSHA256    string        `json:"input_sha256"`
}

type artifactPrior struct {
	ZStrength      map[int64]float64 `json:"z_strength"`
	MissingHistory []int64           `json:"missing_history"`
	Degenerate     bool              `json:"degenerate"`
	FieldSize      int               `json:"field_size"`
}

// FreezeOutput is the validated freeze result the updater hands to Build.
type FreezeOutput struct {
	Season         int64
	FreezeDate     string
	AsOf           time.Time
	CapturedAt     time.Time
	Field          []FieldRow
	ZStrength      map[int64]float64
	MissingHistory []int64
	Degenerate     bool
	InputSHA256    string
}

// Build assembles the versioned artifact from a computed freeze result,
// encodes it deterministically, validates the exact encoded bytes, and
// returns the bytes with the parsed Prior carrying their SHA-256 digest.
func Build(out FreezeOutput) ([]byte, *Prior, error) {
	anchor, err := AnchorInstantFor(out.Season)
	if err != nil {
		return nil, nil, err
	}
	// Encode the field sorted by team ID regardless of the input order so
	// equivalent freeze results encode to identical bytes.
	field := make([]FieldRow, len(out.Field))
	copy(field, out.Field)
	sort.Slice(field, func(i, j int) bool { return field[i].TeamID < field[j].TeamID })
	file := artifact{
		Schema:         Schema,
		Season:         out.Season,
		Anchor:         anchor.In(easternZone()).Format(time.DateOnly),
		AnchorInstant:  anchor,
		CalendarExpiry: anchor.AddDate(0, 0, CalendarDays),
		FreezeDate:     out.FreezeDate,
		AsOf:           out.AsOf.UTC(),
		CapturedAt:     out.CapturedAt.UTC(),
		Field:          field,
		Prior: artifactPrior{
			ZStrength:      out.ZStrength,
			MissingHistory: out.MissingHistory,
			Degenerate:     out.Degenerate,
			FieldSize:      len(field),
		},
		InputSHA256: out.InputSHA256,
	}
	data, err := json.Marshal(file)
	if err != nil {
		return nil, nil, fmt.Errorf("fbsprior: encode prior: %w", err)
	}
	prior, err := Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("fbsprior: built artifact is invalid: %w", err)
	}
	return data, prior, nil
}

// Parse verifies the exact artifact bytes and validates the full
// ncaaf-season-prior/1 contract. It returns an error on an oversized or
// malformed payload, an unsupported (including legacy) schema, an invalid
// clock, or an inconsistent field: the caller must warn and stay on the
// exact baseline rather than block ordinary updater operation.
func Parse(data []byte) (*Prior, error) {
	if len(data) > maxArtifactBytes {
		return nil, errors.New("fbsprior: prior exceeds the 1 MiB size limit")
	}

	var file artifact
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("fbsprior: parse prior: %w", err)
	}
	if err := validate(file); err != nil {
		return nil, fmt.Errorf("fbsprior: invalid prior: %w", err)
	}

	sum := sha256.Sum256(data)
	return &Prior{
		Season:         file.Season,
		Anchor:         file.Anchor,
		AnchorInstant:  file.AnchorInstant,
		CalendarExpiry: file.CalendarExpiry,
		FreezeDate:     file.FreezeDate,
		AsOf:           file.AsOf,
		CapturedAt:     file.CapturedAt,
		Field:          file.Field,
		ZStrength:      file.Prior.ZStrength,
		MissingHistory: file.Prior.MissingHistory,
		Degenerate:     file.Prior.Degenerate,
		InputSHA256:    file.InputSHA256,
		SHA256:         hex.EncodeToString(sum[:]),
	}, nil
}

// NewReconstructed validates prior strengths computed by a ranking rebuild.
// A rebuild was never frozen, so it does not carry capture timestamps or an
// artifact checksum. The fixed season anchor and expiry still apply.
func NewReconstructed(
	season int64,
	field []FieldRow,
	strength map[int64]float64,
	missing []int64,
	inputSHA256 string,
) (*Prior, error) {
	anchor, err := AnchorInstantFor(season)
	if err != nil {
		return nil, err
	}
	if !isHex64(inputSHA256) {
		return nil, errors.New("fbsprior: input hash is not 64 lowercase hex digits")
	}
	if err := validateFieldRows(field); err != nil {
		return nil, err
	}
	if err := validatePayload(field, artifactPrior{
		ZStrength: strength, MissingHistory: missing, FieldSize: len(field),
	}); err != nil {
		return nil, err
	}
	return &Prior{
		Season:         season,
		Anchor:         anchor.In(easternZone()).Format(time.DateOnly),
		AnchorInstant:  anchor,
		CalendarExpiry: anchor.AddDate(0, 0, CalendarDays),
		Field:          field,
		ZStrength:      strength,
		MissingHistory: missing,
		InputSHA256:    inputSHA256,
	}, nil
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

// validate enforces the full artifact contract: header, the fixed August 10
// midnight Eastern anchor and day-56 expiry, an actual Eastern freeze date
// inside the August 10-16 window, ordered timestamps, input-hash format,
// field identity, and strength-payload consistency.
func validate(file artifact) error {
	if file.Schema != Schema {
		return fmt.Errorf("unsupported schema %q", file.Schema)
	}
	if file.Season <= 0 || file.Anchor == "" || file.FreezeDate == "" {
		return errors.New("header is incomplete")
	}
	anchorInstant, err := AnchorInstantFor(file.Season)
	if err != nil {
		return fmt.Errorf("anchor: %w", err)
	}
	anchorDate := anchorInstant.In(easternZone()).Format(time.DateOnly)
	if file.Anchor != anchorDate || !file.AnchorInstant.Equal(anchorInstant) ||
		!file.CalendarExpiry.Equal(anchorInstant.AddDate(0, 0, CalendarDays)) {
		return errors.New("anchor/expiry is not the fixed August 10 midnight Eastern anchor and day-56 boundary")
	}
	if file.AnchorInstant.IsZero() || file.CalendarExpiry.IsZero() ||
		file.AsOf.IsZero() || file.CapturedAt.IsZero() {
		return errors.New("timestamps are incomplete")
	}

	// The freeze date is the real observed Eastern capture date: it must be
	// an August 10-16 date of the season year, but it never moves the
	// anchor or the expiry.
	freezeInstant, err := easternMidnight(file.FreezeDate)
	if err != nil {
		return fmt.Errorf("freeze date: %w", err)
	}
	if freezeInstant.Month() != AnchorMonth ||
		freezeInstant.Day() < AnchorDay || freezeInstant.Day() > MaxFreezeDay ||
		int64(freezeInstant.Year()) != file.Season {
		return errors.New("freeze date is not an August 10-16 Eastern date of the season year")
	}

	if file.AsOf.Before(file.AnchorInstant) || file.CapturedAt.Before(file.AsOf) ||
		file.AsOf.In(easternZone()).Format(time.DateOnly) != file.FreezeDate ||
		file.CapturedAt.In(easternZone()).Format(time.DateOnly) != file.FreezeDate {
		return errors.New("clock or freeze-date ordering violated")
	}
	if !isHex64(file.InputSHA256) {
		return errors.New("input hash is not 64 lowercase hex digits")
	}
	if err := validateFieldRows(file.Field); err != nil {
		return err
	}
	return validatePayload(file.Field, file.Prior)
}

// easternMidnight parses a YYYY-MM-DD date as midnight Eastern (the verified
// preseason anchor convention shared with the research tooling), normalized
// to UTC so encoded bytes are deterministic.
func easternMidnight(date string) (time.Time, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.ParseInLocation(time.DateOnly, date, loc)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

// easternZone is used for display only after parsing the Eastern anchor
// has already confirmed that the time zone is available.
func easternZone() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
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

func validatePayload(field []FieldRow, prior artifactPrior) error {
	if prior.FieldSize != len(field) || len(prior.ZStrength) != len(field) {
		return errors.New("strength map does not cover the frozen field exactly")
	}
	fieldIDs := make(map[int64]bool, len(field))
	for _, row := range field {
		fieldIDs[row.TeamID] = true
	}
	zeros := true
	for id, z := range prior.ZStrength {
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
	for _, id := range prior.MissingHistory {
		if id <= 0 || id <= previous || !fieldIDs[id] {
			return fmt.Errorf("missing history %d is invalid, unsorted or outside the field", id)
		}
		previous = id
	}
	if prior.Degenerate && !zeros {
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
