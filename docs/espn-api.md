# ESPN API Reference

ESPN does not publish official documentation for these endpoints. They are
reverse-engineered from ESPN's web frontend and may change without notice.
Last live-verified: 2026-08-29/30 (site.api range behavior re-verified
2026-08-30 — see the scoreboard section).

## The Two Hosts

ESPN data comes from one host. cdn.espn.com (the former schedule/playbyplay
host) was retired on 2026-08-30 after it intermittently served empty-body
202 bot challenges to automated clients (the 2026-08-29 incident); every
endpoint now lives on site.api.espn.com, which answers plain curl-style
User-Agents cleanly. Every request goes through `makeRequest`
(`internal/espn/request.go`), which still sends a curl-style User-Agent
(site.api 403s requests that CLAIM to be a browser) and retries 5xx/202
responses with backoff.

| | site.api.espn.com (only host) |
|---|---|
| Endpoints | scoreboard (week games, season calendar via `?dates=`, single-day + date-range fetches), `scoreboard/conferences`, `summary`, `teams` |
| User-Agent | plain client UA (`curl/8.5.0`). **403s requests that claim to be a browser** |
| Known hazards | 200-event response cap with silent chronological truncation; date ranges that **start on a Sunday** return degenerate subsets; **date ranges are lossy against single-day fetches** (see Scoreboard) |

The 202-challenge behavior on the old cdn host and the per-host User-Agent
split were both determined empirically 2026-08-29; the site.api range
findings below were characterized with curl on 2026-08-30 across seasons
2021-2025, and the basketball-specific facts (groups filter, conferenceId,
flat calendar, summary shape) were verified live 2026-08-30.

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
`leagues[0].calendar`. Passing a `dates={start}-{end}` range returns a
window of events (how `TeamConferencesByYear` walks a season) — but see the
lossiness hazard below; bulk fetching that must be complete uses one
single-day request per calendar day instead (`GetGamesBySeason`).

**Verified payload facts (2026-08-29/30):**

- Without a `dates` parameter the **football** calendar field is JSON
  **null** — even mid-season. Any code that needs the football calendar must
  pass `dates`. The **basketball** calendar, by contrast, is a flat list of
  ISO date strings and is present even without a `dates` parameter (verified
  2026-08-30).
- The football calendar is a list of season-type objects
  (`{label, value, startDate, endDate, entries: [{label, value, ...}]}`),
  while the basketball calendar is a flat list of ISO date strings. The two
  shapes decode into separate types (`SiteScoreboardLeague.Calendar` vs
  `ScoreboardLeague.Calendar`); `SiteScoreboardLeague`'s calendar decode
  (SiteCalendar) tolerates both shapes, treating the flat date-string list
  as nil.
- The Regular Season entry (`value: "2"`) lists one entry per week numbered
  1..N; the Postseason entry (`value: "3"`) carries its own start date and
  non-week-numbered entries (Bowls = 1, CFP = 999).
- **The `groups` parameter is honored for basketball too** (verified live
  2026-08-30: `?dates=20260117&groups=50` returns 145 D1 games where the
  plain date fetch returns a 21-event subset). Without it the basketball
  scoreboard under-reports, so every basketball scoreboard request carries
  `groups=50`.
- **Basketball competitors carry `team.conferenceId`** (verified live
  2026-08-30 across 2025-26 and 2026-27 payloads; 289 of 290 competitors on
  2026-01-17 had it — the one exception was a non-D1 fill-in, Bethesda
  Flames, whose missing ID decodes to 0 and is skipped). This is what the
  basketball `TeamConferencesByYear` walk consumes.
