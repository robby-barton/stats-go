package espn

import (
	"encoding/json"
	"fmt"
	"strconv"
)

//nolint:gochecknoglobals // overridden in tests
var weekURL = "https://cdn.espn.com/core/college-football/schedule?xhr=1&render=false&userab=18"

type GameScheduleESPN struct {
	Content Content `json:"content"`
}

type Content struct {
	Schedule      map[string]Day `json:"schedule"`
	Parameters    Parameters     `json:"parameters"`
	Defaults      Parameters     `json:"defaults"`
	Calendar      []Calendar     `json:"calendar"`
	ConferenceAPI ConferenceAPI  `json:"conferenceAPI"`
}

type Day struct {
	Games []Game `json:"games"`
}

type Game struct {
	ID           int64         `json:"id,string"`
	Status       Status        `json:"status"`
	Competitions []Competition `json:"competitions"`
}

type Competition struct {
	Competitors []Competitor `json:"competitors"`
}

type Competitor struct {
	ID       int64        `json:"id,string"`
	Team     ScheduleTeam `json:"team"`
	Score    int64        `json:"score,string"`
	HomeAway string       `json:"homeAway"`
}

type ScheduleTeam struct {
	ID           int64 `json:"id,string"`
	ConferenceID int64 `json:"conferenceId,string"`
}

type Status struct {
	StatusType StatusType `json:"type"`
}

type StatusType struct {
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
}

type Parameters struct {
	Week       int64     `json:"week"`
	Year       int64     `json:"year"`
	SeasonType int64     `json:"seasonType"`
	Group      FlexInt64 `json:"group"`
}

type Calendar struct {
	Weeks      []Week `json:"entries"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	SeasonType int64  `json:"value,string"`
}

type Week struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Num       int64  `json:"value,string"`
}

type ConferenceAPI struct {
	Conferences []Conference `json:"conferences"`
}

type Conference struct {
	GroupID       int64     `json:"groupId,string"`
	Name          string    `json:"name"`
	SubGroups     []string  `json:"subGroups"` // is an array of ints though
	Logo          string    `json:"logo"`
	ParentGroupID FlexInt64 `json:"parentGroupId"`
	ShortName     string    `json:"shortName"`
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

func (r GameScheduleESPN) validate() error {
	// Calendar is only present in football schedule responses. Basketball
	// schedule responses omit it entirely, so we cannot require it here.
	// However, both sports must return schedule data.
	if len(r.Content.Schedule) == 0 {
		return fmt.Errorf("empty schedule in response")
	}
	return nil
}
