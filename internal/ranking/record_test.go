package ranking

import (
	"math"
	"testing"
	"time"
)

func TestRecord_BasicRecords(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC), // after all games
	}

	teamList := TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

	if err := r.record(teamList); err != nil {
		t.Fatalf("record: %v", err)
	}

	// Alpha: 4W-0L-0T (games 1001,1003,1005,1008 wins)
	// Record = (1+4+0)/(2+4) = 5/6 ≈ 0.833
	assertRecord(t, "Alpha", teamList[1], 4, 0, 0, 5.0/6.0)

	// Beta: 3W-2L-0T (wins: 1004,1006,1010; losses: 1001,1007)
	// Record = (1+3)/(2+5) = 4/7 ≈ 0.571
	assertRecord(t, "Beta", teamList[2], 3, 2, 0, 4.0/7.0)

	// Gamma: 2W-2L-1T (wins: 1002,1007; losses: 1003,1006; tie: 1009)
	// Record = (1+2+0.5)/(2+5) = 3.5/7 = 0.500
	assertRecord(t, "Gamma", teamList[3], 2, 2, 1, 3.5/7.0)

	// Delta: 0W-3L-1T (losses: 1002,1004,1005; tie: 1009)
	// Record = (1+0+0.5)/(2+4) = 1.5/6 = 0.250
	assertRecord(t, "Delta", teamList[4], 0, 3, 1, 1.5/6.0)
}

func TestRecord_PartialSeason(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	// startTime through week 2 only (before week 3 games)
	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
		startTime: time.Date(2023, 9, 18, 0, 0, 0, 0, time.UTC),
	}

	teamList := TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

	if err := r.record(teamList); err != nil {
		t.Fatalf("record: %v", err)
	}

	// After weeks 1-2 only:
	// Alpha: 2W (1001,1003)
	assertRecord(t, "Alpha", teamList[1], 2, 0, 0, 3.0/4.0)

	// Delta: 0W-2L (1002,1004)
	assertRecord(t, "Delta", teamList[4], 0, 2, 0, 1.0/4.0)
}

func TestRecord_TieHandling(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

	teamList := TeamList{
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

	if err := r.record(teamList); err != nil {
		t.Fatalf("record: %v", err)
	}

	if teamList[3].Record.Ties != 1 {
		t.Errorf("Gamma ties = %d, want 1", teamList[3].Record.Ties)
	}
	if teamList[4].Record.Ties != 1 {
		t.Errorf("Delta ties = %d, want 1", teamList[4].Record.Ties)
	}
}

func assertRecord(t *testing.T, name string, team *Team, wins, losses, ties int64, record float64) {
	t.Helper()
	if team.Record.Wins != wins {
		t.Errorf("%s Wins = %d, want %d", name, team.Record.Wins, wins)
	}
	if team.Record.Losses != losses {
		t.Errorf("%s Losses = %d, want %d", name, team.Record.Losses, losses)
	}
	if team.Record.Ties != ties {
		t.Errorf("%s Ties = %d, want %d", name, team.Record.Ties, ties)
	}
	if math.Abs(team.Record.Record-record) > 0.001 {
		t.Errorf("%s Record = %f, want %f", name, team.Record.Record, record)
	}
}
