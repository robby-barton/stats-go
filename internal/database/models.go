package database

import (
	"time"
)

type Conference struct {
	ConfID    int64  `json:"confId" gorm:"column:conf_id;primaryKey;not null;unique"`
	Name      string `json:"name" gorm:"column:name"`
	Logo      string `json:"logo" gorm:"column:logo"`
	ParentID  int64  `json:"parentId" gorm:"column:parent_id"`
	ShortName string `json:"shortName" gorm:"column:short_name"`
}

func (Conference) TableName() string {
	return "conferences"
}

type TeamName struct {
	TeamID           int64  `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	Name             string `json:"name" gorm:"column:name;not null"`
	Sport            string `json:"sport" gorm:"column:sport;primaryKey;default:ncaaf"`
	Flair            string `json:"flair" gorm:"column:flair"`
	Abbreviation     string `json:"abbreviation" gorm:"column:abbreviation"`
	AltColor         string `json:"altColor" gorm:"column:alt_color"`
	Color            string `json:"color" gorm:"column:color"`
	DisplayName      string `json:"displayName" gorm:"column:display_name"`
	IsActive         bool   `json:"isActive" gorm:"column:is_active"`
	IsAllStar        bool   `json:"isAllstar" gorm:"column:is_allstar"`
	Location         string `json:"location" gorm:"column:location"`
	Logo             string `json:"logo" gorm:"column:logo"`
	LogoDark         string `json:"logoDark" gorm:"column:logo_dark"`
	Nickname         string `json:"nickname" gorm:"column:nickname"`
	ShortDisplayName string `json:"shortDisplayName" gorm:"column:short_display_name"`
	Slug             string `json:"slug" gorm:"column:slug"`
}

func (TeamName) TableName() string {
	return "team_names"
}

type TeamSeason struct {
	TeamID int64  `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	Year   int64  `json:"year" gorm:"column:year;primaryKey"`
	Sport  string `json:"sport" gorm:"column:sport;primaryKey;default:ncaaf"`
	FBS    int64  `json:"fbs" gorm:"column:fbs"`
	Conf   string `json:"conf" gorm:"column:conf"`
}

func (TeamSeason) TableName() string {
	return "team_seasons"
}

