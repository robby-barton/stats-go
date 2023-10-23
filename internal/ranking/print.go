//nolint:forbidigo // ranker doesn't have a logger
package ranking

import (
	"fmt"
	"os"
	"sort"

	"github.com/jedib0t/go-pretty/v6/table"
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

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{
		"Rank", "Team", "Conf", "Record", "SRS", "SoS", "Total",
	})
	for i := 0; i < top; i++ {
		team := teamList[ids[i]]
		t.AppendRow(table.Row{
			team.FinalRank, team.Name, team.Conf, team.Record, team.SRSRank,
			team.SOSRank, fmt.Sprintf("%.5f", team.FinalRaw),
		})
	}
	t.Render()
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

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"Rank", "Team", "Conf", "SRS"})
	for i := 0; i < top; i++ {
		team := teamList[ids[i]]
		t.AppendRow(table.Row{
			team.SRSRank, team.Name, team.Conf, team.SRS,
		})
	}
	t.Render()
}
