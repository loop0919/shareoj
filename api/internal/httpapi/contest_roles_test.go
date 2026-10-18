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

// ADR 0022: problem authors are staff, and staff and participants never overlap.
func TestContestRolesPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_contest_roles_%d", time.Now().UnixNano())
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
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('writer','writer'),('part','part'),('former','former'),('late','late')`)
	const (
		p1  = "11111111-1111-4111-8111-111111111111" // by writer, tested by alice
		p2  = "22222222-2222-4222-8222-222222222222" // by alice
		p3  = "33333333-3333-4333-8333-333333333333" // by alice, outside the contest
		p4  = "44444444-4444-4444-8444-444444444444" // by part, tested by alice
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	difficulty := 3
	draft := problems.Draft{Difficulty: &difficulty, Title: "Secret", Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}, {Input: "3", Output: "4"}}}
	for id, owner := range map[string]string{p1: "writer", p2: "alice", p3: "alice", p4: "part"} {
		if _, err = store.Save(ctx, owner, id, 0, draft); err != nil {
			t.Fatal(err)
		}
	}
	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(f.server.URL, "client")})
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
		return w.Body.String()
	}
	invitation := func(problem, issuer string) string {
		t.Helper()
		var link struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal([]byte(request("POST", "/my/problems/"+problem+"/tester-invitation", issuer, nil, 200)), &link); err != nil {
			t.Fatal(err)
		}
		return "/my/tester-invitations/" + link.Token
	}
	view := func(owner string) contests.Contest {
		t.Helper()
		path := "/contests/" + cid
		if owner != "" {
			path = "/my/contests/" + cid
		}
		var c contests.Contest
		if err := json.Unmarshal([]byte(request("GET", path, owner, nil, 200)), &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	rank := func() []string {
		t.Helper()
		var rows []contests.Standing
		if err := json.Unmarshal([]byte(request("GET", "/contests/"+cid+"/standings", "", nil, 200)), &rows); err != nil {
			t.Fatal(err)
		}
		handles := []string{}
		for _, row := range rows {
			handles = append(handles, row.Handle)
		}
		return handles
	}
	submit := func(owner, problem string, want int) string {
		t.Helper()
		return request("POST", "/my/submissions", owner, map[string]any{"problemId": problem, "contestId": cid, "runtime": "cpp17", "source": "code of " + owner}, want)
	}
	excluded := func() []string {
		t.Helper()
		rows, err := store.Pool().Query(ctx, `SELECT owner_id FROM contest_exclusions WHERE contest_id=$1 ORDER BY owner_id`, cid)
		if err != nil {
			t.Fatal(err)
		}
		owners, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return owners
	}
	joinPath := "/my/contests/" + cid + "/participation"

	// A tester of someone else's problem may enter it; the author then becomes staff of the contest.
	request("POST", invitation(p1, "writer"), "alice", nil, 200)
	request("POST", invitation(p1, "writer"), "former", nil, 200)
	request("POST", invitation(p4, "part"), "alice", nil, 200)
	penalty := 5
	in := contests.Input{Title: "Roles", StartsAt: time.Now().Add(time.Hour), EndsAt: time.Now().Add(2 * time.Hour), PenaltyMinutes: &penalty, Problems: []contests.Problem{{ID: p1, Points: 100}, {ID: p2, Points: 200}}}
	request("PUT", "/my/contests/"+cid, "alice", in, 200)
	author := view("writer")
	if author.Official || !author.Staff || !author.CanViewSubmissions || author.CanEdit ||
		author.Problems[0].ID != p1 || author.Problems[1].ID != "" {
		t.Fatalf("author view: %+v", author)
	}
	if public := view(""); public.Official || public.Staff || public.CanViewSubmissions {
		t.Fatalf("public view: %+v", public)
	}
	if setter := view("alice"); setter.Official || !setter.Staff || !setter.CanEdit {
		t.Fatalf("setter view: %+v", setter)
	}
	if body := request("POST", joinPath, "writer", nil, 409); !strings.Contains(body, "contest_participation_unavailable") {
		t.Fatal(body)
	}
	request("GET", "/my/contests/"+cid+"/problems/"+p1, "writer", nil, 200)
	request("GET", "/my/contests/"+cid+"/problems/"+p2, "writer", nil, 404)
	request("GET", "/my/contests/"+cid+"/submissions", "writer", nil, 200)
	// Like testers, the author may try their own problem in the contest before the start, but not the others.
	submit("writer", p1, 202)
	submit("writer", p2, 409)

	// A participant cannot accept a tester invitation for a contest problem, but may test other problems.
	request("POST", joinPath, "part", nil, 200)
	if joined := view("part"); !joined.Official || joined.Staff || !joined.Participating || joined.CanViewSubmissions {
		t.Fatalf("participant view: %+v", joined)
	}
	invite2 := invitation(p2, "alice")
	if preview := request("GET", invite2, "part", nil, 200); !strings.Contains(preview, `"joined":false`) {
		t.Fatal(preview)
	}
	if body := request("POST", invite2, "part", nil, 409); !strings.Contains(body, "contest_participant_locked") {
		t.Fatal(body)
	}
	request("GET", "/my/problems/"+p2, "part", nil, 404)
	request("POST", invite2, "late", nil, 200)
	request("POST", joinPath, "late", nil, 409)
	if tester := view("late"); tester.Official || !tester.Staff {
		t.Fatalf("tester view: %+v", tester)
	}
	request("POST", invitation(p3, "alice"), "part", nil, 200)

	// A problem that a participant authors or tests cannot enter the contest.
	in.Version = 1
	for _, extra := range []string{p3, p4} {
		attempt := in
		attempt.Problems = append(slices.Clone(in.Problems), contests.Problem{ID: extra, Points: 300})
		if body := request("PUT", "/my/contests/"+cid, "alice", attempt, 409); !strings.Contains(body, "contest_participant_problem") {
			t.Fatal(body)
		}
	}
	if setter := view("alice"); setter.Version != 1 || len(setter.Problems) != 2 {
		t.Fatalf("refused save changed the contest: %+v", setter)
	}
	// Only additions are refused, so a contest that already holds such a problem can still be saved.
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'part')`, p2)
	request("PUT", "/my/contests/"+cid, "alice", in, 200)
	exec(`DELETE FROM problem_testers WHERE problem_id=$1 AND owner_id='part'`, p2)

	// Removing a problem keeps its author and testers out, since they have already seen it.
	in.Version = 2
	in.Problems = []contests.Problem{{ID: p2, Points: 200}}
	request("PUT", "/my/contests/"+cid, "alice", in, 200)
	if got := excluded(); !slices.Equal(got, []string{"alice", "former", "writer"}) {
		t.Fatal("exclusions", got)
	}
	for _, owner := range []string{"writer", "former"} {
		request("POST", joinPath, owner, nil, 409)
		if former := view(owner); former.Official || former.Staff || former.CanViewSubmissions {
			t.Fatalf("former staff view for %s: %+v", owner, former)
		}
	}
	request("GET", "/my/contests/"+cid+"/submissions", "writer", nil, 404)
	// Registrations made before this rule, by former staff and by a tester, never count.
	exec(`INSERT INTO contest_participants(contest_id,owner_id) VALUES($1::uuid,'former'),($1::uuid,'late')`, cid)

	start := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	exec(`UPDATE contests SET starts_at=$2,ends_at=$3 WHERE id=$1`, cid, start, time.Now().Add(time.Hour))
	// Keep the tester's acceptance before the start after moving the test clock.
	exec(`UPDATE problem_testers SET created_at=$1 WHERE owner_id='late'`, start.Add(-time.Minute))
	if body := submit("former", p2, 409); !strings.Contains(body, "contest_participation_required") {
		t.Fatal(body)
	}
	submit("writer", p2, 409)
	var run submissions.Submission
	if err := json.Unmarshal([]byte(submit("part", p2, 202)), &run); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE submissions SET created_at=$2,status='DONE',result='{"verdict":"AC","passed":2,"total":2}' WHERE id=$1`, run.ID, start.Add(10*time.Minute))
	submit("late", p2, 202) // Staff may still submit.
	if got := rank(); !slices.Equal(got, []string{"part"}) {
		t.Fatal("running standings", got)
	}

	// Once the results are final, a participant may become a tester and keeps the final rank.
	exec(`UPDATE contests SET ends_at=$2 WHERE id=$1`, cid, start.Add(30*time.Minute))
	request("POST", invite2, "part", nil, 200)
	if got := rank(); !slices.Equal(got, []string{"part"}) {
		t.Fatal("final standings", got)
	}
	if final := view("part"); !final.Official || !final.Staff || !final.Participating {
		t.Fatalf("late tester view: %+v", final)
	}
	request("POST", joinPath, "part", nil, 200)
}
