package espn

const gameStatsURL = "https://cdn.espn.com/core/college-football/playbyplay" +
	"?gameId=%d&xhr=1&render=false&userab=18"

type GameInfoESPN struct {
	GamePackage GamePackage `json:"gamepackageJSON"`
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
