CREATE TABLE contest_participants (
    contest_id uuid NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    owner_id text NOT NULL REFERENCES user_profiles(owner_id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (contest_id, owner_id)
);

-- Preserve contestants who submitted before explicit registration was introduced.
INSERT INTO contest_participants (contest_id, owner_id, created_at)
SELECT c.id, s.owner_id, min(s.created_at)
FROM submissions s
JOIN contests c ON c.id=s.contest_id
JOIN contest_problems cp ON cp.contest_id=c.id AND cp.problem_id=s.problem_id
WHERE s.created_at>=c.starts_at AND s.created_at<c.ends_at
  AND s.owner_id<>c.owner_id
  AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
  AND NOT EXISTS (
    SELECT 1 FROM contest_problems other JOIN problem_testers t ON t.problem_id=other.problem_id
    WHERE other.contest_id=c.id AND t.owner_id=s.owner_id
  )
GROUP BY c.id, s.owner_id;
