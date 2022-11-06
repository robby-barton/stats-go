package games

const weekUrl = "https://cdn.espn.com/core/college-football/schedule?xhr=1&render=false" +
	"&userab=18&year=%d&week=%d&seasontype=%d&group=%d"

type GameScheduleESPN struct {
	Content Content `json:"content"`
}

type Content struct {
	Schedule map[string]Day `json:"schedule"`
	Calendar []Calendar     `json:"calendar"`
}

type Day struct {
	Games []Game `json:"games"`
}

type Game struct {
	Id     int64  `json:"id,string"`
	Status Status `json:"status"`
}

type Status struct {
	StatusType StatusType `json:"type"`
}

type StatusType struct {
	Name string `json:"name"`
}

type Calendar struct {
	Weeks      []Week `json:"entries"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	SeasonType int64  `json:"value;string"`
}

type Week struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Num       int64  `json:"value;string"`
}
