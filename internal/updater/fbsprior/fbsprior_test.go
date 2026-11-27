package fbsprior

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/ranking"
)

// ---------------------------------------------------------------------------
// Artifact build/parse
// ---------------------------------------------------------------------------

// validArtifact returns a contract-satisfying artifact for season 2026:
// fixed anchor midnight Eastern on 2026-08-10 (04:00Z EDT), day-56 expiry
// midnight Eastern on 2026-10-05, actual freeze date 2026-08-12.
func validArtifact() artifact {
	anchor := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	return artifact{
		Schema:         Schema,
		Season:         2026,
		Anchor:         "2026-08-10",
		AnchorInstant:  anchor,
		CalendarExpiry: time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
		FreezeDate:     "2026-08-12",
		AsOf:           time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
		CapturedAt:     time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC),
		Field: []FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
		},
		Prior: artifactPrior{
			ZStrength:      map[int64]float64{1: 1.0, 2: -1.0},
			MissingHistory: []int64{},
			FieldSize:      2,
		},
		InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
}

// encodeArtifact marshals the artifact to the exact stored bytes.
func encodeArtifact(t *testing.T, file artifact) []byte {
	t.Helper()
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	return data
}

// TestBuildParse_RoundTrip builds through the production freeze-side entry
// point and verifies every parsed field, the exact expiry instant, and that
// parsing the same bytes yields the same digest.
func TestBuildParse_RoundTrip(t *testing.T) {
	data, prior, err := Build(FreezeOutput{
		Season:     2026,
		FreezeDate: "2026-08-12",
		AsOf:       time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
		CapturedAt: time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC),
		Field: []FieldRow{
			{TeamID: 2, TeamName: "Beta"},
			{TeamID: 1, TeamName: "Alpha"},
		},
		ZStrength:   map[int64]float64{1: 1.0, 2: -1.0},
		InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantAnchor := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	wantExpiry := time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)
	if prior.Season != 2026 || prior.Anchor != "2026-08-10" || prior.FreezeDate != "2026-08-12" {
		t.Errorf("season/anchor/freeze = %d/%s/%s", prior.Season, prior.Anchor, prior.FreezeDate)
	}
	if !prior.AnchorInstant.Equal(wantAnchor) {
		t.Errorf("anchor instant = %s, want %s", prior.AnchorInstant, wantAnchor)
	}
	if !prior.CalendarExpiry.Equal(wantExpiry) {
		t.Errorf("calendar expiry = %s, want %s", prior.CalendarExpiry, wantExpiry)
	}
	if len(prior.Field) != 2 || prior.Field[0].TeamID != 1 || prior.Field[0].TeamName != "Alpha" {
		t.Errorf("field = %v, want sorted Alpha/Beta rows", prior.Field)
	}
	if prior.ZStrength[2] != -1.0 || prior.Degenerate {
		t.Errorf("prior payload mismatch: %v degenerate=%t", prior.ZStrength, prior.Degenerate)
	}
	sum := sha256.Sum256(data)
	if prior.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("digest %q does not match the built bytes", prior.SHA256)
	}

	// Field order in the output must not change the stored bytes: Build
	// sorts the field deterministically.
	again, _, err := Build(FreezeOutput{
		Season:     2026,
		FreezeDate: "2026-08-12",
		AsOf:       time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
		CapturedAt: time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC),
		Field: []FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
		},
		ZStrength:   map[int64]float64{1: 1.0, 2: -1.0},
		InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("Build (sorted input): %v", err)
	}
	if string(again) != string(data) {
		t.Error("Build is not deterministic across equivalent field orders")
	}
}

