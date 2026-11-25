-- Folders sort a user's own problems, tested problems and posts on the my page. They are their owner's alone and change
-- no visibility or permission. A folder goes with its parent.
CREATE TABLE content_folders (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id),
    parent_id uuid REFERENCES content_folders(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name = btrim(name) AND char_length(name) BETWEEN 1 AND 40),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
-- Folders under one parent have different names, and so do the top-level ones.
CREATE UNIQUE INDEX content_folders_sibling_name ON content_folders (owner_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), name);
-- A problem or post sits in at most one folder of each person: the author and a tester sort the same problem apart.
-- A placement goes with its folder and with its problem or post; one without a row is unsorted.
CREATE TABLE problem_folder_items (
    owner_id text NOT NULL,
    problem_id uuid NOT NULL REFERENCES problem_drafts(id) ON DELETE CASCADE,
    folder_id uuid NOT NULL REFERENCES content_folders(id) ON DELETE CASCADE,
    PRIMARY KEY (owner_id, problem_id)
);
CREATE INDEX problem_folder_items_folder ON problem_folder_items (folder_id);
CREATE TABLE post_folder_items (
    owner_id text NOT NULL,
    post_id uuid NOT NULL REFERENCES blog_posts(id) ON DELETE CASCADE,
    folder_id uuid NOT NULL REFERENCES content_folders(id) ON DELETE CASCADE,
    PRIMARY KEY (owner_id, post_id)
);
CREATE INDEX post_folder_items_folder ON post_folder_items (folder_id);
-- The folder view is a beta each user turns on for their own my page.
ALTER TABLE user_profiles ADD COLUMN folder_view boolean NOT NULL DEFAULT false;
