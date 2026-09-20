package submissions

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// A normal problem page includes ended contest submissions; a contest page is
// restricted to its own set. Draft/test runs and ongoing contests stay private.
const problemSubmissionVisible = `problem_id=$1 AND ($2='' OR contest_id=NULLIF($2,'')::uuid)
 AND NOT COALESCE((job->>'easyTest')::boolean,false)
 AND NOT COALESCE((job->>'generate')::boolean,false)
 AND NOT COALESCE((job->>'validate')::boolean,false)
 AND (($4 AND owner_id=$3) OR (NOT $4 AND (
  (contest_id IS NULL AND EXISTS(SELECT 1 FROM problem_drafts p WHERE p.id=problem_id
   AND (can_manage_problem(p.id,$3) OR (p.published_draft IS NOT NULL AND NOT COALESCE((submissions.job->>'privateDraft')::boolean,true)))))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=contest_id AND (c.published OR c.owner_id=$3)
   AND (` + contestStaff + ` OR (statement_timestamp()>=c.ends_at AND submissions.created_at>=c.starts_at)))
 )))`

func (s *Store) problemSubmissionsAccess(ctx context.Context, problemID, contestID, viewer string, mine bool) error {
	var allowed bool
	var err error
	if contestID == "" {
		err = s.Pool.QueryRow(ctx, `SELECT published_draft IS NOT NULL OR can_manage_problem(id,$2) FROM problem_drafts WHERE id=$1`, problemID, viewer).Scan(&allowed)
	} else {
		err = s.Pool.QueryRow(ctx, `SELECT `+contestStaff+` OR statement_timestamp()>=c.ends_at OR
   ($4 AND (statement_timestamp()>=c.starts_at OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=cp.problem_id AND t.owner_id=$3)))
   FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=$1 AND c.id=$2 AND (c.published OR c.owner_id=$3)`, problemID, contestID, viewer, mine).Scan(&allowed)
	}
	if err != nil {
		return err
	}
	if !allowed || (mine && viewer == "") {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ProblemList(ctx context.Context, problemID, contestID, viewer string, mine bool, offset int) (ContestSubmissionList, error) {
	if err := s.problemSubmissionsAccess(ctx, problemID, contestID, viewer, mine); err != nil {
		return ContestSubmissionList{}, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+columns+` FROM submissions WHERE `+problemSubmissionVisible+` ORDER BY created_at DESC,id DESC LIMIT 51 OFFSET $5`, problemID, contestID, viewer, mine, offset)
	if err != nil {
		return ContestSubmissionList{}, err
	}
	return submissionList(rows)
}

func (s *Store) ProblemGet(ctx context.Context, problemID, id, viewer string) (Submission, error) {
	if err := s.problemSubmissionsAccess(ctx, problemID, "", viewer, false); err != nil {
		return Submission{}, err
	}
	item, err := scan(s.Pool.QueryRow(ctx, `SELECT `+columns+` FROM submissions WHERE `+problemSubmissionVisible+` AND id=$5`, problemID, "", viewer, false, id))
	return publicSubmission(item), err
}
