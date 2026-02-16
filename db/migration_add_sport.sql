-- Migration: Add sport column to support multiple sports (basketball + football)
-- Run this against an existing PostgreSQL database before deploying the multi-sport code.
-- All existing data is assumed to be college football (ncaaf).
-- NOTE: This migration originally used 'cfb' as the sport value. A subsequent
-- migration (migration_rename_sport.sql) renamed 'cfb' -> 'ncaaf' and 'cbb' -> 'ncaambb'.

BEGIN;

-- 0. Drop FK from team_week_results -> team_names before changing PKs
--    (references old single-column PK on team_names.team_id)
ALTER TABLE team_week_results DROP CONSTRAINT IF EXISTS team_week_result_team_id_fkey;

-- 1. Add sport column to games
ALTER TABLE games ADD COLUMN IF NOT EXISTS sport text DEFAULT 'ncaaf';
UPDATE games SET sport = 'ncaaf' WHERE sport IS NULL;

-- 2. Add sport column to team_names and update PK
ALTER TABLE team_names ADD COLUMN IF NOT EXISTS sport text DEFAULT 'ncaaf';
UPDATE team_names SET sport = 'ncaaf' WHERE sport IS NULL;
ALTER TABLE team_names DROP CONSTRAINT IF EXISTS team_name_pkey;
ALTER TABLE team_names ADD CONSTRAINT team_name_pkey PRIMARY KEY (team_id, sport);

-- 3. Add sport column to team_seasons and update PK
ALTER TABLE team_seasons ADD COLUMN IF NOT EXISTS sport text DEFAULT 'ncaaf';
UPDATE team_seasons SET sport = 'ncaaf' WHERE sport IS NULL;
ALTER TABLE team_seasons DROP CONSTRAINT IF EXISTS team_season_pkey;
ALTER TABLE team_seasons ADD CONSTRAINT team_season_pkey PRIMARY KEY (team_id, year, sport);

-- 4. Add sport column to team_week_results and update PK
ALTER TABLE team_week_results ADD COLUMN IF NOT EXISTS sport text DEFAULT 'ncaaf' NOT NULL;
UPDATE team_week_results SET sport = 'ncaaf' WHERE sport = 'ncaaf'; -- no-op but ensures NOT NULL is safe
ALTER TABLE team_week_results DROP CONSTRAINT IF EXISTS team_week_result_pkey;
ALTER TABLE team_week_results ADD CONSTRAINT team_week_result_pkey PRIMARY KEY (team_id, year, week, postseason, sport);

-- 5. Re-add FK as composite reference now that both tables have sport in their PKs
ALTER TABLE team_week_results
    ADD CONSTRAINT team_week_result_team_id_fkey
    FOREIGN KEY (team_id, sport) REFERENCES team_names(team_id, sport) ON DELETE CASCADE;

COMMIT;
