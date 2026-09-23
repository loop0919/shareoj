package submissions

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func publicSubmission(s Submission) Submission {
	// Public results must not expose private checker diagnostics or test data.
	if s.Result != nil {
		result := &Result{Verdict: s.Result.Verdict, Passed: s.Result.Passed, Total: s.Result.Total,
			CPUTimeMS: s.Result.CPUTimeMS, MemoryBytes: s.Result.MemoryBytes}
		for _, c := range s.Result.Cases {
			result.Cases = append(result.Cases, CaseResult{
				Name: c.Name, Verdict: c.Verdict,
				CPUTimeMS: c.CPUTimeMS, WallTimeMS: c.WallTimeMS, MemoryBytes: c.MemoryBytes,
			})
		}
		s.Result = result
	}
	return s
}

// A tester on any problem can review submissions throughout that contest.
const contestStaff = `(c.owner_id=$3 OR EXISTS (
 SELECT 1 FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id
 WHERE cp.contest_id=c.id AND t.owner_id=$3))`

const contestPublic = `NOT COALESCE((job->>'easyTest')::boolean,false)
 AND NOT COALESCE((job->>'generate')::boolean,false)
 AND NOT COALESCE((job->>'validate')::boolean,false)
 AND EXISTS(SELECT 1 FROM contests c WHERE c.id=$1 AND (
  (contest_id=c.id AND (` + contestStaff + ` OR (statement_timestamp()>=c.ends_at AND submissions.created_at>=c.starts_at)))
  OR (contest_id IS NULL AND ` + contestStaff + ` AND EXISTS(
   SELECT 1 FROM contest_problems cp WHERE cp.contest_id=c.id AND cp.problem_id=submissions.problem_id))))`

func (s *Store) ContestGet(ctx context.Context, contestID, id, viewer string) (Submission, error) {
	item, err := scan(s.Pool.QueryRow(ctx, `SELECT `+columns+` FROM submissions WHERE `+contestPublic+` AND id=$2`, contestID, id, viewer))
	return publicSubmission(item), err
}

type ContestSubmissionList struct {
	Items   []Submission `json:"items"`
	HasMore bool         `json:"hasMore"`
}

func (s *Store) ContestList(ctx context.Context, contestID, viewer string, offset int) (ContestSubmissionList, error) {
	var allowed bool
	err := s.Pool.QueryRow(ctx, `SELECT statement_timestamp()>=c.ends_at OR c.owner_id=$2 OR EXISTS (
 SELECT 1 FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id
 WHERE cp.contest_id=c.id AND t.owner_id=$2) FROM contests c WHERE c.id=$1`, contestID, viewer).Scan(&allowed)
	if err != nil {
		return ContestSubmissionList{}, err
	}
	if !allowed {
		return ContestSubmissionList{}, pgx.ErrNoRows
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+columns+` FROM submissions WHERE `+contestPublic+` ORDER BY created_at DESC,id DESC LIMIT 51 OFFSET $2`, contestID, offset, viewer)
	if err != nil {
		return ContestSubmissionList{}, err
	}
	return submissionList(rows)
}

func submissionList(rows pgx.Rows) (ContestSubmissionList, error) {
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Submission, error) {
		item, e := scan(row)
		item = publicSubmission(item)
		if item.Result != nil {
			item.Result.Cases = nil
		}
		item.Source = ""
		return item, e
	})
	if err != nil {
		return ContestSubmissionList{}, err
	}
	result := ContestSubmissionList{Items: items, HasMore: len(items) > 50}
	if result.HasMore {
		result.Items = items[:50]
	}
	return result, nil
}
