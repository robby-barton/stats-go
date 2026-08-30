package espn

import (
	"testing"
	"time"
)

// checkChunkInvariants asserts the properties every weekChunks result must
// have: no chunk starts on a Sunday (the site.api ?dates= range hazard),
// chunks are contiguous, and the span from the first chunk start through the
// calendar end is fully covered.
func checkChunkInvariants(t *testing.T, start, end time.Time, chunks [][2]time.Time) {
	t.Helper()

	// A Sunday span start is intentionally skipped (shifted to Monday), so
	// expected coverage begins the day after such a start.
	covered := start
	if start.Weekday() == time.Sunday {
		covered = start.AddDate(0, 0, 1)
	}
	for i, c := range chunks {
		if c[0].Weekday() == time.Sunday {
			t.Errorf("chunk %d starts on Sunday %s: degenerate for the site.api range endpoint",
				i, c[0].Format("2006-01-02"))
		}
		if c[0].After(c[1]) {
			t.Errorf("chunk %d start %s after end %s", i, c[0].Format("2006-01-02"), c[1].Format("2006-01-02"))
		}
		if covered.Before(c[0]) {
			t.Errorf("gap in coverage: %s through %s not covered by any chunk",
				covered.Format("2006-01-02"), c[0].AddDate(0, 0, -1).Format("2006-01-02"))
		}
		if c[1].After(covered) {
			covered = c[1].AddDate(0, 0, 1)
		}
	}
	if covered.Before(end.AddDate(0, 0, 1)) && len(chunks) > 0 {
		t.Errorf("coverage ends at %s, want through %s inclusive",
			covered.AddDate(0, 0, -1).Format("2006-01-02"), end.Format("2006-01-02"))
	}
}

func chunkStrs(chunks [][2]time.Time) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c[0].Format("20060102") + "-" + c[1].Format("20060102")
	}
	return out
}

func TestWeekChunks_SaturdayAnchoredSeason(t *testing.T) {
	// Regular-season calendars are Saturday-anchored. A 16-day span starting
	// on a Saturday yields two full weekly chunks plus a final chunk clipped
	// to the calendar end (which lands on a Sunday as a chunk END — safe;
	// only chunk STARTS on a Sunday are degenerate).
	start, _ := parseESPNTime("2026-09-05T07:00Z") // Saturday
	end, _ := parseESPNTime("2026-09-20T07:00Z")   // Sunday

	chunks := weekChunks(start, end)
	checkChunkInvariants(t, start, end, chunks)

	want := []string{"20260905-20260911", "20260912-20260918", "20260919-20260920"}
	got := chunkStrs(chunks)
	if len(got) != len(want) {
		t.Fatalf("len(chunks) = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chunk %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestWeekChunks_SundayStartShiftsToMonday(t *testing.T) {
	// The postseason calendar starts on a Sunday in 2022, 2024, and 2026.
	// Every naive 7-day anchor from a Sunday start is itself a Sunday; each
	// chunk start must shift forward to Monday (2026-12-20 is a Sunday).
	start, _ := parseESPNTime("2026-12-20T07:00Z") // Sunday
	end, _ := parseESPNTime("2027-01-10T07:00Z")   // Sunday

	chunks := weekChunks(start, end)
	checkChunkInvariants(t, start, end, chunks)

	want := []string{"20261221-20261227", "20261228-20270103", "20270104-20270110"}
	got := chunkStrs(chunks)
	if len(got) != len(want) {
		t.Fatalf("len(chunks) = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chunk %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestWeekChunks_ShorterThanWeek(t *testing.T) {
	start, _ := parseESPNTime("2026-09-05T07:00Z") // Saturday
	end, _ := parseESPNTime("2026-09-07T07:00Z")   // Monday

	chunks := weekChunks(start, end)
	checkChunkInvariants(t, start, end, chunks)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if got := chunkStrs(chunks)[0]; got != "20260905-20260907" {
		t.Errorf("chunk = %s, want 20260905-20260907 (clipped to end)", got)
	}
}

func TestWeekChunks_FinalChunkWouldStartOnSunday(t *testing.T) {
	// A span whose tail leaves a lone uncovered Sunday cannot occur with
	// weekday-preserving 7-day stepping — Sundays only surface as the span
	// start (shifted) or as chunk ends. The degenerate residue the helper
	// must handle is a span that is nothing but one Sunday: there is no
	// non-degenerate way to fetch it, so it yields no chunks rather than a
	// Sunday-anchored range.
	start, _ := parseESPNTime("2026-12-20T07:00Z") // Sunday
	end, _ := parseESPNTime("2026-12-20T07:00Z")   // same Sunday

	if chunks := weekChunks(start, end); len(chunks) != 0 {
		t.Errorf("single-Sunday span = %v, want no chunks", chunkStrs(chunks))
	}

	// Tail variant: a Monday-start span ending exactly one week later leaves
	// the final chunk ending on a Sunday; verify the final chunk is emitted
	// with the calendar end inclusive (the naive next anchor would be that
	// Sunday, but stepping keeps every start on Monday).
	start, _ = parseESPNTime("2026-12-21T07:00Z") // Monday
	end, _ = parseESPNTime("2026-12-27T07:00Z")   // Sunday

	chunks := weekChunks(start, end)
	checkChunkInvariants(t, start, end, chunks)
	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d (%v), want 1", len(chunks), chunkStrs(chunks))
	}
	if got := chunkStrs(chunks)[0]; got != "20261221-20261227" {
		t.Errorf("chunk = %s, want 20261221-20261227", got)
	}
}
