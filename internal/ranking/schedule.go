package ranking

import "sort"

func teamSos(team *Team, teamList TeamList) (int, int, int) {
	var oWins, oGames, voWins int
	for _, game := range team.Schedule {
		if opp, ok := teamList[game.Opponent]; ok {
			oWins += int(opp.Record.Wins)
			oGames += len(opp.Schedule)
			if game.Won {
				voWins += int(opp.Record.Wins)
			}
		} else {
			oGames += len(team.Schedule)
		}
	}
	return oWins, oGames, voWins
}

func sos(teamList TeamList) {
	for _, team := range teamList {
		oWins, oGames, voWins := teamSos(team, teamList)

		var ooWins, ooGames, vooWins int
		for _, game := range team.Schedule {
			if opp, ok := teamList[game.Opponent]; ok {
				wins, games, vWins := teamSos(opp, teamList)
				ooWins += wins
				ooGames += games
				vooWins += vWins
			} else {
				ooGames += len(team.Schedule)
			}
		}

		if oGames+ooGames > 0 {
			team.SOS = float64((2*oWins)+ooWins) / float64((2*oGames)+ooGames)
			team.SOV = float64((2*voWins)+vooWins) / float64((2*oGames)+ooGames)
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
	min := teamList[ids[len(ids) - 1]].SOS
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
		team.SOSNorm = (team.SOS - min) / (max - min)
	}

	ids = make([]int64, 0)
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].SOV > teamList[ids[j]].SOV
	})

	max = teamList[ids[0]].SOV
	min = teamList[ids[len(ids) - 1]].SOV
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
		team.SOVNorm = (team.SOV - min) / (max - min)
	}
}
