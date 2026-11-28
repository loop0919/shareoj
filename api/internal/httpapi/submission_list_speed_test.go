package httpapi

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

// The problem and the contest that TestSubmissionListSpeedPostgres lists.
const (
	speedProblem = "b1000000-0000-4000-8000-000000000001"
	speedContest = "b2000000-0000-4000-8000-000000000001"
)

// seedSubmissionListSpeed fills the schema with 200 users and runs shaped like production's, around one large problem
// and one large contest: 5000 plain runs of one problem, 20000 runs of a contest's 10 problems, 2000 plain runs of
// those problems made after the contest, 4000 runs of 40 small contests of 3 problems each, and 4000 plain runs spread
// over 160 other problems. The small contests and problems matter to the planner: a plan prepared once for every
// problem or contest is costed for one of average size, not for the large ones. Real runs carry their test cases in job
// and a result per case, a few KB each once compressed, so that both are TOASTed as in production.
func seedSubmissionListSpeed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	exec := func(query string) {
		t.Helper()
		if _, e := pool.Exec(ctx, query); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO user_profiles(owner_id,handle) SELECT 'u'||i,'user'||i FROM generate_series(1,200) i`)
	// Problem 1 is the listed one, 101 to 110 belong to the large contest, 1001 to 1120 to the small ones, and 2001 to
	// 2160 take the other plain runs.
	exec(`INSERT INTO problem_drafts(id,owner_id,draft,version,published_draft,published_version,published_at)
		SELECT ('b1000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'u1',d,1,d,1,now()-interval '90 days'
		FROM (SELECT generate_series(1,1) UNION ALL SELECT generate_series(101,110) UNION ALL SELECT generate_series(1001,1120)
			UNION ALL SELECT generate_series(2001,2160)) p(n),
		LATERAL (SELECT jsonb_build_object('title','Problem '||n,'timeLimitMs','2000','memoryLimitMb','256') AS d) x`)
	// The large contest ended a day ago, so its runs are public. A run of it falls before the start (a staff test), inside
	// the two hours or in the practice after the end. Small contest k ended k days ago.
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released)
		VALUES ('` + speedContest + `','u1','Speed','',now()-interval '26 hours',now()-interval '24 hours',true)`)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released)
		SELECT ('b2000000-0000-4000-8000-'||lpad((1000+k)::text,12,'0'))::uuid,'u1','Small '||k,'',
			now()-make_interval(days=>k)-interval '2 hours',now()-make_interval(days=>k),true FROM generate_series(1,40) k`)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
		SELECT '` + speedContest + `',('b1000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,n-101,100,'{}',1 FROM generate_series(101,110) n`)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
		SELECT ('b2000000-0000-4000-8000-'||lpad((1000+(n-1001)/3+1)::text,12,'0'))::uuid,('b1000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,
			(n-1001)%3,100,'{}',1 FROM generate_series(1001,1120) n`)
	// The runs are drawn from one table of hashes. Of every 100 runs, 3 are still running, 3 failed to compile and have
	// no time, 3 are easy tests and, outside the contests, 3 are private draft runs; the others are judged, about 55 of
	// every 100 of them accepted, with wrong answers, time and memory limits, runtime errors, output limits and judge
	// errors sharing the rest. Runtimes are 8, with the suffixes stored runs carry, three of them C++.
	exec(`CREATE TABLE speed_runs AS
		SELECT i,kind,array_agg(hashtext(i||'-'||j)&2147483647 ORDER BY j) AS h
		FROM (SELECT i,'plain' AS kind FROM generate_series(1,5000) i
			UNION ALL SELECT i,'contest' FROM generate_series(5001,25000) i
			UNION ALL SELECT i,'practice' FROM generate_series(25001,27000) i
			UNION ALL SELECT i,'small' FROM generate_series(27001,31000) i
			UNION ALL SELECT i,'rest' FROM generate_series(31001,35000) i) r,
		generate_series(1,9) j GROUP BY i,kind`)
	const (
		cases   = `(SELECT jsonb_agg(jsonb_build_object('input',md5(r.i||'i'||k)||md5(k||'i'||r.i),'output',md5((r.i*k)::text))) FROM generate_series(1,30) k)`
		results = `(SELECT jsonb_agg(jsonb_build_object('name','test_'||k,'verdict','AC','cpuTimeMs',(r.h[6]%1500+1)*k/30+r.h[7]%7,
			'wallTimeMs',(r.h[6]%1500+1)*k/30+20,'memoryBytes',(r.h[7]%256+1)*1048576,'checkerLog',jsonb_build_object('text',md5(r.i||'c'||k),'truncated',false))) FROM generate_series(1,30) k)`
		// A small contest's run is of one of its 120 problems, (h[3]%120)/3+1 being its contest.
		small = `(r.h[3]%120)`
	)
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at)
		SELECT gen_random_uuid(),'u'||(r.h[2]%200+1),
			CASE r.kind WHEN 'plain' THEN '` + speedProblem + `'::uuid
				ELSE ('b1000000-0000-4000-8000-'||lpad((CASE r.kind WHEN 'small' THEN 1001+` + small + ` WHEN 'rest' THEN 2001+r.h[3]%160
					ELSE 101+r.h[3]%10 END)::text,12,'0'))::uuid END,
			1,'t',(ARRAY['cpp17','cpp23-gcc-isolate','cpp23-clang-isolate','python314-isolate','pypy311-local','rust2024-isolate','java25-isolate','go127-isolate'])[r.h[4]%8+1],
			left(repeat(md5(r.i::text)||md5(r.i||'s'),200),r.h[5]%6000+50),
			jsonb_build_object('privateDraft',r.kind NOT IN ('contest','small') AND r.h[1]%100 BETWEEN 9 AND 11,'easyTest',r.h[1]%100 BETWEEN 6 AND 8,
				'image','x','timeLimitMs',2000,'memoryLimitMb',256,'cases',` + cases + `),
			CASE WHEN r.h[1]%100<3 THEN 'RUNNING' ELSE 'DONE' END,
			CASE WHEN r.h[1]%100<3 THEN NULL
				WHEN r.h[1]%100<6 THEN jsonb_build_object('verdict','CE','passed',0,'total',30,'compileLog',(SELECT string_agg(md5(r.i||'e'||k),'') FROM generate_series(1,80) k))
				ELSE jsonb_build_object('verdict',CASE WHEN r.h[8]%100<55 THEN 'AC' WHEN r.h[8]%100<70 THEN 'WA' WHEN r.h[8]%100<80 THEN 'TLE'
					WHEN r.h[8]%100<87 THEN 'RE' WHEN r.h[8]%100<92 THEN 'MLE' WHEN r.h[8]%100<96 THEN 'OLE' ELSE 'JE' END,
					'passed',r.h[6]%31,'total',30,'cases',` + results + `) END,
			CASE r.kind WHEN 'contest' THEN '` + speedContest + `'::uuid
				WHEN 'small' THEN ('b2000000-0000-4000-8000-'||lpad((1000+` + small + `/3+1)::text,12,'0'))::uuid END,
			CASE r.kind WHEN 'contest' THEN c.starts_at+make_interval(secs=>r.h[9]%8000-300)
				WHEN 'practice' THEN c.ends_at+make_interval(secs=>r.h[9]%86000)
				WHEN 'small' THEN now()-make_interval(days=>` + small + `/3+1)-interval '2 hours'+make_interval(secs=>r.h[9]%7200)
				ELSE now()-make_interval(secs=>r.h[9]%2592000) END
		FROM speed_runs r, contests c WHERE c.id='` + speedContest + `'`)
	// The planner needs the statistics and the visibility map, which autovacuum keeps in production.
	exec(`VACUUM (ANALYZE) user_profiles,problem_drafts,contests,contest_problems,submissions`)
}

