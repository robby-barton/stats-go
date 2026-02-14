-- Migration: Add sport column to support multiple sports (basketball + football)
-- Run this against an existing PostgreSQL database before deploying the multi-sport code.
-- All existing data is assumed to be college football (cfb).

BEGIN;

-- 1. Add sport column to games
ALTER TABLE games ADD COLUMN IF NOT EXISTS sport text DEFAULT 'cfb';
UPDATE games SET sport = 'cfb' WHERE sport IS NULL;

-- 2. Add sport column to team_names
ALTER TABLE team_names ADD COLUMN IF NOT EXISTS sport text DEFAULT 'cfb';
UPDATE team_names SET sport = 'cfb' WHERE sport IS NULL;

-- 3. Add sport column to team_seasons and update PK
ALTER TABLE team_seasons ADD COLUMN IF NOT EXISTS sport text DEFAULT 'cfb';
UPDATE team_seasons SET sport = 'cfb' WHERE sport IS NULL;
ALTER TABLE team_seasons DROP CONSTRAINT IF EXISTS team_season_pkey;
ALTER TABLE team_seasons ADD CONSTRAINT team_season_pkey PRIMARY KEY (team_id, year, sport);

-- 4. Add sport column to team_week_results and update PK
ALTER TABLE team_week_results ADD COLUMN IF NOT EXISTS sport text DEFAULT 'cfb' NOT NULL;
UPDATE team_week_results SET sport = 'cfb' WHERE sport = 'cfb'; -- no-op but ensures NOT NULL is safe
ALTER TABLE team_week_results DROP CONSTRAINT IF EXISTS team_week_result_pkey;
ALTER TABLE team_week_results ADD CONSTRAINT team_week_result_pkey PRIMARY KEY (team_id, year, week, postseason, sport);

COMMIT;