func TestBuild_RejectsInvalidOutput(t *testing.T) {
	good := func() FreezeOutput {
		return FreezeOutput{
			Season:      2026,
			FreezeDate:  "2026-08-12",
			AsOf:        time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
			CapturedAt:  time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC),
			Field:       []FieldRow{{TeamID: 1, TeamName: "Alpha"}, {TeamID: 2, TeamName: "Beta"}},
			ZStrength:   map[int64]float64{1: 1.0, 2: -1.0},
			InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}
	}

	tests := []struct {
		name   string
		mutate func(*FreezeOutput)
	}{
		{"zero season", func(o *FreezeOutput) { o.Season = 0 }},
		{"freeze date outside window", func(o *FreezeOutput) { o.FreezeDate = "2026-08-20" }},
		{"captured before as_of", func(o *FreezeOutput) {
			o.CapturedAt = time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
		}},
		{"empty field", func(o *FreezeOutput) {
			o.Field = nil
			o.ZStrength = map[int64]float64{}
		}},
		{"nonfinite strength", func(o *FreezeOutput) { o.ZStrength[1] = math.NaN() }},
		{"bad input hash", func(o *FreezeOutput) { o.InputSHA256 = "xyz" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := good()
			tt.mutate(&out)
			if _, _, err := Build(out); err == nil {
				t.Fatalf("Build succeeded, want error")
			}
		})
	}
}

func TestParse_MalformedAndOversized(t *testing.T) {
	if _, err := Parse([]byte("{not json")); err == nil {
		t.Fatal("Parse of malformed JSON succeeded")
	}
	if _, err := Parse(make([]byte, maxArtifactBytes+1)); err == nil ||
		!strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected a size limit error; got %v", err)
	}
}

func TestParse_InvalidArtifacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*artifact)
	}{
		{"legacy schema", func(f *artifact) { f.Schema = "ncaaf-shadow-prior/1" }},
		{"unknown schema", func(f *artifact) { f.Schema = "ncaaf-season-prior/2" }},
		{"zero season", func(f *artifact) { f.Season = 0 }},
		{"anchor not August 10", func(f *artifact) {
			f.Anchor = "2026-08-24"
			f.AnchorInstant = time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC)
			f.CalendarExpiry = time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC)
		}},
		{"anchor instant off the anchor", func(f *artifact) {
			f.AnchorInstant = time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)
		}},
		{"expiry not day-56", func(f *artifact) {
			f.CalendarExpiry = time.Date(2026, 10, 6, 4, 0, 0, 0, time.UTC)
		}},
		{"freeze date the day before the window", func(f *artifact) { f.FreezeDate = "2026-08-09" }},
		{"freeze date the day after the window", func(f *artifact) { f.FreezeDate = "2026-08-17" }},
		{"freeze date midseason", func(f *artifact) {
			f.FreezeDate = "2026-09-15"
			f.AsOf = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
			f.CapturedAt = time.Date(2026, 9, 15, 13, 0, 0, 0, time.UTC)
		}},
		{"freeze year mismatch", func(f *artifact) {
			f.FreezeDate = "2025-08-12"
			f.AsOf = time.Date(2025, 8, 12, 12, 0, 0, 0, time.UTC)
			f.CapturedAt = time.Date(2025, 8, 12, 13, 0, 0, 0, time.UTC)
		}},
		{"as_of before the anchor", func(f *artifact) {
			f.AsOf = time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
		}},
		{"as_of off the freeze date", func(f *artifact) {
			f.AsOf = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
		}},
		{"captured before as_of", func(f *artifact) {
			f.CapturedAt = time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
		}},
		{"captured after the freeze date", func(f *artifact) {
			f.CapturedAt = time.Date(2026, 8, 17, 4, 0, 0, 0, time.UTC)
		}},
		{"zero captured_at", func(f *artifact) { f.CapturedAt = time.Time{} }},
		{"input hash not hex64", func(f *artifact) { f.InputSHA256 = "xyz" }},
		{"empty field", func(f *artifact) {
			f.Field = nil
			f.Prior.FieldSize = 0
			f.Prior.ZStrength = map[int64]float64{}
		}},
		{"unsorted field", func(f *artifact) {
			f.Field = []FieldRow{{TeamID: 2, TeamName: "Beta"}, {TeamID: 1, TeamName: "Alpha"}}
		}},
		{"duplicate field team", func(f *artifact) {
			f.Field = []FieldRow{{TeamID: 1, TeamName: "Alpha"}, {TeamID: 1, TeamName: "Alpha"}}
		}},
		{"missing team name", func(f *artifact) {
			f.Field = []FieldRow{{TeamID: 1, TeamName: ""}, {TeamID: 2, TeamName: "Beta"}}
		}},
		{"field size mismatch", func(f *artifact) { f.Prior.FieldSize = 3 }},
		{"strength covers unknown team", func(f *artifact) { f.Prior.ZStrength[99] = 0.5 }},
		{"strength missing a team", func(f *artifact) { delete(f.Prior.ZStrength, 2) }},
		{"unsorted missing history", func(f *artifact) { f.Prior.MissingHistory = []int64{2, 1} }},
		{"missing history outside field", func(f *artifact) { f.Prior.MissingHistory = []int64{7} }},
		{"degenerate with nonzero strength", func(f *artifact) { f.Prior.Degenerate = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := validArtifact()
			tt.mutate(&file)
			if _, err := Parse(encodeArtifact(t, file)); err == nil {
				t.Fatalf("Parse succeeded, want error")
			}
		})
	}
}

