package espn

import (
	"errors"
	"fmt"
)

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

// SportURLConfig holds ESPN URL templates for a sport. Every endpoint is on
// site.api.espn.com — the cdn.espn.com schedule/playbyplay endpoints were
// retired with the basketball migration (2026-08-30).
type SportURLConfig struct {
	GameStats   string
	TeamInfo    string
	Scoreboard  string
	Conferences string
}

// SportURLs returns the ESPN URL templates for a given sport.
func SportURLs(sport Sport) SportURLConfig {
	switch sport {
	case CollegeBasketball:
		// Box scores come from the site.api summary, same as football (the
		// cdn playbyplay served empty-body 202 bot challenges from 2026-08-29;
		// the summary carries the same header/boxscore data — see GameInfoESPN).
		return SportURLConfig{
			GameStats: "https://site.api.espn.com/apis/site/v2/sports/basketball/" +
				"mens-college-basketball/summary?event=%d",
			TeamInfo:   "https://site.api.espn.com/apis/site/v2/sports/basketball/mens-college-basketball/teams?limit=1000",
			Scoreboard: "https://site.api.espn.com/apis/site/v2/sports/basketball/mens-college-basketball/scoreboard",
			Conferences: "https://site.api.espn.com/apis/site/v2/sports/basketball/" +
				"mens-college-basketball/scoreboard/conferences",
		}
	case CollegeFootball:
		// Box scores come from the site.api summary (cdn.espn.com has been
		// serving empty-body 202 bot challenges since 2026-08-29; the site.api
		// summary carries the same header/boxscore data — see GameInfoESPN).
		return SportURLConfig{
			GameStats:   "https://site.api.espn.com/apis/site/v2/sports/football/college-football/summary?event=%d",
			TeamInfo:    "https://site.api.espn.com/apis/site/v2/sports/football/college-football/teams?limit=1000",
			Scoreboard:  "https://site.api.espn.com/apis/site/v2/sports/football/college-football/scoreboard",
			Conferences: "https://site.api.espn.com/apis/site/v2/sports/football/college-football/scoreboard/conferences",
		}
	default:
		panic(fmt.Sprintf("unknown sport: %q", sport))
	}
}
