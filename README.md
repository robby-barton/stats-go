# College Football Computer Ranking

A Go application that pulls game data from the ESPN API, computes SRS/SOS
composite rankings for college football teams, and exports results to
DigitalOcean Spaces.

## Overview

The system consists of two CLI tools:

- **Ranker** — calculates and prints rankings for a given year/week
- **Updater** — scheduled service that fetches games, updates the database,
  computes rankings, and exports JSON to a CDN

Rankings use a composite algorithm based on Simple Rating System (SRS) and
Strength of Schedule (SOS), supporting both FBS and FCS divisions.

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

Generate and print a ranking:

```sh
make ranker                      # current season, all teams
make ranker OPTS="-t 25"         # top 25
make ranker OPTS="-y 2024 -w 12" # specific year and week
make ranker OPTS="-f"            # rank FCS instead of FBS
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-y` | int | most recent | Year to rank |
| `-w` | int | most recent | Week of the season |
| `-f` | bool | false | Rank FCS instead of FBS |
| `-t` | int | all | Print only the top N teams |
| `-r` | bool | false | Print SRS ratings instead of full ranking |

### Updater

Run one-off operations or start the scheduled service:

```sh
make updater OPTS="schedule"              # run as scheduled service
make updater OPTS="games"                 # update current week's games
make updater OPTS="games --all"           # update all games for current year
make updater OPTS="games --single 12345"  # force-update a single game by ID
make updater OPTS="ranking"               # update current season rankings
make updater OPTS="ranking --all"         # update all rankings
make updater OPTS="teams"                 # update team info
make updater OPTS="season"               # update season info
make updater OPTS="json"                  # rewrite current season JSON
make updater OPTS="json --all"            # rewrite all JSON
```

| Command | Flags | Description |
|---------|-------|-------------|
| `schedule` | | Run as a scheduled service (polls every 5 min Aug-Jan) |
| `games` | `--all`, `--single <id>` | Update games (current week by default) |
| `ranking` | `--all` | Update rankings (current season by default) |
| `teams` | | Update team info from ESPN |
| `season` | | Update season info |
| `json` | `--all` | Rewrite JSON output (current season by default) |

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
games every 5 minutes during the season (August through January).

## CI/CD

GitHub Actions runs two workflows:

- **Pull Requests** — lint and test on PRs targeting master
- **Deploy** — lint, test, build Docker image, and push to DigitalOcean
  Container Registry on merge to master
