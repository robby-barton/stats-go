# ESPN API Reference

ESPN does not publish official documentation for these endpoints. They are
reverse-engineered from ESPN's web frontend and may change without notice.

## Endpoints

### Game Schedule

```
GET https://cdn.espn.com/core/college-football/schedule
    ?xhr=1&render=false&userab=18
    [&year={year}]
    [&week={week}]
    [&group={group}]
    [&seasonType={seasonType}]
```

Returns the schedule for a given week/year. Without parameters, returns the
current week.

**Response shape:** `GameScheduleESPN`
- `Content.Schedule` — map of dates to game lists
- `Content.Calendar` — season calendar with week boundaries
- `Content.Calendar[1].StartDate` — postseason start date
- `Content.Defaults.Year` — current default season year
- `Content.ConferenceAPI.Conferences` — conference metadata

**Used by:** `GetCurrentWeekGames`, `GetGamesByWeek`, `GetCompletedGamesByWeek`,
`GetWeeksInSeason`, `HasPostseasonStarted`, `DefaultSeason`, `ConferenceMap`,
`TeamConferencesByYear`

### Game Schedule (cdn.espn.com)

```
GET https://cdn.espn.com/core/college-football/schedule
    ?xhr=1&render=false&userab=18
    [&year={year}]
    [&week={week}]
    [&group={group}]
    [&seasonType={seasonType}]
```

Returns the schedule for a given week/year. Without parameters, returns the
current week. Note: cdn.espn.com intermittently serves empty-body HTTP 202
bot challenges; the HTTP client retries those (see `request.go`).

**Response shape:** `GameScheduleESPN`
- `Content.Schedule` — map of dates to game lists
- `Content.Calendar` — season calendar with week boundaries
- `Content.Calendar[1].StartDate` — postseason start date
- `Content.Defaults.Year` — current default season year
- `Content.ConferenceAPI.Conferences` — conference metadata

**Used by:** `GetGamesByWeek`, `GetCompletedGamesByWeek`, `GetWeeksInSeason`,
`HasPostseasonStarted`, `DefaultSeason`, `ConferenceMap`, `TeamConferencesByYear`

### Scoreboard (site.api.espn.com)

```
GET https://site.api.espn.com/apis/site/v2/sports/football/college-football/scoreboard
    ?groups={group}
```

Returns the games for the current week (the endpoint ignores year/week
navigation for live data and embeds the current `season.year` and
`week.number` directly). The cdn-style `group` parameter is ignored; the
division filter uses `groups` (verified for FBS=80 / FCS=81).

**Response shape:** `SiteScoreboardESPN`
- `Events` — game list with embedded `status.type` completion flags
- `Events[].Competitions[].Competitors` — same field names as the cdn
  schedule (`id`, `homeAway`, `score`, `team.id`, `team.conferenceId`)
- `Season` / `Week` — current season year/type and week number

**Used by:** `FootballClient.GetCurrentWeekGames` (via `finalGames`, which
applies the same STATUS_FINAL filter as the cdn path)

### Game Stats / Box Score (site.api.espn.com)

```
GET https://site.api.espn.com/apis/site/v2/sports/football/college-football/summary
    ?event={eventID}
```

Returns detailed game info including box score, team stats, and player stats
for a single game. The summary carries the same `header`/`boxscore` objects
as the cdn playbyplay response but at the top level instead of wrapped in
`gamepackageJSON`; `GameInfoESPN.UnmarshalJSON` accepts both shapes, so the
game parser is source-blind.

**Response shape:** `GameInfoESPN`
- `GamePackage.Header` — game metadata (date, teams, scores, week, season)
- `GamePackage.BoxScore.Teams` — team-level statistics
- `GamePackage.BoxScore.Players` — player-level stat categories

**Used by:** `GetGameStats` (football). Basketball box scores still use the
cdn playbyplay endpoint (see below).

### Game Stats / Play-by-Play (cdn.espn.com, basketball)

```
GET https://cdn.espn.com/core/mens-college-basketball/playbyplay
    ?gameId={gameID}&xhr=1&render=false&userab=18
```

Same data as the site.api summary, wrapped in `gamepackageJSON`. Still used
by basketball; football moved to the site.api summary (2026-09).

### Team Info

```
GET https://site.api.espn.com/apis/site/v2/sports/football/college-football/teams
    ?limit=1000
```

Returns metadata for all college football teams.

**Response shape:** `TeamInfoESPN`
- Team ID, abbreviation, display name, nickname
- Colors (primary, alternate)
- Logos (light and dark variants, max 2 per team)
- Location info

**Used by:** `GetTeamInfo`

## Division Group IDs

| Division | Group ID | Constant |
|----------|----------|----------|
| FBS      | 80       | `espn.FBS` |
| FCS      | 81       | `espn.FCS` |
| D-II     | 57       | `espn.DII` |
| D-III    | 58       | `espn.DIII` |

## Season Types

| Type       | Value | Constant |
|------------|-------|----------|
| Regular    | 2     | `espn.Regular` |
| Postseason | 3     | `espn.Postseason` |

## Game Completion Filter

Only games with `Status.StatusType.Completed == true` AND
`Status.StatusType.Name == "STATUS_FINAL"` are considered complete.

## Testing Pattern

ESPN tests build a client via `newTestClient()`-style helpers and point it at
a local `httptest.Server` with `Client.SetURLs`, serving fixture JSON from
the same paths the production URLs use (including trimmed real site.api
responses in `site_testdata_test.go`). This avoids hitting the real API in
tests while validating the full parsing pipeline.
