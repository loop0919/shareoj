-- ADR 0024 (changed 2026-10-19): a team contest also takes participants who enter alone, without a team. Their division
-- is on their own row, as in an individual contest; a member's is their team's.
CREATE OR REPLACE FUNCTION contest_onsite(c contests, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT c.onsite_division AND EXISTS(SELECT 1 FROM contest_participants p LEFT JOIN contest_teams t ON t.id=p.team_id
        WHERE p.contest_id=c.id AND p.owner_id=actor
            AND CASE WHEN p.team_id IS NULL THEN p.onsite_at ELSE t.onsite_at END IS NOT NULL)
$$;
