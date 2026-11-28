package submissions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// planNode is the part of an EXPLAIN (FORMAT JSON) node that the test reads.
type planNode struct {
	NodeType     string     `json:"Node Type"`
	RelationName string     `json:"Relation Name"`
	IndexName    string     `json:"Index Name"`
	Filter       string     `json:"Filter"`
	Plans        []planNode `json:"Plans"`
}

// indexScan finds the scan of submissions on index and tells whether a sort lies between it and the nearest limit
// above it, and what the scan filters on. A contest's page sorts again the two short pages of its halves, which is
// cheap; a sort under the limit would read every run of the list.
func indexScan(n planNode, index string, sorted bool) (found, sortedAbove bool, filter string) {
	if n.RelationName == "submissions" && n.IndexName == index {
		return true, sorted, n.Filter
	}
	sorted = n.NodeType != "Limit" && (sorted || strings.HasSuffix(n.NodeType, "Sort"))
	for _, child := range n.Plans {
		if found, sortedAbove, filter = indexScan(child, index, sorted); found {
			return found, sortedAbove, filter
		}
	}
	return false, false, ""
}

// TestSubmissionListsUseTheirIndexesPostgres checks that the submission lists sorted by code length and by time still
// read a page in the order of the indexes of 052_submission_list_indexes.sql, and that a contest's list and its count
// still read the contest's runs through the indexes whose predicate leaves out the jobs the lists never show. The
// planner uses an index for the order only while the ORDER BY matches its keys, and skips reading job only while the
// WHERE implies its predicate, so rewording order in conditions.go or the first conditions of contestPublic silently
// sends the lists back to reading the job and result of every run, without changing any result.
func TestSubmissionListsUseTheirIndexesPostgres(t *testing.T) {
	ctx, pool := newListTestPool(t, "submission_list_plans")
	exec := func(query string) {
		t.Helper()
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	const problem, contest = "a1000000-0000-4000-8000-000000000001", "a2000000-0000-4000-8000-000000000001"
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob')`)
	exec(`INSERT INTO problem_drafts(id,owner_id,draft,version,published_draft,published_version,published_at)
		VALUES ('` + problem + `','alice','{"title":"p","timeLimitMs":"1000","memoryLimitMb":"256"}',1,'{"title":"p","timeLimitMs":"1000","memoryLimitMb":"256"}',1,now())`)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES ('` + contest + `','alice','c','',now()-interval '3 hours',now()-interval '1 hour',true)`)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) VALUES ('` + contest + `','` + problem + `',0,100,'{}',1)`)
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at) VALUES
		(gen_random_uuid(),'bob','` + problem + `',1,'t','cpp17','a','{"privateDraft":false}','DONE','{"verdict":"AC","cases":[{"cpuTimeMs":1}]}',NULL,now()),
		(gen_random_uuid(),'bob','` + problem + `',1,'t','cpp17','ab','{"privateDraft":false}','DONE','{"verdict":"WA","cases":[{"cpuTimeMs":2}]}','` + contest + `',now()-interval '2 hours')`)
	exec(`ANALYZE submissions`)

	cpp, _ := LanguageRuntimeIDs("cpp")
	narrowed := ListConditions{Sort: SortFastest, Verdict: "AC", Runtimes: StoredRuntimeIDs(cpp), User: "bob"}
	shortestProblem, shortestProblemArgs := problemListQuery(problem, "", "bob", ScopeAll, false, 0, ListConditions{Sort: SortShortest})
	fastestProblem, fastestProblemArgs := problemListQuery(problem, contest, "bob", ScopeAll, false, 0, ListConditions{Sort: SortFastest})
	newest := contestListQueries(contest, "bob", ScopeAll, 0, 50, false, "", ListConditions{})
	shortest := contestListQueries(contest, "bob", ScopeAll, 0, 50, false, "", ListConditions{Sort: SortShortest})
	fastest := contestListQueries(contest, "alice", ScopeAll, 50, 50, true, problem, narrowed)
	own := contestListQueries(contest, "bob", ScopeTeam, 0, 50, false, "", ListConditions{Sort: SortFastest})
	for _, statement := range []struct {
		name  string
		query string
		args  []any
		index string
		// listed tells whether the index's predicate must take the place of the conditions on job.
		listed, ordered bool
	}{
		{"the problem's list sorted by code length", shortestProblem, shortestProblemArgs, "submissions_problem_shortest", false, true},
		{"a contest problem's list sorted by time", fastestProblem, fastestProblemArgs, "submissions_problem_fastest", false, true},
		{"the contest's newest runs", newest.page, newest.pageArgs, "submissions_contest_order", false, true},
		{"the contest's list sorted by code length", shortest.page, shortest.pageArgs, "submissions_contest_shortest", true, true},
		{"the contest's list sorted by time under every condition", fastest.page, fastest.pageArgs, "submissions_contest_fastest", true, true},
		{"the contest's own runs sorted by time", own.page, own.pageArgs, "submissions_contest_fastest", true, true},
		{"the contest's count", newest.count, newest.countArgs, "submissions_contest_shortest", true, false},
		{"the contest's count under every condition", fastest.count, fastest.countArgs, "submissions_contest_fastest", true, false},
	} {
		plan, err := func() (planNode, error) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return planNode{}, err
			}
			defer func() { _ = tx.Rollback(ctx) }()
			// The question is whether the planner may use the index at all, not which plan is cheapest on two runs. So,
			// in a transaction that is rolled back, the other indexes on submissions are dropped and sequential scans,
			// bitmap scans and sorts are priced out: the planner then reads the runs through the index wherever the WHERE
			// allows it, and takes its order wherever the ORDER BY matches it.
			rows, err := tx.Query(ctx, `SELECT indexrelid::regclass::text FROM pg_index WHERE indrelid='submissions'::regclass AND NOT indisprimary
				AND indexrelid::regclass::text<>$1`, statement.index)
			if err != nil {
				return planNode{}, err
			}
			others, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return planNode{}, err
			}
			for _, index := range others {
				if _, err = tx.Exec(ctx, `DROP INDEX `+pgx.Identifier{index}.Sanitize()); err != nil {
					return planNode{}, err
				}
			}
			for _, setting := range []string{"enable_seqscan", "enable_bitmapscan", "enable_sort"} {
				if _, err = tx.Exec(ctx, `SET LOCAL `+setting+`=off`); err != nil {
					return planNode{}, err
				}
			}
			var raw string
			if err = tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+statement.query, statement.args...).Scan(&raw); err != nil {
				return planNode{}, err
			}
			var plans []struct{ Plan planNode }
			if err = json.Unmarshal([]byte(raw), &plans); err != nil {
				return planNode{}, err
			}
			return plans[0].Plan, nil
		}()
		if err != nil {
			t.Fatalf("%s: %v", statement.name, err)
		}
		found, sorted, filter := indexScan(plan, statement.index, false)
		t.Logf("%s: read through %s %v, sorted after %v, filter %q", statement.name, statement.index, found, sorted, filter)
		switch {
		case !found:
			t.Errorf("%s does not read submissions through %s. Its WHERE no longer implies the index's predicate in 052_submission_list_indexes.sql, "+
				"or it no longer bounds the contest's runs by contestRuns, so it reads every run of the table.", statement.name, statement.index)
		case statement.ordered && sorted:
			t.Errorf("%s sorts what it reads through %s. Its ORDER BY no longer matches the index's keys in 052_submission_list_indexes.sql, "+
				"so every page reads and sorts every run of the list.", statement.name, statement.index)
		case statement.listed && strings.Contains(filter, "easyTest"):
			t.Errorf("%s reads job for every run it reads through %s (filter %s). Its WHERE no longer implies the index's predicate word for word.",
				statement.name, statement.index, filter)
		}
	}
}
