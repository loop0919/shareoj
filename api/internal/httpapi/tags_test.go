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

	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/tags"
)

func TestTagsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_tags_%d", time.Now().UnixNano())
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
		featured = "33333333-3333-4333-8333-333333333333"
		post     = "44444444-4444-4444-8444-444444444444"
	)

	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Posts: posts.New(store.Pool()), Profiles: profiles.New(store.Pool()), Tags: &tags.Store{Pool: store.Pool()}, Verifier: newCognitoVerifier(f.server.URL, "client")})
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
	decode := func(raw string, target any) {
		t.Helper()
		if err := json.Unmarshal([]byte(raw), target); err != nil {
			t.Fatal(err)
		}
	}
	problemTags := func(path, viewer string) []string {
		t.Helper()
		var p problems.PublicProblem
		decode(request("GET", path, viewer, nil, 200), &p)
		return p.Tags
	}
	listed := func(path string) []string {
		t.Helper()
		var page struct{ Items []struct{ ID string } }
		decode(request("GET", path, "", nil, 200), &page)
		ids := []string{}
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		return ids
	}
	tagCounts := func() map[string][2]int64 {
		t.Helper()
		var result struct{ Items []tags.Tag }
		decode(request("GET", "/tags", "", nil, 200), &result)
		counts := map[string][2]int64{}
		for _, tag := range result.Items {
			counts[tag.Name] = [2]int64{tag.ProblemCount, tag.PostCount}
		}
		return counts
	}

	// Problem tags are part of the draft and are validated with it.
	difficulty := 2
	draft := problems.Draft{Difficulty: &difficulty, Title: "Public A", Markdown: "statement", Editorial: "official", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
	for _, invalid := range [][]string{{"ＤＰ"}, {"DP", "dp"}, {"a,b"}, {" DP"}} {
		bad := draft
		bad.Tags = invalid
		request("PUT", "/my/problems/"+public, "alice", map[string]any{"version": 0, "draft": bad}, 400)
	}
	draft.Tags = []string{"DP", "グラフ"}
	var saved problems.Problem
	decode(request("PUT", "/my/problems/"+public, "alice", map[string]any{"version": 0, "draft": draft}, 200), &saved)
	if !slices.Equal(saved.Draft.Tags, draft.Tags) {
		t.Fatal("saved problem tags", saved.Draft.Tags)
	}
	if _, err = store.Save(ctx, "alice", featured, 0, draft); err != nil {
		t.Fatal(err)
	}
	if got := tagCounts(); len(got) != 0 {
		t.Fatal("draft tags must stay private", got)
	}
	exec(`UPDATE problem_drafts SET published_draft=draft,published_version=version,published_at=now() WHERE id IN ($1,$2)`, public, featured)
	exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES (now(),'easy','new',$1,2,now()+interval '1 day')`, featured)

	if got := problemTags("/problems/"+public, ""); !slices.Equal(got, []string{"DP", "グラフ"}) {
		t.Fatal("public problem tags", got)
	}
	// A 定期便 problem hides its tags with its editorial, except from its author.
	if got := problemTags("/problems/"+featured, ""); len(got) != 0 {
		t.Fatal("hidden featured tags", got)
	}
	if got := problemTags("/my/problems/"+featured+"/publication", "alice"); len(got) != 2 {
		t.Fatal("author must see featured tags", got)
	}
	if got := listed("/problems?tag=DP"); !slices.Equal(got, []string{public}) {
		t.Fatal("problems tagged DP", got)
	}
	if got := listed("/problems?tag=dp"); len(got) != 0 {
		t.Fatal("tag filter must match exactly", got)
	}
	if got := listed("/problems?author=alice&tag=" + url.QueryEscape("グラフ")); !slices.Equal(got, []string{public}) {
		t.Fatal("author and tag filter", got)
	}
	request("GET", "/problems?tag=a,b", "", nil, 400)
	request("GET", "/posts?tag="+url.QueryEscape("ＤＰ"), "", nil, 400)

	// A revival round also hides an old problem's tags until the round's editorial time.
	exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES (now()-interval '1 hour','easy','revival',$1,2,now()+interval '1 day')`, public)
	if got := problemTags("/problems/"+public, ""); len(got) != 0 {
		t.Fatal("revival round tags", got)
	}
	var round struct{ Items []problems.PublicProblem }
	decode(request("GET", "/problems", "", nil, 200), &round)
	for _, item := range round.Items {
		if len(item.Tags) != 0 {
			t.Fatal("listed tags during a round", item.ID, item.Tags)
		}
	}
	if got := listed("/problems?tag=DP"); len(got) != 0 {
		t.Fatal("tag filter during a round", got)
	}
	if got := tagCounts(); len(got) != 0 {
		t.Fatal("tag counts during a round", got)
	}
	if got := problemTags("/my/problems/"+public+"/publication", "alice"); len(got) != 2 {
		t.Fatal("author must see tags during a round", got)
	}
	exec(`UPDATE featured_slots SET reveal_at=now()-interval '1 minute' WHERE problem_id=$1 AND kind='revival'`, public)
	if got := problemTags("/problems/"+public, ""); len(got) != 2 {
		t.Fatal("tags after the round", got)
	}

	// Post tags are a draft until the post is published.
	save := func(version int64, list any, want int) posts.Post {
		t.Helper()
		body := map[string]any{"version": version, "title": "Bob's notes", "markdown": "body"}
		if list != nil {
			body["tags"] = list
		}
		var result posts.Post
		if raw := request("PUT", "/my/posts/"+post, "bob", body, want); want == 200 {
			decode(raw, &result)
		}
		return result
	}
	save(0, []string{"DP", "Dp"}, 400)
	created := save(0, []string{"DP", "Rust"}, 200)
	if !slices.Equal(created.Tags, []string{"DP", "Rust"}) {
		t.Fatal("saved post tags", created.Tags)
	}
	kept := save(created.Version, nil, 200)
	if !slices.Equal(kept.Tags, created.Tags) {
		t.Fatal("omitted tags must keep the list", kept.Tags)
	}
	if got := listed("/posts?tag=Rust"); len(got) != 0 {
		t.Fatal("draft post tags must stay private", got)
	}
	request("PUT", "/my/posts/"+post+"/publication", "bob", map[string]any{"version": kept.Version, "publish": true}, 200)
	if got := listed("/posts?tag=Rust"); !slices.Equal(got, []string{post}) {
		t.Fatal("posts tagged Rust", got)
	}
	var published posts.Post
	decode(request("GET", "/posts/"+post, "", nil, 200), &published)
	if !slices.Equal(published.Tags, []string{"DP", "Rust"}) {
		t.Fatal("public post tags", published.Tags)
	}
	var page struct{ Items []posts.Post }
	decode(request("GET", "/posts", "", nil, 200), &page)
	if len(page.Items) != 1 || !slices.Equal(page.Items[0].Tags, []string{"DP", "Rust"}) {
		t.Fatal("post list tags", page.Items)
	}
	want := map[string][2]int64{"DP": {1, 1}, "Rust": {0, 1}, "グラフ": {1, 0}}
	if got := tagCounts(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatal("tag counts", got)
	}
	var ordered struct{ Items []tags.Tag }
	decode(request("GET", "/tags", "", nil, 200), &ordered)
	if ordered.Items[0].Name != "DP" {
		t.Fatal("most used tag first", ordered.Items)
	}

	// Editing the draft does not change the published tags until the post is republished.
	edited := save(kept.Version+1, []string{}, 200)
	if len(edited.Tags) != 0 || !slices.Equal(listed("/posts?tag=Rust"), []string{post}) {
		t.Fatal("published tags must wait for publication", edited.Tags)
	}
	request("PUT", "/my/posts/"+post+"/publication", "bob", map[string]any{"version": edited.Version, "publish": true}, 200)
	exec(`UPDATE problem_drafts SET published_draft=NULL WHERE id=$1`, public)
	if got := tagCounts(); len(got) != 0 {
		t.Fatal("unpublished tags must drop out", got)
	}
}
