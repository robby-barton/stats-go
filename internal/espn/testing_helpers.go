package espn

// SetURLs overrides the client's endpoint URLs (game stats, team info,
// scoreboard, conferences). It exists so tests can point a single client at a
// mock HTTP server without touching any global state; each client carries its
// own URL set. It returns a restore function suitable for use with
// t.Cleanup(). An empty argument leaves that URL unchanged.
func (c *Client) SetURLs(gameStats, teamInfo, scoreboard, conferences string) func() {
	orig := [4]string{c.gameStatsURL, c.teamInfoURL, c.scoreboardURL, c.conferenceURL}
	if gameStats != "" {
		c.gameStatsURL = gameStats
	}
	if teamInfo != "" {
		c.teamInfoURL = teamInfo
	}
	if scoreboard != "" {
		c.scoreboardURL = scoreboard
	}
	if conferences != "" {
		c.conferenceURL = conferences
	}
	return func() {
		c.gameStatsURL, c.teamInfoURL, c.scoreboardURL = orig[0], orig[1], orig[2]
		c.conferenceURL = orig[3]
	}
}
