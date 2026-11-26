-- When a post was first saved, so its owner's lists can be ordered by it. A post from before this column takes the
-- earliest time known for it.
ALTER TABLE blog_posts ADD COLUMN created_at timestamptz;
UPDATE blog_posts SET created_at = LEAST(updated_at, COALESCE(published_at, updated_at));
ALTER TABLE blog_posts ALTER COLUMN created_at SET DEFAULT clock_timestamp(), ALTER COLUMN created_at SET NOT NULL;
