package game

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func TestParseTeamStats_AllFields(t *testing.T) {
	stats := []espn.TeamStatistics{
		{Name: "firstDowns", DisplayValue: "22"},
		{Name: "thirdDownEff", DisplayValue: "5-12"},
		{Name: "fourthDownEff", DisplayValue: "1-2"},
		{Name: "netPassingYards", DisplayValue: "285"},
		{Name: "completionAttempts", DisplayValue: "22/35"},
		{Name: "rushingYards", DisplayValue: "150"},
		{Name: "rushingAttempts", DisplayValue: "35"},
		{Name: "totalPenaltiesYards", DisplayValue: "7-65"},
		{Name: "fumblesLost", DisplayValue: "1"},
		{Name: "interceptions", DisplayValue: "2"},
		{Name: "possessionTime", DisplayValue: "32:15"},
	}

	var tgs database.TeamGameStats
	parseTeamStats(stats, &tgs)

	if tgs.FirstDowns != 22 {
		t.Errorf("FirstDowns = %d, want 22", tgs.FirstDowns)
	}
	if tgs.ThirdDownsConv != 5 {
		t.Errorf("ThirdDownsConv = %d, want 5", tgs.ThirdDownsConv)
	}
	if tgs.ThirdDowns != 12 {
		t.Errorf("ThirdDowns = %d, want 12", tgs.ThirdDowns)
	}
	if tgs.FourthDownsConv != 1 {
		t.Errorf("FourthDownsConv = %d, want 1", tgs.FourthDownsConv)
	}
	if tgs.FourthDowns != 2 {
		t.Errorf("FourthDowns = %d, want 2", tgs.FourthDowns)
	}
	if tgs.PassYards != 285 {
		t.Errorf("PassYards = %d, want 285", tgs.PassYards)
	}
	if tgs.Completions != 22 {
		t.Errorf("Completions = %d, want 22", tgs.Completions)
	}
	if tgs.CompletionAttempts != 35 {
		t.Errorf("CompletionAttempts = %d, want 35", tgs.CompletionAttempts)
	}
	if tgs.RushYards != 150 {
		t.Errorf("RushYards = %d, want 150", tgs.RushYards)
	}
	if tgs.RushAttempts != 35 {
		t.Errorf("RushAttempts = %d, want 35", tgs.RushAttempts)
	}
	if tgs.Penalties != 7 {
		t.Errorf("Penalties = %d, want 7", tgs.Penalties)
	}
	if tgs.PenaltyYards != 65 {
		t.Errorf("PenaltyYards = %d, want 65", tgs.PenaltyYards)
	}
	if tgs.Fumbles != 1 {
		t.Errorf("Fumbles = %d, want 1", tgs.Fumbles)
	}
	if tgs.Interceptions != 2 {
		t.Errorf("Interceptions = %d, want 2", tgs.Interceptions)
	}
	// 32:15 = 32*60+15 = 1935 seconds
	if tgs.Possession != 1935 {
		t.Errorf("Possession = %d, want 1935", tgs.Possession)
	}
}

func TestParseTeamStats_ExtraDashes(t *testing.T) {
	// ESPN occasionally puts extra dashes: "3--10" instead of "3-10"
	stats := []espn.TeamStatistics{
		{Name: "thirdDownEff", DisplayValue: "3--10"},
		{Name: "fourthDownEff", DisplayValue: "1---3"},
		{Name: "totalPenaltiesYards", DisplayValue: "5--50"},
		{Name: "completionAttempts", DisplayValue: "15--30"},
	}

	var tgs database.TeamGameStats
	parseTeamStats(stats, &tgs)

	if tgs.ThirdDownsConv != 3 {
		t.Errorf("ThirdDownsConv = %d, want 3", tgs.ThirdDownsConv)
	}
	if tgs.ThirdDowns != 10 {
		t.Errorf("ThirdDowns = %d, want 10", tgs.ThirdDowns)
	}
	if tgs.FourthDownsConv != 1 {
		t.Errorf("FourthDownsConv = %d, want 1", tgs.FourthDownsConv)
	}
	if tgs.FourthDowns != 3 {
		t.Errorf("FourthDowns = %d, want 3", tgs.FourthDowns)
	}
	if tgs.Penalties != 5 {
		t.Errorf("Penalties = %d, want 5", tgs.Penalties)
	}
	if tgs.PenaltyYards != 50 {
		t.Errorf("PenaltyYards = %d, want 50", tgs.PenaltyYards)
	}
	if tgs.Completions != 15 {
		t.Errorf("Completions = %d, want 15", tgs.Completions)
	}
	if tgs.CompletionAttempts != 30 {
		t.Errorf("CompletionAttempts = %d, want 30", tgs.CompletionAttempts)
	}
}

func TestParseTeamStats_IgnoredStats(t *testing.T) {
	stats := []espn.TeamStatistics{
		{Name: "totalYards", DisplayValue: "435"},
		{Name: "yardsPerPass", DisplayValue: "8.1"},
		{Name: "yardsPerRushAttempt", DisplayValue: "4.3"},
		{Name: "turnovers", DisplayValue: "3"},
		{Name: "firstDowns", DisplayValue: "20"},
	}

	var tgs database.TeamGameStats
	parseTeamStats(stats, &tgs)

	// Only firstDowns should be parsed; the rest are ignored
	if tgs.FirstDowns != 20 {
		t.Errorf("FirstDowns = %d, want 20", tgs.FirstDowns)
	}
}
