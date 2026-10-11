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
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

func TestUserEditorialsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_user_editorials_%d", time.Now().UnixNano())
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
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob')`)
	const (
		public   = "11111111-1111-4111-8111-111111111111"
		private  = "22222222-2222-4222-8222-222222222222"
		featured = "33333333-3333-4333-8333-333333333333"
		post     = "44444444-4444-4444-8444-444444444444"
		own      = "55555555-5555-4555-8555-555555555555"
		contest  = "66666666-6666-4666-8666-666666666666"
	)
	difficulty := 2
	draft := problems.Draft{Difficulty: &difficulty, Title: "Public A", Markdown: "statement", Editorial: "official", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
	for _, id := range []string{public, private, featured} {
		if _, err = store.Save(ctx, "alice", id, 0, draft); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE problem_drafts SET published_draft=draft,published_version=version,published_at=now() WHERE id IN ($1,$2)`, public, featured)
	exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES (now(),'easy','new',$1,2,now()+interval '1 day')`, featured)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES ($1,'alice','Ended','',now()-interval '2 hours',now()-interval '1 hour',true)`, contest)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) SELECT $1,id,0,100,draft,version FROM problem_drafts WHERE id=$2`, contest, public)

	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Posts: posts.New(store.Pool()), Profiles: profiles.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:" + strings.Repeat("a", 64), Verifier: newCognitoVerifier(f.server.URL, "client")})
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
	save := func(owner, id string, version int64, ids any, want int) posts.Post {
		t.Helper()
		body := map[string]any{"version": version, "title": "Bob's editorial", "markdown": "body"}
		if ids != nil {
			body["problemIds"] = ids
		}
		var result posts.Post
		if raw := request("PUT", "/my/posts/"+id, owner, body, want); want == 200 {
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
		} else if want == 400 && !strings.Contains(raw, "editorial_problem_unavailable") && !strings.Contains(raw, "invalid_post") {
			t.Fatal("unexpected error", raw)
		}
		return result
	}
	editorials := func(path string) []posts.Editorial {
		t.Helper()
		var result struct{ Items []posts.Editorial }
		if err := json.Unmarshal([]byte(request("GET", path, "", nil, 200)), &result); err != nil {
			t.Fatal(err)
		}
		return result.Items
	}
	userEditorialNotices := func(owner string) int {
		t.Helper()
		var result struct {
			Notifications []struct{ ProblemID, Kind string }
		}
		if err := json.Unmarshal([]byte(request("GET", "/my/notifications?history=1", owner, nil, 200)), &result); err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, item := range result.Notifications {
			if item.Kind == "user_editorial" && item.ProblemID == public {
				n++
			}
		}
		return n
	}

	// Only problems whose official editorial is public can be listed.
	save("bob", post, 0, []string{private}, 400)
	save("bob", post, 0, []string{featured}, 400)
	save("bob", post, 0, []string{public, public}, 400)
	save("bob", post, 0, []string{"not-a-problem"}, 400)
	created := save("bob", post, 0, []string{public}, 200)
	if len(created.Problems) != 1 || created.Problems[0].ID != public || created.Problems[0].Title != "Public A" {
		t.Fatal("saved problems", created.Problems)
	}
	// Older clients that omit the list keep it.
	kept := save("bob", post, created.Version, nil, 200)
	if len(kept.Problems) != 1 || kept.Problems[0].ID != public {
		t.Fatal("omitted problemIds must keep the list", kept.Problems)
	}
	if got := editorials("/problems/" + public + "/editorials"); len(got) != 0 {
		t.Fatal("draft links must stay private", got)
	}
	request("PUT", "/my/posts/"+post+"/publication", "bob", map[string]any{"version": kept.Version, "publish": true}, 200)
	got := editorials("/problems/" + public + "/editorials")
	if len(got) != 1 || got[0].ID != post || got[0].ProblemID != public || got[0].Author != "bob" || got[0].Title != "Bob's editorial" {
		t.Fatal("published editorial", got)
	}
	if got = editorials("/contests/" + contest + "/editorials"); len(got) != 1 || got[0].ProblemID != public {
		t.Fatal("ended contest editorials", got)
	}
	var published posts.Post
	if err := json.Unmarshal([]byte(request("GET", "/posts/"+post, "", nil, 200)), &published); err != nil {
		t.Fatal(err)
	}
	if len(published.Problems) != 1 || published.Problems[0].ID != public || published.Problems[0].Title != "Public A" {
		t.Fatal("public post problems", published.Problems)
	}
	if n := userEditorialNotices("alice"); n != 1 {
		t.Fatal("problem author notifications", n)
	}
	// Republishing does not notify again, and an author's own post does not notify them.
	request("PUT", "/my/posts/"+post+"/publication", "bob", map[string]any{"version": kept.Version + 1, "publish": true}, 200)
	mine := save("alice", own, 0, []string{public}, 200)
	request("PUT", "/my/posts/"+own+"/publication", "alice", map[string]any{"version": mine.Version, "publish": true}, 200)
	if n := userEditorialNotices("alice"); n != 1 {
		t.Fatal("duplicate or self notification", n)
	}
	if got = editorials("/problems/" + public + "/editorials"); len(got) != 2 || got[0].ID != post || got[1].ID != own {
		t.Fatal("editorials must be oldest first", got)
	}

	// A running contest hides its editorials.
	exec(`UPDATE contests SET ends_at=now()+interval '1 hour' WHERE id=$1`, contest)
	if got = editorials("/contests/" + contest + "/editorials"); len(got) != 0 {
		t.Fatal("running contest editorials", got)
	}
	// A problem that stops being public drops out everywhere, and the draft cannot be saved with it.
	exec(`UPDATE problem_drafts SET published_draft=NULL WHERE id=$1`, public)
	if got = editorials("/problems/" + public + "/editorials"); len(got) != 0 {
		t.Fatal("unpublished problem editorials", got)
	}
	if body := request("GET", "/posts/"+post, "", nil, 200); strings.Contains(body, public) {
		t.Fatal("unpublished problem in public post", body)
	}
	var draftView posts.Post
	if err := json.Unmarshal([]byte(request("GET", "/my/posts/"+post, "bob", nil, 200)), &draftView); err != nil {
		t.Fatal(err)
	}
	if len(draftView.Problems) != 1 || draftView.Problems[0].ID != public || draftView.Problems[0].Title != "" {
		t.Fatal("draft must keep the problem without its title", draftView.Problems)
	}
	save("bob", post, draftView.Version, []string{public}, 400)
	cleared := save("bob", post, draftView.Version, []string{}, 200)
	if len(cleared.Problems) != 0 {
		t.Fatal("cleared problems", cleared.Problems)
	}
	// Deleting the post removes its notification.
	request("DELETE", fmt.Sprintf("/my/posts/%s?version=%d", post, cleared.Version), "bob", nil, 204)
	if n := userEditorialNotices("alice"); n != 0 {
		t.Fatal("notification of a deleted post", n)
	}
}
