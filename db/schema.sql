--
-- PostgreSQL database dump
--

-- Dumped from database version 14.10
-- Dumped by pg_dump version 15.6 (Ubuntu 15.6-0ubuntu0.23.10.1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: postgres
--

-- *not* creating schema, since initdb creates it


ALTER SCHEMA public OWNER TO postgres;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: composite; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.composite (
    team_id integer NOT NULL,
    year integer NOT NULL,
    average integer DEFAULT 0,
    rating real DEFAULT 0
);


ALTER TABLE public.composite OWNER TO stats;

--
-- Name: defensive_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.defensive_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    passes_def integer DEFAULT 0,
    qb_hurries integer DEFAULT 0,
    sacks real DEFAULT 0,
    solo_tackles integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    tackles_for_loss real DEFAULT 0,
    total_tackles real DEFAULT 0
);


ALTER TABLE public.defensive_stats OWNER TO stats;

--
-- Name: fumble_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.fumble_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    fumbles integer DEFAULT 0,
    fumbles_lost integer DEFAULT 0,
    fumbles_rec integer DEFAULT 0
);


ALTER TABLE public.fumble_stats OWNER TO stats;

--
-- Name: games; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.games (
    game_id integer NOT NULL,
    neutral boolean DEFAULT false,
    conf_game boolean DEFAULT false,
    sport text DEFAULT 'ncaaf',
    season integer DEFAULT 0,
    week integer DEFAULT 0,
    postseason integer DEFAULT 0,
    home_id integer NOT NULL,
    away_id integer NOT NULL,
    retry integer DEFAULT 0,
    start_time timestamp with time zone,
    home_score integer DEFAULT 0,
    away_score integer DEFAULT 0
);


ALTER TABLE public.games OWNER TO stats;

--
-- Name: interception_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.interception_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    interceptions integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    int_yards integer DEFAULT 0
);


ALTER TABLE public.interception_stats OWNER TO stats;

--
-- Name: kick_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.kick_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    fga integer DEFAULT 0,
    fgm integer DEFAULT 0,
    fg_long integer DEFAULT 0,
    xpa integer DEFAULT 0,
    xpm integer DEFAULT 0,
    points integer DEFAULT 0
);


ALTER TABLE public.kick_stats OWNER TO stats;

--
-- Name: passing_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.passing_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    completions integer DEFAULT 0,
    attempts integer DEFAULT 0,
    yards integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    interceptions integer DEFAULT 0
);


ALTER TABLE public.passing_stats OWNER TO stats;

--
-- Name: players; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.players (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    year integer NOT NULL,
    name text NOT NULL,
    "position" text NOT NULL,
    rating integer DEFAULT 50,
    grade text NOT NULL,
    hometown text NOT NULL,
    status text NOT NULL
);


ALTER TABLE public.players OWNER TO stats;

--
-- Name: punt_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.punt_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    punt_long integer DEFAULT 0,
    punt_no integer DEFAULT 0,
    punt_yards integer DEFAULT 0,
    touchbacks integer DEFAULT 0,
    inside_20 integer DEFAULT 0
);


ALTER TABLE public.punt_stats OWNER TO stats;

--
-- Name: receiving_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.receiving_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    receptions integer DEFAULT 0,
    rec_yards integer DEFAULT 0,
    rec_long integer DEFAULT 0,
    touchdowns integer DEFAULT 0
);


ALTER TABLE public.receiving_stats OWNER TO stats;

--
-- Name: recruiting; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.recruiting (
    team_id integer NOT NULL,
    year integer NOT NULL,
    commits integer DEFAULT 0,
    rating real DEFAULT 0
);


ALTER TABLE public.recruiting OWNER TO stats;

--
-- Name: return_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.return_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    punt_kick text NOT NULL,
    return_no integer DEFAULT 0,
    touchdowns integer DEFAULT 0,
    ret_yards integer DEFAULT 0,
    ret_long integer DEFAULT 0
);


ALTER TABLE public.return_stats OWNER TO stats;

--
-- Name: roster; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.roster (
    player_id integer NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    year integer NOT NULL,
    name text,
    num integer DEFAULT 0,
    "position" text,
    height integer DEFAULT 0,
    weight integer DEFAULT 0,
    grade text,
    hometown text
);


ALTER TABLE public.roster OWNER TO stats;

--
-- Name: rushing_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.rushing_stats (
    player_id integer NOT NULL,
    team_id integer NOT NULL,
    game_id integer NOT NULL,
    carries integer DEFAULT 0,
    rush_yards integer DEFAULT 0,
    rush_long integer DEFAULT 0,
    touchdowns integer DEFAULT 0
);


ALTER TABLE public.rushing_stats OWNER TO stats;

--
-- Name: team_game_stats; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.team_game_stats (
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
    penalty_yards integer DEFAULT 0
);


ALTER TABLE public.team_game_stats OWNER TO stats;

--
-- Name: team_names; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.team_names (
    team_id integer NOT NULL,
    name text NOT NULL,
    sport text DEFAULT 'ncaaf',
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
    slug text
);


ALTER TABLE public.team_names OWNER TO stats;

--
-- Name: team_seasons; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.team_seasons (
    team_id integer NOT NULL,
    year integer NOT NULL,
    sport text DEFAULT 'ncaaf',
    fbs integer DEFAULT 0,
    power_five integer DEFAULT 0,
    conf text
);


