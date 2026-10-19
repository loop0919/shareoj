-- ADR 0024 (changed 2026-10-19): setters only move participants and teams to onsite, so nobody is kept open any more.
-- The API stopped reading these columns before this migration runs.
ALTER TABLE contest_teams DROP COLUMN kept_open;
ALTER TABLE contest_participants DROP COLUMN kept_open;
