package espn

import (
	"fmt"
	"strconv"
)

// SiteScoreboardESPN represents the site.api.espn.com scoreboard response.
// It carries no calendar or conference API data (season navigation stays on
// cdn for that reason) but embeds the current season year and week number
// directly. The football calendar in the payload is a list of season-type
// objects (not the []string shape the basketball ScoreboardLeague expects),
// so Leagues uses a football-specific league type that omits it.
type SiteScoreboardESPN struct {
	Leagues []SiteScoreboardLeague `json:"leagues"`
	Season  SiteSeason             `json:"season"`
	Week    SiteWeek               `json:"week"`
	Events  []SiteEvent            `json:"events"`
}

// SiteScoreboardLeague is the subset of leagues[0] the games fetch needs.
// Its season object differs from the top-level one (type is a nested object
// here), and only the year is consumed - everything else is ignored.
type SiteScoreboardLeague struct {
	Season SiteScoreboardLeagueSeason `json:"season"`
}

// SiteScoreboardLeagueSeason carries the season year from leagues[0].
type SiteScoreboardLeagueSeason struct {
	Year int64 `json:"year"`
}

// SiteSeason identifies the season the scoreboard response belongs to.
type SiteSeason struct {
	Year int64 `json:"year"`
	Type int64 `json:"type"`
}

// SiteWeek is the week number embedded in a scoreboard response.
type SiteWeek struct {
	Number int64 `json:"number"`
}

// SiteEvent is one game in a scoreboard response. Event and competitor IDs
// are JSON strings on site.api (unlike the cdn schedule, where they arrive
// as quoted numbers handled by ,string tags).
type SiteEvent struct {
	ID           string            `json:"id"`
	Date         string            `json:"date"`
	Name         string            `json:"name"`
	Season       SiteSeason        `json:"season"`
	Week         SiteWeek          `json:"week"`
	Status       Status            `json:"status"`
	Competitions []SiteCompetition `json:"competitions"`
}

// SiteCompetition mirrors the competition block of a scoreboard event.
// Competitors reuse the shared Competitor type: the site.api field names
// (id, homeAway, score, team.id, team.conferenceId) match the cdn schedule
// shape, so the updater consumes mapped games without changes.
type SiteCompetition struct {
	Date                  string       `json:"date"`
	NeutralSite           bool         `json:"neutralSite"`
	ConferenceCompetition bool         `json:"conferenceCompetition"`
	Status                Status       `json:"status"`
	Competitors           []Competitor `json:"competitors"`
}

func (r SiteScoreboardESPN) validate() error {
	if len(r.Leagues) == 0 {
		return fmt.Errorf("site scoreboard response missing leagues")
	}
	if r.Season.Year == 0 {
		return fmt.Errorf("site scoreboard response missing season year")
	}
	return nil
}

// finalGames maps scoreboard events to the shared Game shape, keeping only
// games that are fully completed (STATUS_FINAL) — the same filter the cdn
// schedule path applies in completedGames. Team and conference IDs come from
// the competitor blocks; the game start time itself is consumed later from
// the per-game summary fetch, as with the cdn path.
func (r SiteScoreboardESPN) finalGames() ([]Game, error) {
	var games []Game
	for _, ev := range r.Events {
		id, err := strconv.ParseInt(ev.ID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing scoreboard event id %q: %w", ev.ID, err)
		}
		if !ev.Status.StatusType.Completed || ev.Status.StatusType.Name != "STATUS_FINAL" {
			continue
		}

		game := Game{ID: id, Status: ev.Status}
		for _, comp := range ev.Competitions {
			game.Competitions = append(game.Competitions, Competition{
				Competitors: comp.Competitors,
			})
		}
		games = append(games, game)
	}
	return games, nil
}
