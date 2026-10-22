CREATE TABLE contest_clarifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    contest_id uuid NOT NULL,
    problem_id uuid NOT NULL,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id),
    question text NOT NULL CHECK (char_length(question) BETWEEN 1 AND 4000),
    answer text NOT NULL DEFAULT '' CHECK (char_length(answer) <= 4000),
    answered_by text REFERENCES user_profiles(owner_id),
    answered_at timestamptz,
    published_at timestamptz,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (contest_id, problem_id) REFERENCES contest_problems(contest_id, problem_id)
);
CREATE INDEX contest_clarifications_order ON contest_clarifications(contest_id, created_at DESC, id DESC);

-- Co-setter and contest-wide staff rights do not grant access to private questions.
CREATE FUNCTION contest_clarification_manager(contest uuid, problem uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT actor<>'' AND (EXISTS(SELECT 1 FROM contests WHERE id=contest AND owner_id=actor)
        OR can_manage_problem_directly(problem, actor))
$$;
