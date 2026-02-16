package ranking

import (
	"math"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
)

// seedBasketballData inserts 5 basketball teams (all FBS=1) and 6 games across
// 2 weeks for the 2024 season. Also inserts one football game to confirm
// sport filtering excludes it.
func seedBasketballData(t *testing.T, db *gorm.DB) {
	t.Helper()

	teamNames := []database.TeamName{
		{TeamID: 101, Name: "Hoops A", Sport: "ncaambb"},
		{TeamID: 102, Name: "Hoops B", Sport: "ncaambb"},
		{TeamID: 103, Name: "Hoops C", Sport: "ncaambb"},
		{TeamID: 104, Name: "Hoops D", Sport: "ncaambb"},
		{TeamID: 105, Name: "Hoops E", Sport: "ncaambb"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed basketball team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 101, Year: 2024, FBS: 1, Conf: "Big East", Sport: "ncaambb"},
		{TeamID: 102, Year: 2024, FBS: 1, Conf: "Big East", Sport: "ncaambb"},
		{TeamID: 103, Year: 2024, FBS: 1, Conf: "ACC", Sport: "ncaambb"},
		{TeamID: 104, Year: 2024, FBS: 1, Conf: "ACC", Sport: "ncaambb"},
		{TeamID: 105, Year: 2024, FBS: 1, Conf: "Big 12", Sport: "ncaambb"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed basketball team_seasons: %v", err)
	}

	base := time.Date(2024, 1, 2, 19, 0, 0, 0, time.UTC) // Tuesday
	week := 7 * 24 * time.Hour

	games := []database.Game{
		// Week 1
		{GameID: 3001, Season: 2024, Week: 1, HomeID: 101, AwayID: 102,
			HomeScore: 78, AwayScore: 65, ConfGame: true, Sport: "ncaambb", StartTime: base},
		{GameID: 3002, Season: 2024, Week: 1, HomeID: 103, AwayID: 104,
			HomeScore: 70, AwayScore: 68, ConfGame: true, Sport: "ncaambb", StartTime: base.Add(time.Hour)},
		{GameID: 3003, Season: 2024, Week: 1, HomeID: 105, AwayID: 101,
			HomeScore: 60, AwayScore: 72, Sport: "ncaambb", StartTime: base.Add(2 * time.Hour)},
		// Week 2
		{GameID: 3004, Season: 2024, Week: 2, HomeID: 101, AwayID: 103,
			HomeScore: 80, AwayScore: 75, Sport: "ncaambb", StartTime: base.Add(week)},
		{GameID: 3005, Season: 2024, Week: 2, HomeID: 102, AwayID: 104,
			HomeScore: 66, AwayScore: 64, Sport: "ncaambb", StartTime: base.Add(week + time.Hour)},
		{GameID: 3006, Season: 2024, Week: 2, HomeID: 105, AwayID: 103,
			HomeScore: 55, AwayScore: 55, Sport: "ncaambb", StartTime: base.Add(week + 2*time.Hour)},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed basketball games: %v", err)
	}

	// Football game — should be excluded by sport filter
	footballGame := database.Game{
		GameID: 9001, Season: 2024, Week: 1, HomeID: 101, AwayID: 102,
		HomeScore: 35, AwayScore: 10, Sport: "ncaaf",
		StartTime: base,
	}
	if err := db.Create(&footballGame).Error; err != nil {
		t.Fatalf("seed football game: %v", err)
	}
}

func TestSetGlobals_Basketball(t *testing.T) {
	db := setupTestDB(t)
	seedBasketballData(t, db)

	// Also seed football data to ensure it's ignored
	seedTestData(t, db)

	r := &Ranker{DB: db, Sport: "ncaambb"}
	if err := r.setGlobals(); err != nil {
		t.Fatalf("setGlobals: %v", err)
	}

	if r.Year != 2024 {
		t.Errorf("Year = %d, want 2024", r.Year)
	}
	// Latest basketball game is week 2, so Week should be 3
	if r.Week != 3 {
		t.Errorf("Week = %d, want 3", r.Week)
	}
}

func TestCreateTeamList_Basketball(t *testing.T) {
	db := setupTestDB(t)
	seedBasketballData(t, db)

	r := &Ranker{DB: db, Year: 2024, Week: 3, Sport: "ncaambb"}

	// All basketball teams have FBS=1
	teamList, err := r.createTeamList(1)
	if err != nil {
		t.Fatalf("createTeamList(1): %v", err)
	}
	if len(teamList) != 5 {
		t.Fatalf("len(teamList) = %d, want 5", len(teamList))
	}

	// No FCS in basketball — createTeamList(0) should return empty
	teamListFCS, err := r.createTeamList(0)
	if err != nil {
		t.Fatalf("createTeamList(0): %v", err)
	}
	if len(teamListFCS) != 0 {
		t.Errorf("len(teamList FCS) = %d, want 0", len(teamListFCS))
	}
}

func TestRecord_Basketball(t *testing.T) {
	db := setupTestDB(t)
	seedBasketballData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2024,
		Sport:     "ncaambb",
		startTime: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), // after all games
	}

	teamList := TeamList{
		101: &Team{Name: "Hoops A"},
		102: &Team{Name: "Hoops B"},
		103: &Team{Name: "Hoops C"},
		104: &Team{Name: "Hoops D"},
		105: &Team{Name: "Hoops E"},
	}

	if err := r.record(teamList); err != nil {
		t.Fatalf("record: %v", err)
	}

	// Hoops A: 3W-0L (3001, 3003 as away win, 3004)
	assertRecord(t, "Hoops A", teamList[101], 3, 0, 0, 4.0/5.0)

	// Hoops B: 1W-1L (win: 3005, loss: 3001)
	assertRecord(t, "Hoops B", teamList[102], 1, 1, 0, 2.0/4.0)

	// Hoops E: 0W-1L-1T (loss: 3003, tie: 3006)
	assertRecord(t, "Hoops E", teamList[105], 0, 1, 1, 1.5/4.0)
}

func TestSRS_Basketball(t *testing.T) {
	db := setupTestDB(t)
	seedBasketballData(t, db)

	r := &Ranker{
		DB:        db,
		Year:      2024,
		Sport:     "ncaambb",
		startTime: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	teamList := TeamList{
		101: &Team{Name: "Hoops A"},
		102: &Team{Name: "Hoops B"},
		103: &Team{Name: "Hoops C"},
		104: &Team{Name: "Hoops D"},
		105: &Team{Name: "Hoops E"},
	}

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
	db := setupTestDB(t)
	seedBasketballData(t, db)

	r := &Ranker{
		DB:    db,
		Year:  2024,
		Sport: "ncaambb",
	}

	teamList, err := r.CalculateRanking()
	if err != nil {
		t.Fatalf("CalculateRanking: %v", err)
	}

	// All 5 basketball teams should be included (all FBS=1 for cbb)
	if len(teamList) != 5 {
		t.Fatalf("len(teamList) = %d, want 5", len(teamList))
	}

	// Hoops A (3-0) should be rank 1
	if teamList[101].FinalRank != 1 {
		t.Errorf("Hoops A FinalRank = %d, want 1", teamList[101].FinalRank)
	}

	// All ranks should be valid
	for id, team := range teamList {
		if team.FinalRank < 1 || team.FinalRank > 5 {
			t.Errorf("team %d FinalRank = %d, want [1,5]", id, team.FinalRank)
		}
	}
}
