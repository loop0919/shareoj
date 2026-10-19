-- ADR 0025: a run solves its problem when it reaches the full score, not when its verdict is AC. A result without
-- scoring is all-or-nothing, so AC is its full score, as before; a sample check never scores.
CREATE FUNCTION submission_full_score(result jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
    SELECT COALESCE(CASE WHEN jsonb_typeof(result->'scoring')='object'
        THEN (result->'scoring'->>'score')::bigint=(result->'scoring'->>'maxScore')::bigint
        ELSE result->>'verdict'='AC' END, false)
$$;

-- The first-accept notification follows the same rule: the first full score from someone outside the problem.
CREATE OR REPLACE FUNCTION notify_first_accept() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status='DONE' AND submission_full_score(NEW.result)
       AND NOT COALESCE((NEW.job->>'easyTest')::boolean,false)
       AND NOT COALESCE((NEW.job->>'generate')::boolean,false)
       AND NOT COALESCE((NEW.job->>'validate')::boolean,false)
       AND NOT can_manage_problem(NEW.problem_id,NEW.owner_id) THEN
        INSERT INTO notifications(owner_id,problem_id,actor_id,kind)
        SELECT owner_id,id,NEW.owner_id,'first_accept' FROM problem_drafts WHERE id=NEW.problem_id
        ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END
$$;
