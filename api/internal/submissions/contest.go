package submissions

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func publicSubmission(s Submission) Submission {
	// Public results must not expose private checker diagnostics or test data. Scores are public like verdicts; the
	// aggregator's diagnostics stay in the checker log.
	if s.Result != nil {
		result := &Result{Verdict: s.Result.Verdict, Passed: s.Result.Passed, Total: s.Result.Total,
			CPUTimeMS: s.Result.CPUTimeMS, MemoryBytes: s.Result.MemoryBytes}
		if s.Result.Scoring != nil {
			score := *s.Result.Scoring
			score.Groups = append([]GroupScore(nil), score.Groups...)
			result.Scoring = &score
		}
		for _, c := range s.Result.Cases {
			result.Cases = append(result.Cases, CaseResult{
				Name: c.Name, Verdict: c.Verdict,
				CPUTimeMS: c.CPUTimeMS, WallTimeMS: c.WallTimeMS, MemoryBytes: c.MemoryBytes, Score: c.Score,
			})
		}
		s.Result = result
	}
	return s
}

// teamSubmission is publicSubmission for a detail view. Teammates also read the compile errors; checker diagnostics stay
// hidden from them as from the submitter (ADR 0024).
func teamSubmission(s Submission, teammate bool) Submission {
	compileLog := ""
	if s.Result != nil {
		compileLog = s.Result.CompileLog
	}
	s = publicSubmission(s)
	if teammate && s.Result != nil {
		s.Result.CompileLog = compileLog
	}
	return s
}

// teammate is a column telling whether the viewer $3 is on the submitter's team in the submission's contest.
const teammate = `contest_id IS NOT NULL AND owner_id=ANY(contest_teammates(contest_id,$3))`

// onsiteParticipants lists the competitors of contest who are onsite now. The onsite filter keeps their submissions to
// the contest, practice after the end included, and leaves out staff and others practising (ADR 0024).
func onsiteParticipants(contest string) string {
	return `SELECT p.owner_id FROM contest_participants p JOIN contests c ON c.id=p.contest_id
 WHERE p.contest_id=` + contest + ` AND contest_onsite(c,p.owner_id) AND contest_competitor(c,p.owner_id)`
}

// contestSubmissionVisible tells whether the viewer $3 sees a submission to the contest row c. contest is c's id as an
// SQL expression: a caller fixed to one contest passes it as a parameter so that the role and team subqueries run once,
// and a caller covering several contests passes c.id so that each submission is checked in its own contest.
// The submissions table goes without an alias, and its columns take the table name because c also has owner_id and
// created_at.
// Staff, including the author and testers of any problem, review submissions throughout the contest.
// After the end the submitter also sees their own frozen submissions, which others see only once the results are final.
// Members of a team see their team's contest submissions throughout, frozen ones included (ADR 0024).
func contestSubmissionVisible(contest string) string {
	return `(contest_submission_public(c,submissions.created_at) OR (SELECT contest_staff(` + contest + `,$3))
   OR (submissions.owner_id=$3 AND submissions.created_at>=c.starts_at AND c.ends_at<=statement_timestamp())
   OR (submissions.created_at>=c.starts_at AND submissions.owner_id IN (SELECT unnest(contest_teammates(` + contest + `,$3)))))`
}

// Staff also review submissions to the contest's problems made outside any contest.
// $4 keeps only the contest submissions of participants who are onsite now.
var contestPublic = `NOT COALESCE((job->>'easyTest')::boolean,false)
 AND NOT COALESCE((job->>'generate')::boolean,false)
 AND NOT COALESCE((job->>'validate')::boolean,false)
 AND EXISTS(SELECT 1 FROM contests c WHERE c.id=$1 AND (
  (contest_id=c.id AND ` + contestSubmissionVisible("$1::uuid") + `)
  OR (contest_id IS NULL AND (SELECT contest_staff($1::uuid,$3)) AND EXISTS(
   SELECT 1 FROM contest_problems cp WHERE cp.contest_id=c.id AND cp.problem_id=submissions.problem_id))))
 AND (NOT $4 OR (contest_id=$1 AND submissions.owner_id IN (` + onsiteParticipants("$1::uuid") + `)))
 AND (owner_id=$3 OR COALESCE(featured_reveal_at(problem_id),'-infinity')<=statement_timestamp())`

func (s *Store) ContestGet(ctx context.Context, contestID, id, viewer string) (Submission, error) {
	var shared bool
	item, err := scan(s.Pool.QueryRow(ctx, `SELECT `+columns+`,`+teammate+` FROM submissions WHERE `+contestPublic+` AND id=$2`, contestID, id, viewer, false), &shared)
	return teamSubmission(item, shared), err
}

type ContestSubmissionList struct {
	Items   []Submission `json:"items"`
	HasMore bool         `json:"hasMore"`
}

// CountedSubmissionList is a page of the contest's submissions with the number of all of them.
type CountedSubmissionList struct {
	ContestSubmissionList
	Total int `json:"total"`
}

// ContestList lists the contest's submissions; onsite keeps those of participants who are onsite now.
func (s *Store) ContestList(ctx context.Context, contestID, viewer string, offset, limit int, onsite bool) (CountedSubmissionList, error) {
	var allowed bool
	// Members of a team may list from the start; contestPublic keeps only their team's submissions until the end.
	err := s.Pool.QueryRow(ctx, `SELECT c.ends_at<=statement_timestamp() OR contest_staff(c.id,$2)
 OR (c.starts_at<=statement_timestamp() AND cardinality(contest_teammates(c.id,$2))>0) FROM contests c WHERE c.id=$1`, contestID, viewer).Scan(&allowed)
	if err != nil {
		return CountedSubmissionList{}, err
	}
	if !allowed {
		return CountedSubmissionList{}, pgx.ErrNoRows
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+columns+` FROM submissions WHERE `+contestPublic+` ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $2`, contestID, offset, viewer, onsite, limit+1)
	if err != nil {
		return CountedSubmissionList{}, err
	}
	page, err := submissionList(rows, limit)
	if err != nil {
		return CountedSubmissionList{}, err
	}
	result := CountedSubmissionList{ContestSubmissionList: page}
	// contestPublic leaves $2 unused here, so PostgreSQL cannot type it unless the query refers to it.
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE `+contestPublic+` AND $2::integer>=0`, contestID, offset, viewer, onsite).Scan(&result.Total)
	return result, err
}

func submissionList(rows pgx.Rows, limit int) (ContestSubmissionList, error) {
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Submission, error) {
		item, e := scan(row)
		item = publicSubmission(item)
		if item.Result != nil {
			item.Result.Cases = nil
			// A list shows the score only; the group table belongs to the detail.
			if item.Result.Scoring != nil {
				item.Result.Scoring.Groups = nil
			}
		}
		item.Source = ""
		return item, e
	})
	if err != nil {
		return ContestSubmissionList{}, err
	}
	result := ContestSubmissionList{Items: items, HasMore: len(items) > limit}
	if result.HasMore {
		result.Items = items[:limit]
	}
	return result, nil
}
