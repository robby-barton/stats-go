package ranking

// record computes won-loss-tie records for the ranking window from the
// loaded games. Records are tracked for every team of the sport/year (not
// just the division being ranked), matching the original behavior.
func (r *Ranker) record(teamList TeamList) {
	allowedTeam := map[int64]bool{}
	teamRecords := make(map[int64]*Record)
	for _, team := range r.in.Teams {
		allowedTeam[team.ID] = true
		teamRecords[team.ID] = &Record{}
	}

	for _, game := range r.in.Games {
		if game.Season != r.in.Year {
			continue // record covers the current season only
		}
		if allowedTeam[game.HomeID] {
			homeRecord := teamRecords[game.HomeID]
			switch {
			case game.HomeScore > game.AwayScore:
				homeRecord.Wins++
			case game.AwayScore > game.HomeScore:
				homeRecord.Losses++
			default:
				homeRecord.Ties++
			}
			homeRecord.Record = (1 + float64(homeRecord.Wins) + 0.5*float64(homeRecord.Ties)) /
				(2 + float64(homeRecord.Wins+homeRecord.Losses+homeRecord.Ties))
		}
		if allowedTeam[game.AwayID] {
			awayRecord := teamRecords[game.AwayID]
			switch {
			case game.HomeScore > game.AwayScore:
				awayRecord.Losses++
			case game.AwayScore > game.HomeScore:
				awayRecord.Wins++
			default:
				awayRecord.Ties++
			}
			awayRecord.Record = (1 + float64(awayRecord.Wins) + 0.5*float64(awayRecord.Ties)) /
				(2 + float64(awayRecord.Wins+awayRecord.Losses+awayRecord.Ties))
		}
	}

	for id, team := range teamList {
		if record, ok := teamRecords[id]; ok {
			team.Record = *record
		}
	}
}