// TestParse_FreezeWindowBounds checks the exact Eastern window edges: the
// first and last permitted freeze dates parse; their neighbors do not.
func TestParse_FreezeWindowBounds(t *testing.T) {
	accepted := []string{"2026-08-10", "2026-08-16"}
	rejected := []string{"2026-08-09", "2026-08-17"}

	for _, date := range accepted {
		file := validArtifact()
		file.FreezeDate = date
		file.AsOf = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
		file.CapturedAt = time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
		if date == "2026-08-10" {
			file.AsOf = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
			file.CapturedAt = time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
		}
		if _, err := Parse(encodeArtifact(t, file)); err != nil {
			t.Errorf("freeze date %s rejected: %v", date, err)
		}
	}
	for _, date := range rejected {
		file := validArtifact()
		file.FreezeDate = date
		if _, err := Parse(encodeArtifact(t, file)); err == nil {
			t.Errorf("freeze date %s accepted", date)
		}
	}
}

// TestParse_KeySeasonMismatch verifies a payload whose season contradicts
// the anchor year is rejected, whatever key it is stored under.
func TestParse_KeySeasonMismatch(t *testing.T) {
	file := validArtifact()
	file.Season = 2025
	if _, err := Parse(encodeArtifact(t, file)); err == nil {
		t.Fatal("season/anchor-year mismatch accepted")
	}
}

// ---------------------------------------------------------------------------
// Anchor policy
// ---------------------------------------------------------------------------

