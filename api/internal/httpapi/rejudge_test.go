package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

func TestRejudgePostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	schema := fmt.Sprintf("test_rejudge_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) }()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	pool := store.Pool()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob'),('carol','carol'),('dave','dave')`)
	const problem = "11111111-1111-4111-8111-111111111111"
	difficulty := 1
	if _, err := store.Save(ctx, "alice", problem, 0, problems.Draft{Title: "A+B", Markdown: "Add", Difficulty: &difficulty, TimeLimitMS: "2000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "1 2", Output: "4"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, "alice", problem, 1, true); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES ($1,'bob')`, problem)
	// The author fixes the wrong expected output and adds a case.
	if _, err := store.Save(ctx, "alice", problem, 2, problems.Draft{Title: "A+B", Markdown: "Add", Difficulty: &difficulty, TimeLimitMS: "2000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "1 2", Output: "3"}, {Input: "2 3", Output: "5"}}}); err != nil {
		t.Fatal(err)
	}
	const contest = "22222222-2222-4222-8222-222222222222"
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES ($1,'alice','c','d',clock_timestamp()-interval '2 hours',clock_timestamp()-interval '1 hour',true)`, contest)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
 SELECT $1,id,0,100,jsonb_set(published_draft,'{testCases}','[{"input":"9","output":"contest"}]'),7 FROM problem_drafts WHERE id=$2`, contest, problem)
	ids := map[string]string{}
	insert := func(name, owner, runtime, status string, job map[string]any, contestID string) {
		t.Helper()
		id := fmt.Sprintf("%08d-3333-4333-8333-333333333333", len(ids))
		ids[name] = id
		base := map[string]any{"image": "sha256:" + strings.Repeat("0", 64), "easyTest": false, "privateDraft": false, "timeLimitMs": 2000, "memoryLimitMb": 512, "cases": []map[string]string{{"input": "1 2", "output": "4"}}}
		for k, v := range job {
			base[k] = v
		}
		raw, _ := json.Marshal(base)
		exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,status,result,job,contest_id,dispatched_at,finished_at)
 VALUES ($1,$2,$3,1,'old title',$4,'source',$5,CASE WHEN $5='DONE' THEN '{"verdict":"WA","passed":0,"total":1}'::jsonb END,$6,NULLIF($7,'')::uuid,clock_timestamp(),clock_timestamp())`,
			id, owner, problem, runtime, status, raw, contestID)
	}
	insert("wrong", "carol", "cpp17-isolate", "DONE", nil, "")
	insert("sample", "carol", "cpp17-isolate", "DONE", map[string]any{"easyTest": true}, "")
	insert("judging", "carol", "cpp17-isolate", "QUEUED", nil, "")
	insert("retired", "carol", "java24-isolate", "DONE", nil, "")
	insert("private", "alice", "python314-isolate", "DONE", map[string]any{"privateDraft": true}, "")
	insert("contest", "carol", "cpp17-isolate", "DONE", nil, contest)
	f := newSigningFixture(t)
	image := "sha256:" + strings.Repeat("a", 64)
	dispatches := 0
	handler := func(enabled string) *testHandler {
		return &testHandler{t: t, f: f, h: newHandler(AuthConfig{}, handlerDependencies{
			Store: store, Contests: &contests.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool},
			JudgeImage: image, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: enabled, Operators: map[string]bool{"dave": true},
			Verifier: newCognitoVerifier(f.server.URL, "client"), DispatchJudge: func(context.Context) error { dispatches++; return nil },
		})}
	}
	h := handler("cpp17,python314")
	path := "/my/problems/" + problem + "/rejudge"
	for _, owner := range []string{"alice", "bob", "dave"} {
		if got := h.request("GET", path, owner, "", 200); got != `{"pending":0}` {
			t.Fatalf("%s pending: %s", owner, got)
		}
	}
	h.request("GET", path, "carol", "", 404)
	h.request("POST", path, "carol", `{}`, 404)
	h.request("POST", path, "", `{}`, 401)
	h.request("POST", path, "alice", `{"submissionIds":["not-a-uuid"]}`, 400)
	many, _ := json.Marshal(map[string][]string{"submissionIds": strings.Split(strings.Repeat(ids["wrong"]+",", submissions.MaxRejudgeTargets+1), ",")[:submissions.MaxRejudgeTargets+1]})
	h.request("POST", path, "alice", string(many), 400)
	state := func(name string) (status, attempt, verdict string, rejudged bool, job submissions.Job, privateDraft bool, version int64) {
		t.Helper()
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT status,judge_attempt::text,COALESCE(result->>'verdict',''),rejudged_at IS NOT NULL,job,COALESCE((job->>'privateDraft')::boolean,false),problem_version FROM submissions WHERE id=$1`, ids[name]).
			Scan(&status, &attempt, &verdict, &rejudged, &raw, &privateDraft, &version); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &job); err != nil {
			t.Fatal(err)
		}
		return
	}
	_, before, _, _, _, _, _ := state("wrong")
	// A tester selects one submission.
	if got := h.request("POST", path, "bob", `{"submissionIds":["`+ids["wrong"]+`"]}`, 202); got != `{"queued":1}` || dispatches != 1 {
		t.Fatalf("selected rejudge: %s dispatches=%d", got, dispatches)
	}
	status, attempt, verdict, rejudged, job, _, _ := state("wrong")
	if status != "QUEUED" || verdict != "" || !rejudged || attempt == before || job.Image != image || len(job.Cases) != 2 || job.Cases[0].Output != "3" || job.EasyTest {
		t.Fatalf("rejudged submission: %s %s %s %v %+v", status, attempt, verdict, rejudged, job)
	}
	if got := h.request("GET", path, "alice", "", 200); got != `{"pending":1}` {
		t.Fatalf("pending after rejudge: %s", got)
	}
	// The author rejudges the rest while the first one waits; nothing is queued twice.
	if got := h.request("POST", path, "alice", `{}`, 202); got != `{"queued":2}` {
		t.Fatalf("problem rejudge: %s", got)
	}
	for name, want := range map[string]string{"sample": "DONE", "judging": "QUEUED", "retired": "DONE", "private": "QUEUED", "contest": "QUEUED"} {
		if status, _, _, rejudged, _, _, _ := state(name); status != want || rejudged != (name == "private" || name == "contest") {
			t.Fatalf("%s: status=%s rejudged=%v", name, status, rejudged)
		}
	}
	if _, _, _, _, job, privateDraft, _ := state("private"); !privateDraft || len(job.Cases) != 2 {
		t.Fatalf("private submission lost its visibility flag: %+v", job)
	}
	if _, _, _, _, job, _, version := state("contest"); len(job.Cases) != 1 || job.Cases[0].Output != "contest" || version != 7 {
		t.Fatalf("contest submission must use the contest copy: %+v version=%d", job, version)
	}
	if got := h.request("POST", path, "alice", `{}`, 202); got != `{"queued":0}` {
		t.Fatalf("repeated rejudge: %s", got)
	}
	// A judge language that is not enabled cannot judge, so nothing is requeued.
	exec(`UPDATE submissions SET status='DONE',result='{"verdict":"WA","passed":0,"total":1}' WHERE problem_id=$1`, problem)
	exec(`UPDATE problem_drafts SET published_draft=published_draft||'{"checker":{"runtime":"java24","source":"x"}}' WHERE id=$1`, problem)
	h.request("POST", path, "alice", `{"submissionIds":["`+ids["wrong"]+`"]}`, 409)
	exec(`UPDATE problem_drafts SET published_draft=published_draft-'checker' WHERE id=$1`, problem)
	// Operators may rejudge any problem.
	if got := h.request("POST", path, "dave", `{"submissionIds":["`+ids["wrong"]+`"]}`, 202); got != `{"queued":1}` {
		t.Fatalf("operator rejudge: %s", got)
	}
	handler("none").request("POST", path, "alice", `{}`, 503)
}

type testHandler struct {
	t *testing.T
	f signingFixture
	h http.Handler
}

func (h *testHandler) request(method, path, owner, body string, want int) string {
	h.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if owner != "" {
		r.Header.Set("Authorization", "Bearer "+h.f.token(h.t, owner, nil))
	}
	w := httptest.NewRecorder()
	h.h.ServeHTTP(w, r)
	if w.Code != want {
		h.t.Fatalf("%s %s as %q: %d %s", method, path, owner, w.Code, w.Body.String())
	}
	return strings.TrimSpace(w.Body.String())
}
