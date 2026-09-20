CREATE TABLE problem_difficulty_votes (
    problem_id uuid NOT NULL REFERENCES problem_drafts(id) ON DELETE CASCADE,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id) ON DELETE CASCADE,
    difficulty integer NOT NULL CHECK (difficulty BETWEEN 1 AND 10),
    PRIMARY KEY (problem_id, owner_id)
);
