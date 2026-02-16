package ranking

import (
	"testing"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
)

func TestSetGlobals_DefaultYear(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{DB: db, Sport: sportFootball}
	if err := r.setGlobals(); err != nil {
		t.Fatalf("setGlobals: %v", err)
	}

	if r.Year != 2023 {
		t.Errorf("Year = %d, want 2023", r.Year)
	}
}

func TestSetGlobals_WeekSpecified(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{DB: db, Year: 2023, Week: 3, Sport: sportFootball}
	if err := r.setGlobals(); err != nil {
		t.Fatalf("setGlobals: %v", err)
	}

	if r.Week != 3 {
		t.Errorf("Week = %d, want 3", r.Week)
	}
	// startTime should be set to the Tuesday of week 3's game
	if r.startTime.IsZero() {
		t.Error("startTime is zero")
	}
	if r.startTime.Weekday() != time.Tuesday {
		t.Errorf("startTime weekday = %v, want Tuesday", r.startTime.Weekday())
	}
}

func TestSetGlobals_WeekZero(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{DB: db, Year: 2023, Sport: sportFootball}
	if err := r.setGlobals(); err != nil {
		t.Fatalf("setGlobals: %v", err)
	}

	// The latest game is week 5, so Week should be 6 (latestGame.Week + 1)
	if r.Week != 6 {
		t.Errorf("Week = %d, want 6", r.Week)
	}
}

func TestSetGlobals_Postseason(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	// Add a postseason game
	psGame := database.Game{
		GameID:     2001,
		Season:     2023,
		Week:       1,
		Postseason: 1,
		HomeID:     1,
		AwayID:     2,
		HomeScore:  30,
		AwayScore:  20,
		StartTime:  time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&psGame).Error; err != nil {
		t.Fatalf("create postseason game: %v", err)
	}

	r := &Ranker{DB: db, Year: 2023, Sport: sportFootball}
	if err := r.setGlobals(); err != nil {
		t.Fatalf("setGlobals: %v", err)
	}

	if !r.postseason {
		t.Error("postseason = false, want true")
	}
}

func TestCreateTeamList_FBS(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{DB: db, Year: 2023, Week: 6, Sport: sportFootball}
	teamList, err := r.createTeamList(1)
	if err != nil {
		t.Fatalf("createTeamList: %v", err)
	}

	if len(teamList) != 4 {
		t.Fatalf("len(teamList) = %d, want 4", len(teamList))
	}

	expected := map[int64]struct {
		Name string
		Conf string
	}{
		1: {"Alpha", "SEC"},
		2: {"Beta", "SEC"},
		3: {"Gamma", "Big Ten"},
		4: {"Delta", "Big Ten"},
	}

	for id, want := range expected {
		team, ok := teamList[id]
		if !ok {
			t.Errorf("team %d not found", id)
			continue
		}
		if team.Name != want.Name {
			t.Errorf("team %d Name = %q, want %q", id, team.Name, want.Name)
		}
		if team.Conf != want.Conf {
			t.Errorf("team %d Conf = %q, want %q", id, team.Conf, want.Conf)
		}
	}
}

func TestCreateTeamList_FCS(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	r := &Ranker{DB: db, Year: 2023, Week: 6, Sport: sportFootball}
	teamList, err := r.createTeamList(0)
	if err != nil {
		t.Fatalf("createTeamList: %v", err)
	}

	if len(teamList) != 1 {
		t.Fatalf("len(teamList) = %d, want 1", len(teamList))
	}

	if teamList[5] == nil || teamList[5].Name != "Epsilon" {
		t.Errorf("expected Epsilon at ID 5, got %v", teamList[5])
	}
}
