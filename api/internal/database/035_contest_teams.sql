-- ADR 0024: a contest is individual or a team contest, and may have an onsite division with a venue passphrase.
-- max_team_size NULL means unlimited; the API still caps a team at an internal limit. The passphrase is plain text,
-- because setters read it back to hand it out at the venue; only setter-only responses return it.
ALTER TABLE contests ADD COLUMN team_mode boolean NOT NULL DEFAULT false;
ALTER TABLE contests ADD COLUMN max_team_size integer CHECK (max_team_size >= 2);
ALTER TABLE contests ADD COLUMN onsite_division boolean NOT NULL DEFAULT false;
ALTER TABLE contests ADD COLUMN onsite_passphrase text CHECK (onsite_passphrase <> '');
ALTER TABLE contests ADD CONSTRAINT contests_onsite_passphrase CHECK (NOT onsite_division OR onsite_passphrase IS NOT NULL);

-- A team is one row of a team contest's standings. name_key is the normalized name in lower case, so names that differ
-- only in case are the same name. join_code is made by the server and shown to members only.
-- onsite_at and onsite_via record when and how the team became onsite: by the venue passphrase or by a setter.
CREATE TABLE contest_teams (
    id uuid PRIMARY KEY,
    contest_id uuid NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> ''),
    name_key text NOT NULL CHECK (name_key <> ''),
    join_code text NOT NULL CHECK (join_code <> ''),
    onsite_at timestamptz,
    onsite_via text CHECK (onsite_via IN ('passphrase', 'setter')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((onsite_at IS NULL) = (onsite_via IS NULL)),
    UNIQUE (contest_id, id),
    UNIQUE (contest_id, name_key),
    UNIQUE (contest_id, join_code)
);

-- Every member keeps a row of their own, because judge-bridge counts these rows (ADR 0012, ADR 0014).
-- The onsite columns are the participant's in an individual contest; a team contest uses the team's.
ALTER TABLE contest_participants ADD COLUMN team_id uuid;
ALTER TABLE contest_participants ADD CONSTRAINT contest_participants_team
    FOREIGN KEY (contest_id, team_id) REFERENCES contest_teams(contest_id, id);
ALTER TABLE contest_participants ADD COLUMN onsite_at timestamptz;
ALTER TABLE contest_participants ADD COLUMN onsite_via text CHECK (onsite_via IN ('passphrase', 'setter'));
ALTER TABLE contest_participants ADD CONSTRAINT contest_participants_onsite CHECK ((onsite_at IS NULL) = (onsite_via IS NULL));
CREATE INDEX contest_participants_team ON contest_participants(team_id) WHERE team_id IS NOT NULL;

-- Wrong team join codes and venue passphrases, counted per account and contest to throttle guessing.
CREATE TABLE contest_attempt_failures (
    contest_id uuid NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id) ON DELETE CASCADE,
    failed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX contest_attempt_failures_owner ON contest_attempt_failures(owner_id, contest_id, failed_at);
