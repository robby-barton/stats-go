package ranking

import (
	"github.com/robby-barton/stats-go/internal/sport"
)

// setup derives the ranker's window state from the input and builds the
// initial team list for the division being ranked. The input itself is never
// mutated.
func (r *Ranker) setup() TeamList {
	r.startTime = r.in.StartTime
	r.postseason = r.in.Postseason

	teamList := TeamList{}
	for _, team := range r.divisionTeams() {
		entry := &Team{
			Name: team.Name,
			Conf: team.Conf,
			Year: r.in.Year,
			Week: r.in.Week,
		}
		if r.postseason {
			entry.Postseason = 1
		}
		teamList[team.ID] = entry
	}

	return teamList
}

// divisionTeams filters the input's team set down to the division being
// ranked. FCS ranking only applies to football; basketball has no division
// split, so every team is FBS.
func (r *Ranker) divisionTeams() []TeamInfo {
	includeFBS := !r.in.Fcs || r.in.Sport == sport.Basketball

	var teams []TeamInfo
	for _, team := range r.in.Teams {
		if team.FBS == includeFBS {
			teams = append(teams, team)
		}
	}
	return teams
}
