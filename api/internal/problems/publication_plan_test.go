package problems

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// planNode is the part of an EXPLAIN (FORMAT JSON) node that the test reads.
type planNode struct {
	NodeType     string     `json:"Node Type"`
	RelationName string     `json:"Relation Name"`
	IndexName    string     `json:"Index Name"`
	Plans        []planNode `json:"Plans"`
}

// submissionScans lists every scan of submissions in the plan, subplans included, as "node type on index".
func submissionScans(n planNode) []string {
	var scans []string
	if n.RelationName == "submissions" {
		scan := n.NodeType
		if n.IndexName != "" {
			scan += " on " + n.IndexName
		}
		scans = append(scans, scan)
	}
	for _, child := range n.Plans {
		scans = append(scans, submissionScans(child)...)
	}
	return scans
}

// TestSolverQueriesUseTheirIndexesPostgres checks that every statement that counts solvers or reads solved marks is
// still answered from the partial indexes of 050_solver_indexes.sql alone. The planner may use a partial index only
// while the statement's WHERE implies its predicate, so rewording a condition in publication.go can silently send
// those statements back to reading the job and result of every run, without changing any result.
func TestSolverQueriesUseTheirIndexesPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	schema := fmt.Sprintf("test_solver_plans_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	exec := func(query string) {
		t.Helper()
		if _, e := store.pool.Exec(ctx, query); e != nil {
			t.Fatal(e)
		}
	}
	// A plain problem and a contest problem, each with an accepted run, so that both indexes hold an entry.
	const plain, contested, contest = "a1000000-0000-4000-8000-000000000001", "a1000000-0000-4000-8000-000000000002", "a2000000-0000-4000-8000-000000000001"
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob')`)
	exec(`INSERT INTO problem_drafts(id,owner_id,draft,version,published_draft,published_version,published_at)
		SELECT id::uuid,'alice',d,1,d,1,now() FROM (VALUES ('` + plain + `'),('` + contested + `')) p(id),
		LATERAL (SELECT '{"title":"p","timeLimitMs":"1000","memoryLimitMb":"256"}'::jsonb AS d) x`)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES ('` + contest + `','alice','c','',now()-interval '3 hours',now()-interval '1 hour',true)`)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) VALUES ('` + contest + `','` + contested + `',0,100,'{}',1)`)
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at) VALUES
		(gen_random_uuid(),'bob','` + plain + `',1,'t','cpp17','','{"privateDraft":false}','DONE','{"verdict":"AC"}',NULL,now()),
		(gen_random_uuid(),'bob','` + contested + `',1,'t','cpp17','','{"privateDraft":false}','DONE','{"verdict":"AC"}','` + contest + `',now()-interval '2 hours')`)
	// Index-only scans need the visibility map.
	exec(`VACUUM (ANALYZE) submissions`)

	list, listArgs := publicListQuery(nil, 0, 50, ListFilter{Sort: SortSolversDesc})
	filtered, filteredArgs := publicListQuery(nil, 0, 50, ListFilter{Sort: SortSolversDesc, Solved: UnsolvedOnly, Viewer: "bob"})
	count, countArgs := publicCountQuery(ListFilter{Solved: SolvedOnly, Viewer: "bob"})
	for _, statement := range []struct {
		name  string
		query string
		args  []any
	}{
		{"the list sorted by solvers", list, listArgs},
		{"the list with the unsolved filter", filtered, filteredArgs},
		{"the count with the solved filter", count, countArgs},
		{"the problem page", publicGetQuery, []any{plain, ""}},
		{"the solved problems", solvedProblemsQuery, []any{"bob"}},
	} {
		scans, err := func() ([]string, error) {
			tx, err := store.pool.Begin(ctx)
			if err != nil {
				return nil, err
			}
			defer func() { _ = tx.Rollback(ctx) }()
			// The question is whether the planner may use the partial indexes at all, not which plan is cheapest on two
			// runs, where it could pick any of them. So, in a transaction that is rolled back, the other indexes on
			// submissions are dropped and sequential and bitmap scans are priced out: the planner then takes an
			// index-only scan of a partial index wherever the WHERE implies its predicate and the index holds every
			// column the statement reads, and only otherwise falls back to the primary key or a sequential scan.
			rows, err := tx.Query(ctx, `SELECT indexrelid::regclass::text FROM pg_index WHERE indrelid='submissions'::regclass AND NOT indisprimary
				AND indexrelid::regclass::text NOT IN ('submissions_plain_solvers','submissions_contest_solvers')`)
			if err != nil {
				return nil, err
			}
			others, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return nil, err
			}
			for _, index := range others {
				if _, err = tx.Exec(ctx, `DROP INDEX `+pgx.Identifier{index}.Sanitize()); err != nil {
					return nil, err
				}
			}
			if _, err = tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, `SET LOCAL enable_bitmapscan=off`); err != nil {
				return nil, err
			}
			var raw string
			if err = tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+statement.query, statement.args...).Scan(&raw); err != nil {
				return nil, err
			}
			var plans []struct{ Plan planNode }
			if err = json.Unmarshal([]byte(raw), &plans); err != nil {
				return nil, err
			}
			return submissionScans(plans[0].Plan), nil
		}()
		if err != nil {
			t.Fatalf("%s: %v", statement.name, err)
		}
		t.Logf("%s: %v", statement.name, scans)
		kinds := map[string]bool{}
		for _, scan := range scans {
			kinds[scan] = true
		}
		if len(kinds) != 2 || !kinds["Index Only Scan on submissions_plain_solvers"] || !kinds["Index Only Scan on submissions_contest_solvers"] {
			t.Errorf("%s reads submissions with %q, not only with index-only scans of submissions_plain_solvers and submissions_contest_solvers. "+
				"Its WHERE no longer implies their predicates in 050_solver_indexes.sql, or it reads a column they do not hold, so it goes back to the table and its TOASTed job and result. "+
				"Change the predicates to match in a new migration, or revert the condition in publication.go.", statement.name, scans)
		}
	}
}
