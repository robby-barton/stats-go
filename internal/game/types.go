package game

import (
	"time"
)

// Domain types below mirror the persistence models in internal/database, but
// carry no GORM concerns. The espn package owns the raw ESPN DTOs; these
// structs are the parsed, sport-agnostic game data. Mapping to the GORM
// models happens at the persistence boundary in internal/updater.

// Info is the parsed game header.
type Info struct {
	GameID     int64
	StartTime  time.Time
	Neutral    bool
	ConfGame   bool
	Season     int64
	Week       int64
	Postseason int64
	HomeID     int64
	HomeScore  int64
	AwayID     int64
	AwayScore  int64
}

// TeamGameStats holds a single team's box-score team statistics.
type TeamGameStats struct {
	GameID             int64
	TeamID             int64
	Score              int64
	Drives             int64
	PassYards          int64
	Completions        int64
	CompletionAttempts int64
	RushYards          int64
	RushAttempts       int64
	FirstDowns         int64
	ThirdDowns         int64
	ThirdDownsConv     int64
	FourthDowns        int64
	FourthDownsConv    int64
	Fumbles            int64
	Interceptions      int64
	Possession         int64
	Penalties          int64
	PenaltyYards       int64
}

// PassingStats holds one player's passing statistics.
type PassingStats struct {
	PlayerID      int64
	TeamID        int64
	GameID        int64
	Completions   int64
	Attempts      int64
	Yards         int64
	Touchdowns    int64
	Interceptions int64
}

// RushingStats holds one player's rushing statistics.
type RushingStats struct {
	PlayerID   int64
	TeamID     int64
	GameID     int64
	Carries    int64
	RushYards  int64
	RushLong   int64
	Touchdowns int64
}

// ReceivingStats holds one player's receiving statistics.
type ReceivingStats struct {
	PlayerID   int64
	TeamID     int64
	GameID     int64
	Receptions int64
	RecYards   int64
	RecLong    int64
	Touchdowns int64
}

// FumbleStats holds one player's fumble statistics.
type FumbleStats struct {
	PlayerID    int64
	TeamID      int64
	GameID      int64
	Fumbles     int64
	FumblesLost int64
	FumblesRec  int64
}

// DefensiveStats holds one player's defensive statistics.
type DefensiveStats struct {
	PlayerID       int64
	TeamID         int64
	GameID         int64
	PassesDef      int64
	QBHurries      int64
	Sacks          float64
	SoloTackles    int64
	Touchdowns     int64
	TacklesForLoss float64
	TotalTackles   float64
}

// InterceptionStats holds one player's interception statistics.
type InterceptionStats struct {
	PlayerID      int64
	TeamID        int64
	GameID        int64
	Interceptions int64
	Touchdowns    int64
	IntYards      int64
}

// ReturnStats holds one player's kick/punt return statistics.
type ReturnStats struct {
	PlayerID   int64
	TeamID     int64
	GameID     int64
	PuntKick   string
	ReturnNo   int64
	Touchdowns int64
	RetYards   int64
	RetLong    int64
}

// KickStats holds one player's kicking statistics.
type KickStats struct {
	PlayerID int64
	TeamID   int64
	GameID   int64
	FGA      int64
	FGM      int64
	FGLong   int64
	XPA      int64
	XPM      int64
	Points   int64
}

// PuntStats holds one player's punting statistics.
type PuntStats struct {
	PlayerID   int64
	TeamID     int64
	GameID     int64
	PuntLong   int64
	PuntNo     int64
	PuntYards  int64
	Touchbacks int64
	Inside20   int64
}