ALTER TABLE public.team_seasons OWNER TO stats;

--
-- Name: team_week_results; Type: TABLE; Schema: public; Owner: stats
--

CREATE TABLE public.team_week_results (
    team_id integer NOT NULL,
    year integer NOT NULL,
    week integer NOT NULL,
    postseason integer DEFAULT 0 NOT NULL,
    sport text DEFAULT 'ncaaf' NOT NULL,
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
    ties integer DEFAULT 0
);


ALTER TABLE public.team_week_results OWNER TO stats;

--
-- Name: composite composite_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.composite
    ADD CONSTRAINT composite_pkey PRIMARY KEY (team_id, year);


--
-- Name: defensive_stats defensive_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.defensive_stats
    ADD CONSTRAINT defensive_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: fumble_stats fumble_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.fumble_stats
    ADD CONSTRAINT fumble_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: games game_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.games
    ADD CONSTRAINT game_pkey PRIMARY KEY (game_id);


--
-- Name: interception_stats interception_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.interception_stats
    ADD CONSTRAINT interception_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: kick_stats kick_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.kick_stats
    ADD CONSTRAINT kick_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: passing_stats passing_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.passing_stats
    ADD CONSTRAINT passing_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: players players_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.players
    ADD CONSTRAINT players_pkey PRIMARY KEY (player_id, team_id, year);


--
-- Name: punt_stats punt_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.punt_stats
    ADD CONSTRAINT punt_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: receiving_stats receiving_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.receiving_stats
    ADD CONSTRAINT receiving_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: recruiting recruiting_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.recruiting
    ADD CONSTRAINT recruiting_pkey PRIMARY KEY (team_id, year);


--
-- Name: return_stats return_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.return_stats
    ADD CONSTRAINT return_stats_pkey PRIMARY KEY (player_id, team_id, game_id, punt_kick);


--
-- Name: roster roster_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.roster
    ADD CONSTRAINT roster_pkey PRIMARY KEY (player_id, team_id, year);


--
-- Name: rushing_stats rushing_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.rushing_stats
    ADD CONSTRAINT rushing_stats_pkey PRIMARY KEY (player_id, team_id, game_id);


--
-- Name: team_game_stats team_game_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.team_game_stats
    ADD CONSTRAINT team_game_stats_pkey PRIMARY KEY (game_id, team_id);


--
-- Name: team_names team_name_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.team_names
    ADD CONSTRAINT team_name_pkey PRIMARY KEY (team_id, sport);


--
-- Name: team_seasons team_season_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.team_seasons
    ADD CONSTRAINT team_season_pkey PRIMARY KEY (team_id, year, sport);


--
-- Name: team_week_results team_week_result_pkey; Type: CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.team_week_results
    ADD CONSTRAINT team_week_result_pkey PRIMARY KEY (team_id, year, week, postseason, sport);


--
-- Name: fbs_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX fbs_index ON public.team_week_results USING btree (fbs);


--
-- Name: game_away_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX game_away_index ON public.games USING btree (away_id);


--
-- Name: game_home_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX game_home_index ON public.games USING btree (home_id);


--
-- Name: game_retry_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX game_retry_index ON public.games USING btree (retry);


--
-- Name: game_season_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX game_season_index ON public.games USING btree (season);


--
-- Name: game_start_time_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX game_start_time_index ON public.games USING btree (start_time);


--
-- Name: game_week_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX game_week_index ON public.games USING btree (week);


--
-- Name: name_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX name_index ON public.team_names USING btree (name);


--
-- Name: postseason_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX postseason_index ON public.team_week_results USING btree (postseason);


--
-- Name: team_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX team_index ON public.team_week_results USING btree (team_id);


--
-- Name: week_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX week_index ON public.team_week_results USING btree (week);


--
-- Name: year_index; Type: INDEX; Schema: public; Owner: stats
--

CREATE INDEX year_index ON public.team_week_results USING btree (year);


--
-- Name: defensive_stats defensive_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.defensive_stats
    ADD CONSTRAINT defensive_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: fumble_stats fumble_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.fumble_stats
    ADD CONSTRAINT fumble_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: interception_stats interception_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.interception_stats
    ADD CONSTRAINT interception_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: kick_stats kick_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.kick_stats
    ADD CONSTRAINT kick_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: passing_stats passing_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.passing_stats
    ADD CONSTRAINT passing_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: punt_stats punt_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.punt_stats
    ADD CONSTRAINT punt_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: receiving_stats receiving_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.receiving_stats
    ADD CONSTRAINT receiving_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: return_stats return_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.return_stats
    ADD CONSTRAINT return_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: rushing_stats rushing_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.rushing_stats
    ADD CONSTRAINT rushing_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: team_game_stats team_game_stats_game_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.team_game_stats
    ADD CONSTRAINT team_game_stats_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(game_id) ON DELETE CASCADE;


--
-- Name: team_week_results team_week_result_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: stats
--

ALTER TABLE ONLY public.team_week_results
    ADD CONSTRAINT team_week_result_team_id_fkey FOREIGN KEY (team_id, sport) REFERENCES public.team_names(team_id, sport) ON DELETE CASCADE;


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: postgres
--

REVOKE USAGE ON SCHEMA public FROM PUBLIC;
GRANT ALL ON SCHEMA public TO PUBLIC;


--
-- PostgreSQL database dump complete
--

