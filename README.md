# College Football Computer Ranking
A set of Go services for ranking college football teams.

## Development
### Setup
Run `make modules` to sync modules with `go.mod`.

### Environment
Copy `.env-sample` to `.env` and set the variables to the desired values.

The services are currently set up to only connect to PostgreSQL databases.

## Services
### Running and Building
Each of the services can be run locally using `make {ranker,updater}`.

When running the services locally you can pass command line arguments by appending
`OPTS="..."` to the make call: `make ranker OPTS="-t 25"`.

### Ranker
Ranker will generate a ranking for the year/week requested and print the results.

#### Options
| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `-y YEAR` | `int` | most recent | The year to rank |
| `-w WEEK` | `int` | most recent | The week of the season to rank |
| `-f` | `bool` | `false` | Rank the FCS |
| `-t N` | `int` | all | Print the top N teams |
| `-r` | `bool` | `false` | Print the SRS rating instead of full ranking |

### Updater
Updater will update the database with game information and new rankings.

Updater can run on-demand updates or run as a service and update on a schedule.

#### Options
| Option | Default | Description |
| --- | --- | --- |
| `-s` | `true` | Run as scheduler |
| `-g` | `false` | Update games now. Will run once and exit. |
| `-r` | `false` | Update current ranking now. Will run once and exit. |
| `-a` | `false` | Update all games or rankings. Use with `-g` or `-r`. |

#### Scheduler
The updater can run in scheduled mode which will wake up and search for new games finished every 5 minutes. If a new game is found it will add the game info to the database and update the current rankings.
