-- Like problem_ids, a post's tags are a draft until it is published. Problem tags live in the draft JSON.
ALTER TABLE blog_posts ADD COLUMN tags text[] NOT NULL DEFAULT '{}';
ALTER TABLE blog_posts ADD COLUMN published_tags text[] NOT NULL DEFAULT '{}';
CREATE INDEX blog_published_tags ON blog_posts USING gin (published_tags) WHERE published_title IS NOT NULL;
CREATE INDEX problem_published_tags ON problem_drafts USING gin ((published_draft->'tags')) WHERE published_draft IS NOT NULL;

-- Tags hint at the solution. While a 定期便 round features a problem, new or revival, its tags stay hidden.
-- Slots are inserted when their round starts, so a slot before reveal_at is a round in progress.
CREATE FUNCTION problem_tags_hidden(pid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS (SELECT 1 FROM featured_slots WHERE problem_id=pid AND statement_timestamp()<reveal_at)
$$;
