# College Sports Computer Ranking

A Go application that pulls game data from the ESPN API, computes SRS/SOS
composite rankings for college football and basketball teams, and exports
results to DigitalOcean Spaces.

## Overview

The system consists of two CLI tools:

- **Ranker** — calculates and prints rankings for a given year/week
- **Updater** — scheduled service that fetches games, updates the database,
  computes rankings, and exports JSON to a CDN

Rankings use a composite algorithm based on Simple Rating System (SRS) and
Strength of Schedule (SOS). Football supports both FBS and FCS divisions;
basketball ranks D1 teams.

## Getting Started

### Prerequisites

- Go 1.26+
- golangci-lint (for linting)
- PostgreSQL (optional — SQLite is used automatically when no PG env vars are set)

### Setup

```sh
cp .env-sample .env   # configure database and DigitalOcean credentials
make modules          # sync Go module dependencies
```

### Environment Variables

Set in `.env` (see `.env-sample` for the full list):

| Variable | Description |
|----------|-------------|
| `PG_HOST`, `PG_PORT`, `PG_USER`, `PG_PASSWORD`, `PG_DBNAME`, `PG_SSLMODE` | PostgreSQL connection (omit all to use SQLite) |

## Usage

### Ranker

Generate and print a ranking. The ranker uses sport subcommands (`football`,
`basketball`):

```sh
make ranker OPTS="football"                # current football season, all teams
make ranker OPTS="football -t 25"          # top 25 football
make ranker OPTS="football -y 2024 -w 12"  # specific year and week
make ranker OPTS="football -f"             # rank FCS instead of FBS
make ranker OPTS="basketball"              # current basketball season, D1
make ranker OPTS="basketball -t 25"        # top 25 basketball
```

| Subcommand | Flag | Type | Default | Description |
|------------|------|------|---------|-------------|
| `football` | `-y` | int | most recent | Year to rank |
| | `-w` | int | most recent | Week of the season |
| | `-f` | bool | false | Rank FCS instead of FBS |
| | `-t` | int | all | Print only the top N teams |
| | `-r` | bool | false | Print SRS ratings instead of full ranking |
| `basketball` | `-y` | int | most recent | Year to rank |
| | `-w` | int | most recent | Week of the season |
| | `-t` | int | all | Print only the top N teams |
| | `-r` | bool | false | Print SRS ratings instead of full ranking |

### Updater

Run one-off operations or start the scheduled service. One-shot commands are
nested under sport subcommands (`football`, `basketball`). The `schedule`
command runs both sports:

```sh
make updater OPTS="schedule"                        # run scheduled service (both sports)
make updater OPTS="football games"                  # update current week's football games
make updater OPTS="football games --all"            # update all football games for current year
make updater OPTS="football games --single 12345"   # force-update a single game by ID
make updater OPTS="football ranking"                # update current football rankings
make updater OPTS="football ranking --all"          # update all football rankings
make updater OPTS="football teams"                  # update football team info
make updater OPTS="football season"                 # update football season info
make updater OPTS="football json"                   # rewrite current football JSON
make updater OPTS="football json --all"             # rewrite all football JSON
make updater OPTS="basketball games --all"          # update all basketball games
make updater OPTS="basketball ranking"              # update basketball rankings
```

| Subcommand | Command | Flags | Description |
|------------|---------|-------|-------------|
| `schedule` | | | Run as scheduled service (both sports) |
| `football` / `basketball` | `games` | `--all`, `--single <id>` | Update games (current week by default) |
| | `ranking` | `--all` | Update rankings (current season by default) |
| | `teams` | | Update team info from ESPN |
| | `season` | | Update season info |
| | `json` | `--all` | Rewrite JSON output (current season by default) |

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
  updater/            CLI: fetch games, update DB, export JSON
  migrate/            CLI: one-time migration from PostgreSQL to SQLite
internal/
  config/             Environment-based configuration (godotenv)
  database/           GORM models and DB initialization (Postgres + SQLite)
  espn/               ESPN API client (game schedules, stats, team info)
  game/               Game data parsing and stat extraction
  logger/             Structured logging (zap)
  ranking/            Ranking algorithm (SRS, SOS, composite scoring)
  team/               Team info parsing from ESPN
  updater/            Orchestration of DB updates and JSON export
  writer/             Output interface (DigitalOcean Spaces or local files)
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
