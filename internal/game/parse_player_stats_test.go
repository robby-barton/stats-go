package game

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func TestCreateStatMaps_WithTotalsAndAthletes(t *testing.T) {
	stats := espn.PlayerStatistics{
		Labels: []string{"C/ATT", "YDS", "TD", "INT"},
		Totals: []string{"20/30", "250", "3", "1"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 100},
				Stats:   []string{"15/22", "180", "2", "0"},
			},
			{
				Athlete: espn.Athlete{ID: 200},
				Stats:   []string{"5/8", "70", "1", "1"},
			},
		},
	}

	result := createStatMaps(stats)

	if len(result) != 3 {
		t.Fatalf("expected 3 stat maps (1 totals + 2 athletes), got %d", len(result))
	}

	// Totals row should have playerID = -1
	totals := result[0]
	if totals[playerID] != int64(-1) {
		t.Errorf("expected totals playerID = -1, got %v", totals[playerID])
	}
	if totals["C/ATT"] != "20/30" {
		t.Errorf("expected totals C/ATT = '20/30', got %v", totals["C/ATT"])
	}
	if totals["YDS"] != "250" {
		t.Errorf("expected totals YDS = '250', got %v", totals["YDS"])
	}

	// First athlete
	p1 := result[1]
	if p1[playerID] != int64(100) {
		t.Errorf("expected first athlete playerID = 100, got %v", p1[playerID])
	}
	if p1["C/ATT"] != "15/22" {
		t.Errorf("expected first athlete C/ATT = '15/22', got %v", p1["C/ATT"])
	}

	// Second athlete
	p2 := result[2]
	if p2[playerID] != int64(200) {
		t.Errorf("expected second athlete playerID = 200, got %v", p2[playerID])
	}
}

func TestCreateStatMaps_NoTotals(t *testing.T) {
	stats := espn.PlayerStatistics{
		Labels: []string{"CAR", "YDS"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 50},
				Stats:   []string{"10", "80"},
			},
		},
	}

	result := createStatMaps(stats)

	if len(result) != 1 {
		t.Fatalf("expected 1 stat map (no totals), got %d", len(result))
	}
	if result[0][playerID] != int64(50) {
		t.Errorf("expected playerID = 50, got %v", result[0][playerID])
	}
}

func TestCreateStatMaps_Empty(t *testing.T) {
	stats := espn.PlayerStatistics{
		Labels: []string{"CAR", "YDS"},
	}

	result := createStatMaps(stats)

	if len(result) != 0 {
		t.Fatalf("expected 0 stat maps for empty input, got %d", len(result))
	}
}

func TestParsePassingStats(t *testing.T) {
	tests := []struct {
		name     string
		input    espn.PlayerStatistics
		expected []database.PassingStats
	}{
		{
			name: "single player",
			input: espn.PlayerStatistics{
				Labels: []string{"C/ATT", "YDS", "TD", "INT"},
				Athletes: []espn.AthleteStats{
					{
						Athlete: espn.Athlete{ID: 42},
						Stats:   []string{"18/25", "210", "2", "1"},
					},
				},
			},
			expected: []database.PassingStats{
				{
					PlayerID:      42,
					TeamID:        10,
					GameID:        1001,
					Completions:   18,
					Attempts:      25,
					Yards:         210,
					Touchdowns:    2,
					Interceptions: 1,
				},
			},
		},
		{
			name: "with totals row",
			input: espn.PlayerStatistics{
				Labels: []string{"C/ATT", "YDS", "TD", "INT"},
				Totals: []string{"20/30", "250", "3", "1"},
				Athletes: []espn.AthleteStats{
					{
						Athlete: espn.Athlete{ID: 42},
						Stats:   []string{"20/30", "250", "3", "1"},
					},
				},
			},
			expected: []database.PassingStats{
				{
					PlayerID:      -1,
					TeamID:        10,
					GameID:        1001,
					Completions:   20,
					Attempts:      30,
					Yards:         250,
					Touchdowns:    3,
					Interceptions: 1,
				},
				{
					PlayerID:      42,
					TeamID:        10,
					GameID:        1001,
					Completions:   20,
					Attempts:      30,
					Yards:         250,
					Touchdowns:    3,
					Interceptions: 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePassingStats(1001, 10, tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d results, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				got := result[i]
				if got != exp {
					t.Errorf("result[%d] = %+v, expected %+v", i, got, exp)
				}
			}
		})
	}
}

func TestParseRushingStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"CAR", "YDS", "TD", "LONG"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 55},
				Stats:   []string{"15", "98", "1", "32"},
			},
		},
	}

	result := parseRushingStats(2001, 20, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	expected := database.RushingStats{
		PlayerID:   55,
		TeamID:     20,
		GameID:     2001,
		Carries:    15,
		RushYards:  98,
		Touchdowns: 1,
		RushLong:   32,
	}
	if result[0] != expected {
		t.Errorf("got %+v, expected %+v", result[0], expected)
	}
}

func TestParseReceivingStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"REC", "YDS", "TD", "LONG"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 77},
				Stats:   []string{"6", "112", "1", "45"},
			},
		},
	}

	result := parseReceivingStats(3001, 30, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	expected := database.ReceivingStats{
		PlayerID:   77,
		TeamID:     30,
		GameID:     3001,
		Receptions: 6,
		RecYards:   112,
		Touchdowns: 1,
		RecLong:    45,
	}
	if result[0] != expected {
		t.Errorf("got %+v, expected %+v", result[0], expected)
	}
}

func TestParseFumbleStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"FUM", "LOST", "REC"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 33},
				Stats:   []string{"2", "1", "0"},
			},
		},
	}

	result := parseFumbleStats(4001, 40, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	expected := database.FumbleStats{
		PlayerID:    33,
		TeamID:      40,
		GameID:      4001,
		Fumbles:     2,
		FumblesLost: 1,
		FumblesRec:  0,
	}
	if result[0] != expected {
		t.Errorf("got %+v, expected %+v", result[0], expected)
	}
}

func TestParseDefensiveStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"TOT", "SOLO", "SACKS", "TFL", "PD", "QB HUR", "TD"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 88},
				Stats:   []string{"8.0", "5", "1.5", "2.0", "1", "3", "0"},
			},
		},
	}

	result := parseDefensiveStats(5001, 50, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	got := result[0]
	if got.PlayerID != 88 {
		t.Errorf("PlayerID = %d, want 88", got.PlayerID)
	}
	if got.TeamID != 50 {
		t.Errorf("TeamID = %d, want 50", got.TeamID)
	}
	if got.GameID != 5001 {
		t.Errorf("GameID = %d, want 5001", got.GameID)
	}
	if got.TotalTackles != 8.0 {
		t.Errorf("TotalTackles = %f, want 8.0", got.TotalTackles)
	}
	if got.SoloTackles != 5 {
		t.Errorf("SoloTackles = %d, want 5", got.SoloTackles)
	}
	if got.Sacks != 1.5 {
		t.Errorf("Sacks = %f, want 1.5", got.Sacks)
	}
	if got.TacklesForLoss != 2.0 {
		t.Errorf("TacklesForLoss = %f, want 2.0", got.TacklesForLoss)
	}
	if got.PassesDef != 1 {
		t.Errorf("PassesDef = %d, want 1", got.PassesDef)
	}
	if got.QBHurries != 3 {
		t.Errorf("QBHurries = %d, want 3", got.QBHurries)
	}
	if got.Touchdowns != 0 {
		t.Errorf("Touchdowns = %d, want 0", got.Touchdowns)
	}
}

func TestParseInterceptionStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"INT", "YDS", "TD"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 44},
				Stats:   []string{"2", "35", "1"},
			},
		},
	}

	result := parseInterceptionStats(6001, 60, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	expected := database.InterceptionStats{
		PlayerID:      44,
		TeamID:        60,
		GameID:        6001,
		Interceptions: 2,
		IntYards:      35,
		Touchdowns:    1,
	}
	if result[0] != expected {
		t.Errorf("got %+v, expected %+v", result[0], expected)
	}
}

func TestParseReturnStats(t *testing.T) {
	tests := []struct {
		name       string
		returnType string
	}{
		{name: "kick returns", returnType: "kick"},
		{name: "punt returns", returnType: "punt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := espn.PlayerStatistics{
				Labels: []string{"NO", "YDS", "LONG", "TD"},
				Athletes: []espn.AthleteStats{
					{
						Athlete: espn.Athlete{ID: 99},
						Stats:   []string{"3", "75", "40", "1"},
					},
				},
			}

			result := parseReturnStats(7001, 70, input, tt.returnType)

			if len(result) != 1 {
				t.Fatalf("expected 1 result, got %d", len(result))
			}

			expected := database.ReturnStats{
				PlayerID:   99,
				TeamID:     70,
				GameID:     7001,
				PuntKick:   tt.returnType,
				ReturnNo:   3,
				RetYards:   75,
				RetLong:    40,
				Touchdowns: 1,
			}
			if result[0] != expected {
				t.Errorf("got %+v, expected %+v", result[0], expected)
			}
		})
	}
}

func TestParseKickStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"FG", "LONG", "XP", "PTS"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 11},
				Stats:   []string{"2/3", "47", "4/4", "10"},
			},
		},
	}

	result := parseKickStats(8001, 80, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	expected := database.KickStats{
		PlayerID: 11,
		TeamID:   80,
		GameID:   8001,
		FGM:      2,
		FGA:      3,
		FGLong:   47,
		XPM:      4,
		XPA:      4,
		Points:   10,
	}
	if result[0] != expected {
		t.Errorf("got %+v, expected %+v", result[0], expected)
	}
}

func TestParsePuntStats(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"NO", "YDS", "TB", "In 20", "LONG"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 22},
				Stats:   []string{"5", "210", "1", "2", "55"},
			},
		},
	}

	result := parsePuntStats(9001, 90, input)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	expected := database.PuntStats{
		PlayerID:   22,
		TeamID:     90,
		GameID:     9001,
		PuntNo:     5,
		PuntYards:  210,
		Touchbacks: 1,
		Inside20:   2,
		PuntLong:   55,
	}
	if result[0] != expected {
		t.Errorf("got %+v, expected %+v", result[0], expected)
	}
}

func TestParsePassingStats_Empty(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"C/ATT", "YDS", "TD", "INT"},
	}

	result := parsePassingStats(1001, 10, input)

	if len(result) != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", len(result))
	}
}

func TestParseRushingStats_WithTotals(t *testing.T) {
	input := espn.PlayerStatistics{
		Labels: []string{"CAR", "YDS", "TD", "LONG"},
		Totals: []string{"30", "150", "2", "45"},
		Athletes: []espn.AthleteStats{
			{
				Athlete: espn.Athlete{ID: 55},
				Stats:   []string{"20", "100", "1", "45"},
			},
			{
				Athlete: espn.Athlete{ID: 66},
				Stats:   []string{"10", "50", "1", "20"},
			},
		},
	}

	result := parseRushingStats(2001, 20, input)

	if len(result) != 3 {
		t.Fatalf("expected 3 results (1 totals + 2 athletes), got %d", len(result))
	}

	// Totals row
	if result[0].PlayerID != -1 {
		t.Errorf("totals row PlayerID = %d, want -1", result[0].PlayerID)
	}
	if result[0].Carries != 30 {
		t.Errorf("totals Carries = %d, want 30", result[0].Carries)
	}
	if result[0].RushYards != 150 {
		t.Errorf("totals RushYards = %d, want 150", result[0].RushYards)
	}
}