// TestBuildRebuilt_RoundTrip builds a rebuilt artifact through the
// production rebuild-side entry point and verifies the fixed anchor/expiry,
// the real rebuild instant (also exposed as the publication gate's capture
// instant), the absence of fabricated preseason capture metadata, and the
// exact-byte digest. An automatic preseason artifact remains readable, with
// a zero rebuild instant.
func TestBuildRebuilt_RoundTrip(t *testing.T) {
	rebuiltAt := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	data, published, err := BuildRebuilt(2026, rebuiltAt, &Prior{
		Season: 2026,
		Field: []FieldRow{
			{TeamID: 2, TeamName: "Beta"},
			{TeamID: 1, TeamName: "Alpha"},
		},
		ZStrength:   map[int64]float64{1: 1.0, 2: -1.0},
		InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("BuildRebuilt: %v", err)
	}

	wantAnchor := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	wantExpiry := time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)
	if published.Season != 2026 || published.Anchor != "2026-08-10" {
		t.Errorf("season/anchor = %d/%s", published.Season, published.Anchor)
	}
	if !published.AnchorInstant.Equal(wantAnchor) || !published.CalendarExpiry.Equal(wantExpiry) {
		t.Errorf("anchor/expiry = %s/%s", published.AnchorInstant, published.CalendarExpiry)
	}
	if len(published.Field) != 2 || published.Field[0].TeamID != 1 || published.Field[0].TeamName != "Alpha" {
		t.Errorf("field = %v, want sorted Alpha/Beta rows", published.Field)
	}
	if published.FreezeDate != "" || !published.AsOf.IsZero() {
		t.Errorf("fabricated capture metadata = %q/%s, want none", published.FreezeDate, published.AsOf)
	}
	if !published.RebuiltAt.Equal(rebuiltAt) || !published.CapturedAt.Equal(rebuiltAt) {
		t.Errorf("rebuilt/captured = %s/%s, want the real rebuild instant twice",
			published.RebuiltAt, published.CapturedAt)
	}
	sum := sha256.Sum256(data)
	if published.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("digest %q does not match the built bytes", published.SHA256)
	}

	// The exact bytes reparse to the same verified prior.
	again, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse (rebuilt bytes): %v", err)
	}
	if again.SHA256 != published.SHA256 {
		t.Error("reparsing the rebuilt bytes yielded a different digest")
	}

	// An automatic preseason artifact stays fully readable, with a zero
	// rebuild instant.
	v1, frozen, err := Build(FreezeOutput{
		Season:     2026,
		FreezeDate: "2026-08-12",
		AsOf:       time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
		CapturedAt: time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC),
		Field: []FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
		},
		ZStrength:   map[int64]float64{1: 1.0, 2: -1.0},
		InputSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("Build (automatic): %v", err)
	}
	if frozen.RebuiltAt != (time.Time{}) || frozen.FreezeDate != "2026-08-12" {
		t.Errorf("automatic artifact = rebuilt %s freeze %q, want no rebuild instant and the freeze date",
			frozen.RebuiltAt, frozen.FreezeDate)
	}
	if _, err := Parse(v1); err != nil {
		t.Fatalf("Parse (automatic bytes): %v", err)
	}
}

// TestParse_RebuiltArtifactRejections covers the rebuilt-artifact contract:
// the fixed anchor and expiry, the mandatory real rebuild instant and
// source, the refusal to fabricate preseason capture metadata, and the
// field/payload consistency checks shared with the automatic artifact.
func TestParse_RebuiltArtifactRejections(t *testing.T) {
	rebuiltAt := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	base := func() artifact {
		file := validArtifact()
		file.Schema = RebuildSchema
		file.FreezeDate = ""
		file.AsOf = time.Time{}
		file.CapturedAt = time.Time{}
		file.RebuiltAt = &rebuiltAt
		file.Source = RebuildSource
		return file
	}
	if _, err := Parse(encodeArtifact(t, base())); err != nil {
		t.Fatalf("valid rebuilt artifact rejected: %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(file *artifact)
		contains string
	}{
		{
			name: "fabricated preseason capture metadata",
			mutate: func(file *artifact) {
				file.FreezeDate = "2026-08-12"
				file.AsOf = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
				file.CapturedAt = time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
			},
			contains: "must not carry preseason capture metadata",
		},
		{
			name:     "missing rebuild instant",
			mutate:   func(file *artifact) { file.RebuiltAt = nil },
			contains: "rebuilt timestamp is missing",
		},
		{
			name:     "missing rebuild source",
			mutate:   func(file *artifact) { file.Source = "" },
			contains: "rebuild source is missing",
		},
		{
			name: "moving the anchor",
			mutate: func(file *artifact) {
				file.AnchorInstant = file.AnchorInstant.Add(time.Hour)
			},
			contains: "anchor/expiry is not the fixed",
		},
		{
			name: "rebuild instant before the anchor",
			mutate: func(file *artifact) {
				before := file.AnchorInstant.Add(-time.Hour)
				file.RebuiltAt = &before
			},
			contains: "predates the preseason anchor",
		},
		{
			name:     "unsupported schema",
			mutate:   func(file *artifact) { file.Schema = "ncaaf-season-prior/9" },
			contains: "unsupported schema",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := base()
			tt.mutate(&file)
			_, err := Parse(encodeArtifact(t, file))
			if err == nil {
				t.Fatal("invalid rebuilt artifact accepted")
			}
			if !strings.Contains(err.Error(), tt.contains) {
				t.Errorf("error = %v, want it to contain %q", err, tt.contains)
			}
		})
	}
}

