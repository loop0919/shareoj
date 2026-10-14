-- A reservation holds a private snapshot; later edits need an explicit reschedule.
ALTER TABLE problem_drafts ADD COLUMN scheduled_at timestamptz;
ALTER TABLE problem_drafts ADD COLUMN scheduled_draft jsonb;
ALTER TABLE problem_drafts ADD COLUMN scheduled_version bigint;
ALTER TABLE problem_drafts ADD CONSTRAINT problem_schedule_complete CHECK (
    (scheduled_at IS NULL AND scheduled_draft IS NULL AND scheduled_version IS NULL) OR
    (scheduled_at IS NOT NULL AND scheduled_draft IS NOT NULL AND scheduled_version IS NOT NULL AND scheduled_version > 0 AND published_draft IS NULL)
);
CREATE INDEX problem_schedule_due ON problem_drafts(scheduled_at) WHERE scheduled_at IS NOT NULL;

CREATE OR REPLACE FUNCTION remember_problem_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.ever_published := NEW.ever_published OR NEW.published_draft IS NOT NULL;
    IF NEW.published_draft IS NOT NULL THEN
        DELETE FROM featured_applications WHERE problem_id=NEW.id;
        NEW.scheduled_at := NULL;
        NEW.scheduled_draft := NULL;
        NEW.scheduled_version := NULL;
    END IF;
    RETURN NEW;
END
$$;

CREATE OR REPLACE FUNCTION content_image_access(image_id uuid, uploader text, viewer text, include_private boolean)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS (
   SELECT 1 FROM (
     SELECT d.draft->>'markdown' AS body, d.owner_id AS author, d.id AS problem, NULL::timestamptz AS visible_at FROM problem_drafts d
     UNION ALL SELECT d.draft->>'editorial', d.owner_id, d.id, NULL FROM problem_drafts d
     UNION ALL SELECT d.scheduled_draft->>'markdown', d.owner_id, d.id, NULL FROM problem_drafts d
     UNION ALL SELECT d.scheduled_draft->>'editorial', d.owner_id, d.id, NULL FROM problem_drafts d
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
