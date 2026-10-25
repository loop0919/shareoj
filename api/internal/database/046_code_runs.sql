-- Code runs execute a source on a typed input without a problem (ADR 0031). They stay out of submissions, so no
-- submission list, standing, notification or rejudge can count them.
CREATE TABLE code_runs (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id),
    runtime text NOT NULL,
    image text NOT NULL,
    source text NOT NULL,
    stdin text NOT NULL,
    judge_attempt uuid NOT NULL DEFAULT gen_random_uuid(),
    status text NOT NULL DEFAULT 'QUEUED' CHECK (status IN ('QUEUED', 'RUNNING', 'DONE')),
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    dispatched_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX code_runs_owner_order ON code_runs(owner_id, created_at DESC);
CREATE INDEX code_runs_dispatch ON code_runs(created_at) WHERE dispatched_at IS NULL AND status <> 'DONE';
CREATE INDEX code_runs_in_flight ON code_runs(dispatched_at) WHERE dispatched_at IS NOT NULL AND status <> 'DONE';
