# College Sports Computer Ranking

A Go application that pulls game data from the ESPN API and computes SRS/SOS
composite rankings for college football and basketball teams
(men's and women's).

## Overview

The system consists of two CLI tools:

- **Ranker** — calculates and prints rankings for a given year/week
- **Updater** — scheduled service that fetches games, updates the database,
  and computes rankings

Rankings use a composite algorithm based on Simple Rating System (SRS) and
Strength of Schedule (SOS). Football supports both FBS and FCS divisions;
basketball (men's and women's) ranks D1 teams only.

## Getting Started

### Prerequisites

- Go 1.26+
- golangci-lint (for linting)
- PostgreSQL (optional — SQLite is used automatically when no PG env vars are set)

### Setup

```sh
cp .env-sample .env   # configure database credentials
make modules          # sync Go module dependencies
```

### Environment Variables

Set in `.env` (see `.env-sample` for the full list):

| Variable | Description |
|----------|-------------|
| `PG_HOST`, `PG_PORT`, `PG_USER`, `PG_PASSWORD`, `PG_DBNAME`, `PG_SSLMODE` | PostgreSQL connection (omit all to use SQLite) |
| `DEPLOY_SCRIPT` | Path to a script run after each ranking update (optional) |

## Usage

### Ranker

Generate and print a ranking. The ranker uses sport subcommands (`ncaaf`,
`ncaam`, `ncaaw`):

```sh
make ranker OPTS="ncaaf"                # current football season, all teams
make ranker OPTS="ncaaf -t 25"          # top 25 football
make ranker OPTS="ncaaf -y 2024 -w 12"  # specific year and week
make ranker OPTS="ncaaf -f"             # rank FCS instead of FBS
make ranker OPTS="ncaam"                # current basketball season, D1
make ranker OPTS="ncaam -t 25"          # top 25 basketball
make ranker OPTS="ncaaw"                # current women's basketball season, D1
```

| Subcommand | Flag | Type | Default | Description |
|------------|------|------|---------|-------------|
| `ncaaf` | `-y` | int | most recent | Year to rank |
| | `-w` | int | most recent | Week of the season |
| | `-f` | bool | false | Rank FCS instead of FBS |
| | `-t` | int | all | Print only the top N teams |
| | `-r` | bool | false | Print SRS ratings instead of full ranking |
| `ncaam` | `-y` | int | most recent | Year to rank |
| | `-w` | int | most recent | Week of the season |
| | `-t` | int | all | Print only the top N teams |
| | `-r` | bool | false | Print SRS ratings instead of full ranking |
| `ncaaw` | `-y` | int | most recent | Year to rank |
| | `-w` | int | most recent | Week of the season |
| | `-t` | int | all | Print only the top N teams |
| | `-r` | bool | false | Print SRS ratings instead of full ranking |

The `ncaaw` command ranks the same single D-I division with the same
basketball parameters as `ncaam` (25-game SRS quota, one season of lookback,
25/60/15 record/SRS/SOS weights) on women's ESPN data. A women's regular
snapshot covers completed games through the named ESPN week, ending at the
next Monday midnight Eastern. A missing week is an error, not an alias for
the latest ranking. Postseason results update one Final per season.

### Offline NCAAF replay

To check that an explicit-cutoff replay matches the default FBS ranking
calculation on a local PostgreSQL fixture, use `cmd/ncaaf-replay`. It does
not publish rankings or fetch game or poll data. See the
[setup and parity guide](docs/ncaaf-readonly-replay.md).

### Updater

Run one-off operations or start the scheduled service. One-shot commands are
nested under sport subcommands (`ncaaf`, `ncaam`, `ncaaw`). The `schedule`
command runs football and men's basketball. Women's basketball is available
for manual validation only; automatic polling, historical coverage checks,
and public rankings are still pending. Use ESPN's season-ending year for
basketball (`2027` means 2026–27):

```sh
make updater OPTS="schedule"                      # schedule football and men's basketball
make updater OPTS="ncaaf games"                   # update current week's football games
make updater OPTS="ncaaf games --all"             # update all football games for current year
make updater OPTS="ncaaf games --single 12345"    # force-update a single game by ID
make updater OPTS="ncaaf ranking"                 # update current football rankings
make updater OPTS="ncaaf ranking --all"           # update all football rankings
make updater OPTS="ncaaf teams"                   # update football team info
make updater OPTS="ncaaf season"                  # update football season info
make updater OPTS="ncaam games --all"             # update all basketball games
make updater OPTS="ncaam ranking"                 # update basketball rankings
make updater OPTS="ncaam backfill --from 2021 --to 2025"  # backfill a year range
make updater OPTS="ncaaw teams"                   # import women's team metadata
make updater OPTS="ncaaw season --year 2027"       # import 2026–27 D-I team seasons
make updater OPTS="ncaaw games --year 2027"       # import completed 2026–27 women's games
make updater OPTS="ncaaw ranking"                 # compute the latest women's ranking
```

