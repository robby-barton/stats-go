package ranking

import (
	"math"
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

// TestRecord_NoGames verifies that a team with no current-season games gets
// the add-one smoothing baseline Record = (1+0)/(2+0) = 0.5 while Wins/
// Losses/Ties stay zero, rather than a zero Record that would zero out its
// composite contribution.
func TestRecord_NoGames(t *testing.T) {
	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: nil,
	})

	teamList := fbsTeamList()
	r.record(teamList)

	for _, team := range teamList {
		assertRecord(t, team.Name, team, 0, 0, 0, 0.5)
	}
}

// TestRecord_PriorSeasonOnly verifies that when only prior-season games are
// loaded (no current-season games yet), every team is treated as having no
// current-season record: 0-0-0 with the 0.5 smoothing baseline. The 2022
// games must not leak into the current-season Record.
func TestRecord_PriorSeasonOnly(t *testing.T) {
	var games []Game
	for _, g := range footballGames() {
		if g.Season == 2022 {
			games = append(games, g)
		}
	}
	if len(games) == 0 {
		t.Fatal("fixture has no 2022 games")
	}

	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: games,
	})

	teamList := fbsTeamList()
	r.record(teamList)

	// Alpha and Beta appear in the 2022 game 901; Gamma and Delta in 902.
	// None of it counts toward the 2023 record.
	assertRecord(t, "Alpha", teamList[1], 0, 0, 0, 0.5)
	assertRecord(t, "Beta", teamList[2], 0, 0, 0, 0.5)
	assertRecord(t, "Gamma", teamList[3], 0, 0, 0, 0.5)
	assertRecord(t, "Delta", teamList[4], 0, 0, 0, 0.5)
}

// TestRecord_OpeningBye verifies the opening-bye mixture: some teams have
// played week-1 games while an idle team (bye) remains 0-0 with the 0.5
// smoothing baseline. Played teams keep the existing add-one arithmetic.
func TestRecord_OpeningBye(t *testing.T) {
	// Week 1 only: Gamma 21, Delta 10 (game 1002). Alpha and Beta are idle.
	r := newTestRanker(t, Input{
		Year:  2023,
		Sport: sport.Football,
		Teams: footballTeams(),
		Games: []Game{footballGames()[1]}, // game 1002
	})

	teamList := fbsTeamList()
	r.record(teamList)

	// Idle teams: zero W/L/T with the neutral 0.5 baseline.
	assertRecord(t, "Alpha", teamList[1], 0, 0, 0, 0.5)
	assertRecord(t, "Beta", teamList[2], 0, 0, 0, 0.5)
	// Played teams: (1+1)/(2+1) = 2/3 and (1+0)/(2+1) = 1/3, unchanged.
	assertRecord(t, "Gamma", teamList[3], 1, 0, 0, 2.0/3.0)
	assertRecord(t, "Delta", teamList[4], 0, 1, 0, 1.0/3.0)
}

// TestRecord_OpeningBye_Basketball verifies the same neutral 0.5 baseline
// for basketball, which shares record() with football.
func TestRecord_OpeningBye_Basketball(t *testing.T) {
	// Week 1 only: Hoops A 78, Hoops B 65 (game 3001). C/D/E are idle.
	r := newTestRanker(t, Input{
		Year:  2024,
		Sport: sport.Basketball,
		Teams: basketballTeams(),
		Games: []Game{basketballGames()[0]}, // game 3001
	})

	teamList := TeamList{
		101: &Team{Name: "Hoops A"},
		102: &Team{Name: "Hoops B"},
		103: &Team{Name: "Hoops C"},
		104: &Team{Name: "Hoops D"},
		105: &Team{Name: "Hoops E"},
	}
	r.record(teamList)

	assertRecord(t, "Hoops C", teamList[103], 0, 0, 0, 0.5)
	assertRecord(t, "Hoops D", teamList[104], 0, 0, 0, 0.5)
	assertRecord(t, "Hoops E", teamList[105], 0, 0, 0, 0.5)
	assertRecord(t, "Hoops A", teamList[101], 1, 0, 0, 2.0/3.0)
	assertRecord(t, "Hoops B", teamList[102], 0, 1, 0, 1.0/3.0)
}

// TestCalculateRanking_OpeningBye_IdleTeamComposite runs the full pipeline
// with one week-1 game and asserts that an idle football team contributes
// 0.45*0.5 = 0.225 to its FinalRaw (rather than zero) without inventing
// games for it.
func TestCalculateRanking_OpeningBye_IdleTeamComposite(t *testing.T) {
	r := newTestRanker(t, Input{
		Year:  2023,
		Week:  2,
		Sport: sport.Football,
		Teams: footballTeams(),
		// Only game 1001: Alpha 28, Beta 14. Gamma and Delta are idle (bye).
		Games: []Game{footballGames()[0]},
	})

	teamList, err := r.CalculateRanking()
	if err != nil {
		t.Fatalf("CalculateRanking: %v", err)
	}

	// Idle team keeps the neutral baseline record.
	assertRecord(t, "Gamma", teamList[3], 0, 0, 0, 0.5)

	// FinalRaw = Record.Record*0.45 + SRSNorm*0.40 + SOSNorm*0.15, so the
	// idle team's record term must be exactly 0.45*0.5 = 0.225 — not zero.
	for _, id := range []int64{3, 4} {
		team := teamList[id]
		want := 0.45*0.5 + 0.40*team.SRSNorm + 0.15*team.SOSNorm
		if math.Abs(team.FinalRaw-want) > 0.001 {
			t.Errorf("idle team %d FinalRaw = %f, want %f (0.225 record term)",
				id, team.FinalRaw, want)
		}
		if team.FinalRaw <= 0.2 {
			t.Errorf("idle team %d FinalRaw = %f, want > 0.2 (record must contribute)",
				id, team.FinalRaw)
		}
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
