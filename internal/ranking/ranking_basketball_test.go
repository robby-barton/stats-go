package ranking

import (
	"math"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/sport"
)

// basketballInput builds an Input over the basketball fixture set with the
// ranking window already resolved (as internal/ranking/load would).
func basketballInput() Input {
	games := basketballGames()
	// The loader hands games over in descending start-time order.
	for i, j := 0, len(games)-1; i < j; i, j = i+1, j-1 {
		games[i], games[j] = games[j], games[i]
	}
	return Input{
		Year:      2024,
		Week:      3,
		Sport:     sport.Basketball,
		StartTime: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Teams:     basketballTeams(),
		Games:     games,
	}
}

func basketballTeamList() TeamList {
	return TeamList{
		101: &Team{Name: "Hoops A"},
		102: &Team{Name: "Hoops B"},
		103: &Team{Name: "Hoops C"},
		104: &Team{Name: "Hoops D"},
		105: &Team{Name: "Hoops E"},
	}
}

func TestRecord_Basketball(t *testing.T) {
	r := newTestRanker(t, basketballInput())

	teamList := basketballTeamList()

	r.record(teamList)

	// Hoops A: 3W-0L (3001, 3003 as away win, 3004)
	assertRecord(t, "Hoops A", teamList[101], 3, 0, 0, 4.0/5.0)

	// Hoops B: 1W-1L (win: 3005, loss: 3001)
	assertRecord(t, "Hoops B", teamList[102], 1, 1, 0, 2.0/4.0)

	// Hoops E: 0W-1L-1T (loss: 3003, tie: 3006)
	assertRecord(t, "Hoops E", teamList[105], 0, 1, 1, 1.5/4.0)
}

func TestSRS_Basketball(t *testing.T) {
	r := newTestRanker(t, basketballInput())

	teamList := basketballTeamList()

	if err := r.srs(teamList); err != nil {
		t.Fatalf("srs: %v", err)
	}

	// Hoops A (3-0) should have highest SRS
	if teamList[101].SRS <= teamList[104].SRS {
		t.Errorf("Hoops A SRS (%f) should be > Hoops D SRS (%f)", teamList[101].SRS, teamList[104].SRS)
	}

	// All SRSNorm should be in [0, 1]
	for id, team := range teamList {
		if team.SRSNorm < 0 || team.SRSNorm > 1 {
			t.Errorf("team %d SRSNorm = %f, want [0,1]", id, team.SRSNorm)
		}
	}

	// Best team should have SRSNorm = 1.0
	if math.Abs(teamList[101].SRSNorm-1.0) > 0.001 {
		t.Errorf("Hoops A SRSNorm = %f, want 1.0", teamList[101].SRSNorm)
	}
}

func TestCalculateRanking_Basketball(t *testing.T) {
	r := newTestRanker(t, basketballInput())

	teamList, err := r.CalculateRanking()
	if err != nil {
		t.Fatalf("CalculateRanking: %v", err)
	}

	// All 5 basketball teams should be included (no division split for ncaam)
	if len(teamList) != 5 {
		t.Fatalf("len(teamList) = %d, want 5", len(teamList))
	}

	// Hoops A (3-0) should be rank 1
	if teamList[101].FinalRank != 1 {
		t.Errorf("Hoops A FinalRank = %d, want 1", teamList[101].FinalRank)
	}

	// All ranks should be valid
	for _, team := range teamList {
		if team.FinalRank < 1 || team.FinalRank > 5 {
			t.Errorf("team FinalRank = %d, want [1,5]", team.FinalRank)
		}
	}
}
