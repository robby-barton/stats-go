CREATE TABLE composite (
    team_id integer NOT NULL,
    year integer NOT NULL,
    average integer DEFAULT 0,
    rating real DEFAULT 0,
	PRIMARY KEY (team_id, year)
);


CREATE TABLE defensive_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    passes_def integer DEFAULT 0,
    qb_hurries integer DEFAULT 0,
    sacks real DEFAULT 0,
    solo_tackles integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    tackles_for_loss real DEFAULT 0,
    total_tackles real DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE fumble_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    fumbles integer DEFAULT 0,
    fumbles_lost integer DEFAULT 0,
    fumbles_rec integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE games (
    game_id integer NOT NULL,
    neutral boolean DEFAULT false,
    conf_game boolean DEFAULT false,
    season integer DEFAULT 0,
    week integer DEFAULT 0,
    postseason integer DEFAULT 0,
    home_id integer NOT NULL,
    away_id integer NOT NULL,
    retry integer DEFAULT 0,
    start_time timestamp with time zone,
    home_score integer DEFAULT 0,
    away_score integer DEFAULT 0,
	PRIMARY KEY (game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE interception_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    interceptions integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    int_yards integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE kick_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    fga integer DEFAULT 0,
    fgm integer DEFAULT 0,
    fg_long integer DEFAULT 0,
    xpa integer DEFAULT 0,
    xpm integer DEFAULT 0,
    points integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE passing_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    completions integer DEFAULT 0,
    attempts integer DEFAULT 0,
    yards integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    interceptions integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE players (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    year integer NOT NULL,
    name text NOT NULL,
    "position" text NOT NULL,
    rating integer DEFAULT 50,
    grade text NOT NULL,
    hometown text NOT NULL,
    status text NOT NULL,
	PRIMARY KEY (player_id, team_id, year)
);


CREATE TABLE punt_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    punt_long integer DEFAULT 0,
    punt_no integer DEFAULT 0,
    punt_yards integer DEFAULT 0,
    touchbacks integer DEFAULT 0,
    inside_20 integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE receiving_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    receptions integer DEFAULT 0,
    rec_yards integer DEFAULT 0,
    rec_long integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE recruiting (
    team_id integer NOT NULL,
    year integer NOT NULL,
    commits integer DEFAULT 0,
    rating real DEFAULT 0,
	PRIMARY KEY (team_id, year)
);


CREATE TABLE return_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    punt_kick text NOT NULL,
    return_no integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    ret_yards integer DEFAULT 0,
    ret_long integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id, punt_kick),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE roster (
    player_id integer NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    year integer NOT NULL,
    name text,
    num integer DEFAULT 0,
    "position" text,
    height integer DEFAULT 0,
    weight integer DEFAULT 0,
    grade text,
    hometown text,
	PRIMARY KEY (player_id, team_id, year)
);


CREATE TABLE rushing_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    carries integer DEFAULT 0,
    rush_yards integer DEFAULT 0,
    rush_long integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
	PRIMARY KEY (player_id, team_id, game_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE team_game_stats (
    game_id integer NOT NULL,
    team_id integer NOT NULL,
    score integer DEFAULT 0,
    drives integer DEFAULT 0,
    pass_yards integer DEFAULT 0,
    completions integer DEFAULT 0,
    completion_attempts integer DEFAULT 0,
    rush_yards integer DEFAULT 0,
    rush_attempts integer DEFAULT 0,
    first_downs integer DEFAULT 0,
    third_downs integer DEFAULT 0,
    third_downs_conv integer DEFAULT 0,
    fourth_downs integer DEFAULT 0,
    fourth_downs_conv integer DEFAULT 0,
    fumbles integer DEFAULT 0,
    interceptions integer DEFAULT 0,
    possession integer DEFAULT 0,
    penalties integer DEFAULT 0,
    penalty_yards integer DEFAULT 0,
	PRIMARY KEY (game_id, team_id),
	FOREIGN KEY (game_id) REFERENCES games(game_id) ON DELETE CASCADE
);


CREATE TABLE team_names (
    team_id integer NOT NULL,
    name text NOT NULL,
    flair text,
    abbreviation text,
    alt_color text,
    color text,
    display_name text,
    is_active boolean,
    is_allstar boolean,
    location text,
    logo text,
    logo_dark text,
    nickname text,
    short_display_name text,
    slug text,
	PRIMARY KEY (team_id)
);


CREATE TABLE team_seasons (
    team_id integer NOT NULL,
    year integer NOT NULL,
    fbs integer DEFAULT 0,
    power_five integer DEFAULT 0,
    conf text,
	PRIMARY KEY (team_id, year)
);


CREATE TABLE team_week_results (
    team_id integer NOT NULL,
    year integer NOT NULL,
    week integer NOT NULL,
    postseason integer DEFAULT 0 NOT NULL,
    final_rank integer DEFAULT 0,
    final_raw real DEFAULT 0,
    wins integer DEFAULT 0,
    losses integer DEFAULT 0,
    srs_rank integer DEFAULT 0,
    sos_rank integer DEFAULT 0,
    sov_rank integer DEFAULT 0,
    fbs boolean,
    name text,
    conf text,
    sol_rank integer DEFAULT 0,
    ties integer DEFAULT 0,
	PRIMARY KEY (team_id, year, week, postseason)
);


CREATE INDEX fbs_index ON team_week_results (fbs);


CREATE INDEX game_away_index ON games (away_id);


CREATE INDEX game_home_index ON games (home_id);


CREATE INDEX game_retry_index ON games (retry);


CREATE INDEX game_season_index ON games (season);


CREATE INDEX game_start_time_index ON games (start_time);


CREATE INDEX game_week_index ON games (week);


CREATE INDEX name_index ON team_names (name);


CREATE INDEX postseason_index ON team_week_results (postseason);


CREATE INDEX team_index ON team_week_results (team_id);


CREATE INDEX week_index ON team_week_results (week);


CREATE INDEX year_index ON team_week_results (year);


