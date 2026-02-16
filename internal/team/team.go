package team

import (
	"github.com/robby-barton/stats-go/internal/espn"
)

const dark = "dark"

type ParsedTeamInfo struct {
	Abbreviation     string
	AltColor         string
	Color            string
	DisplayName      string
	ID               int64
	IsActive         bool
	IsAllStar        bool
	Location         string
	Logo             string
	LogoDark         string
	Name             string
	Nickname         string
	ShortDisplayName string
	Slug             string
}

func GetTeamInfo(client espn.SportClient) ([]ParsedTeamInfo, error) {
	var parsedTeamInfo []ParsedTeamInfo

	res, err := client.GetTeamInfo()
	if err != nil {
		return nil, err
	}

	teams := res.Sports[0].Leagues[0].Teams
	for _, teamWrap := range teams {
		var teamInfo ParsedTeamInfo

		team := teamWrap.Team

		teamInfo.Abbreviation = team.Abbreviation
		teamInfo.AltColor = team.AltColor
		teamInfo.Color = team.Color
		teamInfo.DisplayName = team.DisplayName
		teamInfo.ID = team.ID
		teamInfo.IsActive = team.IsActive
		teamInfo.IsAllStar = team.IsAllStar
		teamInfo.Location = team.Location
		teamInfo.Name = team.Name
		teamInfo.Nickname = team.Nickname
		teamInfo.ShortDisplayName = team.ShortDisplayName
		teamInfo.Slug = team.Slug

		for _, logo := range team.Logos {
			isDark := false
			for i := len(logo.Rel) - 1; i >= 0; i-- {
				if logo.Rel[i] == dark {
					isDark = true
					break
				}
			}
			if isDark && teamInfo.LogoDark == "" {
				teamInfo.LogoDark = logo.Href
			} else if !isDark && teamInfo.Logo == "" {
				teamInfo.Logo = logo.Href
			}
		}

		parsedTeamInfo = append(parsedTeamInfo, teamInfo)
	}

	return parsedTeamInfo, nil
}
