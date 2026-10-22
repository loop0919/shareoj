package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

func TestRejudgeNotificationsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_rejudge_notify_%d", time.Now().UnixNano())
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
	const (
		plain   = "11111111-1111-4111-8111-111111111111"
		first   = "22222222-2222-4222-8222-222222222222"
		second  = "33333333-3333-4333-8333-333333333333"
		contest = "44444444-4444-4444-8444-444444444444"
		exPub   = "55555555-5555-4555-8555-555555555555"
		future  = "66666666-6666-4666-8666-666666666666"
	)
	difficulty := 1
	for id, title := range map[string]string{plain: "Plain", first: "First", second: "Second"} {
		d := problems.Draft{Title: title, Markdown: "Add", Difficulty: &difficulty, TimeLimitMS: "2000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "1 2", Output: "3"}}}
		if _, err := store.Save(ctx, "alice", id, 0, d); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Publish(ctx, "alice", id, 1, true); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES ($1,'alice','c','d',clock_timestamp()-interval '2 hours',clock_timestamp()-interval '1 hour',true)`, contest)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
 SELECT $1,id,CASE WHEN id=$2 THEN 0 ELSE 1 END,100,jsonb_set(published_draft,'{title}','"Contest copy"'),1 FROM problem_drafts WHERE id IN ($2,$3)`, contest, first, second)
	n := 0
	insert := func(problem, owner string) {
		t.Helper()
		n++
		raw, _ := json.Marshal(map[string]any{"image": "sha256:" + strings.Repeat("0", 64), "easyTest": false, "privateDraft": false, "timeLimitMs": 2000, "memoryLimitMb": 512, "cases": []map[string]string{{"input": "1 2", "output": "3"}}})
		exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,status,result,job,dispatched_at,finished_at)
 VALUES ($1,$2,$3,1,'old','cpp17-isolate','source','DONE','{"verdict":"WA","passed":0,"total":1}'::jsonb,$4,clock_timestamp(),clock_timestamp())`,
			fmt.Sprintf("%08d-5555-4555-8555-555555555555", n), owner, problem, raw)
	}
	for _, id := range []string{plain, second} {
		insert(id, "bob")
		insert(id, "bob")
		insert(id, "carol")
		insert(id, "alice")
	}
	f := newSigningFixture(t)
	h := &testHandler{t: t, f: f, h: newHandler(AuthConfig{}, handlerDependencies{
		Store: store, Contests: &contests.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool},
		Notifications: &notifications.Store{Pool: pool},
		JudgeImage:    "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17",
		Verifier: newCognitoVerifier(f.server.URL, "client"), DispatchJudge: func(context.Context) error { return nil },
	})}
	type item struct {
		Kind, Actor, ProblemID, ContestID, Title string
		ProblemIndex                             *int
	}
	bell := func(owner string) []item {
		t.Helper()
		var page struct{ Notifications []item }
		if err := json.Unmarshal([]byte(h.request("GET", "/my/notifications?history=1", owner, "", 200)), &page); err != nil {
			t.Fatal(err)
		}
		return page.Notifications
	}
	if got := h.request("POST", "/my/problems/"+plain+"/rejudge", "alice", `{}`, 202); got != `{"queued":4}` {
		t.Fatalf("queued: %s", got)
	}
	// The one who rejudges hears about their own submissions too.
	for _, user := range []string{"bob", "carol", "alice"} {
		got := bell(user)
		if len(got) != 1 || got[0].Kind != "rejudge" || got[0].Actor != "alice" || got[0].ProblemID != plain ||
			got[0].ContestID != "" || got[0].ProblemIndex != nil || got[0].Title != "Plain" {
			t.Fatalf("%s: %+v", user, got)
		}
	}
	// The contest problem at position 1 names the contest, its index and the contest copy's title.
	if got := h.request("POST", "/my/problems/"+second+"/rejudge", "alice", `{}`, 202); got != `{"queued":4}` {
		t.Fatalf("queued: %s", got)
	}
	for _, user := range []string{"carol", "alice"} {
		got := bell(user)
		matches := 0
		for _, g := range got {
			if g.ProblemID != second {
				continue
			}
			matches++
			if g.ContestID != contest || g.ProblemIndex == nil || *g.ProblemIndex != 1 || g.Title != "Contest copy" {
				t.Fatalf("contest rejudge: %+v", g)
			}
		}
		if len(got) != 2 || matches != 1 {
			t.Fatalf("%s: %+v", user, got)
		}
	}
	// A problem taken down and put into a contest that has not started stays hidden from its submitter: no contest,
	// no index and no title, neither the contest copy's nor the working draft's.
	d := problems.Draft{Title: "Ex public", Markdown: "Add", Difficulty: &difficulty, TimeLimitMS: "2000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "1 2", Output: "3"}}}
	if _, err := store.Save(ctx, "alice", exPub, 0, d); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, "alice", exPub, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, "alice", exPub, 2, false); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES ($1,'alice','f','d',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '2 hours',false)`, future)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version)
 SELECT $1,id,1,100,jsonb_set(draft,'{title}','"Future copy"'),1 FROM problem_drafts WHERE id=$2`, future, exPub)
	insert(exPub, "dave")
	if got := h.request("POST", "/my/problems/"+exPub+"/rejudge", "alice", `{}`, 202); got != `{"queued":1}` {
		t.Fatalf("queued: %s", got)
	}
	got := bell("dave")
	if len(got) != 1 || got[0].Kind != "rejudge" || got[0].ContestID != "" || got[0].ProblemIndex != nil || got[0].Title != "" {
		t.Fatalf("before the start: %+v", got)
	}
	exec(`UPDATE contests SET starts_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, future)
	got = bell("dave")
	if len(got) != 1 || got[0].ContestID != future || got[0].ProblemIndex == nil || *got[0].ProblemIndex != 1 || got[0].Title != "Future copy" {
		t.Fatalf("after the start: %+v", got)
	}
}
