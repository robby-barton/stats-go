package ranking

import (
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/sport"
)

// TestSportConfig_WomensBasketball verifies that the women's basketball
// launch reuses the men's basketball ranking parameters unchanged: the
// 25-game SRS quota, one season of lookback, MOV caps, and the record/SRS/
// SOS weights of 25/60/15. Inputs stay isolated by sport; no FBS/FCS split
// and no football preseason prior apply.
func TestSportConfig_WomensBasketball(t *testing.T) {
	womens := sportConfig(sport.WomensBasketball)
	mens := sportConfig(sport.Basketball)

	if womens.RequiredGames != mens.RequiredGames || womens.YearsBack != mens.YearsBack ||
		womens.RecordWeight != mens.RecordWeight || womens.SRSWeight != mens.SRSWeight ||
		womens.SOSWeight != mens.SOSWeight ||
		womens.MOVCaps[0] != mens.MOVCaps[0] || womens.MOVCaps[1] != mens.MOVCaps[1] {
		t.Errorf("women's params = %+v, want men's %+v", womens, mens)
	}
	if womens.RequiredGames != 25 || womens.YearsBack != 1 {
		t.Errorf("RequiredGames/YearsBack = %d/%d, want 25/1",
			womens.RequiredGames, womens.YearsBack)
	}
	if womens.RecordWeight != 0.25 || womens.SRSWeight != 0.60 || womens.SOSWeight != 0.15 {
		t.Errorf("weights = %.2f/%.2f/%.2f, want 0.25/0.60/0.15",
			womens.RecordWeight, womens.SRSWeight, womens.SOSWeight)
	}

	if YearsBack(sport.WomensBasketball) != 1 {
		t.Errorf("YearsBack(ncaaw) = %d, want 1", YearsBack(sport.WomensBasketball))
	}
	if RequiredGames(sport.WomensBasketball) != 25 {
		t.Errorf("RequiredGames(ncaaw) = %d, want 25",
			RequiredGames(sport.WomensBasketball))
	}
}

// womensTeams returns a small D-I field. The IDs overlap the men's basketball
// fixture range deliberately — ranking inputs are per-sport plain values, so
// identical school IDs across sports never interact.
func womensTeams() []TeamInfo {
	return []TeamInfo{
		{ID: 101, Name: "UConn", Conf: "Big East", FBS: true},
		{ID: 102, Name: "South Carolina", Conf: "SEC", FBS: true},
		{ID: 103, Name: "UCLA", Conf: "Big Ten", FBS: true},
		{ID: 104, Name: "Iowa", Conf: "Big Ten", FBS: true},
	}
}

// womensGames returns four 2027 games across two weeks.
func womensGames() []Game {
	base := time.Date(2027, 1, 5, 19, 0, 0, 0, time.UTC) // Tuesday
	week := 7 * 24 * time.Hour

	return []Game{
		{GameID: 5001, Season: 2027, Week: 1, HomeID: 101, AwayID: 102,
			HomeScore: 85, AwayScore: 70, StartTime: base},
		{GameID: 5002, Season: 2027, Week: 1, HomeID: 103, AwayID: 104,
			HomeScore: 80, AwayScore: 78, StartTime: base.Add(time.Hour)},
		{GameID: 5003, Season: 2027, Week: 2, HomeID: 101, AwayID: 103,
			HomeScore: 90, AwayScore: 60, StartTime: base.Add(week)},
		{GameID: 5004, Season: 2027, Week: 2, HomeID: 102, AwayID: 104,
			HomeScore: 75, AwayScore: 74, StartTime: base.Add(week + time.Hour)},
	}
}

// TestCalculateRanking_WomensBasketball runs the full women's pipeline over
// the small D-I fixture: every school is top-division, margins sit within
// the basketball MOV caps, and no cross-sport leakage is possible because
// the input carries only ncaaw teams and games.
func TestCalculateRanking_WomensBasketball(t *testing.T) {
	r := newTestRanker(t, Input{
		Sport: sport.WomensBasketball, Year: 2027, Week: 2,
		Teams: womensTeams(), Games: womensGames(),
	})

	teamList, err := r.CalculateRanking()
	if err != nil {
		t.Fatalf("CalculateRanking: %v", err)
	}

	if len(teamList) != 4 {
		t.Fatalf("len(teamList) = %d, want 4", len(teamList))
	}

	// UConn went 2-0 with the best margins and should sit at rank 1.
	if teamList[101].FinalRank != 1 {
		t.Errorf("UConn FinalRank = %d, want 1", teamList[101].FinalRank)
	}
	if teamList[101].Record.Wins != 2 || teamList[101].Record.Losses != 0 {
		t.Errorf("UConn record = %d-%d, want 2-0",
			teamList[101].Record.Wins, teamList[101].Record.Losses)
	}
	if teamList[104].FinalRank != 4 {
		t.Errorf("Iowa FinalRank = %d, want 4 (0-2)", teamList[104].FinalRank)
	}

	// All ranks valid and FinalRaw descending by rank.
	for id, team := range teamList {
		if team.FinalRank < 1 || team.FinalRank > 4 {
			t.Errorf("team %d FinalRank = %d, want [1,4]", id, team.FinalRank)
		}
		for _, other := range teamList {
			if other.FinalRank > team.FinalRank && other.FinalRaw >= team.FinalRaw {
				t.Errorf("rank %d (%.3f) not above rank %d (%.3f)",
					team.FinalRank, team.FinalRaw, other.FinalRank, other.FinalRaw)
			}
		}
	}
}
