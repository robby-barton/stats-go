package ranking

import (
	"math"
	"testing"
	"time"
)

func TestSRS_BasicRanking(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

	teamList := TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

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
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

	teamList := TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

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
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

	teamList := TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

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

func TestCalculateRanking_FullPipeline(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:   db,
		Year: 2023,
		Week: 6,
	}

	// Need to set startTime manually since setGlobals queries for week 6
	// which doesn't exist in our data. Use setup flow with Week=0 instead.
	r.Week = 0
	teamList, err := r.CalculateRanking()
	if err != nil {
		t.Fatalf("CalculateRanking: %v", err)
	}

	if len(teamList) != 4 {
		t.Fatalf("len(teamList) = %d, want 4", len(teamList))
	}

	// Alpha (4-0) should be rank 1
	if teamList[1].FinalRank != 1 {
		t.Errorf("Alpha FinalRank = %d, want 1", teamList[1].FinalRank)
	}

	// FinalRank should span 1-4 (some might tie)
	ranks := map[int64]bool{}
	for _, team := range teamList {
		ranks[team.FinalRank] = true
		if team.FinalRank < 1 || team.FinalRank > 4 {
			t.Errorf("FinalRank = %d, want [1,4]", team.FinalRank)
		}
	}
}
