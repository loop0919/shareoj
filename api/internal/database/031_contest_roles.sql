-- ADR 0022: contest queries ask these functions who is a setter or staff instead of copying the rules.
-- Functions that read other tables never call each other: PostgreSQL plans a SQL function called from
-- another SQL function again on every call. The rest are plain expressions that the planner inlines.

-- People who stopped being staff, such as the author and testers of a problem removed before the start.
-- They saw the problems, so they never compete in that contest. A contest and its draft share the id.
CREATE TABLE contest_exclusions (
    contest_id uuid NOT NULL,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (contest_id, owner_id)
);

-- Setters edit and publish the contest. The owner is the only setter until co-setters arrive.
CREATE FUNCTION contest_setter(contest uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM contests WHERE id=contest AND owner_id=actor)
$$;

-- Authors and invited testers of the contest's problems. Only testers who joined before moment count.
CREATE FUNCTION contest_problem_staff(contest uuid, actor text, moment timestamptz) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM contest_problems cp JOIN problem_drafts d ON d.id=cp.problem_id
            WHERE cp.contest_id=contest AND d.owner_id=actor)
        OR EXISTS(SELECT 1 FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id
            WHERE cp.contest_id=contest AND t.owner_id=actor AND t.created_at<moment)
$$;

CREATE FUNCTION contest_excluded(contest uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM contest_exclusions WHERE contest_id=contest AND owner_id=actor)
$$;

-- Staff review every contest submission and never compete.
CREATE FUNCTION contest_staff(contest uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT contest_setter(contest, actor) OR contest_problem_staff(contest, actor, 'infinity')
$$;

-- Before the start, a problem of the contest is shown only to setters and to its own author and testers.
CREATE FUNCTION contest_problem_access(contest uuid, problem uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT contest_setter(contest, actor) OR can_manage_problem(problem, actor)
$$;

-- When the results become final and public. ADR 0023 moves it to the reveal for frozen contests;
-- NULL means that moment has not come yet.
CREATE FUNCTION contest_results_final_at(c contests) RETURNS timestamptz
LANGUAGE sql STABLE AS $$
    SELECT c.ends_at
$$;

CREATE FUNCTION contest_results_final(c contests) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(contest_results_final_at(c)<=statement_timestamp(), false)
$$;

-- Whether anyone may see a contest submission. Staff runs from before the start stay private.
CREATE FUNCTION contest_submission_public(c contests, submitted_at timestamptz) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT submitted_at>=c.starts_at AND contest_results_final(c)
$$;

-- Whether actor may compete and appear in the standings. Anyone who was staff before the results were
-- final never competes, even after leaving the staff; a participant who tests only afterwards keeps the rank.
CREATE FUNCTION contest_competitor(c contests, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT NOT contest_setter(c.id, actor)
        AND NOT contest_problem_staff(c.id, actor, COALESCE(contest_results_final_at(c), 'infinity'))
        AND NOT contest_excluded(c.id, actor)
$$;
