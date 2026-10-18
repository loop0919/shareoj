-- ADR 0024: members of a team see each other's contest submissions, also during the contest. This lists the members of
-- actor's team in a team contest, actor included, and is empty when actor is in no team. Membership is read now:
-- nobody leaves a team after the start, so a submission belongs to its submitter's current team.
CREATE FUNCTION contest_teammates(contest uuid, actor text) RETURNS text[]
LANGUAGE sql STABLE AS $$
    SELECT ARRAY(SELECT m.owner_id FROM contest_participants v JOIN contests c ON c.id=v.contest_id AND c.team_mode
        JOIN contest_participants m ON m.contest_id=v.contest_id AND m.team_id=v.team_id
        WHERE v.contest_id=contest AND v.owner_id=actor)
$$;

-- Whether actor is in the onsite division now: by their own flag in an individual contest, by their team's in a team
-- contest. Filters read it at display time, so a setter's switch also moves the earlier submissions.
CREATE FUNCTION contest_onsite(c contests, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT c.onsite_division AND EXISTS(SELECT 1 FROM contest_participants p LEFT JOIN contest_teams t ON t.id=p.team_id
        WHERE p.contest_id=c.id AND p.owner_id=actor
            AND CASE WHEN c.team_mode THEN t.onsite_at ELSE p.onsite_at END IS NOT NULL)
$$;
