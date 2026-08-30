package espn

import (
	"encoding/json"
	"errors"
)

// GameInfoESPN is the site.api summary response (both sports): header and
// boxscore at the top level. The old cdn.espn.com playbyplay shape (wrapped
// in "gamepackageJSON") is no longer accepted — both sports moved to the
// site.api summary (football 2026-09, basketball 2026-08-30) after cdn began
// serving empty-body 202 bot challenges.
type GameInfoESPN struct {
	GamePackage GamePackage
}

// UnmarshalJSON decodes the site.api summary shape (header/boxscore at the
// top level) into GamePackage so the game parser keeps its single accessor
// path.
func (g *GameInfoESPN) UnmarshalJSON(data []byte) error {
	var payload GamePackage
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	g.GamePackage = payload
	return nil
}

// MarshalJSON re-encodes the GamePackage payload at the top level — the same
// shape UnmarshalJSON accepts — so test fixtures round-trip through the mock
// HTTP servers.
func (g GameInfoESPN) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.GamePackage)
}

type GamePackage struct {
	Header   Header   `json:"header"`
	Boxscore Boxscore `json:"boxscore"`
}

type Header struct {
	ID           int64          `json:"id,string"`
	Competitions []Competitions `json:"competitions"`
	Season       Season         `json:"season"`
	Week         int64          `json:"week"`
}

type Competitions struct {
	ID          int64         `json:"id,string"`
	Date        string        `json:"date"`
	ConfGame    bool          `json:"conferenceCompetition"`
	Neutral     bool          `json:"neutralSite"`
	Competitors []Competitors `json:"competitors"`
	Status      Status        `json:"status"`
}

type Competitors struct {
	HomeAway string `json:"homeAway"`
	ID       int64  `json:"id,string"`
	Score    int64  `json:"score,string"`
}

type Season struct {
	Year int64 `json:"year"`
	Type int64 `json:"type"`
}

type Boxscore struct {
	Teams   []Teams   `json:"teams"`
	Players []Players `json:"players"`
}

type Teams struct {
	Statistics []TeamStatistics `json:"statistics"`
	Team       Team             `json:"team"`
}

type TeamStatistics struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	DisplayValue string `json:"displayValue"`
}

type Team struct {
	ID int64 `json:"id,string"`
}

type Players struct {
	Statistics []PlayerStatistics `json:"statistics"`
	Team       Team               `json:"team"`
}

type PlayerStatistics struct {
	Name     string         `json:"name"`
	Labels   []string       `json:"labels"`
	Totals   []string       `json:"totals"`
	Athletes []AthleteStats `json:"athletes"`
}

type AthleteStats struct {
	Athlete Athlete  `json:"athlete"`
	Stats   []string `json:"stats"`
}

type Athlete struct {
	ID        int64  `json:"id,string"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

func (g GameInfoESPN) validate() error {
	if g.GamePackage.Header.ID == 0 {
		return errors.New("game info response has zero header ID")
	}
	if len(g.GamePackage.Header.Competitions) == 0 {
		return errors.New("game info response missing competitions")
	}
	if len(g.GamePackage.Header.Competitions[0].Competitors) < 2 {
		return errors.New("game info response has fewer than 2 competitors")
	}
	return nil
}
