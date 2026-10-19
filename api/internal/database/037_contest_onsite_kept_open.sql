-- ADR 0024: a setter who moves a participant (or a team) back to open decides for the rest of the contest; the venue
-- passphrase must not make them onsite again. A setter switching them onsite clears the mark.
ALTER TABLE contest_teams ADD COLUMN kept_open boolean NOT NULL DEFAULT false;
ALTER TABLE contest_participants ADD COLUMN kept_open boolean NOT NULL DEFAULT false;
