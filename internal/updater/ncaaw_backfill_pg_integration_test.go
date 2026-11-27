//go:build integration

package updater

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/sport"
)

func TestIntegration_NCAAWBackfillOnPostgres(t *testing.T) {
	db := openUpdaterIntegrationDB(t)
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	u := ncaawBackfillFixtureUpdater(t, db, defaultNCAAWBFFixture())

	men := database.TeamWeekResult{TeamID: 11, Year: 2027, Week: 1, Sport: sport.Basketball, Name: "Men", Fbs: true}
	if err := db.Create(&men).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := u.BackfillNCAAW(t.Context(), 2026, 2027, 0); err != nil {
			t.Fatal(err)
		}
		if rows := ncaawBFRankings(t, db, "sport = ? and year = ?", sport.WomensBasketball, 2027); len(rows) != 6 {
			t.Fatalf("published %d rows for 2027, want 6", len(rows))
		}
		if rows := ncaawBFRankings(t, db, "sport = ? and year = ?", sport.WomensBasketball, 2026); len(rows) != 0 {
			t.Fatalf("lookback unexpectedly published %d rows for 2026", len(rows))
		}
		if rows := ncaawBFRankings(t, db, "sport = ?", sport.Basketball); len(rows) != 1 || rows[0] != men {
			t.Fatalf("men's ranking changed: %v", rows)
		}
	}
	if err := u.BackfillNCAAW(t.Context(), 2025, 2026, 2026); err != nil {
		t.Fatal(err)
	}
	final := ncaawBFRankings(t, db, "sport = ? and year = ? and postseason = ?", sport.WomensBasketball, 2026, 1)
	if len(final) != 3 {
		t.Fatalf("published %d Final rows for 2026, want 3", len(final))
	}
	for _, row := range final {
		if row.Week != 0 {
			t.Errorf("Final row week = %d, want 0", row.Week)
		}
	}
}
