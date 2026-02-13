package updater

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/team"
)

func apiToDB(teams []team.ParsedTeamInfo) []database.TeamName {
	var ret []database.TeamName

	for _, team := range teams {
		var teamDB database.TeamName

		// Name is actually the nickname as well, so chop off the display name
		// to get the school. Though AllStar teams don't do this so we need
		// to actually check.
		schoolEnd := strings.Index(team.DisplayName, team.Name) - 1
		name := team.DisplayName
		if schoolEnd > 0 {
			name = team.DisplayName[:schoolEnd]
		}

		teamDB.TeamID = team.ID
		teamDB.Name = name
		teamDB.Abbreviation = team.Abbreviation
		teamDB.AltColor = team.AltColor
		teamDB.Color = team.Color
		teamDB.DisplayName = team.DisplayName
		teamDB.IsActive = team.IsActive
		teamDB.IsAllStar = team.IsAllStar
		teamDB.Location = team.Location
		teamDB.Logo = team.Logo
		teamDB.LogoDark = team.LogoDark
		teamDB.Nickname = team.Nickname
		teamDB.ShortDisplayName = team.ShortDisplayName
		teamDB.Slug = team.Slug

		ret = append(ret, teamDB)
	}

	return ret
}

func (u *Updater) insertTeamsToDB(teams []database.TeamName) error {
	return u.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				UpdateAll: true, // upsert
			}).
			CreateInBatches(teams, 100).Error; err != nil {
			return err
		}

		return nil
	})
}

func (u *Updater) UpdateTeamInfo() (int, error) {
	teamInfo, err := team.GetTeamInfo(u.ESPN)
	if err != nil {
		return 0, err
	}

	if err = u.insertTeamsToDB(apiToDB(teamInfo)); err != nil {
		return 0, err
	}

	return len(teamInfo), nil
}