- **The plain basketball scoreboard's `leagues[0].season` can already point
  at the NEXT season** before the new one starts (verified 2026-08-30: the
  plain payload advertised season 2027 with 2026-27 preview events while the
  2025-26 season had just ended). Code that derives "current season" from
  the plain payload sees ESPN's flip, not the last completed season.
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
- **Range responses are lossy against single-day fetches** (discovered
  2026-08-30, football backfill parity check, 2025 season). Even far below
  the 200-event cap, a multi-day range drops games the single-day fetch for
  the same date returns: tail-day evening games vanish (Friday-night games
  dated 22:00Z–02:30Z are absent from a range ending that Friday), and some
  ranges silently lose whole tail days (Aug30–Sep05 returned nothing dated
  after Sep02; Aug29–30 returned the 11 Aug29 games but none of Aug30's
  62). A full range-walk of the 2025 season found 1602 of the 1697
  cdn-ingested games (~5.6% silently lost). **Single-day fetches are the
  only verified-complete form** — each covers exactly one ET calendar day
  (e.g. `dates=20250829` returns games from 22:00Z Aug29 through 02:30Z
  Aug30, i.e. Friday evening ET, which no range containing that Friday
  returns). `GetGamesBySeason` therefore walks the season one day at a
  time; `TeamConferencesByYear` still uses weekly ranges (tolerable for
  conference extraction, see docs/tech-debt.md).
- **Recommendation:** for complete historical data, fetch one single-day
  scoreboard per calendar day of the season spans. If using ranges (e.g.
  for team/conference extraction), never Sunday-anchor them (Saturday or
  Monday starts), keep them weekly, and verify against single-day fetches
  before trusting completeness.

**Response shape:** `SiteScoreboardESPN`
- `Leagues[0].Season.Year` — current season year (`DefaultSeason`)
- `Leagues[0].Calendar` — object-shaped season calendar (`dates` param only;
  basketball's flat date-string list decodes to nil here)
- `Events` — game list with embedded `status.type` completion flags
- `Events[].Competitions[].Competitors` — competitor fields (`id`,
  `homeAway`, `score`, `team.id`, `team.conferenceId`)
- `Season` / `Week` — current season year/type and week number (absent from
  `?dates=` payloads, present — with basketball's flat calendar — on plain
  basketball payloads)

**Used by:** `FootballClient.GetCurrentWeekGames` and
`BasketballClient.GetCurrentWeekGames` (both via `finalGames`, which applies
the STATUS_FINAL filter; basketball walks today+yesterday one ET day at a
time), `DefaultSeason` / `GetWeeksInSeason` / `HasPostseasonStarted`
(season metadata, both sports), `GetGamesBySeason` (both sports' season
backfills — per-day walks with the `groups` param; football over the
calendar spans, basketball over the game-date list), `TeamConferencesByYear`
(both sports' team→conference extraction), `GetScoreboard` +
`GetSeasonDatesForYear` (basketball season metadata and current-season date
navigation)

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
current season only — same limitation the retired cdn path had. It does not
cover the team→conference mapping (`TeamConferencesByYear`): the `/teams`
rows carry no `conferenceId`. Both sports extract that from the scoreboard:
football from weekly site.api scoreboard date ranges, basketball from a
per-day walk (each competitor carries `team.conferenceId`).

### Game Stats / Box Score (site.api.espn.com, both sports)

```
GET https://site.api.espn.com/apis/site/v2/sports/{sport-path}/summary
    ?event={eventID}
```

Returns detailed game info including box score, team stats, and player stats
for a single game. Football moved here 2026-09 and basketball 2026-08-30
(after cdn.espn.com began serving empty-body 202 bot challenges); the
summary carries the same `header`/`boxscore` objects the retired cdn
playbyplay response wrapped in `gamepackageJSON`, so `GameInfoESPN` decodes
one shape for both sports.

**Response shape:** `GameInfoESPN`
- `Header` — game metadata (ID, date, teams, scores, week, season)
- `Boxscore.Teams` — team-level statistics (name/label/displayValue)
- `Boxscore.Players` — player-level stat categories (labels, totals,
  athletes with per-player stats)

**Verified basketball facts (live 2026-08-30, 2025-26 and 2024-25 games):**

- Every field the game parsers consume is present: header ID, season
  (year/type — the game's OWN season, not the current one), week,
  competition date/conferenceCompetition/neutralSite, competitor
  homeAway/id/score, and boxscore team statistics.
- Player-stat category `name` is JSON **null** on basketball payloads
  (labels/keys carry the column names instead). Harmless: the game parser
  only switches on category names for football (`parsePlayerStats` is
  football-only).
- Historical events (e.g. 401708333, January 2025) remain fetchable.

**Used by:** `GetGameStats` (both sports).

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
responses in `site_testdata_test.go` for both sports — football week-1 and
basketball 2025-26 captures). This avoids hitting the real API in tests
while validating the full parsing pipeline.
