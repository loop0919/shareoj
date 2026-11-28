package submissions

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/problems"
)

// newListTestPool gives a pool on a migrated schema of its own, dropped when the test ends.
func newListTestPool(t *testing.T, name string) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	schema := fmt.Sprintf("test_%s_%d", name, time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, store.Pool()
}

// referenceContestListQueries builds ContestList's statements as they were before the list was split into
// contestRuns and contestPlainRuns: one scan of submissions under the filter. They are the reference the split must
// agree with.
func referenceContestListQueries(contestID, viewer string, scope Scope, offset, limit int, onsite bool, problemID string, conditions ListConditions) contestListStatements {
	filter, narrow := contestPublic, any(onsite)
	if scope != ScopeAll {
		filter, narrow = contestOwn, string(scope)
	}
	filter += ` AND ($5::uuid IS NULL OR problem_id=$5::uuid)`
	var problem any
	if problemID != "" {
		problem = problemID
	}
	pageNarrow, pageArgs := conditions.where(7)
	countNarrow, countArgs := conditions.where(6)
	return contestListStatements{
		page:      `SELECT ` + columns + ` FROM submissions WHERE ` + filter + pageNarrow + ` ORDER BY ` + conditions.order() + ` LIMIT $6 OFFSET $2`,
		pageArgs:  append([]any{contestID, offset, viewer, narrow, problem, limit + 1}, pageArgs...),
		count:     `SELECT count(*) FROM submissions WHERE ` + filter + countNarrow + ` AND $2::integer>=0`,
		countArgs: append([]any{contestID, offset, viewer, narrow, problem}, countArgs...),
	}
}