func TestAnchorInstantFor(t *testing.T) {
	anchor, err := AnchorInstantFor(2026)
	if err != nil {
		t.Fatalf("AnchorInstantFor: %v", err)
	}
	// Midnight Eastern on 2026-08-10 is 04:00Z (EDT).
	if !anchor.Equal(time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("anchor = %s, want 2026-08-10T04:00:00Z", anchor)
	}
	if _, err := AnchorInstantFor(0); err == nil {
		t.Error("season 0 accepted")
	}
}

// TestCalendarExpiryForSeason checks the exact October 5 midnight Eastern
// expiry derived from the fixed August 10 anchor.
func TestCalendarExpiryForSeason(t *testing.T) {
	expiry, err := CalendarExpiryForSeason(2026)
	if err != nil {
		t.Fatalf("CalendarExpiryForSeason: %v", err)
	}
	if !expiry.Equal(time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("expiry = %s, want 2026-10-05T04:00:00Z", expiry)
	}
}

// TestSeasonWindow checks the Eastern window resolution across clock zones:
// the target season is the Eastern calendar year and the window is the
// inclusive August 10-16 Eastern date range.
func TestSeasonWindow(t *testing.T) {
	tests := []struct {
		name     string
		now      time.Time
		season   int64
		freeze   string
		inWindow bool
	}{
		{
			name:     "aug 10 eastern midnight",
			now:      time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC),
			season:   2026,
			freeze:   "2026-08-10",
			inWindow: true,
		},
		{
			name:     "aug 16 late evening",
			now:      time.Date(2026, 8, 17, 3, 59, 0, 0, time.UTC), // Aug 16 23:59 EDT
			season:   2026,
			freeze:   "2026-08-16",
			inWindow: true,
		},
		{
			name:     "aug 9 eastern evening",
			now:      time.Date(2026, 8, 10, 3, 59, 0, 0, time.UTC), // Aug 9 23:59 EDT
			season:   2026,
			freeze:   "2026-08-09",
			inWindow: false,
		},
		{
			name:     "aug 17 eastern midnight",
			now:      time.Date(2026, 8, 17, 4, 0, 0, 0, time.UTC),
			season:   2026,
			freeze:   "2026-08-17",
			inWindow: false,
		},
		{
			name:     "midseason",
			now:      time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
			season:   2026,
			freeze:   "2026-10-05",
			inWindow: false,
		},
		{
			name:     "preseason of the prior year",
			now:      time.Date(2025, 8, 12, 12, 0, 0, 0, time.UTC),
			season:   2025,
			freeze:   "2025-08-12",
			inWindow: true,
		},
		{
			name:     "non-eastern instant maps to the eastern date",
			now:      time.Date(2026, 8, 10, 23, 30, 0, 0, time.UTC), // 19:30 EDT
			season:   2026,
			freeze:   "2026-08-10",
			inWindow: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			season, freezeDate, inWindow := SeasonWindow(tt.now)
			if season != tt.season || freezeDate != tt.freeze || inWindow != tt.inWindow {
				t.Errorf("SeasonWindow = (%d, %q, %t), want (%d, %q, %t)",
					season, freezeDate, inWindow, tt.season, tt.freeze, tt.inWindow)
			}
		})
	}
}

func TestValidateSeasonWindow(t *testing.T) {
	if err := ValidateSeasonWindow(2026, "2026-08-16"); err != nil {
		t.Errorf("2026-08-16 rejected: %v", err)
	}
	for _, date := range []string{"2026-08-09", "2026-08-17", "2025-08-12", "not-a-date"} {
		if err := ValidateSeasonWindow(2026, date); err == nil {
			t.Errorf("freeze date %q accepted", date)
		}
	}
}

