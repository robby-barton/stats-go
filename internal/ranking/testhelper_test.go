package ranking

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&database.Game{},
		&database.TeamSeason{},
		&database.TeamName{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func seedTestData(t *testing.T, db *gorm.DB) {
	t.Helper()

	// 5 teams: 4 FBS, 1 FCS
	teamNames := []database.TeamName{
		{TeamID: 1, Name: "Alpha", Sport: "cfb"},
		{TeamID: 2, Name: "Beta", Sport: "cfb"},
		{TeamID: 3, Name: "Gamma", Sport: "cfb"},
		{TeamID: 4, Name: "Delta", Sport: "cfb"},
		{TeamID: 5, Name: "Epsilon", Sport: "cfb"},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 1, Year: 2023, FBS: 1, Conf: "SEC", Sport: "cfb"},
		{TeamID: 2, Year: 2023, FBS: 1, Conf: "SEC", Sport: "cfb"},
		{TeamID: 3, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "cfb"},
		{TeamID: 4, Year: 2023, FBS: 1, Conf: "Big Ten", Sport: "cfb"},
		{TeamID: 5, Year: 2023, FBS: 0, Conf: "FCS", Sport: "cfb"},
		// Historical team_seasons for 2022
		{TeamID: 1, Year: 2022, FBS: 1, Conf: "SEC", Sport: "cfb"},
		{TeamID: 2, Year: 2022, FBS: 1, Conf: "SEC", Sport: "cfb"},
		{TeamID: 3, Year: 2022, FBS: 1, Conf: "Big Ten", Sport: "cfb"},
		{TeamID: 4, Year: 2022, FBS: 1, Conf: "Big Ten", Sport: "cfb"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed team_seasons: %v", err)
	}

	// Base time: Tuesday of week 1, 2023 season
	base := time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour

	games := []database.Game{
		// 2023 season games
		{
			GameID: 1001, Season: 2023, Week: 1, HomeID: 1, AwayID: 2,
			HomeScore: 28, AwayScore: 14, ConfGame: true, Sport: "cfb", StartTime: base,
		},
		{
			GameID: 1002, Season: 2023, Week: 1, HomeID: 3, AwayID: 4,
			HomeScore: 21, AwayScore: 10, ConfGame: true, Sport: "cfb", StartTime: base.Add(time.Hour),
		},
		{
			GameID: 1003, Season: 2023, Week: 2, HomeID: 1, AwayID: 3,
			HomeScore: 35, AwayScore: 17, Sport: "cfb", StartTime: base.Add(week),
		},
		{
			GameID: 1004, Season: 2023, Week: 2, HomeID: 2, AwayID: 4,
			HomeScore: 24, AwayScore: 21, ConfGame: true, Sport: "cfb", StartTime: base.Add(week + time.Hour),
		},
		{
			GameID: 1005, Season: 2023, Week: 3, HomeID: 1, AwayID: 4,
			HomeScore: 42, AwayScore: 7, Sport: "cfb", StartTime: base.Add(2 * week),
		},
		{
			GameID: 1006, Season: 2023, Week: 3, HomeID: 2, AwayID: 3,
			HomeScore: 17, AwayScore: 14, Sport: "cfb", StartTime: base.Add(2*week + time.Hour),
		},
		{
			GameID: 1007, Season: 2023, Week: 4, HomeID: 3, AwayID: 2,
			HomeScore: 28, AwayScore: 21, Sport: "cfb", StartTime: base.Add(3 * week),
		},
		{
			GameID: 1008, Season: 2023, Week: 4, HomeID: 1, AwayID: 5,
			HomeScore: 31, AwayScore: 10, Sport: "cfb", StartTime: base.Add(3*week + time.Hour),
		},
		{
			GameID: 1009, Season: 2023, Week: 5, HomeID: 4, AwayID: 3,
			HomeScore: 14, AwayScore: 14, Sport: "cfb", StartTime: base.Add(4 * week),
		},
		{
			GameID: 1010, Season: 2023, Week: 5, HomeID: 2, AwayID: 5,
			HomeScore: 35, AwayScore: 7, Sport: "cfb", StartTime: base.Add(4*week + time.Hour),
		},
		// 2022 historical games
		{
			GameID: 901, Season: 2022, Week: 1, HomeID: 1, AwayID: 2,
			HomeScore: 24, AwayScore: 17, Sport: "cfb",
			StartTime: time.Date(2022, 9, 6, 19, 0, 0, 0, time.UTC),
		},
		{
			GameID: 902, Season: 2022, Week: 2, HomeID: 3, AwayID: 4,
			HomeScore: 20, AwayScore: 13, Sport: "cfb",
			StartTime: time.Date(2022, 9, 13, 19, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed games: %v", err)
	}
}
