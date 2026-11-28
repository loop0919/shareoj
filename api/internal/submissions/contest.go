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
// hidden from them as from the submitter (ADR 0024). hidden is the casesHidden column.
func teamSubmission(s Submission, teammate, hidden bool) Submission {
	compileLog := ""
	if s.Result != nil {
		compileLog = s.Result.CompileLog
	}
	s = publicSubmission(s)
	if teammate && s.Result != nil {
		s.Result.CompileLog = compileLog
	}
	if hidden {
		s.hideCases()
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

// contestStaffSees tells whether the viewer $3 reviews a submission as staff of the contest row c; contest is c's id as
// in contestSubmissionVisible. From the start, staff review every submission, including the author and testers of any
// problem. Before the start they cannot open each other's problems, so only setters review every problem's submissions
// and the others those to the problems they can open: the code would give away a problem they have not seen.
func contestStaffSees(contest string) string {
	return `((SELECT contest_staff(` + contest + `,$3))
   AND (c.starts_at<=statement_timestamp() OR contest_problem_access(` + contest + `,submissions.problem_id,$3)))`
}

// contestSubmissionVisible tells whether the viewer $3 sees a submission to the contest row c. contest is c's id as an
// SQL expression: a caller fixed to one contest passes it as a parameter so that the role and team subqueries run once,
// and a caller covering several contests passes c.id so that each submission is checked in its own contest.
// The submissions table goes without an alias, and its columns take the table name because c also has owner_id and
// created_at.
// Staff review submissions as contestStaffSees tells.
// After the end the submitter also sees their own frozen submissions, which others see only once the results are final.
// Members of a team see their team's contest submissions throughout, frozen ones included (ADR 0024).
func contestSubmissionVisible(contest string) string {
	return `(contest_submission_public(c,submissions.created_at) OR ` + contestStaffSees(contest) + `
   OR (submissions.owner_id=$3 AND submissions.created_at>=c.starts_at AND c.ends_at<=statement_timestamp())
   OR (submissions.created_at>=c.starts_at AND submissions.owner_id IN (SELECT unnest(contest_teammates(` + contest + `,$3)))))`
}

// Staff also review submissions to the contest's problems made outside any contest.
// $4 keeps only the contest submissions of participants who are onsite now.
// The first three conditions here and in contestOwn are word for word the predicate of the contest's indexes in
// 052_submission_list_indexes.sql, which lets a contest's list and its count skip reading job for each run; they are
// also the expressions of the submissions_job_flags statistics there. Change them together.
var contestPublic = `NOT COALESCE((job->>'easyTest')::boolean,false)
 AND NOT COALESCE((job->>'generate')::boolean,false)
 AND NOT COALESCE((job->>'validate')::boolean,false)
 AND EXISTS(SELECT 1 FROM contests c WHERE c.id=$1 AND (
  (contest_id=c.id AND ` + contestSubmissionVisible("$1::uuid") + `)
  OR (contest_id IS NULL AND ` + contestStaffSees("$1::uuid") + ` AND EXISTS(
   SELECT 1 FROM contest_problems cp WHERE cp.contest_id=c.id AND cp.problem_id=submissions.problem_id))))
 AND (NOT $4 OR (contest_id=$1 AND submissions.owner_id IN (` + onsiteParticipants("$1::uuid") + `)))
 AND (owner_id=$3 OR COALESCE(featured_reveal_at(problem_id),'-infinity')<=statement_timestamp())`

// contestOwn gathers each problem's own list over the contest: the viewer's submissions, with $4='team' also those of
// their current teammates (ADR 0024). They are open at any time, so the list never depends on the contest's state.
var contestOwn = `NOT COALESCE((job->>'easyTest')::boolean,false)
 AND NOT COALESCE((job->>'generate')::boolean,false)
 AND NOT COALESCE((job->>'validate')::boolean,false)
 AND (contest_id=$1::uuid OR (contest_id IS NULL AND (SELECT contest_staff($1::uuid,$3)) AND EXISTS(
  SELECT 1 FROM contest_problems cp WHERE cp.contest_id=$1::uuid AND cp.problem_id=submissions.problem_id)))
 AND (owner_id=$3 OR ($4='team' AND contest_id=$1::uuid AND owner_id IN (SELECT unnest(contest_teammates($1::uuid,$3)))))`

func (s *Store) ContestGet(ctx context.Context, contestID, id, viewer string) (Submission, error) {
	var shared, hidden bool
	item, err := scan(s.Pool.QueryRow(ctx, `SELECT `+columns+`,`+teammate+`,`+casesHidden("$3")+` FROM submissions WHERE `+contestPublic+` AND id=$2`, contestID, id, viewer, false), &shared, &hidden)
	return teamSubmission(item, shared, hidden), err
}

type ContestSubmissionList struct {
	Items   []Submission `json:"items"`
	HasMore bool         `json:"hasMore"`
}

// CountedSubmissionList is a page of the contest's submissions with the number of all of them.
type CountedSubmissionList struct {
	ContestSubmissionList
	Total int `json:"total" doc:"条件に合う提出の総数。"`
}

// ContestList lists the contest's submissions; onsite keeps those of participants who are onsite now.
// ScopeMine and ScopeTeam list the viewer's or their team's own instead, and ignore onsite.
// A problemID keeps the submissions to that problem; empty keeps every problem's. The conditions narrow and order the
// list inside what the viewer may see, and Total counts the submissions left after them.
func (s *Store) ContestList(ctx context.Context, contestID, viewer string, scope Scope, offset, limit int, onsite bool, problemID string, conditions ListConditions) (CountedSubmissionList, error) {
	allowed, staff, err := s.contestListAccess(ctx, contestID, viewer, scope)
	if err != nil {
		return CountedSubmissionList{}, err
	}
	if !allowed || (scope != ScopeAll && viewer == "") {
		return CountedSubmissionList{}, pgx.ErrNoRows
	}
	q := contestListQueries(contestID, viewer, scope, staff, offset, limit, onsite, problemID, conditions)
	rows, err := s.Pool.Query(ctx, q.page, plannedPerCall(q.pageArgs)...)
	if err != nil {
		return CountedSubmissionList{}, err
	}
	page, err := submissionList(rows, limit)
	if err != nil {
		return CountedSubmissionList{}, err
	}
	result := CountedSubmissionList{ContestSubmissionList: page}
	err = s.Pool.QueryRow(ctx, q.count, plannedPerCall(q.countArgs)...).Scan(&result.Total)
	return result, err
}

// contestListAccess tells whether the viewer may list the contest's submissions in scope, and whether they are its
// staff, who alone see the plain runs of its problems in its list.
func (s *Store) contestListAccess(ctx context.Context, contestID, viewer string, scope Scope) (allowed, staff bool, err error) {
	// Members of a team may list from the start; contestPublic keeps only their team's submissions until the end.
	err = s.Pool.QueryRow(ctx, `SELECT $3 OR c.ends_at<=statement_timestamp() OR contest_staff(c.id,$2)
 OR (c.starts_at<=statement_timestamp() AND cardinality(contest_teammates(c.id,$2))>0),contest_staff(c.id,$2) FROM contests c WHERE c.id=$1`, contestID, viewer, scope != ScopeAll).Scan(&allowed, &staff)
	return allowed, staff, err
}

// contestListStatements are ContestList's page and count queries with their arguments.
type contestListStatements struct {
	page, count         string
	pageArgs, countArgs []any
}

// contestListQueries builds ContestList's statements. For staff, both read the list as its two halves, contestRuns and
// contestPlainRuns, each under the whole filter. A page takes from each half its first offset+limit+1 runs in the
// list's order, through the index for that order, and cuts the page from the two; no run further down either half can
// reach the page. For anyone else, the plain half holds nothing (see contestPlainRuns), so the statements read
// contestRuns alone and a page is cut straight from its index. Either way, the page is cut from the runs' id and
// sort keys, and listPage reads the columns of its runs alone.
func contestListQueries(contestID, viewer string, scope Scope, staff bool, offset, limit int, onsite bool, problemID string, conditions ListConditions) contestListStatements {
	filter, narrow := contestPublic, any(onsite)
	if scope != ScopeAll {
		filter, narrow = contestOwn, string(scope)
	}
	filter += ` AND ($5::uuid IS NULL OR problem_id=$5::uuid)`
	var problem any
	if problemID != "" {
		problem = problemID
	}
	// The page query takes $1 to $6 before the conditions; the count query has no $6, so its conditions start at $6.
	pageNarrow, pageArgs := conditions.where(7)
	countNarrow, countArgs := conditions.where(6)
	listed := func(selected, narrow, tail string) string {
		half := func(bound string) string {
			return `SELECT ` + selected + ` FROM submissions WHERE ` + bound + ` AND ` + filter + narrow + tail
		}
		if !staff {
			return half(contestRuns)
		}
		return `(` + half(contestRuns) + `) UNION ALL (` + half(contestPlainRuns) + `)`
	}
	order := ` ORDER BY ` + conditions.order()
	ids := listed(`id`, pageNarrow, order+` LIMIT $6 OFFSET $2`)
	if staff {
		// Each half hands on the sort keys of its runs, so that the two are merged by them without computing them again.
		keys := `id,created_at`
		if key := conditions.key(); key != "" {
			keys += `,` + key + ` AS sort_key`
		}
		ids = `SELECT id FROM (` + listed(keys, pageNarrow, order+` LIMIT $2::bigint+$6::bigint`) + `) listed ORDER BY ` +
			conditions.orderBy(`sort_key`) + ` LIMIT $6 OFFSET $2`
	}
	// The filter leaves $2 unused in the count, so PostgreSQL cannot type it unless the query refers to it.
	return contestListStatements{
		page:      listPage(ids, conditions),
		pageArgs:  append([]any{contestID, offset, viewer, narrow, problem, limit + 1}, pageArgs...),
		count:     `SELECT count(*) FROM (` + listed(`1`, countNarrow+` AND $2::integer>=0`, "") + `) listed`,
		countArgs: append([]any{contestID, offset, viewer, narrow, problem}, countArgs...),
	}
}

// contestRuns and contestPlainRuns split a contest's list in two: the contest's own runs and the plain runs of its
// problems. contestPublic and contestOwn keep no other run, so ANDing each half with them and adding the two gives the
// same rows (TestContestListMatchesReferencePostgres compares them with the filter read in one scan). Each half bounds
// its scan by what an index finds, instead of a scan of every run of the table: contestRuns by contest_id, the key of
// submissions_contest_order and of the indexes of 052_submission_list_indexes.sql, and contestPlainRuns by the
// contest's problems. Only staff see plain runs in a contest's list (contestStaffSees and contestOwn both ask
// contest_staff), so that half asks it first, once, and reads nothing for anyone else. ContestList leaves that half
// out for a viewer whom contestListAccess finds not to be staff; for staff it keeps the half with its own check.
const (
	contestRuns      = `contest_id=$1::uuid`
	contestPlainRuns = `contest_id IS NULL AND (SELECT contest_staff($1::uuid,$3)) AND problem_id=ANY(ARRAY(SELECT cp.problem_id FROM contest_problems cp WHERE cp.contest_id=$1::uuid))`
)

// plannedPerCall makes pgx run a list's statement as the unnamed statement, which PostgreSQL plans with the call's
// values every time. The best plan of a list depends on them: which problem or contest, how many runs it holds, who
// reads it and under which conditions. By default pgx prepares a statement once per connection, and after five calls
// PostgreSQL may keep a generic plan, costed for a problem or contest of average size, that for a large contest sorts
// every run instead of reading a page of an index. This does not help if the database forces generic plans
// (plan_cache_mode=force_generic_plan), so the database must not force generic plans. TestSubmissionListSpeedPostgres
// times the lists after many calls on one connection, and TestSubmissionListsArePlannedPerCallPostgres checks that
// they leave no prepared statement.
// pgx still caches each statement's description on the connection: the types of its parameters, by which it encodes
// the values, and of its columns, by which it asks for each result in text or binary. So a migration that changes the
// type of a column these lists select needs the connections recycled after it runs, and the API is deployed before its
// migrations, so that takes new instances or another deploy. Until then pgx decodes each result by the types the
// server reports for that call; a call that fails to encode, run or scan drops the description, and the next call
// describes the statement again.
func plannedPerCall(args []any) []any {
	return append([]any{pgx.QueryExecModeCacheDescribe}, args...)
}

// listPage selects the columns of the runs that ids picks, in the list's order. ids is a query of their id that orders
// the list and cuts the page from it, and it reads nothing else: a run before the page costs its read through the
// index, the visibility check and its sort key, and the columns, with the job and the subqueries they read, are read
// for the runs of the page alone.
func listPage(ids string, conditions ListConditions) string {
	return `SELECT ` + columns + ` FROM (` + ids + `) page JOIN submissions USING (id) ORDER BY ` + conditions.order()
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
