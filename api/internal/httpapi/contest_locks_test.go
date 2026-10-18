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
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

const (
	lockProblem = "50000000-0000-4000-8000-000000000001"
	lockContest = "e0000000-0000-4000-8000-000000000001"
)

type contestLockTest struct {
	t       *testing.T
	store   *problems.Store
	handler http.Handler
	sign    signingFixture
}

// newContestLockTest sets up a scheduled contest by alice whose only problem she authors.
func newContestLockTest(t *testing.T) contestLockTest {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	schema := fmt.Sprintf("test_contest_locks_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	pool := store.Pool()
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES('alice','alice'),('bob','bob')`); err != nil {
		t.Fatal(err)
	}
	difficulty := 3
	draft := problems.Draft{Difficulty: &difficulty, Title: "Locked", Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
	if _, err = store.Save(ctx, "alice", lockProblem, 0, draft); err != nil {
		t.Fatal(err)
	}
	sign := newSigningFixture(t)
	f := contestLockTest{t: t, store: store, sign: sign}
	f.handler = newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: pool}, Images: &images.Store{Pool: pool}, Notifications: &notifications.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(sign.server.URL, "client")})
	penalty := 5
	start := time.Now().Add(time.Hour)
	f.request("PUT", "/my/contests/"+lockContest, "alice", contests.Input{Title: "Locks", StartsAt: start, EndsAt: start.Add(time.Hour), PenaltyMinutes: &penalty, Problems: []contests.Problem{{ID: lockProblem, Points: 100}}}, 200)
	return f
}

// newRequest builds the request in the test goroutine, so that it can be served from another one.
func (f contestLockTest) newRequest(method, path, owner string, body any) *http.Request {
	f.t.Helper()
	raw := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		raw = string(data)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+f.sign.token(f.t, owner, nil))
	return r
}

func (f contestLockTest) request(method, path, owner string, body any, want int) string {
	f.t.Helper()
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, f.newRequest(method, path, owner, body))
	if w.Code != want {
		f.t.Fatalf("%s %s (%s): got %d want %d: %s", method, path, owner, w.Code, want, w.Body.String())
	}
	return w.Body.String()
}

func (f contestLockTest) token(path string) string {
	f.t.Helper()
	var link struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(f.request("POST", path, "alice", nil, 200)), &link); err != nil {
		f.t.Fatal(err)
	}
	return link.Token
}

func (f contestLockTest) count(query string) int {
	f.t.Helper()
	var n int
	if err := f.store.Pool().QueryRow(context.Background(), query).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// waits holds the contest row with the given lock while another transaction writes the competing role, and checks
// that the request waits for that transaction and then refuses with the given error.
func (f contestLockTest) waits(lock, competing string, r *http.Request, wantError string) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := f.store.Pool().Begin(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM contests WHERE id=$1 FOR `+lock, lockContest); err != nil {
		f.t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, competing); err != nil {
		f.t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		done <- w
	}()
	select {
	case w := <-done:
		f.t.Fatalf("%s %s returned before the competing transaction ended: %d %s", r.Method, r.URL.Path, w.Code, w.Body.String())
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		f.t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), wantError) {
			f.t.Fatalf("%s %s after the competing transaction: got %d %s, want 409 %s", r.Method, r.URL.Path, w.Code, w.Body.String(), wantError)
		}
	case <-ctx.Done():
		f.t.Fatal("request did not finish after the competing transaction ended")
	}
}

// ADR 0022: a participant never becomes staff before the results are final, even when the two requests race.
func TestTesterAcceptanceWaitsForJoinPostgres(t *testing.T) {
	f := newContestLockTest(t)
	token := f.token("/my/problems/" + lockProblem + "/tester-invitation")
	f.waits("SHARE", `INSERT INTO contest_participants(contest_id,owner_id) VALUES('`+lockContest+`','bob')`,
		f.newRequest("POST", "/my/tester-invitations/"+token, "bob", nil), "contest_participant_locked")
	if n := f.count(`SELECT count(*) FROM problem_testers WHERE owner_id='bob'`); n != 0 {
		t.Fatal("participant became a tester", n)
	}
}

func TestJoinWaitsForTesterAcceptancePostgres(t *testing.T) {
	f := newContestLockTest(t)
	f.waits("NO KEY UPDATE", `INSERT INTO problem_testers(problem_id,owner_id) VALUES('`+lockProblem+`','bob')`,
		f.newRequest("POST", "/my/contests/"+lockContest+"/participation", "bob", nil), "contest_participation_unavailable")
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE owner_id='bob'`); n != 0 {
		t.Fatal("tester became a participant", n)
	}
}

func TestSetterAcceptanceWaitsForJoinPostgres(t *testing.T) {
	f := newContestLockTest(t)
	token := f.token("/my/contests/" + lockContest + "/setter-invitation")
	f.waits("SHARE", `INSERT INTO contest_participants(contest_id,owner_id) VALUES('`+lockContest+`','bob')`,
		f.newRequest("POST", "/my/setter-invitations/"+token, "bob", nil), "contest_participant_locked")
	if n := f.count(`SELECT count(*) FROM contest_setters WHERE owner_id='bob'`); n != 0 {
		t.Fatal("participant became a setter", n)
	}
}

func TestJoinWaitsForSetterAcceptancePostgres(t *testing.T) {
	f := newContestLockTest(t)
	f.waits("NO KEY UPDATE", `INSERT INTO contest_setters(contest_id,owner_id) VALUES('`+lockContest+`','bob')`,
		f.newRequest("POST", "/my/contests/"+lockContest+"/participation", "bob", nil), "contest_participation_unavailable")
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE owner_id='bob'`); n != 0 {
		t.Fatal("setter became a participant", n)
	}
}

// ADR 0023: a reveal waits for a rejudge that already holds the problem, then refuses while it is pending.
func TestRevealWaitsForRejudgePostgres(t *testing.T) {
	f := newContestLockTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool := f.store.Pool()
	const sid = "a0000000-0000-4000-8000-0000000000a1"
	for _, statement := range []string{
		`UPDATE contests SET freeze_minutes=10,starts_at=now()-interval '2 hours',ends_at=now()-interval '1 hour'`,
		`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at)
 VALUES('` + sid + `','bob','` + lockProblem + `',1,'Locked','cpp17','code','{}'::jsonb,'DONE','{"verdict":"AC"}'::jsonb,'` + lockContest + `',now()-interval '90 minutes')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	// Release the ended contest's problem first, so that only the reveal itself can wait for the problem row.
	f.request("GET", "/my/contests/"+lockContest, "alice", nil, 200)
	if n := f.count(`SELECT count(*) FROM contests WHERE released`); n != 1 {
		t.Fatal("contest not released", n)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM problem_drafts WHERE id=$1 FOR UPDATE`, lockProblem); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE submissions SET status='QUEUED',result=NULL WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	r := f.newRequest("POST", "/my/contests/"+lockContest+"/reveal", "alice", nil)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		done <- w
	}()
	select {
	case w := <-done:
		t.Fatalf("reveal returned before the rejudge ended: %d %s", w.Code, w.Body.String())
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "contest_results_pending") {
			t.Fatalf("reveal after the rejudge: %d %s", w.Code, w.Body.String())
		}
	case <-ctx.Done():
		t.Fatal("reveal did not finish after the rejudge ended")
	}
	if n := f.count(`SELECT count(*) FROM contests WHERE revealed_at IS NOT NULL`); n != 0 {
		t.Fatal("revealed with a pending rejudge", n)
	}
}