type TeamWeekResult struct {
	TeamID     int64   `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	Name       string  `json:"name" gorm:"column:name;not null"`
	Conf       string  `json:"conf" gorm:"column:conf"`
	Year       int64   `json:"year" gorm:"column:year;primaryKey;not null"`
	Week       int64   `json:"week" gorm:"column:week;primaryKey;not null"`
	Postseason int64   `json:"postseason" gorm:"column:postseason;primaryKey"`
	Sport      string  `json:"sport" gorm:"column:sport;primaryKey;default:ncaaf"`
	FinalRank  int64   `json:"final_rank" gorm:"column:final_rank"`
	FinalRaw   float64 `json:"final_raw" gorm:"column:final_raw"`
	Wins       int64   `json:"wins" gorm:"column:wins"`
	Losses     int64   `json:"losses" gorm:"column:losses"`
	Ties       int64   `json:"ties" gorm:"column:ties"`
	SRSRank    int64   `json:"srs_rank" gorm:"column:srs_rank"`
	SOSRank    int64   `json:"sos_rank" gorm:"column:sos_rank"`
	SOVRank    int64   `json:"sov_rank" gorm:"column:sov_rank"`
	SOLRank    int64   `json:"sol_rank" gorm:"column:sol_rank"`
	Fbs        bool    `json:"fbs" gorm:"column:fbs"`
}

func (TeamWeekResult) TableName() string {
	return "team_week_results"
}

type Game struct {
	GameID     int64     `json:"game_id" gorm:"column:game_id;primaryKey;not null;unique"`
	StartTime  time.Time `json:"start_time" gorm:"column:start_time"`
	Sport      string    `json:"sport" gorm:"column:sport;default:ncaaf"`
	Neutral    bool      `json:"neutral" gorm:"column:neutral"`
	ConfGame   bool      `json:"conf_game" gorm:"column:conf_game"`
	Season     int64     `json:"season" gorm:"column:season"`
	Week       int64     `json:"week" gorm:"column:week"`
	Postseason int64     `json:"postseason" gorm:"column:postseason"`
	HomeID     int64     `json:"home_id" gorm:"column:home_id"`
	HomeScore  int64     `json:"home_score" gorm:"column:home_score"`
	AwayID     int64     `json:"away_id" gorm:"column:away_id"`
	AwayScore  int64     `json:"away_score" gorm:"column:away_score"`
	Retry      int64     `json:"retry" gorm:"column:retry"`
}

func (Game) TableName() string {
	return "games"
}

type TeamGameStats struct {
	GameID             int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	TeamID             int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	Score              int64 `json:"score" gorm:"column:score"`
	Drives             int64 `json:"drives" gorm:"column:drives"`
	PassYards          int64 `json:"pass_yards" gorm:"column:pass_yards"`
	Completions        int64 `json:"completions" gorm:"column:completions"`
	CompletionAttempts int64 `json:"completion_attempts" gorm:"column:completion_attempts"`
	RushYards          int64 `json:"rush_yards" gorm:"column:rush_yards"`
	RushAttempts       int64 `json:"rush_attempts" gorm:"column:rush_attempts"`
	FirstDowns         int64 `json:"first_downs" gorm:"column:first_downs"`
	ThirdDowns         int64 `json:"third_downs" gorm:"column:third_downs"`
	ThirdDownsConv     int64 `json:"third_downs_conv" gorm:"column:third_downs_conv"`
	FourthDowns        int64 `json:"fourth_downs" gorm:"column:fourth_downs"`
	FourthDownsConv    int64 `json:"fourth_downs_conv" gorm:"column:fourth_downs_conv"`
	Fumbles            int64 `json:"fumbles" gorm:"column:fumbles"`
	Interceptions      int64 `json:"interceptions" gorm:"column:interceptions"`
	Possession         int64 `json:"possession" gorm:"column:possession"`
	Penalties          int64 `json:"penalties" gorm:"column:penalties"`
	PenaltyYards       int64 `json:"penalty_yards" gorm:"column:penalty_yards"`
}

func (TeamGameStats) TableName() string {
	return "team_game_stats"
}

type Composite struct {
	TeamID  int64   `json:"team_id" gorm:"column:team_id;primaryKey"`
	Year    int64   `json:"year" gorm:"column:year;primaryKey"`
	Average float64 `json:"average" gorm:"column:average"`
	Rating  float64 `json:"rating" gorm:"column:rating"`
}

func (Composite) TableName() string {
	return "composite"
}

type Recruiting struct {
	TeamID  int64   `json:"team_id" gorm:"column:team_id;primaryKey"`
	Year    int64   `json:"year" gorm:"column:year;primaryKey"`
	Commits int64   `json:"commits" gorm:"column:commits"`
	Rating  float64 `json:"rating" gorm:"column:rating"`
}

func (Recruiting) TableName() string {
	return "recruiting"
}

type Roster struct {
	PlayerID int64  `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID   int64  `json:"team_id" gorm:"column:team_id;primaryKey"`
	Year     int64  `json:"year" gorm:"column:year;primaryKey"`
	Name     string `json:"name" gorm:"column:name"`
	Num      int64  `json:"num" gorm:"column:num"`
	Position string `json:"position" gorm:"column:position"`
	Height   int64  `json:"height" gorm:"column:height"`
	Weight   int64  `json:"weight" gorm:"column:weight"`
	Grade    string `json:"grade" gorm:"column:grade"`
	Hometown string `json:"hometown" gorm:"column:hometown"`
}

func (Roster) TableName() string {
	return "roster"
}

type Player struct {
	PlayerID int64  `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID   int64  `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	Year     int64  `json:"year" gorm:"column:year;primaryKey;not null"`
	Name     string `json:"name" gorm:"column:name;not null"`
	Position string `json:"position" gorm:"column:position;not null"`
	Rating   int64  `json:"rating" gorm:"column:rating"`
	Grade    string `json:"grade" gorm:"column:grade;not null"`
	Hometown string `json:"hometown" gorm:"column:hometown;not null"`
	Status   string `json:"status" gorm:"column:status;not null"`
}

func (Player) TableName() string {
	return "players"
}

type PassingStats struct {
	PlayerID      int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID        int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID        int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	Completions   int64 `json:"completions" gorm:"column:completions"`
	Attempts      int64 `json:"attempts" gorm:"column:attempts"`
	Yards         int64 `json:"yards" gorm:"column:yards"`
	Touchdowns    int64 `json:"touchdowns" gorm:"column:touchdowns"`
	Interceptions int64 `json:"interceptions" gorm:"column:interceptions"`
}

func (PassingStats) TableName() string {
	return "passing_stats"
}

