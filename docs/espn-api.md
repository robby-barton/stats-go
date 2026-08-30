# ESPN API Reference

ESPN does not publish official documentation for these endpoints. They are
reverse-engineered from ESPN's web frontend and may change without notice.
Last live-verified: 2026-08-29/30 (site.api range behavior re-verified
2026-08-30 — see the scoreboard section).

## The Two Hosts

ESPN data comes from two hosts with different bot defenses and different
quirks. Every request goes through `makeRequest` (`internal/espn/request.go`),
which picks the User-Agent per host.

| | site.api.espn.com | cdn.espn.com |
|---|---|---|
| Endpoints | scoreboard (week games, calendar via `?dates=`, date ranges), `scoreboard/conferences`, `summary`, `teams` | `schedule` (per-week), `playbyplay` (basketball box scores) |
| Used for | current-week games, season metadata, conference maps, football box scores, football team→conference walk | historical week fetching, basketball schedule/box scores, basketball team→conference extraction |
| User-Agent | plain client UA (`curl/8.5.0`). **403s requests that claim to be a browser** | current Chrome UA. **Serves empty-body HTTP 202 bot challenges** to old browser UAs; the HTTP client retries those |
| Known hazards | 200-event response cap with silent chronological truncation; date ranges that **start on a Sunday** return degenerate subsets (see Scoreboard) | intermittent 202 challenges; no bulk date-range support (one request = one week) |

The 202-challenge behavior on cdn and the per-host User-Agent split were both
determined empirically 2026-08-29; the site.api range findings below were
characterized with curl on 2026-08-30 across seasons 2021-2025.

## Endpoints

### Scoreboard (site.api.espn.com)

```
GET https://site.api.espn.com/apis/site/v2/sports/football/college-football/scoreboard
    [&groups={group}]
    [&dates={year}]
    [&dates={yyyymmdd}-{yyyymmdd}]
```

Returns the games for the current week (the endpoint ignores year/week
navigation for live data and embeds the current `season.year` and
`week.number` directly). The cdn-style `group` parameter is ignored; the
division filter uses `groups` (verified for FBS=80 / FCS=81).

Passing `dates={year}` additionally returns the season calendar in
`leagues[0].calendar`. Passing a `dates={start}-{end}` range returns the
events in that window and is how `TeamConferencesByYear` walks a season.

**Verified payload facts (2026-08-29/30):**

- Without a `dates` parameter the calendar field is JSON **null** — even
  mid-season. Any code that needs the calendar must pass `dates`.
- The football calendar is a list of season-type objects
  (`{label, value, startDate, endDate, entries: [{label, value, ...}]}`),
  while the basketball calendar is a flat list of ISO date strings. The two
  shapes decode into separate types (`SiteScoreboardLeague.Calendar` vs
  `ScoreboardLeague.Calendar`).
- The Regular Season entry (`value: "2"`) lists one entry per week numbered
  1..N; the Postseason entry (`value: "3"`) carries its own start date and
  non-week-numbered entries (Bowls = 1, CFP = 999).
- **200-event cap:** range responses are hard-capped at 200 events with
  silent chronological truncation — a range containing more games simply
  loses its earliest games, with no error or indicator. Weekly chunks (55-90
  games each) stay safely under the cap; do not widen them into multi-week
  ranges.
- **Sunday-start hazard:** any range that *starts* on a Sunday returns a
  degenerate subset (0-10 events instead of the 53-89 expected),
  deterministically, across all seasons 2021-2025. Regular-season calendar
  starts are Saturday-anchored, but the postseason calendar starts on a
  Sunday in 2022, 2024, and 2026 — `weekChunks`
  (`internal/espn/football.go`) shifts Sunday chunk starts to Monday for
  exactly this reason.
- Ranges crossing the regular/postseason boundary return both season types
  cleanly. Single-day fetches are **not** supersets of ranges (the two
  shapes differ), so don't mix the two forms when comparing behavior.
- **Recommendation:** fetch weekly ranges that are never Sunday-anchored
  (Saturday or Monday starts), one chunk at a time.

**Response shape:** `SiteScoreboardESPN`
- `Leagues[0].Season.Year` — current season year (`DefaultSeason`)
- `Leagues[0].Calendar` — object-shaped season calendar (`dates` param only)
- `Events` — game list with embedded `status.type` completion flags
- `Events[].Competitions[].Competitors` — same field names as the cdn
  schedule (`id`, `homeAway`, `score`, `team.id`, `team.conferenceId`)
- `Season` / `Week` — current season year/type and week number

**Used by:** `FootballClient.GetCurrentWeekGames` (via `finalGames`, which
applies the same STATUS_FINAL filter as the cdn path), `DefaultSeason` /
`GetWeeksInSeason` / `HasPostseasonStarted` (football season metadata),
`TeamConferencesByYear` (football team→conference walk over weekly date
ranges), `GetScoreboard` + `GetSeasonDatesForYear` (basketball season
metadata and current-season date navigation)

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
current week. One request covers a whole week for a division group, which is
why historical week fetching still uses this endpoint — a week would fit in
one site.api range request, but that endpoint's 200-event cap (silent
truncation) and Sunday-start hazard make silent data loss a real risk, and
the cdn response carries the calendar/conferenceAPI blocks `GameScheduleESPN`
exposes (see the Scoreboard section for the site.api range hazards).

**Response shape:** `GameScheduleESPN`
- `Content.Schedule` — map of dates to game lists
- `Content.Calendar` — season calendar with week boundaries
- `Content.ConferenceAPI.Conferences` — conference metadata

**Used by:** `GetGamesByWeek`, `GetCompletedGamesByWeek`,
`GetGamesBySeason` (football historical backfill), and basketball
`TeamConferencesByYear` (team→conference extraction over all game dates of a
season — no site.api endpoint provides a bulk team→conference mapping, see
the Conferences section)

### Conferences (site.api.espn.com)

```
GET https://site.api.espn.com/apis/site/v2/sports/{sport-path}/scoreboard/conferences
    ?groups={group}
```

Returns the conference list of one division group. The plain endpoint returns
**FBS only** — every other group (FCS 81, DII 57, DIII 58, basketball D1 50)
requires an explicit `groups` value (verified live 2026-08-29: FBS returns 11
conferences, FCS 14, DII 17, DIII 30, D1 32). The division-root entry (e.g.
FBS itself) has no `parentGroupId`; child conferences carry their parent as a
string. IDs are JSON strings.

**Response shape:** `ConferencesESPN`
- `Conferences[].groupId` / `parentGroupId` / `shortName` / `name`

**Used by:** `ConferenceMap` (both sports). The endpoint reflects the
current season only — same limitation the cdn path had. It does not cover
the team→conference mapping (`TeamConferencesByYear`): the `/teams` rows
carry no `conferenceId`. Football extracts it from weekly site.api scoreboard
date ranges (see the Scoreboard section); basketball still extracts it from
the cdn schedule, whose calendar dates are flat strings rather than the
week-entry objects football's walk relies on (see docs/tech-debt.md).

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
