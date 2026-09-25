ALTER TABLE problem_drafts ADD COLUMN ever_published boolean NOT NULL DEFAULT false;
UPDATE problem_drafts SET ever_published=true WHERE published_draft IS NOT NULL;

CREATE TABLE featured_applications (
    problem_id uuid PRIMARY KEY REFERENCES problem_drafts(id) ON DELETE CASCADE,
    preference text NOT NULL CHECK (preference IN ('soon','later')),
    entered_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE featured_slots (
    scheduled_at timestamptz NOT NULL,
    slot text NOT NULL CHECK (slot IN ('easy','hard')),
    kind text NOT NULL CHECK (kind IN ('new','revival','missing')),
    problem_id uuid REFERENCES problem_drafts(id) ON DELETE SET NULL,
    difficulty integer CHECK (difficulty BETWEEN 1 AND 10),
    reveal_at timestamptz NOT NULL,
    PRIMARY KEY (scheduled_at,slot),
    CHECK ((kind='missing' AND difficulty IS NULL) OR
           (kind<>'missing' AND ((slot='easy' AND difficulty BETWEEN 1 AND 4) OR
                               (slot='hard' AND difficulty BETWEEN 5 AND 10))))
);
CREATE INDEX featured_slots_problem ON featured_slots(problem_id,scheduled_at DESC);
CREATE UNIQUE INDEX featured_new_problem ON featured_slots(problem_id) WHERE kind='new';

-- Start at the first future delivery, never backfill pre-installation history.
CREATE TABLE featured_schedule (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    next_at timestamptz NOT NULL
);
INSERT INTO featured_schedule(next_at)
SELECT min(day + interval '23 hours') AT TIME ZONE 'Asia/Tokyo'
FROM generate_series(date_trunc('day',now() AT TIME ZONE 'Asia/Tokyo'),
                     date_trunc('day',now() AT TIME ZONE 'Asia/Tokyo')+interval '7 days',interval '1 day') day
WHERE extract(isodow FROM day) IN (1,4)
  AND (day+interval '23 hours') AT TIME ZONE 'Asia/Tokyo'>now();

CREATE FUNCTION remember_problem_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.ever_published := NEW.ever_published OR NEW.published_draft IS NOT NULL;
    IF NEW.published_draft IS NOT NULL THEN
        DELETE FROM featured_applications WHERE problem_id=NEW.id;
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER remember_problem_publication BEFORE INSERT OR UPDATE ON problem_drafts
FOR EACH ROW EXECUTE FUNCTION remember_problem_publication();

CREATE FUNCTION featured_reveal_at(pid uuid) RETURNS timestamptz LANGUAGE sql STABLE AS $$
    SELECT reveal_at FROM featured_slots WHERE problem_id=pid AND kind='new'
$$;

-- Keep the existing image permissions; delay only new featured editorials.
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
     UNION ALL SELECT cp.draft->>'markdown', c.owner_id, cp.problem_id, c.starts_at FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
     UNION ALL SELECT cp.draft->>'editorial', c.owner_id, cp.problem_id, GREATEST(c.ends_at,featured_reveal_at(cp.problem_id)) FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
   ) source
   WHERE strpos(source.body, '/api/images/' || image_id::text) > 0
     AND (source.author=uploader OR can_manage_problem(source.problem,uploader))
     AND (include_private OR source.visible_at<=statement_timestamp()
          OR source.author=viewer OR can_manage_problem(source.problem,viewer))
 )
$$;
