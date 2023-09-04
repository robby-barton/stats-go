package espn

const weekURL = "https://cdn.espn.com/core/college-football/schedule?xhr=1&render=false&userab=18"

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
	ID   int64        `json:"id,string"`
	Team ScheduleTeam `json:"team"`
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
	Week       int64 `json:"week"`
	Year       int64 `json:"year"`
	SeasonType int64 `json:"seasonType"`
	Group      int64 `json:"group,string"`
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
	GroupID       int64    `json:"groupId,string"`
	Name          string   `json:"name"`
	SubGroups     []string `json:"subGroups"` // is an array of ints though
	Logo          string   `json:"logo"`
	ParentGroupID int64    `json:"parentGroupId,string"`
	ShortName     string   `json:"shortName"`
}
