CREATE TABLE creation_quotas (
    owner_id text NOT NULL CHECK (owner_id <> ''),
    kind text NOT NULL CHECK (kind IN ('problem', 'contest', 'post')),
    full_at timestamptz NOT NULL,
    PRIMARY KEY (owner_id, kind)
);
