package ncaafreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// ManifestSchema is the only accepted manifest schema.
const ManifestSchema = "ncaaf-replay-manifest/1"

// maxWindows bounds a replay slice; this tool compares explicitly listed
// windows only and never claims full-study coverage.
const maxWindows = 60

// WindowSpec is one explicitly listed regular-season window. Cutoff is
// optional: when set, the production-resolved window start must equal it
// exactly, otherwise the run fails closed.
type WindowSpec struct {
	Year   int64  `json:"year"`
	Week   int64  `json:"week"`
	Cutoff string `json:"cutoff,omitempty"`
}

// Manifest is the explicit local replay input list. There is no network input.
type Manifest struct {
	Schema  string       `json:"schema"`
	Windows []WindowSpec `json:"windows"`
}

// LoadManifest reads and validates a replay manifest from disk.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read replay manifest: %w", err)
	}
	return ParseManifest(data)
}

// ParseManifest validates manifest bytes strictly: unknown schema, unknown
// fields, out-of-range years, non-positive weeks, duplicate (year, week)
// pairs, oversized window lists and malformed cutoffs are all rejected.
func ParseManifest(data []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse replay manifest: %w", err)
	}
	if manifest.Schema != ManifestSchema {
		return Manifest{}, fmt.Errorf("unsupported replay manifest schema %q", manifest.Schema)
	}
	if len(manifest.Windows) == 0 {
		return Manifest{}, errors.New("replay manifest has no windows")
	}
	if len(manifest.Windows) > maxWindows {
		return Manifest{}, fmt.Errorf("replay manifest exceeds %d windows", maxWindows)
	}
	seen := make(map[[2]int64]bool, len(manifest.Windows))
	for i, window := range manifest.Windows {
		if window.Year < 1936 || window.Year > 9999 {
			return Manifest{}, fmt.Errorf("window %d: year %d out of supported range", i, window.Year)
		}
		if window.Week < 1 {
			return Manifest{}, fmt.Errorf("window %d: week must be positive", i)
		}
		key := [2]int64{window.Year, window.Week}
		if seen[key] {
			return Manifest{}, fmt.Errorf("window %d: duplicate year/week %d/%d", i, window.Year, window.Week)
		}
		seen[key] = true
		if window.Cutoff != "" {
			if _, err := parseCutoff(window.Cutoff); err != nil {
				return Manifest{}, fmt.Errorf("window %d: %w", i, err)
			}
		}
	}
	return manifest, nil
}

// parseCutoff parses an RFC3339 cutoff assertion.
func parseCutoff(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("cutoff must be RFC3339: %w", err)
	}
	if parsed.IsZero() {
		return time.Time{}, errors.New("cutoff must not be zero")
	}
	return parsed, nil
}

// cutoff returns the parsed optional cutoff assertion.
func (w WindowSpec) cutoff() (time.Time, error) {
	if w.Cutoff == "" {
		return time.Time{}, nil
	}
	return parseCutoff(w.Cutoff)
}
