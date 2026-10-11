-- A post lists the problems it explains. Like the title and body, the list is published as a snapshot.
ALTER TABLE blog_posts ADD COLUMN problem_ids uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE blog_posts ADD COLUMN published_problem_ids uuid[] NOT NULL DEFAULT '{}';
CREATE INDEX blog_published_problems ON blog_posts USING gin (published_problem_ids) WHERE published_title IS NOT NULL;

ALTER TABLE notifications ADD COLUMN post_id uuid REFERENCES blog_posts(id) ON DELETE CASCADE;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('favorite','first_accept','tester','user_editorial'));
CREATE UNIQUE INDEX notifications_user_editorial ON notifications(problem_id,post_id) WHERE kind='user_editorial';

-- Tell the problem author once per post, when it is first published for a problem whose editorial is public.
CREATE FUNCTION notify_user_editorial() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO notifications(owner_id,problem_id,actor_id,kind,post_id)
    SELECT d.owner_id,d.id,NEW.owner_id,'user_editorial',NEW.id FROM problem_drafts d
    WHERE d.id=ANY(NEW.published_problem_ids) AND d.owner_id<>NEW.owner_id
     AND d.published_draft IS NOT NULL AND NOT COALESCE(statement_timestamp()<featured_reveal_at(d.id),false)
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END
$$;
CREATE TRIGGER user_editorial_notification AFTER UPDATE OF published_problem_ids ON blog_posts
FOR EACH ROW WHEN (NEW.published_title IS NOT NULL) EXECUTE FUNCTION notify_user_editorial();
