-- Rename sport identifier: ncaambb → ncaam
-- Aligns with standard abbreviation and leaves room for ncaaw in the future.

BEGIN;

-- Drop FK that references team_names(team_id, sport) so both tables
-- can be updated independently within the transaction.
ALTER TABLE team_week_results DROP CONSTRAINT team_week_result_team_id_fkey;

UPDATE games SET sport = 'ncaam' WHERE sport = 'ncaambb';
UPDATE team_names SET sport = 'ncaam' WHERE sport = 'ncaambb';
UPDATE team_seasons SET sport = 'ncaam' WHERE sport = 'ncaambb';
UPDATE team_week_results SET sport = 'ncaam' WHERE sport = 'ncaambb';

-- Re-add FK now that both tables use the new sport values
ALTER TABLE team_week_results
    ADD CONSTRAINT team_week_result_team_id_fkey
    FOREIGN KEY (team_id, sport) REFERENCES team_names(team_id, sport) ON DELETE CASCADE;

COMMIT;
