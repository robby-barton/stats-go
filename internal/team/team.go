package team

import (
	"context"

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

// ParsedFromESPN converts an ESPN team object into the package-neutral
// parsed form, resolving the light/dark logo variants (first of each wins).
func ParsedFromESPN(team espn.TeamInfo) ParsedTeamInfo {
	var teamInfo ParsedTeamInfo

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

	return teamInfo
}

func GetTeamInfo(ctx context.Context, client espn.SportClient) ([]ParsedTeamInfo, error) {
	var parsedTeamInfo []ParsedTeamInfo

	res, err := client.GetTeamInfo(ctx)
	if err != nil {
		return nil, err
	}

	teams := res.Sports[0].Leagues[0].Teams
	for _, teamWrap := range teams {
		parsedTeamInfo = append(parsedTeamInfo, ParsedFromESPN(teamWrap.Team))
	}

	return parsedTeamInfo, nil
}
