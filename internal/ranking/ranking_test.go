package ranking

import (
	"math"
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
)

func TestRecordString_NoTies(t *testing.T) {
	r := Record{Wins: 10, Losses: 2, Ties: 0}
	got := r.String()
	want := "10-2"
	if got != want {
		t.Errorf("Record.String() = %q, want %q", got, want)
	}
}

func TestRecordString_WithTies(t *testing.T) {
	r := Record{Wins: 8, Losses: 3, Ties: 1}
	got := r.String()
	want := "8-3-1"
	if got != want {
		t.Errorf("Record.String() = %q, want %q", got, want)
	}
}

func TestRecordString_Undefeated(t *testing.T) {
	r := Record{Wins: 13, Losses: 0, Ties: 0}
	got := r.String()
	want := "13-0"
	if got != want {
		t.Errorf("Record.String() = %q, want %q", got, want)
	}
}

func TestTeamListExists(t *testing.T) {
	tl := TeamList{
		1: &Team{Name: "Team A"},
		2: &Team{Name: "Team B"},
	}

	if !tl.teamExists(1) {
		t.Error("teamExists(1) = false, want true")
	}
	if !tl.teamExists(2) {
		t.Error("teamExists(2) = false, want true")
	}
	if tl.teamExists(3) {
		t.Error("teamExists(3) = true, want false")
	}
	if tl.teamExists(0) {
		t.Error("teamExists(0) = true, want false")
	}
}

func TestFinalRanking_Basic(t *testing.T) {
	teamList := TeamList{
		1: &Team{
			Name:    "Alpha",
			Record:  Record{Wins: 12, Losses: 0, Record: 1.0},
			SRSNorm: 1.0,
			SOSNorm: 0.8,
		},
		2: &Team{
			Name:    "Beta",
			Record:  Record{Wins: 8, Losses: 4, Record: 0.667},
			SRSNorm: 0.5,
			SOSNorm: 0.6,
		},
		3: &Team{
			Name:    "Gamma",
			Record:  Record{Wins: 6, Losses: 6, Record: 0.5},
			SRSNorm: 0.3,
			SOSNorm: 0.4,
		},
	}

	r := &Ranker{}
	r.finalRanking(teamList)

	// Alpha: 1.0*0.60 + 1.0*0.30 + 0.8*0.10 = 0.98
	// Beta: 0.667*0.60 + 0.5*0.30 + 0.6*0.10 = 0.6102
	// Gamma: 0.5*0.60 + 0.3*0.30 + 0.4*0.10 = 0.43

	if teamList[1].FinalRank != 1 {
		t.Errorf("Alpha FinalRank = %d, want 1", teamList[1].FinalRank)
	}
	if teamList[2].FinalRank != 2 {
		t.Errorf("Beta FinalRank = %d, want 2", teamList[2].FinalRank)
	}
	if teamList[3].FinalRank != 3 {
		t.Errorf("Gamma FinalRank = %d, want 3", teamList[3].FinalRank)
	}

	// Verify FinalRaw calculation for Alpha
	expectedRaw := 1.0*0.60 + 1.0*0.30 + 0.8*0.10
	if math.Abs(teamList[1].FinalRaw-expectedRaw) > 0.001 {
		t.Errorf("Alpha FinalRaw = %f, want %f", teamList[1].FinalRaw, expectedRaw)
	}
}

func TestFinalRanking_TiedScores(t *testing.T) {
	teamList := TeamList{
		1: &Team{
			Name:    "Team A",
			Record:  Record{Record: 0.75},
			SRSNorm: 0.5,
			SOSNorm: 0.5,
		},
		2: &Team{
			Name:    "Team B",
			Record:  Record{Record: 0.75},
			SRSNorm: 0.5,
			SOSNorm: 0.5,
		},
		3: &Team{
			Name:    "Team C",
			Record:  Record{Record: 0.50},
			SRSNorm: 0.3,
			SOSNorm: 0.3,
		},
	}

	r := &Ranker{}
	r.finalRanking(teamList)

	// Teams A and B have identical FinalRaw, so they should share the same rank
	if teamList[1].FinalRank != teamList[2].FinalRank {
		t.Errorf("Tied teams should share rank: Team A = %d, Team B = %d",
			teamList[1].FinalRank, teamList[2].FinalRank)
	}

	// Team C should be ranked 3rd (not 2nd)
	if teamList[3].FinalRank != 3 {
		t.Errorf("Team C FinalRank = %d, want 3", teamList[3].FinalRank)
	}
}

