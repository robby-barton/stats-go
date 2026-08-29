package ranking

import (
	"math"
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
)

func TestSRS_BasicRanking(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
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
		Sport:     sportFootball,
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
		Sport:     sportFootball,
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
		DB:    db,
		Year:  2023,
		Week:  6,
		Sport: sportFootball,
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

func TestSRS_EmptyTeamList(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

	// Must not panic and must not touch any team.
	if err := r.srs(TeamList{}); err != nil {
		t.Fatalf("srs on empty team list: %v", err)
	}
}

func TestSOS_EmptyTeamList(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

	if err := r.sos(TeamList{}); err != nil {
		t.Fatalf("sos on empty team list: %v", err)
	}
}

// TestSRS_DegenerateMOVRange covers the all-MOVs-equal case: every game is a
// cycle (A beats B, B beats C, C beats A, all by the same margin), so every
// adjusted rating is identical and (rating-minMOV)/(maxMOV-minMOV) would be
// 0/0 = NaN without the zero-range guard.
func TestSRS_DegenerateMOVRange(t *testing.T) {
	db := setupTestDB(t)

	teamNames := []database.TeamName{
		{TeamID: 1, Name: "Alpha", Sport: "ncaaf"},
		{TeamID: 2, Name: "Beta", Sport: "ncaaf"},
		{TeamID: 3, Name: "Gamma", Sport: "ncaaf"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed team_names: %v", err)
	}
	teamSeasons := []database.TeamSeason{
		{TeamID: 1, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 2, Year: 2023, FBS: 1, Conf: "SEC", Sport: "ncaaf"},
		{TeamID: 3, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "ncaaf"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed team_seasons: %v", err)
	}

	base := time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	// Spreads larger than any MOV cap so all spreads clamp to the cap.
	games := []database.Game{
		{
			GameID: 1, Season: 2023, Week: 1,
			HomeID: 1, AwayID: 2, HomeScore: 40, AwayScore: 0,
			Sport: "ncaaf", StartTime: base,
		},
		{GameID: 2, Season: 2023, Week: 2, HomeID: 2, AwayID: 3,
			HomeScore: 40, AwayScore: 0, Sport: "ncaaf", StartTime: base.Add(week)},
		{GameID: 3, Season: 2023, Week: 3, HomeID: 3, AwayID: 1,
			HomeScore: 40, AwayScore: 0, Sport: "ncaaf", StartTime: base.Add(2 * week)},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed games: %v", err)
	}

	r := &Ranker{
		DB:        db,
		Year:      2023,
		Sport:     sportFootball,
		startTime: time.Date(2023, 10, 10, 0, 0, 0, 0, time.UTC),
	}

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

func TestFinalRanking_ZeroScoreTie(t *testing.T) {
	db := setupTestDB(t)

	r := &Ranker{
		DB:    db,
		Year:  2023,
		Sport: sportFootball,
	}

	// All teams have FinalRaw exactly zero — the first team in sorted order
	// must get rank 1, not 0.
	teamList := TeamList{
		1: &Team{Name: "Alpha", FinalRaw: 0},
		2: &Team{Name: "Beta", FinalRaw: 0},
		3: &Team{Name: "Gamma", FinalRaw: 0},
	}

	r.finalRanking(teamList)

	for id, team := range teamList {
		if team.FinalRank != 1 {
			t.Errorf("team %d FinalRank = %d, want 1 (all tied at zero)", id, team.FinalRank)
		}
	}
}