// TestContestListMatchesReferencePostgres compares ContestList's page and count with the reference, contest by
// contest, viewer by viewer and scope by scope, for every sort with each kind of condition and pages that cut through
// the list. The runs are of every kind: contest runs from before the start, inside the window, in the freeze and after
// the end, plain runs of the contest's problems, a contest's runs of a problem it does not set, queued and running runs,
// results with and without times, and the jobs that the lists leave out.
func TestContestListMatchesReferencePostgres(t *testing.T) {
	ctx, pool := newListTestPool(t, "contest_list_reference")
	exec := func(query string) {
		t.Helper()
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	users := []string{"alice", "setter", "author", "tester", "part1", "part2", "mate1", "mate2", "outsider"}
	exec(`INSERT INTO user_profiles(owner_id,handle) SELECT u,u FROM unnest(ARRAY['alice','setter','author','tester','part1','part2','mate1','mate2','outsider']) u`)
	// Four contests: ended with an onsite division, ended with the freeze still on, a running team contest with an onsite
	// division, and one not started yet. Each sets two problems; a ninth problem belongs to no contest.
	exec(`CREATE TABLE ref_contests AS SELECT k,('c0000000-0000-4000-8000-00000000000'||k)::uuid AS id,
		CASE k WHEN 3 THEN now()-interval '1 hour' WHEN 4 THEN now()+interval '1 hour' ELSE now()-interval '1 day' END AS starts_at
		FROM generate_series(1,4) k`)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released,freeze_minutes,team_mode,max_team_size,onsite_division,onsite_passphrase)
		SELECT id,'alice','c'||k,'',starts_at,starts_at+interval '2 hours',true,CASE WHEN k=2 THEN 60 END,k=3,CASE WHEN k=3 THEN 3 END,k IN (1,3),CASE WHEN k IN (1,3) THEN 'pass' END
		FROM ref_contests`)
	exec(`CREATE TABLE ref_problems AS SELECT n,('a0000000-0000-4000-8000-00000000000'||n)::uuid AS id,CASE WHEN n<=8 THEN (n+1)/2 END AS contest FROM generate_series(1,9) n`)
	exec(`INSERT INTO problem_drafts(id,owner_id,draft,version,published_draft,published_version,published_at)
		SELECT id,CASE WHEN n<=8 THEN 'author' ELSE 'alice' END,d,1,d,1,now()-interval '30 days' FROM ref_problems,
		LATERAL (SELECT jsonb_build_object('title','p'||n,'timeLimitMs','1000','memoryLimitMb','256') AS d) x`)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
		SELECT c.id,p.id,(p.n+1)%2,100,'{}',1 FROM ref_problems p JOIN ref_contests c ON c.k=p.contest`)
	exec(`INSERT INTO problem_testers(problem_id,owner_id,created_at) SELECT id,'tester',now()-interval '40 days' FROM ref_problems WHERE n%2=1 AND n<=8`)
	exec(`INSERT INTO contest_setters(contest_id,owner_id) SELECT id,'setter' FROM ref_contests WHERE k IN (1,3,4)`)
	// Problem 2 is a featured problem whose runs stay hidden until tomorrow.
	exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at)
		SELECT now(),'easy','new',id,2,now()+interval '1 day' FROM ref_problems WHERE n=2`)
	exec(`INSERT INTO contest_participants(contest_id,owner_id,onsite_at,onsite_via)
		SELECT c.id,u,CASE WHEN c.k=1 AND u='part1' THEN now() END,CASE WHEN c.k=1 AND u='part1' THEN 'setter' END
		FROM ref_contests c, unnest(ARRAY['part1','part2','mate1']) u WHERE c.k<>3`)
	exec(`INSERT INTO contest_teams(id,contest_id,name,name_key,join_code,onsite_at,onsite_via)
		SELECT ('d0000000-0000-4000-8000-00000000000'||t)::uuid,c.id,'t'||t,'t'||t,'j'||t,CASE WHEN t=1 THEN now() END,CASE WHEN t=1 THEN 'setter' END
		FROM ref_contests c, generate_series(1,2) t WHERE c.k=3`)
	exec(`INSERT INTO contest_participants(contest_id,owner_id,team_id)
		SELECT c.id,u,('d0000000-0000-4000-8000-00000000000'||CASE WHEN u LIKE 'mate%' THEN 1 ELSE 2 END)::uuid
		FROM ref_contests c, unnest(ARRAY['mate1','mate2','part1','part2']) u WHERE c.k=3`)
	// Each run draws its owner, runtime, source length, job, status, result, contest and time from independent
	// hashes. A run of a contest's problem is a contest run, a plain run, or now and then a run of the next contest; a
	// run of the ninth problem is mostly plain. Times fall 30 minutes before the start, inside the window, in the last
	// hour (frozen in the second contest) or after the end, but never in the future.
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at)
		SELECT gen_random_uuid(),(ARRAY['alice','setter','author','tester','part1','part2','mate1','mate2','outsider'])[h[1]%9+1],p.id,1,'t',
			(ARRAY['cpp17','cpp23-gcc-isolate','python314-isolate','rust2024-local'])[h[2]%4+1],repeat('x',h[3]%5*10),
			(CASE h[4]%10 WHEN 0 THEN '{"privateDraft":true}' WHEN 1 THEN '{"privateDraft":false,"easyTest":true}' WHEN 2 THEN '{"privateDraft":false,"generate":true}'
				WHEN 3 THEN '{"privateDraft":false,"validate":true}' WHEN 4 THEN '{}' ELSE '{"privateDraft":false}' END)::jsonb,
			CASE h[5]%10 WHEN 0 THEN 'QUEUED' WHEN 1 THEN 'RUNNING' ELSE 'DONE' END,
			(CASE h[6]%6 WHEN 0 THEN NULL WHEN 1 THEN '{"verdict":"CE"}' WHEN 2 THEN '{"verdict":"WA","cases":[{"cpuTimeMs":'||h[7]%3*5||'}]}'
				ELSE '{"verdict":"AC","cases":[{"cpuTimeMs":'||h[7]%4*5||'},{"cpuTimeMs":1}]}' END)::jsonb,
			CASE WHEN p.contest IS NULL THEN CASE WHEN h[8]%5=0 THEN c.id END
				WHEN h[8]%30=0 THEN (SELECT n.id FROM ref_contests n WHERE n.k=p.contest%4+1) WHEN h[8]%3=0 THEN NULL ELSE c.id END,
			LEAST(c.starts_at+make_interval(mins=>(ARRAY[-30,10,100,150])[h[9]%4+1]),now())
		FROM generate_series(1,900) i, LATERAL (SELECT array_agg(hashtext(i||'-'||j)&2147483647 ORDER BY j) AS h FROM generate_series(1,11) j) x
		JOIN ref_problems p ON p.n=h[10]%9+1 JOIN ref_contests c ON c.k=COALESCE(p.contest,h[11]%4+1)`)

	cpp, _ := LanguageRuntimeIDs("cpp")
	languages := StoredRuntimeIDs(cpp)
	type view struct {
		scope  Scope
		onsite bool
	}
	views := []view{{ScopeAll, false}, {ScopeAll, true}, {ScopeMine, false}, {ScopeTeam, false}}
	type page struct {
		conditions    ListConditions
		problem       bool
		offset, limit int
	}
	// Every sort with each kind of condition, on pages that start inside the list and end before it does.
	var pages []page
	for _, sort := range []string{SortNewest, SortOldest, SortShortest, SortFastest} {
		pages = append(pages,
			page{conditions: ListConditions{Sort: sort}, offset: 4, limit: 5},
			page{conditions: ListConditions{Sort: sort, Verdict: "AC"}, offset: 2, limit: 3},
			page{conditions: ListConditions{Sort: sort, Runtimes: languages}, problem: true, limit: 50},
			page{conditions: ListConditions{Sort: sort, User: "part1"}, offset: 1, limit: 2},
			page{conditions: ListConditions{Sort: sort, Verdict: "AC", Runtimes: languages}, limit: 5},
		)
	}
	listed := func(statement string, args []any) []string {
		t.Helper()
		rows, err := pool.Query(ctx, statement, args...)
		if err != nil {
			t.Fatal(err)
		}
		items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
			s, e := scan(row)
			// The contest of each item tells the halves apart in the checks below.
			return s.ID + "/" + s.ContestID, e
		})
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	counted := func(statement string, args []any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, statement, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var compared, mixed, plainOnly, empty int
	compare := func(k int, viewer string, v view, p page) {
		t.Helper()
		contest := fmt.Sprintf("c0000000-0000-4000-8000-00000000000%d", k)
		problemID := ""
		if p.problem {
			problemID = fmt.Sprintf("a0000000-0000-4000-8000-00000000000%d", 2*k-1)
		}
		got := contestListQueries(contest, viewer, v.scope, p.offset, p.limit, v.onsite, problemID, p.conditions)
		want := referenceContestListQueries(contest, viewer, v.scope, p.offset, p.limit, v.onsite, problemID, p.conditions)
		gotItems, wantItems := listed(got.page, got.pageArgs), listed(want.page, want.pageArgs)
		gotTotal, wantTotal := counted(got.count, got.countArgs), counted(want.count, want.countArgs)
		if !slices.Equal(gotItems, wantItems) || gotTotal != wantTotal {
			t.Errorf("contest %d, viewer %q, scope %q onsite %v, %+v problem %v offset %d limit %d: got %v total %d, want %v total %d",
				k, viewer, v.scope, v.onsite, p.conditions, p.problem, p.offset, p.limit, gotItems, gotTotal, wantItems, wantTotal)
		}
		compared++
		plain := slices.ContainsFunc(wantItems, func(item string) bool { return item[len(item)-1] == '/' })
		switch {
		case len(wantItems) == 0:
			empty++
		case plain && slices.ContainsFunc(wantItems, func(item string) bool { return item[len(item)-1] != '/' }):
			mixed++
		case plain:
			plainOnly++
		}
	}
	// Each contest, viewer and scope gets the first page with no condition and the next of the pages above in turn, so
	// that every page above is read in some contest by some viewer, and each contest and viewer read several of them.
	turn := 0
	for k := 1; k <= 4; k++ {
		for _, viewer := range append(users, "") {
			for _, v := range views {
				if v.scope != ScopeAll && viewer == "" {
					continue
				}
				compare(k, viewer, v, page{conditions: ListConditions{}, limit: 50})
				compare(k, viewer, v, pages[turn%len(pages)])
				turn++
			}
		}
	}
	// Staff read both halves, so their pages are where the halves merge: every page above, and every sort paged through
	// in steps of five.
	for k := 1; k <= 4; k++ {
		for _, p := range pages {
			compare(k, "alice", views[0], p)
		}
		for _, sort := range []string{SortNewest, SortOldest, SortShortest, SortFastest} {
			for offset := 0; offset < 30; offset += 5 {
				compare(k, "setter", views[0], page{conditions: ListConditions{Sort: sort}, offset: offset, limit: 5})
			}
		}
	}
	// The comparisons are not vacuous: pages mix the contest's runs with plain runs, some hold plain runs only, and
	// some viewers see nothing.
	if mixed == 0 || plainOnly == 0 || empty == 0 || empty == compared {
		t.Fatalf("seed too uniform: of %d pages, %d mix both halves, %d hold plain runs only, %d are empty", compared, mixed, plainOnly, empty)
	}
	t.Logf("compared %d pages: %d mix both halves, %d hold plain runs only, %d are empty", compared, mixed, plainOnly, empty)
}
