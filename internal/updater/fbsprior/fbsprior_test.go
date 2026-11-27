package fbsprior

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/ranking"
)

// freezeDate2026 is a representative predeclared preseason Eastern anchor:
// midnight Eastern on 2026-08-24 is 04:00Z (EDT).
const freezeDate2026 = "2026-08-24"

// validArtifact returns a contract-satisfying artifact for season 2026.
func validArtifact() artifactFile {
	return artifactFile{
		Schema:         Schema,
		Season:         2026,
		FreezeDate:     freezeDate2026,
		Cutoff:         time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC),
		CalendarExpiry: time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC),
		AsOf:           time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		CapturedAt:     time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC),
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
		Limitations: []string{"metadata only"},
	}
}

// writeArtifact marshals the artifact, writes it to a temp file, and returns
// the path plus the exact byte digest.
func writeArtifact(t *testing.T, file artifactFile) (string, string) {
	t.Helper()
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	path := filepath.Join(t.TempDir(), "prior.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

func TestLoad_ValidArtifact(t *testing.T) {
	path, pin := writeArtifact(t, validArtifact())

	prior, err := Load(path, pin)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prior.Season != 2026 || prior.FreezeDate != freezeDate2026 {
		t.Errorf("season/freeze date = %d/%q, want 2026/%q", prior.Season, prior.FreezeDate, freezeDate2026)
	}
	wantCutoff := time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC)
	if !prior.Cutoff.Equal(wantCutoff) {
		t.Errorf("cutoff = %s, want %s", prior.Cutoff, wantCutoff)
	}
	wantExpiry := time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC)
	if !prior.CalendarExpiry.Equal(wantExpiry) {
		t.Errorf("calendar expiry = %s, want %s", prior.CalendarExpiry, wantExpiry)
	}
	if len(prior.Field) != 2 || prior.Field[0].TeamID != 1 || prior.Field[0].TeamName != "Alpha" {
		t.Errorf("field = %v, want sorted Alpha/Beta rows", prior.Field)
	}
	if prior.ZStrength[2] != -1.0 || prior.Degenerate {
		t.Errorf("prior payload mismatch: %v degenerate=%t", prior.ZStrength, prior.Degenerate)
	}
	if prior.SHA256 != pin {
		t.Errorf("recorded sha256 = %q, want pin %q", prior.SHA256, pin)
	}
}

func TestLoad_RejectsWrongPin(t *testing.T) {
	path, pin := writeArtifact(t, validArtifact())

	wrong := "ffffffff" + pin[8:]
	if _, err := Load(path, wrong); err == nil {
		t.Fatal("Load with wrong pin succeeded, want digest-mismatch error")
	}

	// Tampered bytes: valid pin over different content must fail.
	tampered := validArtifact()
	tampered.Prior.ZStrength[1] = 2.0
	tamperPath, _ := writeArtifact(t, tampered)
	if _, err := Load(tamperPath, pin); err == nil {
		t.Fatal("Load of tampered artifact with original pin succeeded")
	}
}

func TestLoad_RejectsBadPinFormat(t *testing.T) {
	path, _ := writeArtifact(t, validArtifact())

	badPins := []string{"", "abc", "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"}
	for _, pin := range badPins {
		if _, err := Load(path, pin); err == nil {
			t.Errorf("Load with pin %q succeeded, want error", pin)
		}
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.json"),
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("Load of missing file succeeded")
	}
}

func TestLoad_RejectsNonRegularAndOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	pin := strings.Repeat("a", 64)
	if _, err := Load(dir, pin); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected a non-regular file error; got %v", err)
	}
	path := filepath.Join(dir, "oversized.json")
	if err := os.WriteFile(path, make([]byte, maxArtifactBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, pin); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected a size limit error; got %v", err)
	}
}

func TestLoad_MalformedAndInvalidArtifacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*artifactFile)
	}{
		{"wrong schema", func(f *artifactFile) { f.Schema = "ncaaf-shadow-prior/2" }},
		{"zero season", func(f *artifactFile) { f.Season = 0 }},
		{"unparseable freeze date", func(f *artifactFile) { f.FreezeDate = "not-a-date" }},
		{"cutoff off the anchor", func(f *artifactFile) {
			f.Cutoff = time.Date(2026, 8, 24, 5, 0, 0, 0, time.UTC)
		}},
		{"expiry not day-56", func(f *artifactFile) {
			f.CalendarExpiry = time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)
		}},
		{"as_of off the freeze date", func(f *artifactFile) {
			f.AsOf = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
		}},
		{"as_of before cutoff", func(f *artifactFile) {
			f.AsOf = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
		}},
		{"captured before as_of", func(f *artifactFile) {
			f.CapturedAt = time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)
		}},
		{"freeze year mismatch", func(f *artifactFile) {
			f.FreezeDate = "2025-08-25"
			f.Cutoff = time.Date(2025, 8, 25, 4, 0, 0, 0, time.UTC)
			f.CalendarExpiry = time.Date(2025, 10, 20, 4, 0, 0, 0, time.UTC)
			f.AsOf = time.Date(2025, 8, 25, 12, 0, 0, 0, time.UTC)
			f.CapturedAt = time.Date(2025, 8, 25, 13, 0, 0, 0, time.UTC)
		}},
		{"input hash not hex64", func(f *artifactFile) { f.InputSHA256 = "xyz" }},
		{"empty field", func(f *artifactFile) {
			f.Field = nil
			f.Prior.FieldSize = 0
			f.Prior.ZStrength = map[int64]float64{}
		}},
		{"unsorted field", func(f *artifactFile) {
			f.Field = []FieldRow{{TeamID: 2, TeamName: "Beta"}, {TeamID: 1, TeamName: "Alpha"}}
		}},
		{"duplicate field team", func(f *artifactFile) {
			f.Field = []FieldRow{{TeamID: 1, TeamName: "Alpha"}, {TeamID: 1, TeamName: "Alpha"}}
		}},
		{"missing team name", func(f *artifactFile) {
			f.Field = []FieldRow{{TeamID: 1, TeamName: ""}, {TeamID: 2, TeamName: "Beta"}}
		}},
		{"field size mismatch", func(f *artifactFile) { f.Prior.FieldSize = 3 }},
		{"strength covers unknown team", func(f *artifactFile) {
			f.Prior.ZStrength[99] = 0.5
		}},
		{"strength missing a team", func(f *artifactFile) {
			delete(f.Prior.ZStrength, 2)
		}},
		{"unsorted missing history", func(f *artifactFile) {
			f.Prior.MissingHistory = []int64{2, 1}
		}},
		{"missing history outside field", func(f *artifactFile) {
			f.Prior.MissingHistory = []int64{7}
		}},
		{"degenerate with nonzero strength", func(f *artifactFile) {
			f.Prior.Degenerate = true
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := validArtifact()
			tt.mutate(&file)
			path, pin := writeArtifact(t, file)
			if _, err := Load(path, pin); err == nil {
				t.Fatalf("Load succeeded, want error")
			}
		})
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prior.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if _, err := Load(path, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("Load of malformed JSON succeeded")
	}
}

// TestLoad_DSTDifferingAnchorExpiry checks that the day-56 expiry is computed
// with Eastern AddDate semantics: an anchor in EDT yields a 04:00Z expiry,
// while an anchor late enough in the season to cross the fall daylight-saving
// transition yields a 05:00Z expiry (midnight Eastern on the expiry date).
func TestLoad_DSTCrossingExpiry(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load Eastern zone: %v", err)
	}
	tests := []struct {
		name       string
		freezeDate string
	}{
		{name: "EDT anchor", freezeDate: "2026-08-24"},
		{name: "crossing the fall transition", freezeDate: "2026-10-25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := validArtifact()
			file.FreezeDate = tt.freezeDate
			anchor, err := time.ParseInLocation(time.DateOnly, tt.freezeDate, ny)
			if err != nil {
				t.Fatalf("parse anchor: %v", err)
			}
			file.Cutoff = anchor
			file.CalendarExpiry = anchor.AddDate(0, 0, CalendarDays)
			// as_of must remain on the freeze date in Eastern time.
			file.AsOf = file.Cutoff.Add(8 * time.Hour)
			file.CapturedAt = file.AsOf.Add(time.Hour)
			path, pin := writeArtifact(t, file)

			prior, err := Load(path, pin)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !prior.CalendarExpiry.Equal(file.CalendarExpiry) {
				t.Errorf("expiry = %s, want %s", prior.CalendarExpiry, file.CalendarExpiry)
			}
			// Midnight Eastern must land at 04:00Z in EDT and 05:00Z in EST,
			// proving AddDate crossed the transition on wall-clock days.
			wantOffset := 4
			if tt.freezeDate == "2026-10-25" {
				// 56 Eastern days later falls after the fall transition.
				if prior.CalendarExpiry.UTC().Hour() != 5 {
					t.Errorf("expiry hour = %d, want 5 (EST)", prior.CalendarExpiry.UTC().Hour())
				}
			} else if prior.CalendarExpiry.UTC().Hour() != wantOffset {
				t.Errorf("expiry hour = %d, want 4 (EDT)", prior.CalendarExpiry.UTC().Hour())
			}
		})
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
		Cutoff:         time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC),
		CalendarExpiry: time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC),
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
		Cutoff:         time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC),
		CalendarExpiry: time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC),
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
	if adjusted, err := Apply(teams, prior, prior.Cutoff); err == nil || adjusted != nil {
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
