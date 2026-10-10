-- Unpublished contests live apart from contests, so no public query or schedule can reach them.
CREATE TABLE contest_drafts (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id),
    draft jsonb NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX contest_drafts_owner ON contest_drafts(owner_id, updated_at DESC, id DESC);

-- Images in a draft description count as in use, but stay private to the author.
CREATE OR REPLACE FUNCTION content_image_access(image_id uuid, uploader text, viewer text, include_private boolean)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS (
   SELECT 1 FROM (
     SELECT d.draft->>'markdown' AS body, d.owner_id AS author, d.id AS problem, NULL::timestamptz AS visible_at FROM problem_drafts d
     UNION ALL SELECT d.draft->>'editorial', d.owner_id, d.id, NULL FROM problem_drafts d
     UNION ALL SELECT d.published_draft->>'markdown', d.owner_id, d.id, '-infinity'::timestamptz FROM problem_drafts d
     UNION ALL SELECT d.published_draft->>'editorial', d.owner_id, d.id, COALESCE(featured_reveal_at(d.id),'-infinity'::timestamptz) FROM problem_drafts d
     UNION ALL SELECT b.markdown, b.owner_id, NULL, NULL FROM blog_posts b
     UNION ALL SELECT b.published_markdown, b.owner_id, NULL, '-infinity'::timestamptz FROM blog_posts b
     UNION ALL SELECT c.description, c.owner_id, NULL, '-infinity'::timestamptz FROM contests c
     UNION ALL SELECT cd.draft->>'description', cd.owner_id, NULL, NULL FROM contest_drafts cd
     UNION ALL SELECT cp.draft->>'markdown', c.owner_id, cp.problem_id, c.starts_at FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
     UNION ALL SELECT cp.draft->>'editorial', c.owner_id, cp.problem_id, GREATEST(c.ends_at,featured_reveal_at(cp.problem_id)) FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
   ) source
   WHERE strpos(source.body, '/api/images/' || image_id::text) > 0
     AND (source.author=uploader OR can_manage_problem(source.problem,uploader))
     AND (include_private OR source.visible_at<=statement_timestamp()
          OR source.author=viewer OR can_manage_problem(source.problem,viewer))
 )
$$;
