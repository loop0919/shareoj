-- ADR 0022: co-setters join a contest through its invitation link and work on it like the owner.
-- A draft and its contest share the id, so these rows have no foreign key on it and survive publication.

-- Leaving or being removed sets left_at instead of deleting the row, so images a former setter put in the contest stay valid.
CREATE TABLE contest_setters (
    contest_id uuid NOT NULL,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    left_at timestamptz,
    PRIMARY KEY (contest_id, owner_id)
);
CREATE INDEX contest_setters_owner ON contest_setters(owner_id, contest_id) WHERE left_at IS NULL;

-- One link per contest. Regenerating it replaces the token, so the old link stops working.
CREATE TABLE contest_setter_invitations (
    contest_id uuid PRIMARY KEY,
    token text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

-- Setters are the owner of the contest or of its draft, and the co-setters who have not left.
CREATE OR REPLACE FUNCTION contest_setter(contest uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM contests WHERE id=contest AND owner_id=actor)
        OR EXISTS(SELECT 1 FROM contest_drafts WHERE id=contest AND owner_id=actor)
        OR EXISTS(SELECT 1 FROM contest_setters WHERE contest_id=contest AND owner_id=actor AND left_at IS NULL)
$$;

-- Direct management: the author and invited testers. Tester links and the problems a contest may take count only these,
-- so a contest never justifies itself and contest rights never turn into tester rights that nobody can revoke.
CREATE FUNCTION can_manage_problem_directly(problem uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM problem_drafts WHERE id=problem AND owner_id=actor)
        OR EXISTS(SELECT 1 FROM problem_testers WHERE problem_id=problem AND owner_id=actor)
$$;

-- Setters of a published contest also manage its problems like testers until the results are final.
-- A draft's problem list grants nothing, since a draft may name any problem.
CREATE OR REPLACE FUNCTION can_manage_problem(problem uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM problem_drafts WHERE id=problem AND owner_id=actor)
        OR EXISTS(SELECT 1 FROM problem_testers WHERE problem_id=problem AND owner_id=actor)
        OR EXISTS(SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
            WHERE cp.problem_id=problem AND NOT contest_results_final(c)
              AND (c.owner_id=actor OR EXISTS(SELECT 1 FROM contest_setters s
                WHERE s.contest_id=c.id AND s.owner_id=actor AND s.left_at IS NULL)))
$$;

-- Setters see every problem of their contest already, so only direct management is left to check.
CREATE OR REPLACE FUNCTION contest_problem_access(contest uuid, problem uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT contest_setter(contest, actor) OR can_manage_problem_directly(problem, actor)
$$;

-- A notification about a contest names it instead of a problem. The id may be a draft's, so it has no foreign key.
ALTER TABLE notifications ALTER COLUMN problem_id DROP NOT NULL;
ALTER TABLE notifications ADD COLUMN contest_id uuid;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('favorite','first_accept','tester','user_editorial','contest_setter'));
ALTER TABLE notifications ADD CONSTRAINT notifications_subject CHECK (
    CASE WHEN kind='contest_setter' THEN problem_id IS NULL AND contest_id IS NOT NULL ELSE problem_id IS NOT NULL AND contest_id IS NULL END
);
CREATE INDEX notifications_contest ON notifications(contest_id) WHERE contest_id IS NOT NULL;

-- Any setter, including one who has since left, may have put an image in the contest description or, while managing
-- them, in its problems. Draft descriptions stay private to the current setters.
CREATE OR REPLACE FUNCTION content_image_access(image_id uuid, uploader text, viewer text, include_private boolean)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS (
   SELECT 1 FROM (
     SELECT d.draft->>'markdown' AS body, d.owner_id AS author, d.id AS problem, NULL::uuid AS contest, NULL::timestamptz AS visible_at FROM problem_drafts d
     UNION ALL SELECT d.draft->>'editorial', d.owner_id, d.id, NULL, NULL FROM problem_drafts d
     UNION ALL SELECT d.scheduled_draft->>'markdown', d.owner_id, d.id, NULL, NULL FROM problem_drafts d
     UNION ALL SELECT d.scheduled_draft->>'editorial', d.owner_id, d.id, NULL, NULL FROM problem_drafts d
     UNION ALL SELECT d.published_draft->>'markdown', d.owner_id, d.id, NULL, '-infinity'::timestamptz FROM problem_drafts d
     UNION ALL SELECT d.published_draft->>'editorial', d.owner_id, d.id, NULL, COALESCE(featured_reveal_at(d.id),'-infinity'::timestamptz) FROM problem_drafts d
     UNION ALL SELECT b.markdown, b.owner_id, NULL, NULL, NULL FROM blog_posts b
     UNION ALL SELECT b.published_markdown, b.owner_id, NULL, NULL, '-infinity'::timestamptz FROM blog_posts b
     UNION ALL SELECT c.description, c.owner_id, NULL, c.id, '-infinity'::timestamptz FROM contests c
     UNION ALL SELECT cd.draft->>'description', cd.owner_id, NULL, cd.id, NULL FROM contest_drafts cd
     UNION ALL SELECT cp.draft->>'markdown', c.owner_id, cp.problem_id, c.id, c.starts_at FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
     UNION ALL SELECT cp.draft->>'editorial', c.owner_id, cp.problem_id, c.id, GREATEST(c.ends_at,featured_reveal_at(cp.problem_id)) FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
   ) source
   WHERE strpos(source.body, '/api/images/' || image_id::text) > 0
     AND (source.author=uploader OR can_manage_problem(source.problem,uploader)
          OR EXISTS(SELECT 1 FROM contest_setters s WHERE s.contest_id=source.contest AND s.owner_id=uploader)
          OR EXISTS(SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=source.problem
               AND (c.owner_id=uploader OR EXISTS(SELECT 1 FROM contest_setters s WHERE s.contest_id=c.id AND s.owner_id=uploader))))
     AND (include_private OR source.visible_at<=statement_timestamp()
          OR source.author=viewer OR can_manage_problem(source.problem,viewer)
          OR EXISTS(SELECT 1 FROM contest_setters s WHERE s.contest_id=source.contest AND s.owner_id=viewer AND s.left_at IS NULL))
 )
$$;
