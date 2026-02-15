package espn

import (
	"errors"
	"fmt"
)

//nolint:gochecknoglobals // overridden in tests
var teamInfoURL = "https://site.api.espn.com/apis/site/v2/sports/football/college-football/teams?limit=1000"

type TeamInfoESPN struct {
	Sports []TeamInfoSport `json:"sports"`
}

type TeamInfoSport struct {
	ID      int64    `json:"id,string"`
	Leagues []League `json:"leagues"`
	Name    string   `json:"name"`
	Slug    string   `json:"slug"`
}

type League struct {
	Abbreviation string     `json:"abbreviation"`
	ID           int64      `json:"id,string"`
	Name         string     `json:"name"`
	ShortName    string     `json:"shortName"`
	Slug         string     `json:"slug"`
	Teams        []TeamWrap `json:"teams"`
	Year         int64      `json:"year"`
}

type TeamWrap struct {
	Team TeamInfo `json:"team"`
}

type TeamInfo struct {
	Abbreviation     string `json:"abbreviation"`
	AltColor         string `json:"alternateColor"`
	Color            string `json:"color"`
	DisplayName      string `json:"displayName"`
	ID               int64  `json:"id,string"`
	IsActive         bool   `json:"isActive"`
	IsAllStar        bool   `json:"isAllStar"`
	Links            []Link `json:"links"`
	Location         string `json:"location"`
	Logos            []Logo `json:"logos"`
	Name             string `json:"name"`
	Nickname         string `json:"nickname"`
	ShortDisplayName string `json:"shortDisplayName"`
	Slug             string `json:"slug"`
}

type Link struct {
	Href       string   `json:"href"`
	IsExternal bool     `json:"isExternal"`
	IsHidden   bool     `json:"isHidden"`
	IsPremium  bool     `json:"isPremium"`
	Language   string   `json:"language"`
	Rel        []string `json:"rel"`
	ShortText  string   `json:"shortText"`
	Text       string   `json:"text"`
}

type Logo struct {
	Alt    string   `json:"alt"`
	Height int64    `json:"height"`
	Href   string   `json:"href"`
	Rel    []string `json:"rel"`
	Width  int64    `json:"width"`
}

func (r TeamInfoESPN) validate() error {
	if len(r.Sports) == 0 {
		return errors.New("team info response missing sports")
	}
	if len(r.Sports[0].Leagues) == 0 {
		return errors.New("team info response missing leagues")
	}
	if len(r.Sports[0].Leagues[0].Teams) == 0 {
		return errors.New("team info response missing teams")
	}
	return nil
}

// SportURLs returns the ESPN URL templates for a given sport.
// SportURLConfig holds ESPN URL templates for a sport.
type SportURLConfig struct {
	Schedule   string
	GameStats  string
	TeamInfo   string
	Scoreboard string
}

// SportURLs returns the ESPN URL templates for a given sport.
func SportURLs(sport Sport) SportURLConfig {
	switch sport {
	case CollegeBasketball:
		return SportURLConfig{
			Schedule:   "https://cdn.espn.com/core/mens-college-basketball/schedule?xhr=1&render=false&userab=18",
			GameStats:  "https://cdn.espn.com/core/mens-college-basketball/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
			TeamInfo:   "https://site.api.espn.com/apis/site/v2/sports/basketball/mens-college-basketball/teams?limit=1000",
			Scoreboard: "https://site.api.espn.com/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard",
		}
	case CollegeFootball:
		return SportURLConfig{
			Schedule:   "https://cdn.espn.com/core/college-football/schedule?xhr=1&render=false&userab=18",
			GameStats:  "https://cdn.espn.com/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
			TeamInfo:   "https://site.api.espn.com/apis/site/v2/sports/football/college-football/teams?limit=1000",
			Scoreboard: "https://site.api.espn.com/apis/site/v2/sports/football/college-football/scoreboard",
		}
	default:
		panic(fmt.Sprintf("unknown sport: %q", sport))
	}
}
