ALTER TABLE contests ADD COLUMN published boolean NOT NULL DEFAULT true;
ALTER TABLE contests ALTER COLUMN published SET DEFAULT false;

CREATE OR REPLACE FUNCTION content_image_access(image_id uuid, uploader text, viewer text, include_private boolean)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS (
   SELECT 1 FROM (
     SELECT d.draft->>'markdown' AS body, d.owner_id AS author, d.id AS problem, NULL::timestamptz AS visible_at FROM problem_drafts d
     UNION ALL SELECT d.draft->>'editorial', d.owner_id, d.id, NULL FROM problem_drafts d
     UNION ALL SELECT d.published_draft->>'markdown', d.owner_id, d.id, '-infinity'::timestamptz FROM problem_drafts d
     UNION ALL SELECT d.published_draft->>'editorial', d.owner_id, d.id, '-infinity'::timestamptz FROM problem_drafts d
     UNION ALL SELECT b.markdown, b.owner_id, NULL, NULL FROM blog_posts b
     UNION ALL SELECT b.published_markdown, b.owner_id, NULL, '-infinity'::timestamptz FROM blog_posts b
     UNION ALL SELECT c.description, c.owner_id, NULL, CASE WHEN c.published THEN '-infinity'::timestamptz END FROM contests c
     UNION ALL SELECT cp.draft->>'markdown', c.owner_id, cp.problem_id, CASE WHEN c.published THEN c.starts_at END FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
     UNION ALL SELECT cp.draft->>'editorial', c.owner_id, cp.problem_id, CASE WHEN c.published THEN c.ends_at END FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
   ) source
   WHERE strpos(source.body, '/api/images/' || image_id::text) > 0
     AND (source.author=uploader OR can_manage_problem(source.problem,uploader))
     AND (include_private OR source.visible_at<=statement_timestamp()
          OR source.author=viewer OR can_manage_problem(source.problem,viewer))
 )
$$;
