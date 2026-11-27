package ncaafreplay

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseManifestAcceptsValidWindows(t *testing.T) {
	manifest, err := ParseManifest([]byte(`{
		"schema": "ncaaf-replay-manifest/1",
		"windows": [
			{"year": 2023, "week": 3},
			{"year": 2023, "week": 7, "cutoff": "2023-10-10T04:00:00Z"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Windows) != 2 || manifest.Windows[1].Year != 2023 || manifest.Windows[1].Week != 7 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	cutoff, err := manifest.Windows[1].cutoff()
	if err != nil || cutoff.IsZero() {
		t.Fatalf("cutoff parse: %v %v", cutoff, err)
	}
	empty, err := manifest.Windows[0].cutoff()
	if err != nil || !empty.IsZero() {
		t.Fatalf("optional cutoff must stay zero: %v %v", empty, err)
	}
}

func TestParseManifestRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name, body string
	}{
		{"unknown schema", `{"schema": "other/1", "windows": [{"year": 2023, "week": 1}]}`},
		{"unknown field", `{"schema": "ncaaf-replay-manifest/1", "windows": [{"year": 2023, "week": 1, "extra": 1}]}`},
		{"unknown top-level field", `{"schema": "ncaaf-replay-manifest/1", "windows": [], "note": "x"}`},
		{"no windows", `{"schema": "ncaaf-replay-manifest/1", "windows": []}`},
		{"week zero", `{"schema": "ncaaf-replay-manifest/1", "windows": [{"year": 2023, "week": 0}]}`},
		{"week negative", `{"schema": "ncaaf-replay-manifest/1", "windows": [{"year": 2023, "week": -2}]}`},
		{"year too old", `{"schema": "ncaaf-replay-manifest/1", "windows": [{"year": 1935, "week": 1}]}`},
		{"year too new", `{"schema": "ncaaf-replay-manifest/1", "windows": [{"year": 10000, "week": 1}]}`},
		{"duplicate window", `{"schema": "ncaaf-replay-manifest/1", "windows": [` +
			`{"year": 2023, "week": 3}, {"year": 2023, "week": 3}]}`},
		{"malformed cutoff", `{"schema": "ncaaf-replay-manifest/1", "windows": [` +
			`{"year": 2023, "week": 3, "cutoff": "not-a-time"}]}`},
		{"not json", `nonsense`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(tc.body)); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	// Distinct weeks isolate the window-count bound from duplicate rejection.
	windows := make([]string, 0, maxWindows+1)
	for week := 1; week <= maxWindows+1; week++ {
		windows = append(windows, fmt.Sprintf(`{"year": 2023, "week": %d}`, week))
	}
	oversized := `{"schema": "ncaaf-replay-manifest/1", "windows": [` + strings.Join(windows, ",") + `]}`
	if _, err := ParseManifest([]byte(oversized)); err == nil {
		t.Fatal("oversized manifest accepted")
	}
}
