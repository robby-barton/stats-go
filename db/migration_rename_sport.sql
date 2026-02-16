BEGIN;
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
COMMIT;
