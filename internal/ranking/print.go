package ranking

import (
	"fmt"
	"sort"
)

func (r *Ranker) PrintRankings(teamList TeamList, top int) {
	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].FinalRank < teamList[ids[j]].FinalRank
	})

	if r.postseason {
		fmt.Printf("%d Final\n", r.Year)
	} else {
		fmt.Printf("%d Week %d\n", r.Year, r.Week)
	}
	fmt.Printf("Games up to %v\n", r.startTime)
	fmt.Printf("%-5s %-25s %-7s %-8s %-5s %-5s %-5s %-5s %7s\n",
		"Rank", "Team", "Conf", "Record", "SRS", "SoS", "SoV", "SoL", "Total")
	for i := 0; i < top; i++ {
		team := teamList[ids[i]]
		fmt.Printf("%-5d %-25s %-7s %-8s %-5d %-5d %-5d %-5d %.5f\n",
			team.FinalRank, team.Name, team.Conf, team.Record, team.SRSRank,
			team.SOSRank, team.SOVRank, team.SOLRank, team.FinalRaw)
	}
}

func (r *Ranker) PrintSRS(teamList TeamList, top int) {
	var ids []int64
	for id := range teamList {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return teamList[ids[i]].SRSRank < teamList[ids[j]].SRSRank
	})

	if r.postseason {
		fmt.Printf("%d Final\n", r.Year)
	} else {
		fmt.Printf("%d Week %d\n", r.Year, r.Week)
	}
	fmt.Printf("Games up to %v\n", r.startTime)
	fmt.Printf("%-5s %-25s %-7s %9s\n", "Rank", "Team", "Conf", "SRS")
	for i := 0; i < top; i++ {
		team := teamList[ids[i]]
		fmt.Printf("%-5d %-25s %-7s % 7.5f\n",
			team.SRSRank, team.Name, team.Conf, team.SRS)
	}
}
