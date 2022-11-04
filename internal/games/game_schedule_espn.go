package games

const weekUrl = "https://site.api.espn.com/apis/site/v2/sports/football/" +
	"college-football/scoreboard?limit=1000&dates=%d&week=%d&seasontype=%d&groups=%d"

type GameScheduleESPN struct {
	Events []Events `json:"events"`
}

type Events struct {
	Id     int64  `json:"id,string"`
	Status Status `json:"status"`
}

type Status struct {
	StatusType StatusType `json:"type"`
}

type StatusType struct {
	Name string `json:"name"`
}