// ---------------------------------------------------------------------------
// Blend math
// ---------------------------------------------------------------------------

// blendTeams builds a TeamList from (id, year, name, raw, wins, losses, ties,
// rank) tuples.
func blendTeams(t *testing.T, rows ...[8]any) ranking.TeamList {
	t.Helper()
	teams := ranking.TeamList{}
	for _, r := range rows {
		id := r[0].(int64)
		teams[id] = &ranking.Team{
			Name: r[2].(string),
			Year: r[1].(int64),
			Record: ranking.Record{
				Wins:   r[4].(int64),
				Losses: r[5].(int64),
				Ties:   r[6].(int64),
			},
			FinalRaw:  r[3].(float64),
			FinalRank: r[7].(int64),
		}
	}
	return teams
}

func blendPrior() *Prior {
	return &Prior{
		Season:         2026,
		Anchor:         "2026-08-10",
		AnchorInstant:  time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC),
		CalendarExpiry: time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
		Field: []FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
		},
		ZStrength: map[int64]float64{1: 1.0, 2: -1.0},
	}
}

// TestApply_GoldenTwoTeams verifies the researched arithmetic exactly:
// mean 0.5, population SD 0.25 (binary-exact raws), rho 0.9, lambda
// 1-games/4, no restandardization, and bypassed teams keep their exact
// baseline score.
func TestApply_GoldenTwoTeams(t *testing.T) {
	teams := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.75, int64(1), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.25, int64(0), int64(0), int64(0), int64(2)},
	)
	prior := blendPrior()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	adjusted, err := Apply(teams, prior, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Alpha: z=(0.75-0.5)/0.25=1, lambda=0.75.
	wantAlpha := 0.5 + 0.25*((1-0.75)*1.0+0.75*(Rho*1.0))
	if adjusted[1].FinalRaw != wantAlpha {
		t.Errorf("alpha raw = %v, want %v", adjusted[1].FinalRaw, wantAlpha)
	}
	// Beta: z=-1, lambda=1.
	wantBeta := 0.5 + 0.25*(Rho*-1.0)
	if adjusted[2].FinalRaw != wantBeta {
		t.Errorf("beta raw = %v, want %v", adjusted[2].FinalRaw, wantBeta)
	}
	if adjusted[1].FinalRank != 1 || adjusted[2].FinalRank != 2 {
		t.Errorf("ranks = %d/%d, want 1/2", adjusted[1].FinalRank, adjusted[2].FinalRank)
	}
}

func TestApply_SDFloor(t *testing.T) {
	// Near-degenerate field: population SD 2^-20 floors to 0.05. The deltas
	// are exact binary fractions so the expectations are exact too.
	delta := 1.0 / 1048576
	teams := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.5 + delta, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.5 - delta, int64(0), int64(0), int64(0), int64(2)},
	)
	prior := blendPrior()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	adjusted, err := Apply(teams, prior, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	wantAlpha := 0.5 + SDFloor*(Rho*1.0) // lambda=1, z=(0.50004-0.5)/0.05 is unused for lambda=1
	if adjusted[1].FinalRaw != wantAlpha {
		t.Errorf("alpha raw = %v, want %v (SD floor applied)", adjusted[1].FinalRaw, wantAlpha)
	}
	if adjusted[2].FinalRaw != 0.5+SDFloor*(Rho*-1.0) {
		t.Errorf("beta raw = %v, want floored blend", adjusted[2].FinalRaw)
	}
}