| Subcommand | Command | Flags | Description |
|------------|---------|-------|-------------|
| `schedule` | | `--fbs-early-season-prior` | Run as scheduled service (football and men's basketball); the sole prior flag is opt-in (default off) and only affects scheduled football rankings — see [the early-season prior document](docs/fbs-early-season-prior.md) |
| `ncaaf` / `ncaam` / `ncaaw` | `games` | `--all`, `--single <id>`, `--year <y>` | Update games (current week by default) |
| | `ranking` | `--all`, `--fbs-early-season-prior` | Update rankings (current season by default; the prior flag requires `--all` and is football-only) |
| | `teams` | | Update team info from ESPN |
| | `season` | | Update season info |
| | `backfill` | `--from`, `--to` | Backfill seasons, games, and rankings for a year range |
| `ncaaw` | `backfill` | `--publish-from` (optional) | Earliest verified year to publish (defaults to `--to`) |

### Verified women's basketball backfill

```sh
make updater OPTS="ncaaw backfill --from 2026 --to 2027"
```

This audits completed games and D-I team metadata for both seasons, then
publishes **only 2026–27**. The prior season is loaded for the model's lookback,
not published. To include an archive, set `--publish-from` and include the
preceding lookback season in `--from` (for example, `--from 2025 --to 2027
--publish-from 2026`). Years outside the publication range are not modified.

A failed audit leaves existing rankings in place; successfully imported games
and metadata can remain for the next attempt. Start on a disposable database,
allow time for the rate-limited game summaries, and don't run another women's
import or ranking update concurrently. Use the backfill, not the one-off
`ranking` command, for a verified publication. See the
[launch plan](docs/ncaaw-launch-plan.md) for release checks.

### FBS early-season prior (opt-in, default off)

By default the updater publishes exact baseline rankings. With the sole
`--fbs-early-season-prior` flag, the `schedule` command automatically
freezes one validated FBS preseason prior per season (stored immutably in
the `fbs_season_priors` table, checksum-verified on every read) during the
bounded August 10–16 Eastern window — including automatic retries after the
annual football season initialization — and blends it into early-season
scheduled football rankings only. The anchor is always midnight Eastern on
August 10 of the season year (the fixed policy anchor), with a full bypass
56 Eastern calendar days later.

```sh
updater schedule --fbs-early-season-prior
```

The same flag also opts the one-shot football ranking command into the same
policy: `updater ncaaf ranking --all --fbs-early-season-prior` rebuilds all
rankings while reconstructing one prior freshly in memory per season in scope
(including the current season) with exactly the scheduled freeze method —
stored prior values are never used as inputs. Because the rebuild is the
authoritative rerun, it then publishes the freshly computed priors over the
stored rows (replacing existing rows, current season included, midseason
allowed) atomically with the rebuilt ranking upserts, and invalidates the
stale prior of any season it cannot reconstruct; any write failure rolls the
whole publication back. Because it recomputes from current stored data, its
numbers can differ from any hypothetical historical snapshot when team
metadata or game data has been revised; and `ranking --all` (with or without
the flag) can overwrite rows the scheduler blended earlier. Plain
`ncaaf ranking` and backfill recomputation stay at the exact baseline.

See [docs/fbs-early-season-prior.md](docs/fbs-early-season-prior.md) for the
automatic freeze policy, the stored-artifact contract, baseline fallback,
and how to disable it. Without the flag no prior job is registered, the
stored table is never read or written, and nothing changes.

## Development

```sh
make ranker           # build and run ranker
make updater          # build and run updater
go test ./...         # run all tests
make lint             # run golangci-lint
make format           # go fmt
```

## Project Structure

```
cmd/
  ranker/             CLI: calculate and print rankings
  updater/            CLI: fetch games, update DB, compute rankings
  migrate/            CLI: one-time migration from PostgreSQL to SQLite
internal/
  config/             Environment-based configuration (godotenv)
  database/           GORM models and DB initialization (Postgres + SQLite)
  espn/               ESPN API client (game schedules, stats, team info)
  game/               Game data parsing and stat extraction
  logger/             Structured logging (zap)
  ranking/            Ranking algorithm (SRS, SOS, composite scoring)
  team/               Team info parsing from ESPN
  updater/            Orchestration of DB updates and ranking computation
```

## Deployment

The project uses a Docker multi-stage build to produce a minimal Alpine image
running the updater in scheduled mode.

```sh
make local-deploy     # build and run via docker compose
```

In production, the container runs `updater schedule`, which polls for completed
games during each sport's season (football: Aug–Jan, basketball: Nov–Apr).

## CI/CD

GitHub Actions runs two workflows:

- **Pull Requests** — lint and test on PRs targeting master
- **Deploy** — lint, test, build Docker image, and push to DigitalOcean
  Container Registry on merge to master
