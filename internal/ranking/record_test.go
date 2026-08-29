package ranking

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/sport"
)

// allGamesExceptWeeks returns the football fixture games, optionally limited
// to a subset of weeks (mirroring the StartTime filter the load package
// applies when resolving the ranking window).
func gamesThroughWeek(weeks int64) []Game {
	var games []Game
	for _, g := range footballGames() {
		if g.Season == 2023 && g.Week > weeks {
			continue
		}
		games = append(games, g)
	}
	// Descending start-time order, as the loader provides.
	for i, j := 0, len(games)-1; i < j; i, j = i+1, j-1 {
		games[i], games[j] = games[j], games[i]
	}
	return games
}

func TestRecord_BasicRecords(t *testing.T) {
	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: gamesThroughWeek(5), // all games
	})

	teamList := fbsTeamList()

	r.record(teamList)

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
	// Window through week 2 only (before week 3 games)
	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: gamesThroughWeek(2),
	})

	teamList := fbsTeamList()

	r.record(teamList)

	// After weeks 1-2 only:
	// Alpha: 2W (1001,1003)
	assertRecord(t, "Alpha", teamList[1], 2, 0, 0, 3.0/4.0)

	// Delta: 0W-2L (1002,1004)
	assertRecord(t, "Delta", teamList[4], 0, 2, 0, 1.0/4.0)
}

func TestRecord_TieHandling(t *testing.T) {
	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: gamesThroughWeek(5),
	})

	teamList := TeamList{
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}

	r.record(teamList)

	if teamList[3].Record.Ties != 1 {
		t.Errorf("Gamma ties = %d, want 1", teamList[3].Record.Ties)
	}
	if teamList[4].Record.Ties != 1 {
		t.Errorf("Delta ties = %d, want 1", teamList[4].Record.Ties)
	}
}

// TestRecord_IgnoreOtherSeasons verifies that record only counts games from
// the input's season, even when older games are loaded in the window.
func TestRecord_IgnoreOtherSeasons(t *testing.T) {
	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: gamesThroughWeek(5), // includes the two 2022 games
	})

	teamList := fbsTeamList()
	r.record(teamList)

	// Alpha played a 2022 game (901); it must not affect the 2023 record.
	assertRecord(t, "Alpha", teamList[1], 4, 0, 0, 5.0/6.0)
	assertRecord(t, "Beta", teamList[2], 3, 2, 0, 4.0/7.0)
}
