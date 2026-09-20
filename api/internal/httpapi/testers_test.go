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

func TestTesterInvitationsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_testers_%d", time.Now().UnixNano())
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

	path := "/my/problems/" + a
	request("POST", path+"/tester-invitation", "", nil, 401)
	request("POST", path+"/tester-invitation", "bob", nil, 404)
	var link struct {
		Token string `json:"token"`
	}
	if err = json.Unmarshal([]byte(request("POST", path+"/tester-invitation", "alice", nil, 200)), &link); err != nil {
		t.Fatal(err)
	}
	if !testerToken.MatchString(link.Token) {
		t.Fatalf("invalid token %q", link.Token)
	}
	if repeated := request("POST", path+"/tester-invitation", "alice", nil, 200); !strings.Contains(repeated, link.Token) {
		t.Fatal("link changed on retry")
	}
	invite := "/my/tester-invitations/" + link.Token
	request("GET", invite, "", nil, 401)
	request("POST", invite, "", nil, 401)
	request("GET", "/my/tester-invitations/invalid", "bob", nil, 404)
	request("POST", "/my/tester-invitations/"+strings.Repeat("A", 32), "bob", nil, 404)
	preview := request("GET", invite, "bob", nil, 200)
	if !strings.Contains(preview, `"joined":false`) || strings.Contains(preview, "secret statement") {
		t.Fatalf("invalid preview %s", preview)
	}
	request("GET", path, "bob", nil, 404)
	request("PUT", path, "bob", map[string]any{"version": 1, "draft": draft}, 404)
	request("GET", path+"/submissions", "bob", nil, 404)
	request("POST", invite, "bob", nil, 200)
	request("POST", invite, "bob", nil, 200)
	request("POST", invite, "alice", nil, 200) // The author does not become a tester.
	var count int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM problem_testers WHERE problem_id=$1`, a).Scan(&count); err != nil || count != 1 {
		t.Fatalf("memberships %d: %v", count, err)
	}
	if own := request("GET", "/my/problems", "bob", nil, 200); strings.Contains(own, a) {
		t.Fatal("tested problem in owned list")
	}
	if tested := request("GET", "/my/problems?role=tester", "bob", nil, 200); !strings.Contains(tested, a) || strings.Contains(tested, b) {
		t.Fatal("incorrect tested list: " + tested)
	}
	if tested := request("GET", "/my/problems?role=tester", "alice", nil, 200); strings.Contains(tested, a) {
		t.Fatal("author in tested list")
	}
	request("GET", "/my/problems?role=invalid", "bob", nil, 400)
	got := request("GET", path, "bob", nil, 200)
	if !strings.Contains(got, `"testers":["bob"]`) {
		t.Fatal("missing tester attribution: " + got)
	}
	if !strings.Contains(got, `"author":"alice"`) {
		t.Fatal("author attribution changed: " + got)
	}
	request("GET", "/my/problems/"+b, "bob", nil, 404)
	request("POST", path+"/tester-invitation", "bob", nil, 200)
	request("PUT", path, "bob", map[string]any{"version": 1, "draft": draft}, 200)
	request("PUT", path, "alice", map[string]any{"version": 1, "draft": draft}, 409)
	request("POST", "/my/submissions", "bob", map[string]any{"problemId": a, "source": "int main() {}", "runtime": "cpp17"}, 202)
	request("POST", "/my/submissions", "alice", map[string]any{"problemId": a, "source": "int main() {}", "runtime": "cpp17"}, 202)
	listing := request("GET", path+"/submissions", "bob", nil, 200)
	if !strings.Contains(listing, `"author":"alice"`) || !strings.Contains(listing, `"author":"bob"`) {
		t.Fatal("tester cannot see shared submissions: " + listing)
	}
	mine := request("GET", path+"/submissions?mine=1", "bob", nil, 200)
	if strings.Contains(mine, `"author":"alice"`) {
		t.Fatal("mine includes author's submission")
	}
	if _, err = queue.CreateGeneration(ctx, submissions.GenerationInput{RunInput: submissions.RunInput{Owner: "bob", ID: newSubmissionID(), ProblemID: a, Source: "int main() {}", Runtime: "cpp17-local"}, ProblemVersion: 2, Job: submissions.Job{Generate: true}}); err != nil {
		t.Fatal("tester generation: ", err)
	}
	request("PUT", path+"/publication", "bob", map[string]any{"version": 2, "publish": true}, 200)
	public := request("GET", "/problems/"+a, "", nil, 200)
	if !strings.Contains(public, `"testers":["bob"]`) {
		t.Fatal("missing public tester attribution: " + public)
	}
	if !strings.Contains(public, `"author":"alice"`) {
		t.Fatal("publication changed author")
	}
	request("PUT", path+"/publication", "bob", map[string]any{"version": 3, "publish": false}, 200)
	// Both parties can save each other's immutable files, but never another problem's files.
	file := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	exec(`INSERT INTO test_files(id,owner_id,problem_id,object_key,version_id,sha256,size,ready) VALUES($1,'alice',$2,'tests/shared','1',$3,1,true)`, file, a, strings.Repeat("a", 64))
	draft.TestCases = []problems.TestCase{{InputFile: &problems.TestFile{ID: file, Size: 1, SHA256: strings.Repeat("a", 64)}, Output: "2"}}
	request("PUT", path, "bob", map[string]any{"version": 4, "draft": draft}, 200)
	exec(`UPDATE test_files SET owner_id='bob' WHERE id=$1`, file)
	request("PUT", path, "alice", map[string]any{"version": 5, "draft": draft}, 200)
	request("PUT", "/my/problems/"+b, "alice", map[string]any{"version": 1, "draft": draft}, 400)
	request("DELETE", path+"?version=6", "carol", nil, 404)
	request("DELETE", path+"?version=6", "bob", nil, 204)
	request("GET", invite, "bob", nil, 404)
	request("GET", path, "alice", nil, 404)
	if tested := request("GET", "/my/problems?role=tester", "bob", nil, 200); strings.Contains(tested, a) {
		t.Fatal("deleted problem in tested list")
	}
}
