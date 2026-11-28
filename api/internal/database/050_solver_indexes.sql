-- Solver counts and solved marks read only the accepted runs that count, so they need neither job nor result, which
-- are a few KB each and TOASTed. Each index holds one kind of those runs; its predicate is word for word a condition
-- of internal/problems/publication.go (plainAcceptedRun and contestAcceptedRun), which lets the planner answer from
-- the index alone. Change both together, or the planner silently stops using the index.
CREATE INDEX submissions_plain_solvers ON submissions(problem_id, owner_id)
    WHERE status='DONE' AND submission_full_score(result)
    AND NOT COALESCE((job->>'easyTest')::boolean,false)
    AND NOT COALESCE((job->>'generate')::boolean,false)
    AND NOT COALESCE((job->>'validate')::boolean,false)
    AND contest_id IS NULL AND NOT COALESCE((job->>'privateDraft')::boolean,true);
-- A contest run counts by its contest and time, which the index carries for that check.
CREATE INDEX submissions_contest_solvers ON submissions(problem_id, owner_id) INCLUDE (contest_id, created_at)
    WHERE status='DONE' AND submission_full_score(result)
    AND NOT COALESCE((job->>'easyTest')::boolean,false)
    AND NOT COALESCE((job->>'generate')::boolean,false)
    AND NOT COALESCE((job->>'validate')::boolean,false)
    AND contest_id IS NOT NULL;
