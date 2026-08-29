package ranking

import (
	"math"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/sport"
)

// footballInput builds an Input over the full football fixture set with the
// ranking window already resolved (as internal/ranking/load would).
func footballInput() Input {
	games := footballGames()
	// The loader hands games over in descending start-time order.
	for i, j := 0, len(games)-1; i < j; i, j = i+1, j-1 {
		games[i], games[j] = games[j], games[i]
	}
	return Input{
		Year:      2023,
		Sport:     sport.Football,
		StartTime: time.Date(2023, 10, 20, 0, 0, 0, 0, time.UTC),
		Teams:     footballTeams(),
		Games:     games,
	}
}

func TestSRS_BasicRanking(t *testing.T) {
	r := newTestRanker(t, footballInput())

	teamList := fbsTeamList()

	if err := r.srs(teamList); err != nil {
		t.Fatalf("srs: %v", err)
	}

	// Alpha won all games so should have highest SRS
	if teamList[1].SRS <= teamList[4].SRS {
		t.Errorf("Alpha SRS (%f) should be > Delta SRS (%f)", teamList[1].SRS, teamList[4].SRS)
	}

	// All SRSNorm should be in [0, 1]
	for id, team := range teamList {
		if team.SRSNorm < 0 || team.SRSNorm > 1 {
			t.Errorf("team %d SRSNorm = %f, want [0,1]", id, team.SRSNorm)
		}
	}

	// Alpha should have SRSNorm = 1.0 (highest), Delta SRSNorm = 0.0 (lowest)
	if math.Abs(teamList[1].SRSNorm-1.0) > 0.001 {
		t.Errorf("Alpha SRSNorm = %f, want 1.0", teamList[1].SRSNorm)
	}
	if math.Abs(teamList[4].SRSNorm-0.0) > 0.001 {
		t.Errorf("Delta SRSNorm = %f, want 0.0", teamList[4].SRSNorm)
	}
}

func TestSRS_RankAssignment(t *testing.T) {
	r := newTestRanker(t, footballInput())

	teamList := fbsTeamList()

	if err := r.srs(teamList); err != nil {
		t.Fatalf("srs: %v", err)
	}

	// Ranks should be 1-based
	for id, team := range teamList {
		if team.SRSRank < 1 || team.SRSRank > 4 {
			t.Errorf("team %d SRSRank = %d, want [1,4]", id, team.SRSRank)
		}
	}

	// Alpha should be rank 1
	if teamList[1].SRSRank != 1 {
		t.Errorf("Alpha SRSRank = %d, want 1", teamList[1].SRSRank)
	}
}

func TestSOS_BasicRanking(t *testing.T) {
	r := newTestRanker(t, footballInput())

	teamList := fbsTeamList()

	if err := r.sos(teamList); err != nil {
		t.Fatalf("sos: %v", err)
	}

	// All SOSNorm should be in [0, 1]
	for id, team := range teamList {
		if team.SOSNorm < 0 || team.SOSNorm > 1 {
			t.Errorf("team %d SOSNorm = %f, want [0,1]", id, team.SOSNorm)
		}
	}

	// Ranks should be 1-based
	for id, team := range teamList {
		if team.SOSRank < 1 || team.SOSRank > 4 {
			t.Errorf("team %d SOSRank = %d, want [1,4]", id, team.SOSRank)
		}
	}
}

func TestSRS_EmptyTeamList(t *testing.T) {
	r := newTestRanker(t, footballInput())

	// Must not panic and must not touch any team.
	if err := r.srs(TeamList{}); err != nil {
		t.Fatalf("srs on empty team list: %v", err)
	}
}

func TestSOS_EmptyTeamList(t *testing.T) {
	r := newTestRanker(t, footballInput())

	if err := r.sos(TeamList{}); err != nil {
		t.Fatalf("sos on empty team list: %v", err)
	}
}

// TestSRS_DegenerateMOVRange covers the all-MOVs-equal case: every game is a
// cycle (A beats B, B beats C, C beats A, all by the same margin), so every
// adjusted rating is identical and (rating-minMOV)/(maxMOV-minMOV) would be
// 0/0 = NaN without the zero-range guard.
func TestSRS_DegenerateMOVRange(t *testing.T) {
	base := time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour

	// Spreads larger than any MOV cap so all spreads clamp to the cap.
	games := []Game{
		{
			GameID: 1, Season: 2023, Week: 1,
			HomeID: 1, AwayID: 2, HomeScore: 40, AwayScore: 0,
			StartTime: base,
		},
		{GameID: 2, Season: 2023, Week: 2, HomeID: 2, AwayID: 3,
			HomeScore: 40, AwayScore: 0, StartTime: base.Add(week)},
		{GameID: 3, Season: 2023, Week: 3, HomeID: 3, AwayID: 1,
			HomeScore: 40, AwayScore: 0, StartTime: base.Add(2 * week)},
	}
	// Descending start-time order, as the loader provides.
	for i, j := 0, len(games)-1; i < j; i, j = i+1, j-1 {
		games[i], games[j] = games[j], games[i]
	}

	r := newTestRanker(t, Input{
		Year:      2023,
		Sport:     sport.Football,
		StartTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
		Teams:     footballTeams()[:3],
		Games:     games,
	})

	teamList := TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
	}

	if err := r.srs(teamList); err != nil {
		t.Fatalf("srs: %v", err)
	}

	for id, team := range teamList {
		if math.IsNaN(team.SRS) || math.IsInf(team.SRS, 0) {
			t.Errorf("team %d SRS = %f, want a finite value", id, team.SRS)
		}
		if team.SRSNorm < 0 || team.SRSNorm > 1 {
			t.Errorf("team %d SRSNorm = %f, want [0,1]", id, team.SRSNorm)
		}
		// All teams are perfectly balanced, so all SRS values must tie at the
		// same rank — and that rank must be 1, not 0.
		if team.SRSRank != 1 {
			t.Errorf("team %d SRSRank = %d, want 1 (all teams tied)", id, team.SRSRank)
		}
	}
}
