package ranking

import (
	"testing"
	"time"
)

// footballTeams returns the team set used by the football fixtures: 4 FBS
// teams and 1 FCS team. It mirrors the team_names ⋈ team_seasons join the
// load package performs (the ranking package itself never touches the
// database).
func footballTeams() []TeamInfo {
	return []TeamInfo{
		{ID: 1, Name: "Alpha", Conf: "SEC", FBS: true},
		{ID: 2, Name: "Beta", Conf: "SEC", FBS: true},
		{ID: 3, Name: "Gamma", Conf: "Big Ten", FBS: true},
		{ID: 4, Name: "Delta", Conf: "Big Ten", FBS: true},
		{ID: 5, Name: "Epsilon", Conf: "FCS", FBS: false},
	}
}

// basketballTeams returns the team set used by the basketball fixtures (all
// top-division; basketball has no FBS/FCS split).
func basketballTeams() []TeamInfo {
	return []TeamInfo{
		{ID: 101, Name: "Hoops A", Conf: "Big East", FBS: true},
		{ID: 102, Name: "Hoops B", Conf: "Big East", FBS: true},
		{ID: 103, Name: "Hoops C", Conf: "ACC", FBS: true},
		{ID: 104, Name: "Hoops D", Conf: "ACC", FBS: true},
		{ID: 105, Name: "Hoops E", Conf: "Big 12", FBS: true},
	}
}

// footballGames returns the 2023 season game set used by the football
// fixtures, plus two 2022 historical games for the srs back-season window.
// Games are listed in ascending start-time order; the loader hands them to
// the ranking in descending order, which the fixtures replicate by
// reversing this slice when a strict order matters.
func footballGames() []Game {
	base := time.Date(2023, 9, 5, 19, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour

	return []Game{
		// 2023 season games
		{GameID: 1001, Season: 2023, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 28, AwayScore: 14, StartTime: base},
		{GameID: 1002, Season: 2023, Week: 1, HomeID: 3, AwayID: 4,
			HomeScore: 21, AwayScore: 10, StartTime: base.Add(time.Hour)},
		{GameID: 1003, Season: 2023, Week: 2, HomeID: 1, AwayID: 3,
			HomeScore: 35, AwayScore: 17, StartTime: base.Add(week)},
		{GameID: 1004, Season: 2023, Week: 2, HomeID: 2, AwayID: 4,
			HomeScore: 24, AwayScore: 21, StartTime: base.Add(week + time.Hour)},
		{GameID: 1005, Season: 2023, Week: 3, HomeID: 1, AwayID: 4,
			HomeScore: 42, AwayScore: 7, StartTime: base.Add(2 * week)},
		{GameID: 1006, Season: 2023, Week: 3, HomeID: 2, AwayID: 3,
			HomeScore: 17, AwayScore: 14, StartTime: base.Add(2*week + time.Hour)},
		{GameID: 1007, Season: 2023, Week: 4, HomeID: 3, AwayID: 2,
			HomeScore: 28, AwayScore: 21, StartTime: base.Add(3 * week)},
		{GameID: 1008, Season: 2023, Week: 4, HomeID: 1, AwayID: 5,
			HomeScore: 31, AwayScore: 10, StartTime: base.Add(3*week + time.Hour)},
		{GameID: 1009, Season: 2023, Week: 5, HomeID: 4, AwayID: 3,
			HomeScore: 14, AwayScore: 14, StartTime: base.Add(4 * week)},
		{GameID: 1010, Season: 2023, Week: 5, HomeID: 2, AwayID: 5,
			HomeScore: 35, AwayScore: 7, StartTime: base.Add(4*week + time.Hour)},
		// 2022 historical games
		{GameID: 901, Season: 2022, Week: 1, HomeID: 1, AwayID: 2, HomeScore: 24, AwayScore: 17,
			StartTime: time.Date(2022, 9, 6, 19, 0, 0, 0, time.UTC)},
		{GameID: 902, Season: 2022, Week: 2, HomeID: 3, AwayID: 4, HomeScore: 20, AwayScore: 13,
			StartTime: time.Date(2022, 9, 13, 19, 0, 0, 0, time.UTC)},
	}
}

// basketballGames returns the 2024 basketball game set used by the basketball
// fixtures (6 games across 2 weeks).
func basketballGames() []Game {
	base := time.Date(2024, 1, 2, 19, 0, 0, 0, time.UTC) // Tuesday
	week := 7 * 24 * time.Hour

	return []Game{
		// Week 1
		{GameID: 3001, Season: 2024, Week: 1, HomeID: 101, AwayID: 102,
			HomeScore: 78, AwayScore: 65, StartTime: base},
		{GameID: 3002, Season: 2024, Week: 1, HomeID: 103, AwayID: 104,
			HomeScore: 70, AwayScore: 68, StartTime: base.Add(time.Hour)},
		{GameID: 3003, Season: 2024, Week: 1, HomeID: 105, AwayID: 101,
			HomeScore: 60, AwayScore: 72, StartTime: base.Add(2 * time.Hour)},
		// Week 2
		{GameID: 3004, Season: 2024, Week: 2, HomeID: 101, AwayID: 103,
			HomeScore: 80, AwayScore: 75, StartTime: base.Add(week)},
		{GameID: 3005, Season: 2024, Week: 2, HomeID: 102, AwayID: 104,
			HomeScore: 66, AwayScore: 64, StartTime: base.Add(week + time.Hour)},
		{GameID: 3006, Season: 2024, Week: 2, HomeID: 105, AwayID: 103,
			HomeScore: 55, AwayScore: 55, StartTime: base.Add(week + 2*time.Hour)},
	}
}

// newTestRanker builds a Ranker via the validated constructor.
func newTestRanker(t *testing.T, in Input) *Ranker {
	t.Helper()

	r, err := NewRanker(in)
	if err != nil {
		t.Fatalf("NewRanker: %v", err)
	}
	return r
}

// fbsTeamList builds the division team list for the FBS teams of the football
// fixture (mirrors what divisionTeams produces for Fcs=false).
func fbsTeamList() TeamList {
	return TeamList{
		1: &Team{Name: "Alpha"},
		2: &Team{Name: "Beta"},
		3: &Team{Name: "Gamma"},
		4: &Team{Name: "Delta"},
	}
}

// assertRecord checks a team's won-loss-tie record.
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
	if team.Record.Record-record > 0.001 || record-team.Record.Record > 0.001 {
		t.Errorf("%s Record = %f, want %f", name, team.Record.Record, record)
	}
}
