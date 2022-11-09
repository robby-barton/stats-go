package ranking

import (
	"sort"

	"github.com/robby-barton/stats-api/internal/database"
)

func (r *Ranker) getComposites(teamList TeamList) error {
	var composites []database.Composite
	ids := make([]int64, len(teamList))
	i := 0
	for id := range teamList {
		ids[i] = id
		i++
	}
	if err := r.DB.Where("year = ? and team_id in (?)", year, ids).
		Find(&composites).Error; err != nil {

		return err
	}

	sort.Slice(composites, func(i, j int) bool {
		return composites[i].Rating > composites[j].Rating
	})

	// max and min for normalization
	max := composites[0].Rating
	min := composites[len(composites)-1].Rating
	var prev float64
	var prevRank int64
	for rank, composite := range composites {
		team := teamList[composite.TeamId]

		team.Composite = composite.Rating
		if composite.Rating == prev {
			team.CompositeRank = prevRank
		} else {
			team.CompositeRank = int64(rank + 1)
			prev = float64(composite.Rating)
			prevRank = team.CompositeRank
		}

		if max-min > 0 {
			team.CompositeNorm = (composite.Rating - min) / (max - min)
		}
	}

	return nil
}
