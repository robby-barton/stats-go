package espn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// statusFinal is ESPN's completed-game state name.
const statusFinal = "STATUS_FINAL"

// Game is one game from a scoreboard response (both the site.api events and
// the cdn schedule shape it replaced), carrying the status used for the
// completion filter and the competitor blocks consumed by the updater.
type Game struct {
	ID           int64         `json:"id,string"`
	Status       Status        `json:"status"`
	Competitions []Competition `json:"competitions"`
}

// Competition wraps the competitor list of one game.
type Competition struct {
	Competitors []Competitor `json:"competitors"`
}

// Competitor is one team side of a game. IDs arrive as quoted numbers on the
// wire (","string tags); ScheduleTeam carries the team ID and conference ID.
type Competitor struct {
	ID       int64        `json:"id,string"`
	Team     ScheduleTeam `json:"team"`
	Score    int64        `json:"score,string"`
	HomeAway string       `json:"homeAway"`
}

// ScheduleTeam is the team block of a competitor. ConferenceID arrives as a
// JSON number on some payloads and as a quoted string on others, hence
// FlexInt64. A few opponents (transition D-II schools, non-D1 fill-ins)
// carry no conferenceId at all — it decodes to 0 and consumers skip it.
type ScheduleTeam struct {
	ID           int64     `json:"id,string"`
	ConferenceID FlexInt64 `json:"conferenceId"`
}

// Status wraps the status type of a game or competition.
type Status struct {
	StatusType StatusType `json:"type"`
}

// StatusType carries ESPN's completion state (name STATUS_FINAL and the
// completed flag).
type StatusType struct {
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
}

// FlexInt64 unmarshals a JSON value that may be a number, a quoted string, or null.
type FlexInt64 int64

func (f *FlexInt64) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = 0
		return nil
	}
	// Try as a bare number first.
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		*f = FlexInt64(n)
		return nil
	}
	// Fall back to a quoted string.
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = FlexInt64(n)
	return nil
}

// SiteScoreboardESPN represents the site.api.espn.com scoreboard response.
// It embeds the current season year and week number directly and, when a
// `dates` query parameter is passed, the season calendar. The football
// calendar in the payload is a list of season-type objects (not the []string
// shape the basketball ScoreboardLeague expects), so Leagues uses a
// football-specific league type with an object-shaped calendar. Without a
// `dates` parameter the calendar field is JSON null and decodes to nil.
type SiteScoreboardESPN struct {
	Leagues []SiteScoreboardLeague `json:"leagues"`
	Season  SiteSeason             `json:"season"`
	Week    SiteWeek               `json:"week"`
	Events  []SiteEvent            `json:"events"`
}

// SiteScoreboardLeague is the subset of leagues[0] the games fetch and the
// football season navigation need. Its season object differs from the
// top-level one (type is a nested object here).
//
// The calendar shape differs per sport: football sends season-type objects
// (SiteCalendarType), basketball sends a flat list of ISO date strings. The
// SiteCalendar decode below accepts both — flat date strings decode to nil
// (basketball never consumes the object calendar, and its date navigation
// reads the calendar through the basketball-specific ScoreboardLeague).
type SiteScoreboardLeague struct {
	Season   SiteScoreboardLeagueSeason `json:"season"`
	Calendar SiteCalendar               `json:"calendar"`
}

// SiteCalendar is a tolerant decode of the leagues[0].calendar array, which
// is a list of season-type objects for football and a flat list of date
// strings for basketball (verified live 2026-08-30 on the basketball
// scoreboard ?dates= payload, where the calendar is always present).
type SiteCalendar []SiteCalendarType

