package espn

import "errors"

// ScoreboardESPN represents the top-level response from the ESPN scoreboard API.
// Used primarily for basketball where the schedule endpoint lacks season metadata.
type ScoreboardESPN struct {
	Leagues []ScoreboardLeague `json:"leagues"`
}

// ScoreboardLeague contains season metadata and a flat calendar of game dates.
type ScoreboardLeague struct {
	Season   ScoreboardSeason `json:"season"`
	Calendar []string         `json:"calendar"`
}

// ScoreboardSeason holds the year and date range for a season.
type ScoreboardSeason struct {
	Year      int64                `json:"year"`
	StartDate string               `json:"startDate"`
	EndDate   string               `json:"endDate"`
	Type      ScoreboardSeasonType `json:"type"`
}

// ScoreboardSeasonType identifies the current phase of a season.
type ScoreboardSeasonType struct {
	ID   int64  `json:"type"`
	Name string `json:"name"`
}

func (r ScoreboardESPN) validate() error {
	if len(r.Leagues) == 0 {
		return errors.New("scoreboard response missing leagues")
	}
	return nil
}

// GetScoreboard fetches the scoreboard endpoint for season metadata.
func (c *Client) GetScoreboard() (*ScoreboardESPN, error) {
	var res ScoreboardESPN
	if err := c.makeRequest(c.ScoreboardURL(), &res); err != nil {
		return nil, err
	}
	return &res, nil
}