func TestApply_TiesUseCompetitionRanks(t *testing.T) {
	// Alpha and Beta both bypass (4 games) with identical raw 0.6; Gamma and
	// Delta share an identical raw and prior strength, so they blend to an
	// identical score. Expected ranks: 1,1,3,3.
	teams := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(4), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.6, int64(4), int64(0), int64(0), int64(1)},
		[8]any{int64(3), int64(2026), "Gamma", 0.5, int64(2), int64(0), int64(0), int64(3)},
		[8]any{int64(4), int64(2026), "Delta", 0.5, int64(2), int64(0), int64(0), int64(4)},
	)
	prior := &Prior{
		Season:         2026,
		Anchor:         "2026-08-10",
		AnchorInstant:  time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC),
		CalendarExpiry: time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
		Field: []FieldRow{
			{TeamID: 1, TeamName: "Alpha"},
			{TeamID: 2, TeamName: "Beta"},
			{TeamID: 3, TeamName: "Gamma"},
			{TeamID: 4, TeamName: "Delta"},
		},
		// Chosen so Gamma and Delta blend to the same score.
		ZStrength: map[int64]float64{1: 0.0, 2: 0.0, 3: 0.5, 4: 0.5},
	}
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	adjusted, err := Apply(teams, prior, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if adjusted[1].FinalRaw != 0.6 || adjusted[2].FinalRaw != 0.6 {
		t.Errorf("bypassed teams must keep the exact baseline raw: %v/%v",
			adjusted[1].FinalRaw, adjusted[2].FinalRaw)
	}
	if adjusted[3].FinalRaw != adjusted[4].FinalRaw {
		t.Fatalf("gamma/delta should tie: %v vs %v", adjusted[3].FinalRaw, adjusted[4].FinalRaw)
	}
	gotRanks := []int64{adjusted[1].FinalRank, adjusted[2].FinalRank,
		adjusted[3].FinalRank, adjusted[4].FinalRank}
	wantRanks := []int64{1, 1, 3, 3}
	for i := range wantRanks {
		if gotRanks[i] != wantRanks[i] {
			t.Fatalf("ranks = %v, want %v", gotRanks, wantRanks)
		}
	}
}

func TestApply_AllBypassedPreservesRanks(t *testing.T) {
	// Baseline ranks deliberately disagree with score order to prove the
	// original ranks are preserved rather than recomputed.
	teams := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.4, int64(4), int64(0), int64(0), int64(2)},
		[8]any{int64(2), int64(2026), "Beta", 0.6, int64(3), int64(0), int64(1), int64(1)},
	)
	prior := blendPrior()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	adjusted, err := Apply(teams, prior, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if adjusted[1].FinalRaw != 0.4 || adjusted[2].FinalRaw != 0.6 {
		t.Errorf("scores must be the exact baseline")
	}
	if adjusted[1].FinalRank != 2 || adjusted[2].FinalRank != 1 {
		t.Errorf("ranks = %d/%d, want the exact baseline 2/1",
			adjusted[1].FinalRank, adjusted[2].FinalRank)
	}
}

func TestApply_ZeroGamesFullShrinkage(t *testing.T) {
	teams := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.75, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.25, int64(0), int64(0), int64(0), int64(2)},
	)
	prior := blendPrior()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	adjusted, err := Apply(teams, prior, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// lambda=1: the baseline z drops out entirely.
	if adjusted[1].FinalRaw != 0.5+0.25*(Rho*1.0) {
		t.Errorf("alpha raw = %v, want pure prior blend", adjusted[1].FinalRaw)
	}
}

func TestApply_FourthGameBoundary(t *testing.T) {
	prior := blendPrior()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	three := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(3), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.4, int64(0), int64(0), int64(0), int64(2)},
	)
	adjusted, err := Apply(three, prior, now)
	if err != nil {
		t.Fatalf("Apply (3 games): %v", err)
	}
	if adjusted[1].FinalRaw == 0.6 {
		t.Error("3 games must still shrink (lambda 0.25), got the exact baseline")
	}

	four := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(4), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.4, int64(0), int64(0), int64(0), int64(2)},
	)
	adjusted, err = Apply(four, prior, now)
	if err != nil {
		t.Fatalf("Apply (4 games): %v", err)
	}
	if adjusted[1].FinalRaw != 0.6 {
		t.Error("4 games must bypass with the exact baseline raw")
	}
	if adjusted[2].FinalRaw == 0.4 {
		t.Error("0-game team must still shrink even when another team bypasses")
	}
}

