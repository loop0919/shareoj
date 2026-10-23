package submissions

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Scope narrows a problem's submission list: ScopeMine to the viewer's own submissions, ScopeTeam to those of the viewer
// and their current teammates in the contest (ADR 0024).
type Scope string

const (
	ScopeAll  Scope = ""
	ScopeMine Scope = "mine"
	ScopeTeam Scope = "team"
)

// Staff can review ordinary draft submissions from the contest problem page.
// Public views still hide draft runs, and contest submissions follow contestSubmissionVisible in their own contest
// (c.id), since $2 may be empty.
// The author and testers also review a featured problem's submissions before they unlock.
// Roles in the requested contest are subqueries so that they are checked once, not for every row.
// $4 is the Scope, and $6 keeps only the contest submissions of participants who are onsite now.
var problemSubmissionVisible = `problem_id=$1 AND ($2='' OR contest_id=NULLIF($2,'')::uuid OR
 (contest_id IS NULL AND (SELECT contest_staff(NULLIF($2,'')::uuid,$3))))
 AND NOT COALESCE((job->>'easyTest')::boolean,false)
 AND NOT COALESCE((job->>'generate')::boolean,false)
 AND NOT COALESCE((job->>'validate')::boolean,false)
 AND (($4<>'' AND owner_id=$3)
  OR ($4='team' AND contest_id=NULLIF($2,'')::uuid AND owner_id IN (SELECT unnest(contest_teammates(NULLIF($2,'')::uuid,$3))))
  OR ($4='' AND (
  (contest_id IS NULL AND EXISTS(SELECT 1 FROM problem_drafts p WHERE p.id=problem_id
   AND (can_manage_problem(p.id,$3) OR (SELECT contest_staff(NULLIF($2,'')::uuid,$3))
    OR (p.published_draft IS NOT NULL AND NOT COALESCE((submissions.job->>'privateDraft')::boolean,true)))))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=contest_id AND ` + contestSubmissionVisible("c.id") + `)
 )))
 AND (NOT $6 OR (contest_id=NULLIF($2,'')::uuid AND submissions.owner_id IN (` + onsiteParticipants("NULLIF($2,'')::uuid") + `)))
 AND (owner_id=$3 OR can_manage_problem(problem_id,$3) OR COALESCE(featured_reveal_at(problem_id),'-infinity')<=statement_timestamp())`

func (s *Store) problemSubmissionsAccess(ctx context.Context, problemID, contestID, viewer string, scope Scope) error {
	var allowed bool
	var err error
	if contestID == "" {
		err = s.Pool.QueryRow(ctx, `SELECT published_draft IS NOT NULL OR can_manage_problem(id,$2) FROM problem_drafts WHERE id=$1`, problemID, viewer).Scan(&allowed)
	} else {
		// Staff already cover each problem's own testers before the start.
		err = s.Pool.QueryRow(ctx, `SELECT c.ends_at<=statement_timestamp() OR contest_staff(c.id,$3) OR ($4<>'' AND statement_timestamp()>=c.starts_at)
   FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=$1 AND c.id=$2`, problemID, contestID, viewer, string(scope)).Scan(&allowed)
	}
	if err != nil {
		return err
	}
	if !allowed || (scope != ScopeAll && viewer == "") {
		return pgx.ErrNoRows
	}
	return nil
}

// ProblemList lists a problem's submissions, in the contest when contestID is set. onsite keeps the contest submissions
// of participants who are onsite now.
func (s *Store) ProblemList(ctx context.Context, problemID, contestID, viewer string, scope Scope, onsite bool, offset int) (ContestSubmissionList, error) {
	if err := s.problemSubmissionsAccess(ctx, problemID, contestID, viewer, scope); err != nil {
		return ContestSubmissionList{}, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+columns+` FROM submissions WHERE `+problemSubmissionVisible+` ORDER BY created_at DESC,id DESC LIMIT 51 OFFSET $5`, problemID, contestID, viewer, string(scope), offset, onsite)
	if err != nil {
		return ContestSubmissionList{}, err
	}
	return submissionList(rows, 50)
}

func (s *Store) ProblemGet(ctx context.Context, problemID, id, viewer string) (Submission, error) {
	if err := s.problemSubmissionsAccess(ctx, problemID, "", viewer, ScopeAll); err != nil {
		return Submission{}, err
	}
	var shared bool
	item, err := scan(s.Pool.QueryRow(ctx, `SELECT `+columns+`,`+teammate+` FROM submissions WHERE `+problemSubmissionVisible+` AND id=$5`, problemID, "", viewer, string(ScopeAll), id, false), &shared)
	return teamSubmission(item, shared), err
}

// Resolve a shared ID through the same visibility rules as the submission lists.
func (s *Store) VisibleGet(ctx context.Context, id, viewer string) (Submission, error) {
	var problemID, contestID string
	if err := s.Pool.QueryRow(ctx, `SELECT problem_id,COALESCE(contest_id::text,'') FROM submissions WHERE id=$1`, id).Scan(&problemID, &contestID); err != nil {
		return Submission{}, err
	}
	if contestID != "" {
		return s.ContestGet(ctx, contestID, id, viewer)
	}
	return s.ProblemGet(ctx, problemID, id, viewer)
}
