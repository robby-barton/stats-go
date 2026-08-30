package espn

// SetURLs overrides the client's endpoint URLs (schedule, game stats, team
// info, scoreboard). It exists so tests can point a single client at a mock
// HTTP server without touching any global state; each client carries its own
// URL set. It returns a restore function suitable for use with t.Cleanup().
// An empty argument leaves that URL unchanged.
func (c *Client) SetURLs(schedule, gameStats, teamInfo, scoreboard string) func() {
	orig := [4]string{c.scheduleURL, c.gameStatsURL, c.teamInfoURL, c.scoreboardURL}
	if schedule != "" {
		c.scheduleURL = schedule
	}
	if gameStats != "" {
		c.gameStatsURL = gameStats
	}
	if teamInfo != "" {
		c.teamInfoURL = teamInfo
	}
	if scoreboard != "" {
		c.scoreboardURL = scoreboard
	}
	return func() {
		c.scheduleURL, c.gameStatsURL, c.teamInfoURL, c.scoreboardURL = orig[0], orig[1], orig[2], orig[3]
	}
}
