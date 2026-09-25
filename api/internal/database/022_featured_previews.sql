-- Keep the next selection stable between its preview and publication.
CREATE TABLE featured_previews (
    scheduled_at timestamptz NOT NULL,
    slot text NOT NULL CHECK (slot IN ('easy', 'hard')),
    problem_id uuid NOT NULL REFERENCES problem_drafts(id) ON DELETE CASCADE,
    PRIMARY KEY (scheduled_at, slot)
);
