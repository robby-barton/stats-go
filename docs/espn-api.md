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

### Game Stats / Play-by-Play

```
GET https://cdn.espn.com/core/college-football/playbyplay
    ?gameId={gameID}&xhr=1&render=false&userab=18
```

Returns detailed game info including box score, team stats, and player stats
for a single game.

**Response shape:** `GameInfoESPN`
- `GamePackage.Header` — game metadata (date, teams, scores, venue)
- `GamePackage.BoxScore.Teams` — team-level statistics
- `GamePackage.BoxScore.Players` — player-level stat categories

**Used by:** `GetGameStats`

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

ESPN tests override the package-level URL vars to point at a local
`httptest.Server` that serves fixture JSON. This avoids hitting the real API
in tests while validating the full parsing pipeline.
