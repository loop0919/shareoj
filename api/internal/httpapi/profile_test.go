package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
)

func TestAvatarValidation(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	if _, err := cleanAvatar(valid); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 257, 1))); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"https://example.com/avatar.png", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,broken", "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), strings.Repeat("x", 180001)} {
		if _, err := cleanAvatar(value); err == nil {
			t.Fatal("accepted invalid avatar")
		}
	}
}

func TestProfilesPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_profiles_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) }()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
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
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profileStore := profiles.New(store.Pool())
	f := newSigningFixture(t)
	handler := newHandler(AuthConfig{}, PrivateProblems{Store: store, Profiles: profileStore, Posts: posts.New(store.Pool()), Operators: map[string]bool{"alice": true}, Verifier: newCognitoVerifier(f.server.URL, "client")})
	request := func(method, path, owner, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if owner != "" {
			r.Header.Set("Authorization", "Bearer "+f.token(t, owner, nil))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/my/profile", "", ""); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := request("GET", "/my/profile", "alice", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"profile":null`) {
		t.Fatalf("missing: %d %s", w.Code, w.Body.String())
	}
	if w := request("GET", "/my/problems", "alice", ""); w.Code != 403 {
		t.Fatalf("onboarding gate: %d", w.Code)
	}
	for _, body := range []string{`{"handle":"ab","version":0}`, `{"handle":"9alice","version":0}`, `{"handle":"alice","owner":"bob","version":0}`, `{"handle":"alice","avatar":"https://example.com/x","version":0}`} {
		if w := request("PUT", "/my/profile", "alice", body); w.Code != 400 {
			t.Fatalf("invalid accepted: %d", w.Code)
		}
	}
	if w := request("PUT", "/my/profile", "alice", `{"handle":"ALICE","avatar":"","version":0}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"handle":"alice"`) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := request("GET", "/my/problems", "alice", ""); w.Code != 200 {
		t.Fatalf("registered gate: %d", w.Code)
	}
	if w := request("PUT", "/my/profile", "bob", `{"handle":"Alice","avatar":"","version":0}`); w.Code != 409 || !strings.Contains(w.Body.String(), "handle_taken") {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
	if w := request("GET", "/my/profile", "bob", ""); !strings.Contains(w.Body.String(), `"profile":null`) {
		t.Fatal("profile leaked")
	}
	if w := request("PUT", "/my/profile", "alice", `{"handle":"alice_new","avatar":"","version":1,"accounts":{"x":"@alice_x","atcoder":"tourist","codeforces":"tourist","yukicoder":"123"}}`); w.Code != 200 {
		t.Fatalf("edit: %d", w.Code)
	}
	if w := request("PUT", "/my/profile", "alice", `{"handle":"stale","avatar":"","version":1}`); w.Code != 409 {
		t.Fatalf("stale: %d", w.Code)
	}
	var result struct {
		Profile profiles.Profile `json:"profile"`
	}
	if err = json.Unmarshal(request("GET", "/my/profile", "alice", "").Body.Bytes(), &result); err != nil || result.Profile.Handle != "alice_new" || result.Profile.Accounts.X != "alice_x" || result.Profile.Accounts.AtCoder != "tourist" || result.Profile.Accounts.Codeforces != "tourist" || result.Profile.Accounts.Yukicoder != "123" {
		t.Fatal("profile not persisted")
	}

	for _, handle := range []string{"alice", "missing", "!"} {
		if w := request("GET", "/users/"+handle, "", ""); w.Code != 404 {
			t.Fatalf("missing public profile: %d", w.Code)
		}
	}
	if w := request("GET", "/users/alice_new", "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "version") || strings.Contains(w.Body.String(), "owner") || !strings.Contains(w.Body.String(), `"handle":"alice_new"`) || !strings.Contains(w.Body.String(), `"atcoder":"tourist"`) {
		t.Fatalf("public identity: %d %s", w.Code, w.Body.String())
	}

	// Public snapshots never expose later private edits; all mutation paths enforce ownership and version.
	if w := request("PUT", "/my/profile", "bob", `{"handle":"bob","avatar":"","version":0}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	testContentImages(t, request, store)
	for _, kind := range []string{"problems", "posts"} {
		id := "22222222-2222-4222-8222-222222222222"
		private := "/my/" + kind + "/" + id
		public := "/" + kind + "/" + id
		body := func(version int, text string) string {
			if kind == "problems" {
				return fmt.Sprintf(`{"version":%d,"draft":{"title":"Published title","markdown":%q,"timeLimitMs":"2000","memoryLimitMb":"256","difficulty":%d}}`, version, text, min(10, 4+version*3))
			}
			return fmt.Sprintf(`{"version":%d,"title":"Published title","markdown":%q}`, version, text)
		}
		check := func(method, path, owner, body string, code int) string {
			t.Helper()
			w := request(method, path, owner, body)
			if w.Code != code {
				t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
			return w.Body.String()
		}
		check("PUT", private, "", body(0, "public body"), 401)
		check("PUT", private, "alice", body(0, "public body"), 200)
		check("GET", public, "", "", 404)
		if result := check("GET", "/"+kind+"?author=alice_new", "", "", 200); strings.Contains(result, id) {
			t.Fatal("private draft leaked", result)
		}

		if kind == "problems" {
			check("PUT", "/my/favorites/"+id, "bob", `{"favorited":true}`, 404)
			for _, level := range []string{"0", "11", "1.5", `"4"`} {
				invalid := strings.Replace(body(1, "invalid"), `"difficulty":7`, `"difficulty":`+level, 1)
				check("PUT", private, "alice", invalid, 400)
			}
		}
		check("PUT", private+"/publication", "bob", `{"version":1,"publish":true}`, 404)
		check("PUT", private+"/publication", "alice", `{"version":1}`, 400)
		check("PUT", private+"/publication", "alice", `{"version":1,"publish":true}`, 200)
		visible := check("GET", public, "", "", 200)
		if result := check("GET", "/"+kind+"?author=alice_new", "", "", 200); !strings.Contains(result, id) {
			t.Fatal("published item missing", result)
		}
		if result := check("GET", "/"+kind+"?author=bob", "", "", 200); strings.Contains(result, id) {
			t.Fatal("another author's item leaked", result)
		}
		check("GET", "/"+kind+"?author=invalid!", "", "", 400)
		cursor := nextContentCursor(id, time.Now().Add(time.Hour))
		if result := check("GET", "/"+kind+"?author=alice_new&cursor="+cursor, "", "", 200); !strings.Contains(result, id) {
			t.Fatal("filtered pagination missing item", result)
		}

		var initial struct {
			PublishedAt time.Time `json:"publishedAt"`
		}
		if err := json.Unmarshal([]byte(visible), &initial); err != nil || initial.PublishedAt.IsZero() {
			t.Fatalf("initial publication: %s (%v)", visible, err)
		}
		if !strings.Contains(visible, "public body") || !strings.Contains(visible, `"author":"alice_new"`) || strings.Contains(visible, "owner") {
			t.Fatal(visible)
		}
		if kind == "posts" && !strings.Contains(visible, `"isOperator":true`) {
			t.Fatal("missing operator badge")
		}
		if kind == "problems" {
			if !strings.Contains(visible, `"solverCount":0`) {
				t.Fatal(visible)
			}
			// Repeated ACs count once; draft, sample, generation, validation,
			// unfinished, and incorrect submissions must not count as solves.
			_, err := store.Pool().Exec(ctx, `INSERT INTO submissions
				(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
				SELECT gen_random_uuid(),v.owner_id,$1,1,'test','cpp17','',v.job::jsonb,v.status,jsonb_build_object('verdict',v.verdict)
				FROM (VALUES
					('bob','{"privateDraft":false}','DONE','AC'),
					('bob','{"privateDraft":false}','DONE','AC'),
					('alice','{"privateDraft":true}','DONE','AC'),
					('alice','{}','DONE','AC'),
					('alice','{"privateDraft":false,"easyTest":true}','DONE','AC'),
					('alice','{"privateDraft":false,"generate":true}','DONE','AC'),
					('alice','{"privateDraft":false,"validate":true}','DONE','AC'),
					('alice','{"privateDraft":false}','RUNNING','AC'),
					('alice','{"privateDraft":false}','DONE','WA')
				) AS v(owner_id,job,status,verdict)`, id)
			if err != nil {
				t.Fatal(err)
			}
			// Recent failures must not hide an older AC.
			if _, err := store.Pool().Exec(ctx, `INSERT INTO submissions
				(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
				SELECT gen_random_uuid(),'bob',$1,1,'test','cpp17','','{"privateDraft":false}','DONE','{"verdict":"WA"}'
				FROM generate_series(1,51)`, id); err != nil {
				t.Fatal(err)
			}
			check("GET", "/my/solved-problems", "", "", 401)
			if result := check("GET", "/my/solved-problems", "bob", "", 200); strings.TrimSpace(result) != `{"items":["`+id+`"]}` {
				t.Fatal(result)
			}
			if result := check("GET", "/my/solved-problems", "alice", "", 200); strings.TrimSpace(result) != `{"items":[]}` {
				t.Fatal(result)
			}
			for _, path := range []string{public, "/problems"} {
				if result := check("GET", path, "", "", 200); !strings.Contains(result, `"solverCount":1`) {
					t.Fatal(result)
				}
			}
			if _, err := store.Pool().Exec(ctx, `INSERT INTO submissions
				(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
				VALUES(gen_random_uuid(),'alice',$1,1,'test','cpp17','','{"privateDraft":false}','DONE','{"verdict":"AC"}')`, id); err != nil {
				t.Fatal(err)
			}
			vote := "/my/difficulty-votes/" + id
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				check(method, vote, "", `{"difficulty":5}`, 401)
			}
			for _, invalid := range []string{`{}`, `{"difficulty":null}`, `{"difficulty":0}`, `{"difficulty":11}`, `{"difficulty":1.5}`, `{"difficulty":"5"}`} {
				check("PUT", vote, "bob", invalid, 400)
			}
			assertVote := func(method, owner, body string, difficulty any, average any, count int64) {
				t.Helper()
				var got problems.DifficultyVote
				if err := json.Unmarshal([]byte(check(method, vote, owner, body, 200)), &got); err != nil {
					t.Fatal(err)
				}
				want, _ := json.Marshal(map[string]any{"difficulty": difficulty, "difficultyAverage": average, "difficultyVoteCount": count})
				actual, _ := json.Marshal(map[string]any{"difficulty": got.Difficulty, "difficultyAverage": got.Average, "difficultyVoteCount": got.Count})
				if string(actual) != string(want) {
					t.Fatalf("vote got %s want %s", actual, want)
				}
			}
			assertVote("GET", "bob", "", nil, nil, 0)
			for range 2 {
				assertVote("PUT", "bob", `{"difficulty":1}`, 1, 1, 1)
			}
			assertVote("PUT", "alice", `{"difficulty":10}`, 10, 5.5, 2)
			assertVote("GET", "bob", "", 1, 5.5, 2)
			assertVote("PUT", "bob", `{"difficulty":4}`, 4, 7, 2)
			if result := check("GET", public, "", "", 200); !strings.Contains(result, `"difficultyAverage":7`) || !strings.Contains(result, `"difficultyVoteCount":2`) || !strings.Contains(result, `"difficulty":4`) {
				t.Fatal(result)
			}
			for range 2 {
				assertVote("DELETE", "alice", "", nil, 4, 1)
			}
			assertVote("DELETE", "bob", "", nil, nil, 0)
			assertVote("PUT", "bob", `{"difficulty":6}`, 6, 6, 1)
			favorite := "/my/favorites/" + id
			check("GET", favorite, "", "", 401)
			check("PUT", favorite, "", `{"favorited":true}`, 401)
			check("PUT", favorite, "bob", `{}`, 400)
			check("PUT", favorite, "bob", `{"favorited":null}`, 400)
			for range 2 {
				result := check("PUT", favorite, "bob", `{"favorited":true}`, 200)
				if !strings.Contains(result, `"favoriteCount":1`) || !strings.Contains(result, `"favorited":true`) {
					t.Fatal(result)
				}
			}
			result := check("GET", favorite, "alice", "", 200)
			if !strings.Contains(result, `"favorited":false`) || !strings.Contains(result, `"favoriteCount":1`) {
				t.Fatal(result)
			}
			check("PUT", favorite, "alice", `{"favorited":true}`, 200)
			result = check("GET", public, "", "", 200)
			if !strings.Contains(result, `"favoriteCount":2`) {
				t.Fatal(result)
			}
			for range 2 {
				check("PUT", favorite, "alice", `{"favorited":false}`, 200)
			}
			result = check("GET", favorite, "bob", "", 200)
			if !strings.Contains(result, `"favoriteCount":1`) || !strings.Contains(result, `"favorited":true`) {
				t.Fatal(result)
			}
		}
		check("PUT", private, "alice", body(2, "private secret"), 200)
		if strings.Contains(check("GET", public, "", "", 200), "private secret") {
			t.Fatal("draft leaked")
		}
		listing := check("GET", "/"+kind, "", "", 200)
		if !strings.Contains(listing, id) || strings.Contains(listing, "private secret") {
			t.Fatal(listing)
		}
		if kind == "problems" {
			for _, field := range []string{`"difficulty":4`, `"timeLimitMs":"2000"`, `"memoryLimitMb":"256"`, `"favoriteCount":1`, `"solverCount":2`} {
				if !strings.Contains(listing, field) {
					t.Fatalf("missing %s: %s", field, listing)
				}
			}
		}
		check("PUT", private+"/publication", "alice", `{"version":2,"publish":true}`, 409)
		check("PUT", private+"/publication", "alice", `{"version":3,"publish":true}`, 200)
		var updated struct {
			PublishedAt time.Time `json:"publishedAt"`
		}
		if err := json.Unmarshal([]byte(check("GET", public, "", "", 200)), &updated); err != nil || !updated.PublishedAt.Equal(initial.PublishedAt) {
			t.Fatalf("%s publication time changed after update: %v -> %v (%v)", kind, initial.PublishedAt, updated.PublishedAt, err)
		}
		if !strings.Contains(check("GET", public, "", "", 200), "private secret") {
			t.Fatal("snapshot not updated")
		}
		if kind == "problems" && !strings.Contains(check("GET", public, "", "", 200), `"difficulty":10`) {
			t.Fatal("difficulty not published")
		}
		check("PUT", private+"/publication", "alice", `{"version":4,"publish":false}`, 200)
		if kind == "problems" {
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				check(method, "/my/difficulty-votes/"+id, "bob", `{"difficulty":5}`, 404)
			}
			check("GET", "/my/favorites/"+id, "bob", "", 404)
			if result := check("GET", "/my/solved-problems", "bob", "", 200); strings.TrimSpace(result) != `{"items":[]}` {
				t.Fatal(result)
			}
			check("PUT", "/my/favorites/"+id, "bob", `{"favorited":true}`, 404)
		}
		check("GET", public, "", "", 404)
		if strings.Contains(check("GET", "/"+kind, "", "", 200), id) {
			t.Fatal("unpublished listed")
		}
		check("PUT", private, "alice", body(5, ""), 200)
		check("PUT", private+"/publication", "alice", `{"version":6,"publish":true}`, 400)
		check("PUT", private, "alice", body(6, "publish again"), 200)
		check("PUT", private+"/publication", "alice", `{"version":7,"publish":true}`, 200)
		check("DELETE", private+"?version=8", "bob", "", 404)
		check("DELETE", private+"?version=7", "alice", "", 409)
		check("DELETE", private+"?version=8", "alice", "", 204)
		if kind == "problems" {
			var count int
			if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM problem_favorites WHERE problem_id=$1`, id).Scan(&count); err != nil || count != 0 {
				t.Fatalf("favorite cleanup: %d %v", count, err)
			}
			if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=$1`, id).Scan(&count); err != nil || count != 0 {
				t.Fatalf("vote cleanup: %d %v", count, err)
			}
		}
		check("GET", public, "", "", 404)
	}
	// Two different owners cannot claim the same handle concurrently.
	var wg sync.WaitGroup
	statuses := make(chan error, 2)
	for _, owner := range []string{"one", "two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := profileStore.Save(ctx, owner, "unique_name", "", 0, profiles.Accounts{})
			statuses <- e
		}()
	}
	wg.Wait()
	close(statuses)
	success := 0
	for e := range statuses {
		if e == nil {
			success++
		} else if e != profiles.ErrHandleTaken {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatalf("unique winners: %d", success)
	}
	// Upgrading a version-1 database preserves saved problems and creates profiles.
	id := "11111111-1111-4111-8111-111111111111"
	if _, err = store.Save(ctx, "alice", id, 0, problems.Draft{Title: "before upgrade"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `DROP FUNCTION content_image_access(uuid,text,text,boolean); DROP TABLE content_images; DROP TABLE notifications; DROP FUNCTION notify_problem_activity() CASCADE; DROP FUNCTION notify_first_accept() CASCADE; DROP FUNCTION can_manage_problem(uuid,text); DROP TABLE problem_tester_invitations; DROP TABLE problem_difficulty_votes; DROP TABLE problem_favorites; DROP TABLE test_files; DROP TABLE submissions; DROP TABLE contest_problems; DROP TABLE contests; DROP TABLE problem_testers; DROP TABLE blog_posts; DROP TABLE user_profiles; ALTER TABLE problem_drafts DROP COLUMN published_draft, DROP COLUMN published_version, DROP COLUMN published_at; DELETE FROM schema_migrations WHERE version>=2`); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if value, e := store.Get(ctx, "alice", id); e != nil || value.Draft.Title != "before upgrade" {
		t.Fatal("migration lost draft")
	}

	// Backfill old sample names once, including the published snapshot, while
	// preserving explicit selections and every other test-case field.
	legacy := `{"testCases":[{"name":"sample_legacy","input":"keep","output":"output"},{"name":"normal"},{"name":"sample_off","isSample":false},{"name":"custom","isSample":true}]}`
	if _, err = store.Pool().Exec(ctx, `UPDATE problem_drafts SET draft=$2::jsonb,published_draft=$2::jsonb WHERE id=$1`, id, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `DELETE FROM schema_migrations WHERE version=11`); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var draftJSON, publishedJSON []byte
	if err = store.Pool().QueryRow(ctx, `SELECT draft,published_draft FROM problem_drafts WHERE id=$1`, id).Scan(&draftJSON, &publishedJSON); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{draftJSON, publishedJSON} {
		var draft problems.Draft
		if err = json.Unmarshal(data, &draft); err != nil {
			t.Fatal(err)
		}
		if len(draft.TestCases) != 4 || !draft.TestCases[0].IsSample || draft.TestCases[1].IsSample || draft.TestCases[2].IsSample || !draft.TestCases[3].IsSample || draft.TestCases[0].Input != "keep" || draft.TestCases[0].Output != "output" {
			t.Fatalf("incorrect migrated samples: %s", data)
		}
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAccountValidation(t *testing.T) {
	valid := profiles.Accounts{X: " @alice_123 ", AtCoder: " tourist ", Codeforces: "a.b-c_d", Yukicoder: "123"}
	if !cleanAccounts(&valid) || valid.X != "alice_123" || valid.AtCoder != "tourist" {
		t.Fatal(valid)
	}
	if !cleanAccounts(&profiles.Accounts{}) {
		t.Fatal("empty accounts rejected")
	}
	for _, value := range []profiles.Accounts{
		{X: "https://x.com/alice"}, {X: strings.Repeat("a", 16)},
		{AtCoder: "../admin"}, {AtCoder: strings.Repeat("a", 17)},
		{Codeforces: "foo;bar"}, {Codeforces: "ab"}, {Codeforces: strings.Repeat("a", 25)},
		{Yukicoder: "alice"}, {Yukicoder: "1/2"},
	} {
		if cleanAccounts(&value) {
			t.Fatalf("accepted %+v", value)
		}
	}
}
