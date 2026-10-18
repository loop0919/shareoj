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

	"judge/api/internal/accounts"
	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

func raceStore(t *testing.T, ctx context.Context, name string) *problems.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_%s_%d", name, time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
		_ = conn.Close(ctx)
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
	return store
}

// Writes that a setter's removal or a publication must see wait for them and then judge again (ADR 0022).
func TestContestSetterRacesPostgres(t *testing.T) {
	ctx := context.Background()
	store := raceStore(t, ctx, "contest_setter_races")
	pool := store.Pool()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, handle := range []string{"olive", "cora", "mal", "pat"} {
		exec(`INSERT INTO user_profiles(owner_id,handle) VALUES($1,$1)`, handle)
	}
	const (
		pO  = "20000000-0000-4000-8000-000000000001" // by olive
		cid = "e0000000-0000-4000-8000-000000000001"
		nid = "e0000000-0000-4000-8000-000000000002"
	)
	difficulty := 3
	draft := problems.Draft{Difficulty: &difficulty, Title: "Olive", Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
	if _, err := store.Save(ctx, "olive", pO, 0, draft); err != nil {
		t.Fatal(err)
	}
	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: pool}, Images: &images.Store{Pool: pool}, Notifications: &notifications.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(f.server.URL, "client")})
	// send may run off the test goroutine, so it reports instead of failing.
	send := func(method, path, owner string, body any) (int, string) {
		raw := ""
		if body != nil {
			data, _ := json.Marshal(body)
			raw = string(data)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if owner != "" {
			r.Header.Set("Authorization", "Bearer "+f.token(t, owner, nil))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	request := func(method, path, owner string, body any, want int) string {
		t.Helper()
		code, text := send(method, path, owner, body)
		if code != want {
			t.Fatalf("%s %s (%s): got %d want %d: %s", method, path, owner, code, want, text)
		}
		return text
	}
	decode := func(body string, value any) {
		t.Helper()
		if err := json.Unmarshal([]byte(body), value); err != nil {
			t.Fatal(err, body)
		}
	}
	// waits runs a request while another transaction holds a lock, checks that it waits, and returns its result
	// after release commits that transaction.
	type result struct {
		code int
		body string
	}
	waits := func(release func(), method, path, owner string, body any) result {
		t.Helper()
		done := make(chan result, 1)
		go func() {
			code, text := send(method, path, owner, body)
			done <- result{code, text}
		}()
		select {
		case r := <-done:
			t.Fatalf("%s %s did not wait: %d %s", method, path, r.code, r.body)
		case <-time.After(150 * time.Millisecond):
		}
		release()
		select {
		case r := <-done:
			return r
		case <-time.After(10 * time.Second):
			t.Fatalf("%s %s still waits", method, path)
		}
		return result{}
	}
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	draftBody := map[string]any{"title": "Race", "description": "", "startsAt": start, "durationMinutes": 60, "penaltyMinutes": 5, "problems": []map[string]any{{"id": pO, "points": 100}}}
	request("PUT", "/my/contests/"+cid+"/draft", "olive", map[string]any{"version": 0, "draft": draftBody}, 200)
	var link struct {
		Token string `json:"token"`
	}
	decode(request("POST", "/my/contests/"+cid+"/setter-invitation", "olive", nil, 200), &link)
	invite := "/my/setter-invitations/" + link.Token
	request("POST", invite, "cora", nil, 200)

	// removing holds the lock that removal holds and takes cora off inside it, as RemoveSetter does.
	removing := func(table string) pgx.Tx {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback(ctx) })
		lock := " FOR UPDATE"
		if table == "contests" {
			lock = " FOR NO KEY UPDATE"
		}
		for _, statement := range []string{`SELECT id FROM ` + table + ` WHERE id=$1` + lock,
			`UPDATE contest_setters SET left_at=clock_timestamp() WHERE contest_id=$1 AND owner_id='cora'`,
			`UPDATE contest_setter_invitations SET token=translate(upper(md5(random()::text)),'0189','QRST') WHERE contest_id=$1`} {
			if _, err = tx.Exec(ctx, statement, cid); err != nil {
				t.Fatal(err)
			}
		}
		return tx
	}
	commit := func(tx pgx.Tx) func() {
		return func() {
			if err := tx.Commit(ctx); err != nil {
				t.Error(err)
			}
		}
	}
	rejoin := func() {
		t.Helper()
		exec(`UPDATE contest_setters SET left_at=NULL WHERE contest_id=$1 AND owner_id='cora'`, cid)
		decode(request("POST", "/my/contests/"+cid+"/setter-invitation", "olive", nil, 200), &link)
		invite = "/my/setter-invitations/" + link.Token
	}

	// A co-setter whose save, publication or link request waits on their removal is refused once it commits.
	if r := waits(commit(removing("contest_drafts")), "PUT", "/my/contests/"+cid+"/draft", "cora", map[string]any{"version": 1, "draft": draftBody}); r.code != 404 {
		t.Fatalf("save after removal: %d %s", r.code, r.body)
	}
	rejoin()
	if r := waits(commit(removing("contest_drafts")), "PUT", "/my/contests/"+cid+"/publication", "cora", map[string]any{"version": 1}); r.code != 404 {
		t.Fatalf("publication after removal: %d %s", r.code, r.body)
	}
	rejoin()
	for _, method := range []string{"POST", "PUT"} {
		if r := waits(commit(removing("contest_drafts")), method, "/my/contests/"+cid+"/setter-invitation", "cora", nil); r.code != 404 || strings.Contains(r.body, "token") {
			t.Fatalf("%s link after removal: %d %s", method, r.code, r.body)
		}
		rejoin()
	}
	request("PUT", "/my/contests/"+cid+"/publication", "olive", map[string]any{"version": 1}, 200)
	if r := waits(commit(removing("contests")), "POST", "/my/contests/"+cid+"/setter-invitation", "cora", nil); r.code != 404 {
		t.Fatalf("contest link after removal: %d %s", r.code, r.body)
	}
	rejoin()

	// A draft created while a contest with its id is being published waits for it and is refused.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text,709091022))`, nid); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,penalty_minutes) VALUES($1,'olive','Other','',$2,$3,5)`, nid, start, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	empty := map[string]any{"title": "Mine", "description": "", "problems": []map[string]any{}}
	if r := waits(commit(tx), "PUT", "/my/contests/"+nid+"/draft", "mal", map[string]any{"version": 0, "draft": empty}); r.code != 409 {
		t.Fatalf("draft beside a new contest: %d %s", r.code, r.body)
	}

	// A stray draft under a contest's id, as the race left before, makes nobody a setter and clears nothing.
	exec(`INSERT INTO contest_drafts(id,owner_id,draft) VALUES($1,'mal','{"title":"Stray"}')`, cid)
	var mine contests.Contest
	decode(request("GET", "/my/contests/"+cid, "mal", nil, 200), &mine)
	if mine.IsSetter || mine.CanEdit || mine.Staff || !mine.Official {
		t.Fatalf("stray draft owner: %+v", mine)
	}
	request("POST", "/my/contests/"+cid+"/setter-invitation", "mal", nil, 404)
	request("GET", "/my/contests/"+cid+"/draft", "mal", nil, 404)
	request("DELETE", "/my/contests/"+cid+"/draft?version=1", "mal", nil, 204)
	var kept int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM contest_setters WHERE contest_id=$1 AND owner_id='cora' AND left_at IS NULL`, cid).Scan(&kept); err != nil || kept != 1 {
		t.Fatal("co-setter after stray draft deletion", kept, err)
	}
	request("GET", invite, "pat", nil, 200)

	// Another setter's problem names its author in the testing list.
	var page struct {
		Items []problems.Summary `json:"items"`
	}
	decode(request("GET", "/my/problems?role=tester", "cora", nil, 200), &page)
	if len(page.Items) != 1 || page.Items[0].ViaContest == nil || *page.Items[0].ViaContest != (problems.ContestRef{ID: cid, Title: "Race", Author: "olive"}) {
		t.Fatalf("testing list: %+v", page.Items)
	}

	// A former setter is told apart from a signed-out viewer, who is never excluded.
	request("DELETE", "/my/contests/"+cid+"/setters/cora", "cora", nil, 204)
	var view contests.Contest
	decode(request("GET", "/my/contests/"+cid, "cora", nil, 200), &view)
	if !view.Excluded || view.Staff || view.Official {
		t.Fatalf("former setter: %+v", view)
	}
	for _, viewer := range []string{"pat", "olive", ""} {
		path := "/my/contests/" + cid
		if viewer == "" {
			path = "/contests/" + cid
		}
		view = contests.Contest{}
		decode(request("GET", path, viewer, nil, 200), &view)
		if view.Excluded {
			t.Fatalf("%q excluded: %+v", viewer, view)
		}
	}

	// Once the results are final the link is neither handed out nor accepted, and its preview says so.
	exec(`UPDATE contests SET starts_at=now()-interval '2 hours',ends_at=now()-interval '1 hour' WHERE id=$1`, cid)
	for _, method := range []string{"POST", "PUT"} {
		if body := request(method, "/my/contests/"+cid+"/setter-invitation", "olive", nil, 409); !strings.Contains(body, "contest_invitation_closed") {
			t.Fatal(body)
		}
	}
	request("POST", "/my/contests/"+cid+"/setter-invitation", "pat", nil, 404)
	var preview contests.SetterInvitation
	decode(request("GET", invite, "pat", nil, 200), &preview)
	if !preview.Closed || !preview.Published || preview.Joined {
		t.Fatalf("closed preview: %+v", preview)
	}
}

// Two owners who co-set each other's drafts both delete their accounts at once; the deadlock is retried.
func TestMutualCoSetterDeletionPostgres(t *testing.T) {
	ctx := context.Background()
	store := raceStore(t, ctx, "mutual_deletion")
	pool := store.Pool()
	const (
		da = "e1000000-0000-4000-8000-000000000001"
		db = "e1000000-0000-4000-8000-000000000002"
	)
	for _, statement := range []string{
		`INSERT INTO user_profiles(owner_id,handle) VALUES('ada','ada'),('bob','bob')`,
		`INSERT INTO contest_drafts(id,owner_id,draft) VALUES('` + da + `','ada','{}'),('` + db + `','bob','{}')`,
		`INSERT INTO contest_setters(contest_id,owner_id) VALUES('` + da + `','bob'),('` + db + `','ada')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	// Holding both drafts lets each deletion leave the other's draft before either hands its own over.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err = tx.Exec(ctx, `SELECT id FROM contest_drafts WHERE id IN ($1,$2) FOR UPDATE`, da, db); err != nil {
		t.Fatal(err)
	}
	deletions := &accounts.Store{Pool: pool}
	done := make(chan error, 2)
	for _, owner := range []string{"ada", "bob"} {
		go func() {
			done <- deletions.Delete(ctx, owner, owner, func(context.Context) error { return nil })
		}()
	}
	time.Sleep(300 * time.Millisecond)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal("deletion:", err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("deletion still waits")
		}
	}
	var gone int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM user_profiles WHERE deleted_at IS NOT NULL`).Scan(&gone); err != nil || gone != 2 {
		t.Fatal("deleted accounts", gone, err)
	}
}
