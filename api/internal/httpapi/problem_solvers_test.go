package httpapi

import (
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/problems"
)

// The conditions on accepted runs as publication.go wrote them before 050_solver_indexes.sql, kept word for word as
// the reference that the queries reading those indexes must agree with.
const (
	referenceAcceptedRun = `s.status='DONE' AND submission_full_score(s.result)
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 AND NOT COALESCE((s.job->>'generate')::boolean,false)
 AND NOT COALESCE((s.job->>'validate')::boolean,false)`
	referencePublicAccepted = referenceAcceptedRun + `
 AND ((s.contest_id IS NULL AND NOT COALESCE((s.job->>'privateDraft')::boolean,true))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=s.contest_id AND contest_submission_public(c,s.created_at)))`
	referenceOwnAccepted = referenceAcceptedRun + `
 AND ((s.contest_id IS NULL AND NOT COALESCE((s.job->>'privateDraft')::boolean,true))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=s.contest_id AND s.created_at>=c.starts_at AND c.ends_at<=statement_timestamp()))`
	referenceSolverCount = `(SELECT count(DISTINCT s.owner_id) FROM submissions s WHERE s.problem_id=d.id AND ` + referencePublicAccepted + `)`
)

// TestProblemSolversMatchReferencePostgres compares the solver counts and the solved sets with the reference, problem
// by problem and viewer by viewer, on runs of every kind: accepted or not, still queued or running, each job that does
// not count, partial and full scores, and plain and contest runs from before the start, inside the window, in the
// freeze and after the end, in contests in each state.
func TestProblemSolversMatchReferencePostgres(t *testing.T) {
	viewers := []string{"own1", "own2", "own3", "own4", "own5", "own6", "nobody"}
	f := newTeamTest(t, append([]string{"alice"}, viewers...)...)
	ctx, store := f.ctx, f.store
	// Five contests of two problems each: ended without a freeze, ended with the freeze on, revealed after a freeze,
	// running, and not started yet.
	f.exec(`CREATE TABLE ref_contests AS SELECT k,gen_random_uuid() AS id,s AS starts_at FROM generate_series(1,5) k,
		LATERAL (SELECT CASE k WHEN 4 THEN now()-interval '1 hour' WHEN 5 THEN now()+interval '1 hour' ELSE now()-make_interval(days=>k) END AS s) x`)
	f.exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released,freeze_minutes,revealed_at)
		SELECT id,'alice','c'||k,'',starts_at,starts_at+interval '2 hours',k<=3,CASE WHEN k IN (2,3) THEN 60 END,CASE WHEN k=3 THEN starts_at+interval '3 hours' END
		FROM ref_contests`)
	// 14 published problems, the last 10 of them set by the contests, and one problem that is not published.
	f.exec(`CREATE TABLE ref_problems AS SELECT n,gen_random_uuid() AS id,CASE WHEN n BETWEEN 5 AND 14 THEN (n-5)/2+1 END AS contest FROM generate_series(1,15) n`)
	f.exec(`INSERT INTO problem_drafts(id,owner_id,draft,version,published_draft,published_version,published_at)
		SELECT id,'alice',d,1,CASE WHEN n<15 THEN d END,CASE WHEN n<15 THEN 1 ELSE 0 END,CASE WHEN n<15 THEN now()-make_interval(mins=>n) END
		FROM ref_problems, LATERAL (SELECT jsonb_build_object('title','Problem '||n,'timeLimitMs','1000','memoryLimitMb','256') AS d) x`)
	f.exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
		SELECT c.id,p.id,p.n%2,100,'{}',1 FROM ref_problems p JOIN ref_contests c ON c.k=p.contest`)
	// Each run draws its owner, problem, job, status, result, contest and time from independent hashes. A contest run
	// falls before the start, inside the window, in the last hour (frozen where there is a freeze) or after the end.
	f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at)
		SELECT gen_random_uuid(),'own'||(h[2]%6+1),p.id,1,'t','cpp17','',
			(CASE h[3]%8 WHEN 0 THEN '{"privateDraft":true}' WHEN 1 THEN '{"privateDraft":false,"easyTest":true}' WHEN 2 THEN '{"privateDraft":false,"generate":true}'
				WHEN 3 THEN '{"privateDraft":false,"validate":true}' WHEN 4 THEN '{}' ELSE '{"privateDraft":false,"easyTest":false}' END)::jsonb,
			CASE h[4]%10 WHEN 0 THEN 'QUEUED' WHEN 1 THEN 'RUNNING' ELSE 'DONE' END,
			(CASE h[5]%6 WHEN 0 THEN '{"verdict":"WA"}' WHEN 1 THEN '{"verdict":"WA","scoring":{"score":100,"maxScore":100}}'
				WHEN 2 THEN '{"verdict":"AC","scoring":{"score":70,"maxScore":100}}' WHEN 3 THEN NULL ELSE '{"verdict":"AC"}' END)::jsonb,
			CASE WHEN h[6]%3<>0 THEN c.id END,
			CASE WHEN c.id IS NULL THEN now()-make_interval(mins=>h[7]%1000)
				ELSE LEAST(c.starts_at+make_interval(mins=>(ARRAY[-30,10,90,150])[h[7]%4+1]),now()) END
		FROM generate_series(1,700) i,
		LATERAL (SELECT array_agg(hashtext(i||'-'||j)&2147483647 ORDER BY j) AS h FROM generate_series(1,7) j) x
		JOIN ref_problems p ON p.n=h[1]%15+1 LEFT JOIN ref_contests c ON c.k=p.contest`)

	// The seed is not vacuous: owners solve a problem through plain runs only, contest runs only, both, or not at all,
	// some solve one more than once, and some own marks differ from the public count.
	var plainOnly, contestOnly, both, neither, repeated, ownOnly, solverCounts int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FILTER (WHERE plain>0 AND contest=0),count(*) FILTER (WHERE contest>0 AND plain=0),
		count(*) FILTER (WHERE plain>0 AND contest>0),count(*) FILTER (WHERE plain+contest=0),count(*) FILTER (WHERE plain+contest>1),count(*) FILTER (WHERE own AND plain+contest=0),
		(SELECT count(DISTINCT n) FROM (SELECT `+referenceSolverCount+` AS n FROM problem_drafts d WHERE d.published_draft IS NOT NULL) counts)
		FROM (SELECT count(*) FILTER (WHERE plain) AS plain,count(*) FILTER (WHERE contest) AS contest,bool_or(own) AS own FROM (
			SELECT s.owner_id,s.problem_id,s.contest_id IS NULL AND `+referencePublicAccepted+` AS plain,s.contest_id IS NOT NULL AND `+referencePublicAccepted+` AS contest,`+referenceOwnAccepted+` AS own
			FROM submissions s) runs GROUP BY owner_id,problem_id) pairs`).Scan(&plainOnly, &contestOnly, &both, &neither, &repeated, &ownOnly, &solverCounts); err != nil {
		t.Fatal(err)
	}
	if plainOnly == 0 || contestOnly == 0 || both == 0 || neither == 0 || repeated == 0 || ownOnly == 0 || solverCounts < 3 {
		t.Fatalf("seed too uniform: %d plain only, %d contest only, %d both, %d neither, %d repeated, %d own only, %d distinct solver counts",
			plainOnly, contestOnly, both, neither, repeated, ownOnly, solverCounts)
	}
	t.Logf("seed: %d plain only, %d contest only, %d both, %d neither, %d repeated, %d own only, %d distinct solver counts", plainOnly, contestOnly, both, neither, repeated, ownOnly, solverCounts)

	ids := func(query string, args ...any) []string {
		t.Helper()
		rows, err := store.Pool().Query(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		return got
	}
	// The solver count of each problem, on the list sorted by it and on the problem page.
	rows, err := store.Pool().Query(ctx, `SELECT d.id::text,`+referenceSolverCount+` FROM problem_drafts d WHERE d.published_draft IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{}
	var id string
	var count int64
	if _, err = pgx.ForEachRow(rows, []any{&id, &count}, func() error {
		want[id] = count
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	list, err := store.PublicList(ctx, nil, 0, 100, problems.ListFilter{Sort: problems.SortSolversDesc})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, p := range list {
		got[p.ID] = p.SolverCount
		detail, e := store.PublicGet(ctx, p.ID, "")
		if e != nil {
			t.Fatal(e)
		}
		if detail.SolverCount != want[p.ID] {
			t.Errorf("problem %s page: %d solvers, want %d", p.ID, detail.SolverCount, want[p.ID])
		}
	}
	if !maps.Equal(got, want) {
		t.Errorf("list solvers %v, want %v", got, want)
	}
	// Each viewer's own solved problems, as /my/solved-problems lists them and as the list's filters pick them.
	for _, viewer := range viewers {
		solved := ids(`SELECT DISTINCT s.problem_id::text FROM submissions s JOIN problem_drafts d ON d.id=s.problem_id
			WHERE s.owner_id=$1 AND d.published_draft IS NOT NULL AND `+referenceOwnAccepted, viewer)
		unsolved := ids(`SELECT d.id::text FROM problem_drafts d WHERE d.published_draft IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM submissions s WHERE s.problem_id=d.id AND s.owner_id=$1 AND `+referenceOwnAccepted+`)`, viewer)
		marks, e := store.SolvedProblems(ctx, viewer)
		if e != nil {
			t.Fatal(e)
		}
		slices.Sort(marks)
		if !slices.Equal(marks, solved) {
			t.Errorf("%s solved problems %v, want %v", viewer, marks, solved)
		}
		for solvedFilter, want := range map[string][]string{problems.SolvedOnly: solved, problems.UnsolvedOnly: unsolved} {
			filter := problems.ListFilter{Solved: solvedFilter, Viewer: viewer}
			items, e := store.PublicList(ctx, nil, 0, 100, filter)
			if e != nil {
				t.Fatal(e)
			}
			total, e := store.PublicCount(ctx, filter)
			if e != nil {
				t.Fatal(e)
			}
			listed := []string{}
			for _, p := range items {
				listed = append(listed, p.ID)
			}
			slices.Sort(listed)
			if !slices.Equal(listed, want) || total != len(want) {
				t.Errorf("%s %s: listed %v total %d, want %v", viewer, solvedFilter, listed, total, want)
			}
		}
	}
}