func TestApply_CalendarExpiryExactBaseline(t *testing.T) {
	teams := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.4, int64(0), int64(0), int64(0), int64(2)},
	)
	prior := blendPrior()
	// Kickoffs long past, observation exactly at the expiry instant.
	now := prior.CalendarExpiry

	adjusted, err := Apply(teams, prior, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if adjusted[1].FinalRaw != 0.6 || adjusted[1].FinalRank != 1 ||
		adjusted[2].FinalRaw != 0.4 || adjusted[2].FinalRank != 2 {
		t.Errorf("expired blend must be the exact baseline identity: %+v", adjusted)
	}
}

func TestApply_FailureLeavesInputUnmutated(t *testing.T) {
	good := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.4, int64(0), int64(0), int64(0), int64(2)},
	)
	prior := blendPrior()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	if _, err := Apply(good, prior, now); err != nil {
		t.Fatalf("Apply (valid): %v", err)
	}
	for id, want := range map[int64]float64{1: 0.6, 2: 0.4} {
		if good[id].FinalRaw != want {
			t.Errorf("team %d FinalRaw mutated to %v", id, good[id].FinalRaw)
		}
	}

	// A nonfinite baseline on the last team (highest ID) fails the whole
	// blend; nothing may be adjusted.
	bad := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", math.NaN(), int64(0), int64(0), int64(0), int64(2)},
	)
	adjusted, err := Apply(bad, prior, now)
	if err == nil {
		t.Fatal("Apply with nonfinite baseline succeeded")
	}
	if adjusted != nil {
		t.Errorf("failed Apply returned adjustments: %+v", adjusted)
	}

	negative := blendTeams(t,
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(0), int64(0), int64(-1), int64(1)},
		[8]any{int64(2), int64(2026), "Beta", 0.4, int64(0), int64(0), int64(0), int64(2)},
	)
	if _, err := Apply(negative, prior, now); err == nil {
		t.Fatal("Apply with negative record succeeded")
	}
}

func TestApply_NilBaselineEntryRejected(t *testing.T) {
	prior := blendPrior()
	teams := make(ranking.TeamList, len(prior.Field))
	for _, row := range prior.Field {
		teams[row.TeamID] = nil
	}
	if prior.CoversField(teams) {
		t.Fatal("field unexpectedly covers nil baseline teams")
	}
	if adjusted, err := Apply(teams, prior, prior.AnchorInstant); err == nil || adjusted != nil {
		t.Fatalf("expected baseline validation error, got adjusted=%v err=%v", adjusted, err)
	}
}

func TestApply_WrongSeasonRejected(t *testing.T) {
	teams := blendTeams(t,
		[8]any{int64(1), int64(2025), "Alpha", 0.6, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(2), int64(2025), "Beta", 0.4, int64(0), int64(0), int64(0), int64(2)},
	)
	if _, err := Apply(teams, blendPrior(), time.Now()); err == nil {
		t.Fatal("Apply with mismatched prior season succeeded")
	}
}

func TestCoversField(t *testing.T) {
	teams := blendTeams(t,
		[8]any{int64(2), int64(2026), "Beta", 0.5, int64(0), int64(0), int64(0), int64(1)},
		[8]any{int64(1), int64(2026), "Alpha", 0.6, int64(0), int64(0), int64(0), int64(2)},
	)
	prior := blendPrior()
	if !prior.CoversField(teams) {
		t.Error("matching field (any input order) must be covered")
	}
	teams[3] = &ranking.Team{Name: "Gamma", Year: 2026, FinalRank: 3}
	if prior.CoversField(teams) {
		t.Error("extra team must not be covered")
	}
	delete(teams, 3)
	teams[2].Name = "Renamed"
	if prior.CoversField(teams) {
		t.Error("renamed team must not be covered")
	}
}