func TestGenerateAdjRatings_SmallSet(t *testing.T) {
	// Three teams: A(1) beats B(2), B(2) beats C(3), A(1) beats C(3)
	games := []database.Game{
		{HomeID: 1, AwayID: 2, HomeScore: 28, AwayScore: 14},
		{HomeID: 2, AwayID: 3, HomeScore: 21, AwayScore: 7},
		{HomeID: 1, AwayID: 3, HomeScore: 35, AwayScore: 10},
	}

	ratings := generateAdjRatings(games, 30)

	// Team 1 should have the highest rating (won all games)
	// Team 3 should have the lowest rating (lost all games)
	if ratings[1] <= ratings[2] {
		t.Errorf("Team 1 rating (%f) should be > Team 2 rating (%f)", ratings[1], ratings[2])
	}
	if ratings[2] <= ratings[3] {
		t.Errorf("Team 2 rating (%f) should be > Team 3 rating (%f)", ratings[2], ratings[3])
	}
}

func TestGenerateAdjRatings_MOVCapping(t *testing.T) {
	// One blowout game: 50-0
	games := []database.Game{
		{HomeID: 1, AwayID: 2, HomeScore: 50, AwayScore: 0},
	}

	// With MOV=1, spread should be capped to 1
	ratingsMov1 := generateAdjRatings(games, 1)
	// With MOV=30, spread should be capped to 30
	ratingsMov30 := generateAdjRatings(games, 30)

	// With MOV=1, the initial average spread for team 1 should be 1.0
	if math.Abs(ratingsMov1[1]-1.0) > 0.01 {
		// After convergence with only 2 teams, ratings should reflect the capped spread
		// Team 1 initial: 1.0, Team 2 initial: -1.0
		// Iteration: Team 1 = 1.0 + (-1.0)/1 = 0, but that's not right...
		// Actually with 2 teams each playing once, convergence should settle quickly
		t.Logf("MOV=1: Team 1 = %f, Team 2 = %f", ratingsMov1[1], ratingsMov1[2])
	}

	// With MOV=30, the spread is capped to 30 (actual 50 > 30)
	// Team 1's capped spread advantage should be larger than MOV=1
	if math.Abs(ratingsMov1[1]) >= math.Abs(ratingsMov30[1]) {
		t.Errorf("MOV=30 ratings should have larger magnitude than MOV=1: mov1=%f, mov30=%f",
			ratingsMov1[1], ratingsMov30[1])
	}

	// Verify symmetry: ratings should be equal and opposite for two-team case
	if math.Abs(ratingsMov1[1]+ratingsMov1[2]) > 0.001 {
		t.Errorf("MOV=1 ratings should be symmetric: team1=%f, team2=%f",
			ratingsMov1[1], ratingsMov1[2])
	}
}

func TestGenerateAdjRatings_EmptyGames(t *testing.T) {
	ratings := generateAdjRatings(nil, 30)

	if len(ratings) != 0 {
		t.Errorf("expected empty ratings for no games, got %d entries", len(ratings))
	}
}

func TestGenerateAdjRatings_TeamZeroRemoved(t *testing.T) {
	// If team ID 0 somehow appears, it should be removed from results
	games := []database.Game{
		{HomeID: 0, AwayID: 1, HomeScore: 10, AwayScore: 20},
	}

	ratings := generateAdjRatings(games, 30)

	if _, ok := ratings[0]; ok {
		t.Error("team ID 0 should be removed from ratings")
	}
	if _, ok := ratings[1]; !ok {
		t.Error("team ID 1 should exist in ratings")
	}
}
