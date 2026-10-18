-- A draft makes its owner a setter only while no contest has its id. Publication keeps the id and deletes the draft,
-- so a stray draft under a published contest's id must not make its owner a setter of someone else's contest.
CREATE OR REPLACE FUNCTION contest_setter(contest uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM contests WHERE id=contest AND owner_id=actor)
        OR (EXISTS(SELECT 1 FROM contest_drafts WHERE id=contest AND owner_id=actor)
            AND NOT EXISTS(SELECT 1 FROM contests WHERE id=contest))
        OR EXISTS(SELECT 1 FROM contest_setters WHERE contest_id=contest AND owner_id=actor AND left_at IS NULL)
$$;
