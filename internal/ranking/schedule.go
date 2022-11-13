package ranking

import (
	"sort"

	"github.com/robby-barton/stats-go/internal/database"
)

type sosCalc struct {
	games  []database.Game
	oGames int64
	oWins  int64
	voWins int64
}

func (r *Ranker) recordAndSos(teamList TeamList) error {
	var games []database.Game
	if err := r.DB.Where("season = ? and start_time <= ?", r.Year, r.startTime).
		Find(&games).Error; err != nil {

		return err
	}

	var allTeams []int64
	if err := r.DB.Model(database.TeamSeason{}).Where("year = ?", r.Year).
		Pluck("team_id", &allTeams).Error; err != nil {

		return err
	}

	teamsMap := map[int64]bool{}
	for _, team := range allTeams {
		teamsMap[team] = true
	}

	teamSOS := make(map[int64]*sosCalc)
	teamRecords := make(map[int64]*Record)
	for _, game := range games {
		// create structs if they don't exist
		if _, ok := teamSOS[game.HomeId]; !ok {
			teamSOS[game.HomeId] = &sosCalc{}
		}
		if _, ok := teamSOS[game.AwayId]; !ok {
			teamSOS[game.AwayId] = &sosCalc{}
		}
		if _, ok := teamRecords[game.HomeId]; !ok {
			teamRecords[game.HomeId] = &Record{}
		}
		if _, ok := teamRecords[game.AwayId]; !ok {
			teamRecords[game.AwayId] = &Record{}
		}

		teamSOS[game.HomeId].games = append(teamSOS[game.HomeId].games, game)
		teamSOS[game.AwayId].games = append(teamSOS[game.AwayId].games, game)

		homeRecord := teamRecords[game.HomeId]
		awayRecord := teamRecords[game.AwayId]

		if game.HomeScore > game.AwayScore {
			homeRecord.Wins++
			awayRecord.Losses++
		} else if game.AwayScore > game.HomeScore {
			homeRecord.Losses++
			awayRecord.Wins++
		}
		homeRecord.Record = float64(homeRecord.Wins) /
			float64(homeRecord.Wins+homeRecord.Losses)
		awayRecord.Record = float64(awayRecord.Wins) /
			float64(awayRecord.Wins+awayRecord.Losses)
	}

	for id, team := range teamList {
		if record, ok := teamRecords[id]; ok {
			team.Record = *record
		}
	}

	for team, sos := range teamSOS {
		for _, game := range sos.games {
			var oppId int64
			var won bool
			if game.HomeId == team {
				won = game.HomeScore > game.AwayScore
				oppId = game.AwayId
			} else {
				won = game.AwayScore > game.HomeScore
				oppId = game.HomeId
			}
			if _, ok := teamsMap[oppId]; ok {
				opp := teamRecords[oppId]
				sos.oWins += opp.Wins
				sos.oGames += (opp.Wins + opp.Losses)
				if won {
					sos.voWins += opp.Wins
				}
			} else {
				sos.oGames += int64(len(sos.games))
			}
		}
	}

	for id, team := range teamList {
		sosVals, ok := teamSOS[id]
		if !ok {
			continue
		}

		var ooWins, ooGames, vooWins int64
		for _, game := range sosVals.games {
			var oppId int64
			var won bool
			if game.HomeId == id {
				won = game.HomeScore > game.AwayScore
				oppId = game.AwayId
			} else {
				won = game.AwayScore > game.HomeScore
				oppId = game.HomeId
			}
			if _, ok := teamsMap[oppId]; ok {
				oppSosVals := teamSOS[oppId]
				ooWins += oppSosVals.oWins
				ooGames += oppSosVals.oGames
				if won {
					vooWins += oppSosVals.voWins
				}
			} else {
				ooGames += int64(len(sosVals.games))
			}
		}

		if sosVals.oGames+ooGames > 0 {
			team.SOS = float64((2*sosVals.oWins)+ooWins) / float64((2*sosVals.oGames)+ooGames)
			team.SOV = float64((2*sosVals.voWins)+vooWins) / float64((2*sosVals.oGames)+ooGames)
		}
	}

	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].SOS > teamList[ids[j]].SOS
	})

	max := teamList[ids[0]].SOS
	min := teamList[ids[len(ids)-1]].SOS
	var prev float64
	var prevRank int64
	for rank, id := range ids {
		team := teamList[id]
		if team.SOS == prev {
			team.SOSRank = prevRank
		} else {
			team.SOSRank = int64(rank + 1)
			prev = team.SOS
			prevRank = team.SOSRank
		}
		if max-min != 0 {
			team.SOSNorm = (team.SOS - min) / (max - min)
		}
	}

	ids = make([]int64, 0)
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].SOV > teamList[ids[j]].SOV
	})

	max = teamList[ids[0]].SOV
	min = teamList[ids[len(ids)-1]].SOV
	prev = 0
	prevRank = 0
	for rank, id := range ids {
		team := teamList[id]
		if team.SOV == prev {
			team.SOVRank = prevRank
		} else {
			team.SOVRank = int64(rank + 1)
			prev = team.SOV
			prevRank = team.SOVRank
		}
		if max-min != 0 {
			team.SOVNorm = (team.SOV - min) / (max - min)
		}
	}

	return nil
}
