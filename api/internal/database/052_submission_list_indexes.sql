-- ANALYZE gathers how many runs the three flags and each verdict (ListConditions.where) leave. Without these the
-- planner guesses that each flag keeps half the runs and a verdict almost none, and then sorts every run of the problem
-- or the contest instead of reading the first page of an index in order. The expressions are word for word those of the
-- lists. CREATE STATISTICS gathers nothing by itself, and autovacuum analyses the table only after a tenth of its rows
-- have changed, so the migration analyses it here. It does so before building the indexes: ANALYZE and CREATE
-- STATISTICS take a lock that lets submissions be written, which the index builds then stop until the migration commits.
CREATE STATISTICS submissions_job_flags ON (COALESCE((job->>'easyTest')::boolean,false)), (COALESCE((job->>'generate')::boolean,false)),
    (COALESCE((job->>'validate')::boolean,false)) FROM submissions;
CREATE STATISTICS submissions_verdict ON (result->>'verdict') FROM submissions;
ANALYZE submissions;
-- The submission lists sort by code length and by time (ListConditions.order in internal/submissions/conditions.go).
-- Without an index in that order, a page reads and sorts every run of the problem or the contest, along with its job
-- and result, which are a few KB each and TOASTed. Each index keys a sort after the problem or the contest, word for
-- word as order writes it, so that a page reads only its own runs. A migration that replaces submission_cpu_time_ms
-- must also REINDEX submissions_problem_fastest and submissions_contest_fastest, since they hold the times the old
-- function computed. The problem lists count nothing and read about a page of runs, so their indexes keep every run.
CREATE INDEX submissions_problem_shortest ON submissions(problem_id, octet_length(source), created_at DESC, id DESC);
CREATE INDEX submissions_problem_fastest ON submissions(problem_id, submission_cpu_time_ms(result), created_at DESC, id DESC);
-- A contest's list reads the contest's own runs by contest_id (contestRuns in internal/submissions/contest.go), and its
-- count reads every one of them. These indexes hold only the runs that the list does not leave out as easy tests,
-- generator runs or validator runs: their predicate is word for word the first three conditions of contestPublic and
-- contestOwn, which lets the planner skip those conditions, and with them reading job, for every run it counts. Change
-- both together, or the count silently goes back to reading the job of every run.
CREATE INDEX submissions_contest_shortest ON submissions(contest_id, octet_length(source), created_at DESC, id DESC)
    WHERE contest_id IS NOT NULL
    AND NOT COALESCE((job->>'easyTest')::boolean,false)
    AND NOT COALESCE((job->>'generate')::boolean,false)
    AND NOT COALESCE((job->>'validate')::boolean,false);
CREATE INDEX submissions_contest_fastest ON submissions(contest_id, submission_cpu_time_ms(result), created_at DESC, id DESC)
    WHERE contest_id IS NOT NULL
    AND NOT COALESCE((job->>'easyTest')::boolean,false)
    AND NOT COALESCE((job->>'generate')::boolean,false)
    AND NOT COALESCE((job->>'validate')::boolean,false);
