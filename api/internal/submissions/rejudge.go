package submissions

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/problems"
)

// MaxRejudgeTargets bounds an explicit submission list; an empty list selects every eligible one.
const MaxRejudgeTargets = 100

type RejudgeInput struct {
	Actor     string
	Operator  bool // operators may rejudge any problem
	ProblemID string
	// SubmissionIDs narrows the rejudge; empty means every finished submission of the problem.
	SubmissionIDs   []string
	Image           string
	Runtimes        []string // submission runtimes the judge accepts now
	CheckerRuntimes []string
}

// rejudgeTarget selects finished judgements of the problem. A submission still being judged is
// skipped, so repeated requests never stack. Sample checks and generation jobs keep their result,
// and a retired language cannot be judged again.
const rejudgeTarget = `sub.problem_id=$1 AND sub.status='DONE' AND sub.runtime=ANY($2::text[])
  AND NOT COALESCE((sub.job->>'easyTest')::boolean,false)
  AND NOT COALESCE((sub.job->>'generate')::boolean,false)
  AND NOT COALESCE((sub.job->>'validate')::boolean,false)
  AND (cardinality($3::text[])=0 OR sub.id::text=ANY($3::text[]))`

// Rejudge requeues finished submissions with the judging data the problem has now: the
// contest copy for contest submissions, otherwise the published or private draft, as at
// submission. A new attempt ID makes any late result of the previous run stale.
func (s *Store) Rejudge(ctx context.Context, in RejudgeInput) (int64, error) {
	if len(in.CheckerRuntimes) == 0 {
		in.CheckerRuntimes = []string{"cpp17"}
	}
	if in.SubmissionIDs == nil {
		in.SubmissionIDs = []string{}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Locking the problem serializes rejudges with each other and with saves, so one
	// rejudge never mixes two versions.
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT $3::boolean OR can_manage_problem(id,$2) FROM problem_drafts WHERE id=$1 FOR UPDATE`, in.ProblemID, in.Actor, in.Operator).Scan(&allowed)
	// A setter's right ends when the results are final, and the lock may have waited past that moment, while
	// can_manage_problem judged with the time the statement began. A new statement judges with the time after the wait.
	if err == nil && allowed {
		err = tx.QueryRow(ctx, `SELECT $3::boolean OR can_manage_problem($1,$2)`, in.ProblemID, in.Actor, in.Operator).Scan(&allowed)
	}
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !allowed) {
		return 0, problems.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `UPDATE submissions s SET status='QUEUED',result=NULL,progress=NULL,judge_attempt=gen_random_uuid(),
  dispatched_at=NULL,started_at=NULL,finished_at=NULL,rejudged_at=clock_timestamp(),
  problem_version=t.version,problem_title=t.draft->>'title',
  job=jsonb_build_object('privateDraft',COALESCE(s.job->'privateDraft','false'::jsonb),'image',$4::text,'easyTest',false,
    'cases',t.draft->'testCases','checker',t.draft->'checker','interactor',t.draft->'interactor',
    'timeLimitMs',(t.draft->>'timeLimitMs')::int,'memoryLimitMb',(t.draft->>'memoryLimitMb')::int)
  FROM (
    SELECT sub.id,
      CASE WHEN sub.contest_id IS NULL THEN COALESCE(p.published_draft,p.draft) ELSE cp.draft END AS draft,
      CASE WHEN sub.contest_id IS NULL THEN CASE WHEN p.published_draft IS NULL THEN p.version ELSE p.published_version END ELSE cp.problem_version END AS version
    FROM submissions sub JOIN problem_drafts p ON p.id=sub.problem_id
    LEFT JOIN contest_problems cp ON cp.contest_id=sub.contest_id AND cp.problem_id=sub.problem_id
    WHERE `+rejudgeTarget+`
  ) t
  WHERE s.id=t.id AND s.status='DONE' AND t.draft IS NOT NULL
  AND jsonb_array_length(COALESCE(t.draft->'testCases','[]'::jsonb)) BETWEEN 1 AND 100
  AND (t.draft->>'memoryLimitMb')::int BETWEEN 64 AND 512
  AND (COALESCE(t.draft->'checker','null'::jsonb) = 'null'::jsonb OR
    (t.draft->'checker'->>'runtime'=ANY($5::text[]) AND
     length(btrim(t.draft->'checker'->>'source', E' \t\r\n'))>0 AND
     (s.runtime <> 'cpp17-local' OR t.draft->'checker'->>'runtime'='cpp17')))
  AND (COALESCE(t.draft->'interactor','null'::jsonb) = 'null'::jsonb OR
    (s.runtime <> 'cpp17-local' AND COALESCE(t.draft->'checker','null'::jsonb) = 'null'::jsonb AND
     t.draft->'interactor'->>'runtime'=ANY($5::text[]) AND
     length(btrim(t.draft->'interactor'->>'source', E' \t\r\n'))>0))`,
		in.ProblemID, in.Runtimes, in.SubmissionIDs, in.Image, in.CheckerRuntimes)
	if err != nil {
		return 0, err
	}
	queued := tag.RowsAffected()
	if queued == 0 {
		// Nothing queued: either nothing to rejudge, or the current problem cannot be judged.
		var eligible bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM submissions sub WHERE `+rejudgeTarget+`)`, in.ProblemID, in.Runtimes, in.SubmissionIDs).Scan(&eligible); err != nil {
			return 0, err
		}
		if eligible {
			return 0, ErrNotReady
		}
	}
	return queued, tx.Commit(ctx)
}

// PendingRejudges counts rejudged submissions of the problem that are not judged yet.
func (s *Store) PendingRejudges(ctx context.Context, actor, problemID string, operator bool) (int64, error) {
	var allowed bool
	var pending int64
	err := s.Pool.QueryRow(ctx, `SELECT $3::boolean OR can_manage_problem(id,$2),
  (SELECT COUNT(*) FROM submissions WHERE problem_id=$1 AND rejudged_at IS NOT NULL AND status<>'DONE')
  FROM problem_drafts WHERE id=$1`, problemID, actor, operator).Scan(&allowed, &pending)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !allowed) {
		return 0, problems.ErrNotFound
	}
	return pending, err
}