func (sc *SiteCalendar) UnmarshalJSON(data []byte) error {
	// Peek at the first element: a quoted string means the flat basketball
	// date list, which this type deliberately ignores.
	trimmed := bytes.TrimLeft(data, " \t\n\r")
	if len(trimmed) > 0 && trimmed[0] == '[' {
		inner := bytes.TrimLeft(trimmed[1:], " \t\n\r")
		if len(inner) > 0 && inner[0] == '"' {
			*sc = nil
			return nil
		}
	}
	var types []SiteCalendarType
	if err := json.Unmarshal(data, &types); err != nil {
		return err
	}
	*sc = types
	return nil
}

// SiteScoreboardLeagueSeason carries the season year from leagues[0].
type SiteScoreboardLeagueSeason struct {
	Year int64 `json:"year"`
}

// SiteCalendarType is one season-type entry in the football scoreboard
// calendar (e.g. Regular Season, Postseason, Off Season), each carrying the
// week entries for that phase. `value` arrives as a JSON string on the wire
// ("2"), so it is decoded through FlexInt64.
type SiteCalendarType struct {
	Label     string             `json:"label"`
	Value     FlexInt64          `json:"value"`
	StartDate string             `json:"startDate"`
	EndDate   string             `json:"endDate"`
	Entries   []SiteCalendarWeek `json:"entries"`
}

// SiteCalendarWeek is one week entry inside a calendar season type. The
// entry value is the week number used by the scoreboard's week parameter
// (postseason entries number Bowls as 1 and the CFP as 999).
type SiteCalendarWeek struct {
	Label     string    `json:"label"`
	Value     FlexInt64 `json:"value"`
	StartDate string    `json:"startDate"`
	EndDate   string    `json:"endDate"`
}

// calendarType returns the calendar entry for the given season type (2 =
// Regular Season, 3 = Postseason), or nil if the calendar lacks it.
func (r SiteScoreboardESPN) calendarType(seasonType SeasonType) *SiteCalendarType {
	for i := range r.Leagues[0].Calendar {
		if int64(r.Leagues[0].Calendar[i].Value) == int64(seasonType) {
			return &r.Leagues[0].Calendar[i]
		}
	}
	return nil
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

// getScoreboardDay fetches the site.api scoreboard for one ET calendar day
// (dates=YYYYMMDD), filtered server-side to a division group via the groups
// parameter (basketball verified for D1=50, football for FBS=80/FCS=81), and
// returns its completed games via finalGames.
func (c *Client) getScoreboardDay(ctx context.Context, date string, group Group) ([]Game, error) {
	url := c.ScoreboardURL() + fmt.Sprintf("?dates=%s&groups=%d", date, group)

	var res SiteScoreboardESPN
	if err := makeRequest(ctx, c, url, &res); err != nil {
		return nil, err
	}
	return res.finalGames()
}

func (r SiteScoreboardESPN) validate() error {
	if len(r.Leagues) == 0 {
		return fmt.Errorf("site scoreboard response missing leagues")
	}
	// The season year lives in leagues[0].season on every payload shape; the
	// top-level "season" object is only present WITHOUT a dates parameter
	// (verified 2026-08-29), so it must not be required here.
	if r.Leagues[0].Season.Year == 0 {
		return fmt.Errorf("site scoreboard response missing season year")
	}
	return nil
}

// finalGames maps scoreboard events to the shared Game shape, keeping only
// games that are fully completed (statusFinal). Team and conference IDs come from
// the competitor blocks; the game start time itself is consumed later from
// the per-game summary fetch, as with the cdn path.
func (r SiteScoreboardESPN) finalGames() ([]Game, error) {
	var games []Game
	for _, ev := range r.Events {
		// ESPN occasionally emits a degenerate empty event object (verified
		// live 2026-08-30: an FCS date-range response in week 8 of the 2025
		// season contained one `{}` event). Skip it — there is no game data —
		// but still error on IDs that are present yet malformed.
		if ev.ID == "" {
			continue
		}
		id, err := strconv.ParseInt(ev.ID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing scoreboard event id %q: %w", ev.ID, err)
		}
		if !ev.Status.StatusType.Completed || ev.Status.StatusType.Name != statusFinal {
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
