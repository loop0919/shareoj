package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
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

func TestNotificationsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_notifications_%d", time.Now().UnixNano())
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
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	draft := problems.Draft{Title: "Secret A", Markdown: "secret statement", Editorial: "secret editorial", TimeLimitMS: "1000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}, {Input: "private input", Output: "private output"}}}
	for _, id := range []string{a, b} {
		if _, err = store.Save(ctx, "alice", id, 0, draft); err != nil {
			t.Fatal(err)
		}
	}
	f := newSigningFixture(t)
	queue := &submissions.Store{Pool: store.Pool()}
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: queue, JudgeImage: "sha256:" + strings.Repeat("a", 64), Verifier: newCognitoVerifier(f.server.URL, "client")})
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

	request("GET", "/my/notifications", "", nil, 401)
	list := func(owner string) []struct{ ID, Kind string } {
		t.Helper()
		var result struct{ Notifications []struct{ ID, Kind string } }
		if err := json.Unmarshal([]byte(request("GET", "/my/notifications", owner, nil, 200)), &result); err != nil {
			t.Fatal(err)
		}
		return result.Notifications
	}
	count := func(want int) {
		t.Helper()
		if got := len(list("alice")); got != want {
			t.Fatalf("notifications = %d, want %d", got, want)
		}
	}
	exec(`INSERT INTO problem_favorites VALUES ($1,'alice'),($1,'bob') ON CONFLICT DO NOTHING`, a)
	exec(`INSERT INTO problem_favorites VALUES ($1,'bob') ON CONFLICT DO NOTHING`, a)
	count(1)
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES ($1,'tester') ON CONFLICT DO NOTHING`, a)
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES ($1,'tester') ON CONFLICT DO NOTHING`, a)
	count(2)
	submit := func(owner, job, verdict string) {
		t.Helper()
		var id string
		err := store.Pool().QueryRow(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job)
   VALUES(gen_random_uuid(),$1,$2,1,'A','cpp17-local','',$3) RETURNING id`, owner, a, job).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		exec(`UPDATE submissions SET status='DONE',result=jsonb_build_object('verdict',$2::text) WHERE id=$1`, id, verdict)
	}
	submit("alice", `{}`, "AC")
	submit("tester", `{}`, "AC")
	submit("bob", `{"easyTest":true}`, "AC")
	submit("bob", `{"generate":true}`, "AC")
	submit("bob", `{"validate":true}`, "AC")
	submit("bob", `{}`, "WA")
	count(2)
	submit("bob", `{}`, "AC")
	submit("carol", `{}`, "AC")
	count(3)
	kinds := map[string]int{}
	for _, n := range list("alice") {
		kinds[n.Kind]++
	}
	for _, kind := range []string{"favorite", "tester", "first_accept"} {
		if kinds[kind] != 1 {
			t.Fatalf("kinds: %v", kinds)
		}
	}
	if len(list("bob")) != 0 {
		t.Fatal("another user's notifications leaked")
	}
	request("POST", "/my/notifications/read", "", nil, 401)
	request("POST", "/my/notifications/read", "bob", nil, 200)
	count(3)
	var opened struct{ Notifications []struct{ ID, Kind string } }
	if err := json.Unmarshal([]byte(request("POST", "/my/notifications/read", "alice", nil, 200)), &opened); err != nil || len(opened.Notifications) != 3 {
		t.Fatalf("bulk read did not return all notifications: %+v, %v", opened, err)
	}
	count(0)
	request("POST", "/my/notifications/read", "alice", nil, 200)
	var history struct{ Notifications []struct{ ID, Kind string } }
	if err := json.Unmarshal([]byte(request("GET", "/my/notifications?history=1", "alice", nil, 200)), &history); err != nil || len(history.Notifications) != 3 {
		t.Fatalf("read notifications missing from history: %+v, %v", history, err)
	}
	if err := json.Unmarshal([]byte(request("GET", "/my/notifications?history=1", "bob", nil, 200)), &history); err != nil || len(history.Notifications) != 0 {
		t.Fatal("another user's history leaked", err)
	}
	submit("carol", `{}`, "AC")
	count(0)
	exec(`INSERT INTO problem_favorites VALUES ($1,'carol')`, a)
	count(1)
	request("GET", "/my/notifications?history=1", "alice", nil, 200)
	count(1) // Browsing history must not read newly arrived notifications.
}