// unapplyListIndexes takes back migration 052, so that the next Migrate applies it to a table that already holds runs,
// as it will in production: its ANALYZE then finds the runs, and its indexes are built at once instead of growing with
// the seed.
const unapplyListIndexes = `DROP INDEX submissions_problem_shortest,submissions_problem_fastest,submissions_contest_shortest,submissions_contest_fastest;
	DROP STATISTICS submissions_job_flags,submissions_verdict; DELETE FROM schema_migrations WHERE version=52`

// TestSubmissionListSpeedPostgres times every sort and filter of the submission lists of a problem, of a contest and of
// a contest's problem on runs as large as production's, right after migration 052 is applied to them. Each list is timed
// as it runs, again after each statement has run eight times on one connection, and, logged only, under plans that the
// server is set to make generic. It only runs with OPENOJ_LIST_SPEED=1 so that the ordinary go test and CI stay fast.
func TestSubmissionListSpeedPostgres(t *testing.T) {
	if os.Getenv("OPENOJ_LIST_SPEED") != "1" {
		t.Skip("OPENOJ_LIST_SPEED=1 required")
	}
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
	schema := fmt.Sprintf("test_submission_speed_%d", time.Now().UnixNano())
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
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	pool := store.Pool()
	if _, err = pool.Exec(ctx, unapplyListIndexes); err != nil {
		t.Fatal(err)
	}
	seedSubmissionListSpeed(t, ctx, pool)
	start := time.Now()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("migration 052 on the seed: %dms", time.Since(start).Milliseconds())
	// pgx prepares a statement once on each connection, and PostgreSQL may keep a generic plan for it after five calls.
	// A pool of one connection, which runs each statement eight times before the timed calls, finds such a switch.
	single, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	single.MaxConns = 1
	repeated, err := pgxpool.NewWithConfig(ctx, single)
	if err != nil {
		t.Fatal(err)
	}
	defer repeated.Close()
	// The lists are planned with their values on every call (plannedPerCall), so they meet a generic plan only when the
	// server is set to force one. Those plans are timed and logged, not held to the budget.
	q.Set("plan_cache_mode", "force_generic_plan")
	u.RawQuery = q.Encode()
	generic, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer generic.Close()

	var plain, contested, runs, jobBytes, resultBytes int
	var toastBytes int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE problem_id='`+speedProblem+`'),count(*) FILTER (WHERE contest_id='`+speedContest+`'),
		count(*),avg(pg_column_size(job))::int,avg(pg_column_size(result))::int,
		(SELECT pg_relation_size(reltoastrelid) FROM pg_class WHERE oid='submissions'::regclass) FROM submissions`).Scan(&plain, &contested, &runs, &jobBytes, &resultBytes, &toastBytes); err != nil {
		t.Fatal(err)
	}
	if plain != 5000 || contested != 20000 || jobBytes < 2000 || resultBytes < 1500 || toastBytes < 100<<20 {
		t.Fatalf("seed too small: %d runs of the problem, %d of the contest, %d bytes of job and %d of result stored per run, %d bytes TOASTed",
			plain, contested, jobBytes, resultBytes, toastBytes)
	}
	t.Logf("seed: %d runs (%d of the problem, %d of the contest), %d bytes of job and %d of result stored per run, %d MB TOASTed",
		runs, plain, contested, jobBytes, resultBytes, toastBytes>>20)
	indexes, err := pool.Query(ctx, `SELECT indexrelid::regclass::text,pg_relation_size(indexrelid) FROM pg_index WHERE indrelid='submissions'::regclass ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	var size int64
	if _, err = pgx.ForEachRow(indexes, []any{&name, &size}, func() error {
		t.Logf("index %s: %d kB", name, size>>10)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Guard against a seed that degenerates into tied sort keys or filters that keep everything or nothing.
	var verdicts, runtimes, times, lengths, running, untimed, easy, drafts int
	if err = pool.QueryRow(ctx, `SELECT count(DISTINCT result->>'verdict'),count(DISTINCT runtime),count(DISTINCT submission_cpu_time_ms(result)),
		count(DISTINCT octet_length(source)),count(*) FILTER (WHERE status<>'DONE'),count(*) FILTER (WHERE status='DONE' AND submission_cpu_time_ms(result) IS NULL),
		count(*) FILTER (WHERE (job->>'easyTest')::boolean),count(*) FILTER (WHERE (job->>'privateDraft')::boolean)
		FROM submissions`).Scan(&verdicts, &runtimes, &times, &lengths, &running, &untimed, &easy, &drafts); err != nil {
		t.Fatal(err)
	}
	if verdicts != 8 || runtimes != 8 || times < 1000 || lengths < 1000 || running < runs/50 || untimed < runs/50 || easy < runs/50 || drafts < 200 {
		t.Fatalf("seed too uniform: %d verdicts, %d runtimes, %d distinct times, %d distinct lengths, %d running, %d without a time, %d easy tests, %d draft runs",
			verdicts, runtimes, times, lengths, running, untimed, easy, drafts)
	}
	t.Logf("seed: %d verdicts, %d runtimes, %d distinct times, %d distinct lengths, %d running, %d without a time, %d easy tests, %d draft runs",
		verdicts, runtimes, times, lengths, running, untimed, easy, drafts)
	// The large problem and contest stand among many small ones, as the planner sees them: a generic plan is costed for
	// a problem or contest of the average size these statistics give.
	var problemsRun, contestsRun int
	var problemDistinct, contestDistinct, contestNulls float64
	if err = pool.QueryRow(ctx, `SELECT count(DISTINCT problem_id),count(DISTINCT contest_id),
		(SELECT n_distinct FROM pg_stats WHERE schemaname=current_schema() AND tablename='submissions' AND attname='problem_id'),
		(SELECT n_distinct FROM pg_stats WHERE schemaname=current_schema() AND tablename='submissions' AND attname='contest_id'),
		(SELECT null_frac FROM pg_stats WHERE schemaname=current_schema() AND tablename='submissions' AND attname='contest_id')
		FROM submissions`).Scan(&problemsRun, &contestsRun, &problemDistinct, &contestDistinct, &contestNulls); err != nil {
		t.Fatal(err)
	}
	if problemsRun < 250 || contestsRun < 40 {
		t.Fatalf("seed too concentrated: runs of %d problems and %d contests", problemsRun, contestsRun)
	}
	t.Logf("seed: runs of %d problems (n_distinct %g) and %d contests (n_distinct %g, null_frac %g)",
		problemsRun, problemDistinct, contestsRun, contestDistinct, contestNulls)

	cpp, _ := submissions.LanguageRuntimeIDs("cpp")
	languages := submissions.StoredRuntimeIDs(cpp)
	filters := []struct {
		name  string
		apply func(*submissions.ListConditions)
	}{
		{"none", func(*submissions.ListConditions) {}},
		{"verdict=AC", func(c *submissions.ListConditions) { c.Verdict = "AC" }},
		{"language=cpp", func(c *submissions.ListConditions) { c.Runtimes = languages }},
		{"user=user7", func(c *submissions.ListConditions) { c.User = "user7" }},
		{"verdict=AC&language=cpp", func(c *submissions.ListConditions) { c.Verdict, c.Runtimes = "AC", languages }},
	}
	sorts := []string{submissions.SortNewest, submissions.SortOldest, submissions.SortShortest, submissions.SortFastest}
	// The viewer is a signed-in user who is neither staff nor the author, as most readers of these lists are.
	const viewer = "u2"
	// The design's budget, 200 ms for a page and its count, holds for the lists of a problem and of a contest as such
	// readers see them. The other two lists are measured too, against a looser budget of 400 ms: staff also read the
	// plain runs of the contest's problems in its list, which grow with the practice after it (2000 runs here and about
	// 180 ms), and a contest problem's list is a subset of the problem's.
	const budget, extraBudget = 200 * time.Millisecond, 400 * time.Millisecond
	type list struct {
		name   string
		budget time.Duration
		page   func(*submissions.Store, submissions.ListConditions) (int, error)
	}
	contestList := func(reader string) func(*submissions.Store, submissions.ListConditions) (int, error) {
		return func(s *submissions.Store, c submissions.ListConditions) (int, error) {
			page, e := s.ContestList(ctx, speedContest, reader, submissions.ScopeAll, 0, 50, false, "", c)
			if e == nil && page.Total < len(page.Items) {
				e = fmt.Errorf("total %d below the %d items of the page", page.Total, len(page.Items))
			}
			return len(page.Items), e
		}
	}
	lists := []list{
		{"problem", budget, func(s *submissions.Store, c submissions.ListConditions) (int, error) {
			page, e := s.ProblemList(ctx, speedProblem, "", viewer, submissions.ScopeAll, false, 0, c)
			return len(page.Items), e
		}},
		{"contest", budget, contestList(viewer)},
		{"contest as staff", extraBudget, contestList("u1")},
		{"contest problem", extraBudget, func(s *submissions.Store, c submissions.ListConditions) (int, error) {
			page, e := s.ProblemList(ctx, "b1000000-0000-4000-8000-000000000101", speedContest, viewer, submissions.ScopeAll, false, 0, c)
			return len(page.Items), e
		}},
	}
	passes := []struct {
		plans         string
		store         *submissions.Store
		warmup, timed int
		gated         bool
	}{
		{"per-call", &submissions.Store{Pool: pool}, 0, 3, true},
		{"repeated", &submissions.Store{Pool: repeated}, 8, 3, true},
		{"forced-generic", &submissions.Store{Pool: generic.Pool()}, 0, 3, false},
	}
	for _, pass := range passes {
		for _, l := range lists {
			for _, filter := range filters {
				for _, order := range sorts {
					conditions := submissions.ListConditions{Sort: order}
					filter.apply(&conditions)
					best := time.Duration(1<<62 - 1)
					for i := range pass.warmup + pass.timed {
						start := time.Now()
						items, e := l.page(pass.store, conditions)
						if e != nil {
							t.Fatalf("plans=%s list=%s sort=%s filter=%s: %v", pass.plans, l.name, order, filter.name, e)
						}
						if elapsed := time.Since(start); i >= pass.warmup && elapsed < best {
							best = elapsed
						}
						// One submitter has fewer runs than a page; every other filter leaves more.
						if items == 0 || (conditions.User == "" && items != 50) {
							t.Fatalf("plans=%s list=%s sort=%s filter=%s: got %d items", pass.plans, l.name, order, filter.name, items)
						}
					}
					t.Logf("plans=%s list=%s sort=%s filter=%s best=%dms", pass.plans, l.name, order, filter.name, best.Milliseconds())
					if pass.gated && best > l.budget {
						t.Errorf("plans=%s list=%s sort=%s filter=%s took %v, want at most %v", pass.plans, l.name, order, filter.name, best, l.budget)
					}
				}
			}
		}
	}
}
