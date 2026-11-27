-- Synthetic, disposable seed for manual ncaaf-replay smoke tests against a
-- throwaway PostgreSQL instance. No production-derived data. It matches the
-- schema shaped by internal/database/models.go (same tables as Go tests use).
-- Load AFTER the schema migration (cmd/migrate-schema):
--   psql "$DSN" -f internal/ncaafreplay/testdata/seed.sql

INSERT INTO team_names (team_id, name, sport) VALUES
  (1, 'Alpha', 'ncaaf'), (2, 'Beta', 'ncaaf'), (3, 'Gamma', 'ncaaf'),
  (4, 'Delta', 'ncaaf'), (5, 'Epsilon', 'ncaaf');

INSERT INTO team_seasons (team_id, year, conf, fbs, sport) VALUES
  (1, 2023, 'SEC', 1, 'ncaaf'), (2, 2023, 'SEC', 1, 'ncaaf'),
  (3, 2023, 'Big Ten', 1, 'ncaaf'), (4, 2023, 'Big Ten', 1, 'ncaaf'),
  (5, 2023, 'FCS', 0, 'ncaaf');

INSERT INTO games
  (game_id, season, week, start_time, home_id, home_score, away_id, away_score, sport, postseason)
VALUES
  (1001, 2023, 1, '2023-09-05 19:00:00+00', 1, 28, 2, 14, 'ncaaf', 0),
  (1002, 2023, 1, '2023-09-05 20:00:00+00', 3, 21, 4, 10, 'ncaaf', 0),
  (1003, 2023, 2, '2023-09-12 19:00:00+00', 1, 35, 3, 17, 'ncaaf', 0),
  (1004, 2023, 2, '2023-09-12 20:00:00+00', 2, 24, 4, 21, 'ncaaf', 0),
  (1005, 2023, 2, '2023-09-12 21:00:00+00', 1, 31, 5, 10, 'ncaaf', 0),
  (901, 2022, 1, '2022-09-06 19:00:00+00', 1, 24, 2, 17, 'ncaaf', 0),
  (902, 2022, 2, '2022-09-13 19:00:00+00', 3, 20, 4, 13, 'ncaaf', 0),
  (903, 2022, 3, '2022-09-20 19:00:00+00', 2, 14, 3, 10, 'ncaaf', 0),
  (4000, 2020, 1, '2020-09-05 19:00:00+00', 1, 20, 2, 7, 'ncaaf', 0),
  (4001, 2020, 2, '2020-09-06 19:00:00+00', 1, 21, 2, 7, 'ncaaf', 0),
  (4002, 2020, 3, '2020-09-07 19:00:00+00', 1, 22, 2, 7, 'ncaaf', 0);
