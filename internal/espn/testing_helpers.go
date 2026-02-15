package espn

// SetTestURLs overrides the package-level URL vars for testing.
// It returns a restore function suitable for use with t.Cleanup().
func SetTestURLs(scheduleURL, gameURL, teamURL string) func() {
	orig := [3]string{weekURL, gameStatsURL, teamInfoURL}
	weekURL = scheduleURL
	gameStatsURL = gameURL
	teamInfoURL = teamURL
	return func() { weekURL, gameStatsURL, teamInfoURL = orig[0], orig[1], orig[2] }
}

// SetTestScoreboardURL sets a test override on a Client's scoreboard URL.
// Returns a restore function suitable for use with t.Cleanup().
func SetTestScoreboardURL(c *Client, url string) func() {
	orig := c.scoreboardURL
	c.scoreboardURL = url
	return func() { c.scoreboardURL = orig }
}