type RushingStats struct {
	PlayerID   int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID     int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID     int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	Carries    int64 `json:"carries" gorm:"column:carries"`
	RushYards  int64 `json:"rush_yards" gorm:"column:rush_yards"`
	RushLong   int64 `json:"rush_long" gorm:"column:rush_long"`
	Touchdowns int64 `json:"touchdowns" gorm:"column:touchdowns"`
}

func (RushingStats) TableName() string {
	return "rushing_stats"
}

type ReceivingStats struct {
	PlayerID   int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID     int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID     int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	Receptions int64 `json:"receptions" gorm:"column:receptions"`
	RecYards   int64 `json:"rec_yards" gorm:"column:rec_yards"`
	RecLong    int64 `json:"rec_long" gorm:"column:rec_long"`
	Touchdowns int64 `json:"touchdowns" gorm:"column:touchdowns"`
}

func (ReceivingStats) TableName() string {
	return "receiving_stats"
}

type ReturnStats struct {
	PlayerID   int64  `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID     int64  `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID     int64  `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	PuntKick   string `json:"punt_kick" gorm:"column:punt_kick;primaryKey;not null"`
	ReturnNo   int64  `json:"return_no" gorm:"column:return_no"`
	Touchdowns int64  `json:"touchdowns" gorm:"column:touchdowns"`
	RetYards   int64  `json:"ret_yards" gorm:"column:ret_yards"`
	RetLong    int64  `json:"ret_long" gorm:"ret_long"`
}

func (ReturnStats) TableName() string {
	return "return_stats"
}

type KickStats struct {
	PlayerID int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID   int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID   int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	FGA      int64 `json:"fga" gorm:"column:fga"`
	FGM      int64 `json:"fgm" gorm:"column:fgm"`
	FGLong   int64 `json:"long" gorm:"column:fg_long"`
	XPA      int64 `json:"xpa" gorm:"column:xpa"`
	XPM      int64 `json:"xpm" gorm:"column:xpm"`
	Points   int64 `json:"points" gorm:"column:points"`
}

func (KickStats) TableName() string {
	return "kick_stats"
}

type PuntStats struct {
	PlayerID   int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID     int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID     int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	PuntLong   int64 `json:"punt_long" gorm:"column:punt_long"`
	PuntNo     int64 `json:"punt_no" gorm:"column:punt_no"`
	PuntYards  int64 `json:"punt_yards" gorm:"column:punt_yards"`
	Touchbacks int64 `json:"touchbacks" gorm:"column:touchbacks"`
	Inside20   int64 `json:"inside_20" gorm:"column:inside_20"`
}

func (PuntStats) TableName() string {
	return "punt_stats"
}

type InterceptionStats struct {
	PlayerID      int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID        int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID        int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	Interceptions int64 `json:"interceptions" gorm:"column:interceptions"`
	Touchdowns    int64 `json:"touchdowns" gorm:"column:touchdowns"`
	IntYards      int64 `json:"int_yards" gorm:"column:int_yards"`
}

func (InterceptionStats) TableName() string {
	return "interception_stats"
}

type FumbleStats struct {
	PlayerID    int64 `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID      int64 `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID      int64 `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	Fumbles     int64 `json:"fumbles" gorm:"column:fumbles"`
	FumblesLost int64 `json:"fumbles_lost" gorm:"column:fumbles_lost"`
	FumblesRec  int64 `json:"fumbles_rec" gorm:"column:fumbles_rec"`
}

func (FumbleStats) TableName() string {
	return "fumble_stats"
}

type DefensiveStats struct {
	PlayerID       int64   `json:"player_id" gorm:"column:player_id;primaryKey;not null"`
	TeamID         int64   `json:"team_id" gorm:"column:team_id;primaryKey;not null"`
	GameID         int64   `json:"game_id" gorm:"column:game_id;primaryKey;not null"`
	PassesDef      int64   `json:"passes_def" gorm:"column:passes_def"`
	QBHurries      int64   `json:"qb_hurries" gorm:"column:qb_hurries"`
	Sacks          float64 `json:"sacks" gorm:"column:sacks"`
	SoloTackles    int64   `json:"solo_tackles" gorm:"column:solo_tackles"`
	Touchdowns     int64   `json:"touchdowns" gorm:"column:touchdowns"`
	TacklesForLoss float64 `json:"tackles_for_loss" gorm:"column:tackles_for_loss"`
	TotalTackles   float64 `json:"total_tackles" gorm:"column:total_tackles"`
}

func (DefensiveStats) TableName() string {
	return "defensive_stats"
}
