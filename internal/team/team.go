package team

import (
	"errors"
	"fmt"

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

func GetTeamInfo() ([]ParsedTeamInfo, error) {
	var parsedTeamInfo []ParsedTeamInfo

	res, err := espn.GetTeamInfo()
	if err != nil {
		return nil, err
	}

	//nolint:gocritic // need to check nesting parse
	if len(res.Sports) == 0 {
		return nil, errors.New("no sport")
	} else if len(res.Sports[0].Leagues) == 0 {
		return nil, errors.New("no league")
	} else if len(res.Sports[0].Leagues[0].Teams) == 0 {
		return nil, errors.New("no teams")
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

		if len(team.Logos) > 2 {
			return nil, fmt.Errorf("too many logos for %d", team.ID)
		}

		for _, logo := range team.Logos {
			theme := ""
			for i := len(logo.Rel) - 1; i >= 0; i-- {
				if logo.Rel[i] == dark {
					theme = dark
					break
				}
			}
			if theme == dark {
				teamInfo.LogoDark = logo.Href
			} else {
				teamInfo.Logo = logo.Href
			}
		}

		parsedTeamInfo = append(parsedTeamInfo, teamInfo)
	}

	return parsedTeamInfo, nil
}
