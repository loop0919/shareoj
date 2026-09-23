package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

func TestContestsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_contests_%d", time.Now().UnixNano())
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := store.Pool().Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob'),('tester','tester'),('carol','carol')`)
	t.Run("start time order with creation time tie break", func(t *testing.T) {
		const older = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		const newer = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,created_at)
 VALUES ($1,'alice','Older','',now()+interval '3 hours',now()+interval '4 hours',now()-interval '1 day'),
 ($2,'alice','Newer','',now()+interval '1 hour',now()+interval '2 hours',now())`, older, newer)
		defer exec(`DELETE FROM contests WHERE id IN ($1,$2)`, older, newer)
		catalogue := &contests.Store{Pool: store.Pool()}
		checkOrder := func(first, second string) {
			t.Helper()
			for _, owner := range []string{"", "alice"} {
				items, err := catalogue.List(ctx, owner, 0)
				if err != nil || len(items) != 2 || items[0].ID != first || items[1].ID != second {
					t.Fatalf("contest order for %q: %+v (%v)", owner, items, err)
				}
				page, err := catalogue.List(ctx, owner, 1)
				if err != nil || len(page) != 1 || page[0].ID != second {
					t.Fatalf("next page: %+v (%v)", page, err)
				}
			}
		}
		checkOrder(older, newer)
		other, err := catalogue.Get(ctx, newer, "alice")
		if err != nil {
			t.Fatal(err)
		}
		penalty := 5
		if err := catalogue.Save(ctx, "alice", older, contests.Input{Title: "Edited", StartsAt: other.StartsAt, EndsAt: other.EndsAt, PenaltyMinutes: &penalty, Version: 1}, contests.JudgePolicy{KnownRuntimes: submissions.RuntimeIDs()}); err != nil {
			t.Fatal(err)
		}
		checkOrder(newer, older)
	})
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	const cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const other = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	draft := problems.Draft{Title: "Secret A", Markdown: "secret statement", Editorial: "secret editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}, {Input: "private input", Output: "private output"}}}
	for _, id := range []string{a, b} {
		if _, err = store.Save(ctx, "alice", id, 0, draft); err != nil {
			t.Fatal(err)
		}
	}
	f := newSigningFixture(t)
	queue := &submissions.Store{Pool: store.Pool()}
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: queue, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(f.server.URL, "client")})
	request := func(method, path, owner string, body any, want int) string {
		t.Helper()
		raw := ""
		if body != nil {
			data, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			raw = string(data)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if owner != "" {
			r.Header.Set("Authorization", "Bearer "+f.token(t, owner, nil))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable contest response")
		}
		return w.Body.String()
	}
	penalty := 5
	in := contests.Input{Title: "Contest", Description: "Description", StartsAt: time.Now().Add(time.Hour), EndsAt: time.Now().Add(2 * time.Hour), PenaltyMinutes: &penalty, Problems: []contests.Problem{{ID: a, Points: 100}, {ID: b, Points: 200}}}
	request("PUT", "/my/contests/"+cid, "", in, 401)
	request("PUT", "/my/contests/"+cid, "bob", in, 409)
	request("PUT", "/my/contests/"+cid, "alice", in, 200)
	request("PUT", "/my/contests/"+other, "alice", in, 409)
	bad := in
	bad.Problems = []contests.Problem{{ID: a, Points: 100}, {ID: a, Points: 200}}
	request("PUT", "/my/contests/"+other, "alice", bad, 400)
	bad = in
	bad.StartsAt = time.Now().Add(-time.Minute)
	request("PUT", "/my/contests/"+other, "alice", bad, 409)
	request("GET", "/contests", "", nil, 200)
	request("GET", "/contests?offset=-1", "", nil, 400)
	detail := request("GET", "/contests/"+cid, "", nil, 200)
	if strings.Contains(detail, "Secret") || !strings.Contains(detail, `"problems":[]`) {
		t.Fatal("prestart leak", detail)
	}
	var credits contests.Contest
	if err := json.Unmarshal([]byte(detail), &credits); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(credits.ProblemAuthors, []string{"alice"}) || len(credits.Testers) != 0 {
		t.Fatal("initial contest credits", credits)
	}
	// Credits cover all registered problems, including those hidden before the start.
	exec(`UPDATE problem_drafts SET owner_id='bob' WHERE id=$1`, b)
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester'),($2,'tester'),($2,'carol')`, a, b)
	if err := json.Unmarshal([]byte(request("GET", "/contests/"+cid, "", nil, 200)), &credits); err != nil {
		t.Fatal(err)
	}
	if len(credits.Problems) != 0 || !slices.Equal(credits.ProblemAuthors, []string{"alice", "bob"}) || !slices.Equal(credits.Testers, []string{"carol", "tester"}) {
		t.Fatal("contest credits must be unique and include hidden problems", credits)
	}
	exec(`DELETE FROM problem_testers WHERE problem_id IN ($1,$2)`, a, b)
	exec(`UPDATE problem_drafts SET owner_id='alice' WHERE id=$1`, b)
	request("GET", "/contests/"+cid+"/problems/"+a, "", nil, 404)
	request("GET", "/problems/"+a, "", nil, 404)
	if detail = request("GET", "/my/contests/"+cid+"/problems/"+a, "alice", nil, 200); !strings.Contains(detail, "secret editorial") || strings.Contains(detail, "private input") {
		t.Fatal(detail)
	}
	request("GET", "/my/contests/"+cid+"/problems/"+a, "bob", nil, 404)
	request("PUT", "/my/problems/"+a+"/publication", "alice", map[string]any{"version": 1, "publish": true}, 409)
	request("DELETE", "/my/problems/"+a+"?version=1", "alice", nil, 409)
	// The owner can resave and reorder before the start. Stale writes are rejected.
	in.Version = 1
	in.Problems = []contests.Problem{{ID: b, Points: 200}, {ID: a, Points: 100}}
	request("PUT", "/my/contests/"+cid, "alice", in, 200)
	request("PUT", "/my/contests/"+cid, "alice", in, 409)
	request("PUT", "/my/contests/"+cid, "bob", in, 404)
	// Only the registered problem is visible to a tester before the start.
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, a)
	request("GET", "/my/contests/"+cid+"/problems/"+a, "tester", nil, 200)
	request("GET", "/my/contests/"+cid+"/problems/"+b, "tester", nil, 404)
	var testerView contests.Contest
	if err := json.Unmarshal([]byte(request("GET", "/my/contests/"+cid, "tester", nil, 200)), &testerView); err != nil {
		t.Fatal(err)
	}
	if testerView.Official || !testerView.CanViewSubmissions || len(testerView.Problems) != 1 {
		t.Fatal(testerView)
	}
	if testerView.Problems[0].TimeLimitMS != "1000" || testerView.Problems[0].MemoryLimitMB != "256" {
		t.Fatal("missing contest problem limits", testerView.Problems)
	}
	// Registration is authenticated, idempotent and immediately visible before the start.
	joinPath := "/my/contests/" + cid + "/participation"
	request("POST", joinPath, "", nil, 401)
	request("POST", "/my/contests/"+other+"/participation", "bob", nil, 404)
	request("POST", joinPath, "alice", nil, 409)
	request("POST", joinPath, "tester", nil, 409)
	for i := 0; i < 2; i++ {
		var joined contests.Contest
		if err := json.Unmarshal([]byte(request("POST", joinPath, "bob", nil, 200)), &joined); err != nil {
			t.Fatal(err)
		}
		if !joined.Participating || !joined.Official || joined.Status != "scheduled" {
			t.Fatal(joined)
		}
	}
	var registered []contests.Standing
	if err := json.Unmarshal([]byte(request("GET", "/contests/"+cid+"/standings", "", nil, 200)), &registered); err != nil {
		t.Fatal(err)
	}
	if len(registered) != 1 || registered[0].Handle != "bob" || registered[0].Points != 0 || registered[0].TimeMS != 0 || len(registered[0].Problems) != 0 {
		t.Fatalf("zero submission participant: %+v", registered)
	}
	// Becoming a tester excludes a previously registered contestant too.
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'bob')`, a)
	if body := request("GET", "/contests/"+cid+"/standings", "", nil, 200); strings.TrimSpace(body) != "[]" {
		t.Fatal(body)
	}
	request("POST", joinPath, "bob", nil, 409)
	exec(`DELETE FROM problem_testers WHERE problem_id=$1 AND owner_id='bob'`, a)
	submit := func(owner, pid string, easy bool) submissions.Submission {
		t.Helper()
		var s submissions.Submission
		raw := request("POST", "/my/submissions", owner, map[string]any{"problemId": pid, "contestId": cid, "runtime": "cpp17", "source": "code of " + owner, "easyTest": easy}, 202)
		if e := json.Unmarshal([]byte(raw), &s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	request("POST", "/my/submissions", "bob", map[string]any{"problemId": a, "contestId": cid, "runtime": "cpp17", "source": "code"}, 409)
	detail = request("GET", "/my/problems/"+a, "tester", nil, 200)
	if !strings.Contains(detail, `"contestId":"`+cid+`"`) {
		t.Fatal("missing contest membership", detail)
	}
	// Source saves follow through without resaving the contest, including tester edits.
	draft.Title = "Updated before start"
	draft.TimeLimitMS = "2000"
	request("PUT", "/my/problems/"+a, "tester", map[string]any{"version": 1, "draft": draft}, 200)
	detail = request("GET", "/my/contests/"+cid, "alice", nil, 200)
	if !strings.Contains(detail, "Updated before start") {
		t.Fatal("title did not follow source", detail)
	}
	var updatedContest contests.Contest
	if err := json.Unmarshal([]byte(detail), &updatedContest); err != nil {
		t.Fatal(err)
	}
	for _, p := range updatedContest.Problems {
		wantTime := "1000"
		if p.ID == a {
			wantTime = "2000"
		}
		if p.TimeLimitMS != wantTime || p.MemoryLimitMB != "256" {
			t.Fatal("contest problem limits did not follow source", p)
		}
	}
	pre := submit("tester", a, false)
	preSetter := submit("alice", b, false)
	request("GET", "/my/contests/"+cid+"/submissions", "tester", nil, 200)
	request("GET", "/my/contests/"+cid+"/submissions/"+preSetter.ID, "tester", nil, 200)
	request("GET", "/my/contests/"+cid+"/submissions", "bob", nil, 404)
	start := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	end := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)
	exec(`UPDATE contests SET starts_at=$2,ends_at=$3 WHERE id=$1`, cid, start, end)
	// Preserve the actual pre-start relation after moving the test clock.
	exec(`UPDATE submissions SET created_at=$2 WHERE id=$1`, pre.ID, start.Add(-time.Minute))
	in.Version = 2
	request("PUT", "/my/contests/"+cid, "alice", in, 409)
	detail = request("GET", "/contests/"+cid+"/problems/"+a, "", nil, 200)
	if !strings.Contains(detail, "secret statement") || strings.Contains(detail, "secret editorial") || strings.Contains(detail, "private input") {
		t.Fatal("running leak", detail)
	}
	// Running contests follow source edits; already accepted jobs remain immutable.
	draft.Title = "Changed draft"
	draft.Markdown = "changed statement"
	draft.TestCases[0].Output = "changed output"
	if _, err = store.Save(ctx, "alice", a, 2, draft); err != nil {
		t.Fatal(err)
	}
	accepted := submit("bob", a, false)
	var job []byte
	if err = store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, accepted.ID).Scan(&job); err != nil || !strings.Contains(string(job), "changed output") {
		t.Fatalf("snapshot %s %v", job, err)
	}
	var savedJob submissions.Job
	if err := json.Unmarshal(job, &savedJob); err != nil || savedJob.MemoryLimitMB != 256 {
		t.Fatalf("contest submission memory limit: %s (%v)", job, err)
	}
	if accepted.ProblemVersion != 3 {
		t.Fatal("new submission version", accepted.ProblemVersion)
	}
	detail = request("GET", "/contests/"+cid+"/problems/"+a, "", nil, 200)
	if !strings.Contains(detail, "changed statement") || !strings.Contains(detail, "Changed draft") {
		t.Fatal("running statement did not follow source", detail)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, pre.ID).Scan(&job); err != nil || strings.Contains(string(job), "changed output") || !strings.Contains(string(job), `"timeLimitMs": 2000`) {
		t.Fatalf("accepted job changed or missed prestart update: %s %v", job, err)
	}
	if accepted.ContestID != cid {
		t.Fatal("missing contest context")
	}
	// Create out of judging order; ranking uses acceptance order, including delayed results.
	set := func(s submissions.Submission, minute int, verdict string) {
		t.Helper()
		exec(`UPDATE submissions SET created_at=$2,status='DONE',result=jsonb_build_object('verdict',$3::text,'passed',1,'total',1,'checkerLog','private diagnostic') WHERE id=$1`, s.ID, start.Add(time.Duration(minute)*time.Minute), verdict)
	}
	set(accepted, 30, "AC")
	wa := submit("bob", a, false)
	set(wa, 10, "WA")
	ce := submit("bob", a, false)
	set(ce, 11, "CE")
	re := submit("bob", a, false)
	set(re, 12, "RE")
	after := submit("bob", a, false)
	set(after, 31, "WA")
	unsolved := submit("bob", b, false)
	set(unsolved, 5, "WA")
	easy := submit("bob", b, true)
	set(easy, 20, "AC")
	creator := submit("alice", b, false)
	set(creator, 15, "AC")
	tester := submit("tester", b, false)
	set(tester, 15, "AC")
	if body := request("POST", "/my/submissions", "carol", map[string]any{"problemId": a, "contestId": cid, "runtime": "cpp17", "source": "code"}, 409); !strings.Contains(body, "contest_participation_required") {
		t.Fatal(body)
	}
	request("POST", joinPath, "carol", nil, 200)
	tied := submit("carol", a, false)
	set(tied, 40, "AC")
	checkSolved := func(owner string, wantA, wantB bool) {
		t.Helper()
		path := "/my/contests/" + cid
		if owner == "" {
			path = "/contests/" + cid
		}
		var view contests.Contest
		if err := json.Unmarshal([]byte(request("GET", path, owner, nil, 200)), &view); err != nil {
			t.Fatal(err)
		}
		solved := map[string]bool{}
		for _, problem := range view.Problems {
			solved[problem.ID] = problem.Solved
		}
		if len(view.Problems) != 2 || solved[a] != wantA || solved[b] != wantB {
			t.Fatalf("solved for %q: %+v", owner, view.Problems)
		}
	}
	checkSolved("bob", true, false)   // Sample AC on B does not count.
	checkSolved("alice", false, true) // A participant's AC does not belong to the setter.
	checkSolved("", false, false)

	rank := func() []contests.Standing {
		t.Helper()
		var rows []contests.Standing
		if err := json.Unmarshal([]byte(request("GET", "/contests/"+cid+"/standings", "", nil, 200)), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rows := rank()
	if len(rows) != 2 || rows[0].Points != 100 || rows[0].TimeMS != 40*60000 || rows[0].Rank != 1 || rows[1].Rank != 1 {
		t.Fatalf("rank: %+v", rows)
	}
	// Simulate upgrading a database with pre-registration submissions and a used migration 19.
	exec(`DROP TABLE contest_participants`)
	exec(`DELETE FROM schema_migrations WHERE version=20`)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if migrated := rank(); len(migrated) != 2 || migrated[0].Points != 100 || migrated[1].Points != 100 {
		t.Fatalf("migrated standings: %+v", migrated)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	request("GET", "/contests/"+cid+"/submissions", "", nil, 404)
	request("GET", "/contests/"+cid+"/submissions/"+accepted.ID, "", nil, 404)
	request("GET", "/my/submissions/"+accepted.ID, "carol", nil, 404)
	// Ordinary participants cannot view other submissions before the end.
	for _, viewer := range []string{"bob", "carol"} {
		request("GET", "/my/contests/"+cid+"/problems/"+a+"/submissions", viewer, nil, 404)
		request("GET", "/my/contests/"+cid+"/submissions/"+accepted.ID, viewer, nil, 404)
	}
	request("GET", "/contests/"+cid+"/problems/"+a+"/submissions", "", nil, 404)
	request("GET", "/contests/"+cid+"/problems/"+a+"/submissions?mine=1", "", nil, 401)
	request("GET", "/my/contests/"+cid+"/problems/"+a+"/submissions?mine=bad", "bob", nil, 400)
	request("GET", "/my/contests/"+cid+"/problems/"+other+"/submissions?mine=1", "bob", nil, 404)
	mine := request("GET", "/my/contests/"+cid+"/problems/"+a+"/submissions?mine=1", "bob", nil, 200)
	if !strings.Contains(mine, accepted.ID) || strings.Contains(mine, unsolved.ID) || strings.Contains(mine, tied.ID) || strings.Contains(mine, "code of") || strings.Contains(mine, "private diagnostic") {
		t.Fatal("mine filter/diagnostic", mine)
	}
	setter := request("GET", "/my/contests/"+cid+"/problems/"+a+"/submissions", "alice", nil, 200)
	if !strings.Contains(setter, tied.ID) || !strings.Contains(setter, pre.ID) || strings.Contains(setter, unsolved.ID) || strings.Contains(setter, easy.ID) {
		t.Fatal("setter problem filter", setter)
	}
	request("GET", "/my/contests/"+cid+"/submissions", "alice", nil, 200)
	setter = request("GET", "/my/contests/"+cid+"/submissions/"+accepted.ID, "alice", nil, 200)
	if !strings.Contains(setter, "code of bob") || strings.Contains(setter, "private diagnostic") {
		t.Fatal("setter source", setter)
	}
	// A tester of A can review both A and B, including submitted source.
	checkTesterAccess := func(allowed bool) {
		t.Helper()
		want := 404
		if allowed {
			want = 200
		}
		var view contests.Contest
		if err := json.Unmarshal([]byte(request("GET", "/my/contests/"+cid, "tester", nil, 200)), &view); err != nil {
			t.Fatal(err)
		}
		if view.CanViewSubmissions != allowed {
			t.Fatalf("tester permission: %+v", view)
		}
		for _, path := range []string{
			"/submissions", "/problems/" + a + "/submissions", "/problems/" + b + "/submissions",
			"/submissions/" + accepted.ID, "/submissions/" + unsolved.ID,
		} {
			body := request("GET", "/my/contests/"+cid+path, "tester", nil, want)
			if allowed && strings.Contains(body, "private diagnostic") {
				t.Fatal("diagnostic leak", body)
			}
			if allowed && (path == "/submissions/"+accepted.ID || path == "/submissions/"+unsolved.ID) && !strings.Contains(body, "code of bob") {
				t.Fatal("missing submitted source", body)
			}
		}
	}
	checkTesterAccess(true)
	request("GET", "/my/contests/"+cid+"/submissions/"+easy.ID, "tester", nil, 404)
	// Membership is checked on each request; unrelated contests grant no access.
	exec(`DELETE FROM problem_testers WHERE problem_id=$1 AND owner_id='tester'`, a)
	checkTesterAccess(false)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at) VALUES($1,'alice','Unrelated','',now(),now()+interval '1 hour')`, other)
	request("GET", "/my/contests/"+other+"/submissions", "tester", nil, 404)
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, a)
	checkTesterAccess(true)
	request("GET", "/my/contests/"+other+"/submissions", "tester", nil, 404)
	exec(`DELETE FROM contests WHERE id=$1`, other)
	// Normal problem routes cannot bypass the contest embargo.
	request("GET", "/problems/"+a+"/submissions", "", nil, 404)
	request("GET", "/my/problems/"+a+"/submissions", "bob", nil, 404)
	request("GET", "/problems/"+a+"/submissions/"+accepted.ID, "", nil, 404)
	request("GET", "/my/problems/"+a+"/submissions/"+accepted.ID, "bob", nil, 404)
	// Stop at 50 minutes, then finish a previously queued submission.
	end = start.Add(50 * time.Minute)
	delayed := submit("carol", b, false)
	exec(`UPDATE submissions SET created_at=$2 WHERE id=$1`, delayed.ID, end.Add(-time.Microsecond))
	boundary := submit("bob", b, false)
	exec(`UPDATE submissions SET created_at=$2 WHERE id=$1`, boundary.ID, end)
	exec(`UPDATE contests SET ends_at=$2 WHERE id=$1`, cid, end)
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES('late','late')`)
	request("POST", joinPath, "late", nil, 409)
	request("POST", joinPath, "bob", nil, 200)
	latePractice := submit("late", a, false)
	if latePractice.ID == "" {
		t.Fatal("unregistered practice rejected")
	}

	detail = request("GET", "/problems/"+a, "", nil, 200)
	if !strings.Contains(detail, "secret editorial") || !strings.Contains(detail, "changed statement") {
		t.Fatal("auto publication", detail)
	}
	var public problems.PublicProblem
	if err := json.Unmarshal([]byte(detail), &public); err != nil {
		t.Fatal(err)
	}
	if !public.PublishedAt.Equal(end) {
		t.Fatal(public.PublishedAt, end)
	}
	published, err := store.Get(ctx, "alice", a)
	if err != nil {
		t.Fatal(err)
	}
	request("GET", "/problems", "", nil, 200)
	again, _ := store.Get(ctx, "alice", a)
	if again.Version != published.Version {
		t.Fatal("release is not idempotent")
	}
	exec(`UPDATE submissions SET status='DONE',result='{"verdict":"AC","passed":1,"total":1}' WHERE id=ANY($1::uuid[])`, []string{delayed.ID, boundary.ID})
	rows = rank()
	if len(rows) != 2 || rows[0].Handle != "carol" || rows[0].Points != 300 || rows[1].Points != 100 {
		t.Fatalf("late results: %+v", rows)
	}
	practice := submit("bob", b, false)
	exec(`UPDATE submissions SET status='DONE',result='{"verdict":"AC","passed":1,"total":1}' WHERE id=$1`, practice.ID)
	checkSolved("bob", true, true) // Practice AC is reflected outside official standings.
	checkSolved("", false, false)
	if rows = rank(); rows[1].Points != 100 {
		t.Fatal("practice counted", rows)
	}
	exec(`UPDATE submissions SET result=result || '{"cases":[{"name":"a","verdict":"AC","cpuTimeMs":12.25,"memoryBytes":1000000},{"name":"b","verdict":"AC","cpuTimeMs":0,"memoryBytes":2500000}]}'::jsonb WHERE id=$1`, accepted.ID)
	detail = request("GET", "/contests/"+cid+"/submissions/"+accepted.ID, "", nil, 200)
	if !strings.Contains(detail, "code of bob") || strings.Contains(detail, "private diagnostic") {
		t.Fatal("public source/diagnostic", detail)
	}
	request("GET", "/contests/"+cid+"/submissions/"+easy.ID, "", nil, 404)
	request("GET", "/contests/"+cid+"/submissions/"+pre.ID, "", nil, 404)
	var endedView contests.Contest
	if err := json.Unmarshal([]byte(request("GET", "/contests/"+cid, "", nil, 200)), &endedView); err != nil {
		t.Fatal(err)
	}
	if !endedView.CanViewSubmissions {
		t.Fatal("ended contest must allow public submission viewing")
	}
	list := request("GET", "/contests/"+cid+"/submissions", "", nil, 200)
	if strings.Contains(list, "code of") || !strings.Contains(list, accepted.ID) {
		t.Fatal(list)
	}
	list = request("GET", "/contests/"+cid+"/problems/"+a+"/submissions", "", nil, 200)
	if !strings.Contains(list, accepted.ID) || !strings.Contains(list, tied.ID) || strings.Contains(list, unsolved.ID) || strings.Contains(list, pre.ID) {
		t.Fatal("ended problem filter", list)
	}
	request("GET", "/contests/"+cid+"/problems/"+a+"/submissions?offset=-1", "", nil, 400)
	list = request("GET", "/problems/"+a+"/submissions", "", nil, 200)
	if !strings.Contains(list, accepted.ID) || strings.Contains(list, pre.ID) || strings.Contains(list, "code of") {
		t.Fatal("normal problem list", list)
	}
	for _, path := range []string{"/contests/" + cid + "/submissions", "/contests/" + cid + "/problems/" + a + "/submissions", "/problems/" + a + "/submissions", "/my/problems/" + a + "/submissions?mine=1", "/my/submissions"} {
		var response submissions.ContestSubmissionList
		if err := json.Unmarshal([]byte(request("GET", path, "bob", nil, 200)), &response); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range response.Items {
			if item.ID != accepted.ID {
				continue
			}
			found = true
			if item.Result.CPUTimeMS == nil || *item.Result.CPUTimeMS != 12.25 || item.Result.MemoryBytes == nil || *item.Result.MemoryBytes != 2500000 {
				t.Fatalf("missing usage summary on %s: %+v", path, item.Result)
			}
			if path != "/my/submissions" && len(item.Result.Cases) != 0 {
				t.Fatal("public list exposed case details")
			}
		}
		if !found {
			t.Fatal("missing submission", path)
		}
	}
	var measured submissions.Submission
	if err := json.Unmarshal([]byte(request("GET", "/problems/"+a+"/submissions/"+accepted.ID, "", nil, 200)), &measured); err != nil {
		t.Fatal(err)
	}
	if measured.Result.CPUTimeMS == nil || *measured.Result.CPUTimeMS != 12.25 || measured.Result.MemoryBytes == nil || *measured.Result.MemoryBytes != 2500000 {
		t.Fatal("missing detail usage summary")
	}

	request("GET", "/problems/"+b+"/submissions/"+accepted.ID, "", nil, 404)
	request("GET", "/problems/"+a+"/submissions/"+pre.ID, "", nil, 404)
	request("GET", "/problems/"+b+"/submissions/"+easy.ID, "", nil, 404)
	request("GET", "/problems/"+a+"/submissions?mine=1", "", nil, 401)
	// Becoming a tester on either problem removes the user from the entire official table.
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'bob')`, b)
	if rows = rank(); len(rows) != 1 || rows[0].Handle != "carol" {
		t.Fatal(rows)
	}
	// Ordinary practice works after automatic publication, without contest context.
	normal := request("POST", "/my/submissions", "bob", map[string]any{"problemId": a, "runtime": "cpp17", "source": "practice"}, 202)
	var practiceNormal submissions.Submission
	if err := json.Unmarshal([]byte(normal), &practiceNormal); err != nil {
		t.Fatal(err)
	}
	list = request("GET", "/problems/"+a+"/submissions", "", nil, 200)
	if !strings.Contains(list, practiceNormal.ID) {
		t.Fatal("missing practice", list)
	}
	list = request("GET", "/my/problems/"+a+"/submissions?mine=1", "bob", nil, 200)
	if !strings.Contains(list, practiceNormal.ID) || !strings.Contains(list, accepted.ID) || strings.Contains(list, tied.ID) {
		t.Fatal("normal mine filter", list)
	}
	list = request("GET", "/my/contests/"+cid+"/problems/"+a+"/submissions?mine=1", "bob", nil, 200)
	if strings.Contains(list, practiceNormal.ID) {
		t.Fatal("practice outside contest", list)
	}
	request("GET", "/problems/"+a+"/submissions/"+practiceNormal.ID, "", nil, 200)
	// Simulate upgrading an old submission without its privacy marker.
	exec(`UPDATE submissions SET job=job-'privateDraft' WHERE id=$1`, practiceNormal.ID)
	exec(`DELETE FROM schema_migrations WHERE version=13; DROP INDEX submissions_problem_order`)
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	request("GET", "/problems/"+a+"/submissions/"+practiceNormal.ID, "", nil, 200)
	// Updating the public problem must not remove earlier public submissions.
	exec(`UPDATE problem_drafts SET published_at=clock_timestamp() WHERE id=$1`, a)
	request("GET", "/problems/"+a+"/submissions/"+practiceNormal.ID, "", nil, 200)
	// Older ordinary draft submissions remain private even after publication.
	exec(`UPDATE submissions SET created_at=$2,job=job || '{"privateDraft":true}'::jsonb WHERE id=$1`, practiceNormal.ID, start.Add(-time.Minute))
	request("GET", "/problems/"+a+"/submissions/"+practiceNormal.ID, "", nil, 404)
	request("GET", "/my/submissions/"+practiceNormal.ID, "bob", nil, 200)
	// Pagination is applied after the problem and owner filters.
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job)
      SELECT gen_random_uuid(),'bob',$1,1,'practice','cpp17-local','code','{}'::jsonb FROM generate_series(1,51)`, a)
	var firstPage, nextPage submissions.ContestSubmissionList
	if err := json.Unmarshal([]byte(request("GET", "/my/problems/"+a+"/submissions?mine=1", "bob", nil, 200)), &firstPage); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(request("GET", "/my/problems/"+a+"/submissions?mine=1&offset=50", "bob", nil, 200)), &nextPage); err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Items) != 50 || !firstPage.HasMore || len(nextPage.Items) == 0 || nextPage.HasMore {
		t.Fatal("pagination", firstPage, nextPage)
	}
	seen := map[string]bool{}
	for _, item := range firstPage.Items {
		seen[item.ID] = true
	}
	for _, item := range nextPage.Items {
		if seen[item.ID] {
			t.Fatal("overlapping pages", item.ID)
		}
	}
}
