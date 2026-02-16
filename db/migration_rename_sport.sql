BEGIN;

-- Drop FK that references team_names(team_id, sport) so both tables
-- can be updated independently within the transaction.
ALTER TABLE team_week_results DROP CONSTRAINT team_week_result_team_id_fkey;

-- Rename sport values in all four shared tables
UPDATE games SET sport = 'ncaaf' WHERE sport = 'cfb';
UPDATE games SET sport = 'ncaambb' WHERE sport = 'cbb';

UPDATE team_names SET sport = 'ncaaf' WHERE sport = 'cfb';
UPDATE team_names SET sport = 'ncaambb' WHERE sport = 'cbb';

UPDATE team_seasons SET sport = 'ncaaf' WHERE sport = 'cfb';
UPDATE team_seasons SET sport = 'ncaambb' WHERE sport = 'cbb';

UPDATE team_week_results SET sport = 'ncaaf' WHERE sport = 'cfb';
UPDATE team_week_results SET sport = 'ncaambb' WHERE sport = 'cbb';

-- Update column defaults
ALTER TABLE games ALTER COLUMN sport SET DEFAULT 'ncaaf';
ALTER TABLE team_names ALTER COLUMN sport SET DEFAULT 'ncaaf';
ALTER TABLE team_seasons ALTER COLUMN sport SET DEFAULT 'ncaaf';
ALTER TABLE team_week_results ALTER COLUMN sport SET DEFAULT 'ncaaf';

-- Re-add FK now that both tables use the new sport values
ALTER TABLE team_week_results
    ADD CONSTRAINT team_week_result_team_id_fkey
    FOREIGN KEY (team_id, sport) REFERENCES team_names(team_id, sport) ON DELETE CASCADE;

COMMIT;
