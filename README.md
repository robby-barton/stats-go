# College Football Computer Ranking
A set of Go services for ranking college football teams.

## Development
### Setup
Run `make modules` to sync modules with `go.mod`.

### Environment
Copy `.env-sample` to `.env` and set the variables to the desired values.

The services are currenlty set up to only connect to Postgres databases.

## Services
### Running and Building
Each of the services can be run locally using `make run-{ranker,updater,server}` and their
binaries can be built with `make {ranker,updater,server}`.

When running the services with `make run-*` you can pass command line arguments by appending
`OPTS="..."` to the make call: `make run-ranker OPTS="-t 25"`.

### Ranker
Ranker will generate a ranking for the year/week requested and print the results.

#### Options
The ranker has the following options:
| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `-y YEAR` | `int` | most recent | The year to rank |
| `-w WEEK` | `int` | most recent | The week of the season to rank |
| `-f` | `bool` | `false` | Rank the FCS |
| `-t N` | `int` | all | Print the top N teams |
| `-r` | `bool` | `false` | Print the SRS rating instead of full ranking |

### Updater
Updater will update the database with missing game information.

Updater can run on-demand updates or run as a service and update on a schedule.

#### Options
| Option | Default | Description |
| --- | --- | --- |
| `-s` | `true` | Run as scheduler |
| `-g` | `false` | Update games now. Will run once and exit. |

### Server
Server is an API server for a pending site to host rankings.
