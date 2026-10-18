package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
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

// ADR 0023: frozen submissions stay hidden until a setter or an operator reveals the results.
func TestContestFreezePostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_contest_freeze_%d", time.Now().UnixNano())
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
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('tess','tess'),('bob','bob'),('carol','carol'),('sam','sam'),('ops','ops'),('cos','cos'),('dan','dan')`)
	const (
		p1  = "11111111-1111-4111-8111-111111111111"
		p2  = "22222222-2222-4222-8222-222222222222"
		p3  = "33333333-3333-4333-8333-333333333333"
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		did = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	difficulty := 3
	draft := problems.Draft{Difficulty: &difficulty, Title: "Secret", Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}, {Input: "3", Output: "4"}}}
	for _, id := range []string{p1, p2} {
		if _, err = store.Save(ctx, "alice", id, 0, draft); err != nil {
			t.Fatal(err)
		}
	}
	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Operators: map[string]bool{"ops": true}, Verifier: newCognitoVerifier(f.server.URL, "client")})
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
	decode := func(body string, v any) {
		t.Helper()
		if err := json.Unmarshal([]byte(body), v); err != nil {
			t.Fatal(err)
		}
	}
	view := func(owner string) contests.Contest {
		t.Helper()
		path := "/contests/" + cid
		if owner != "" {
			path = "/my/contests/" + cid
		}
		var c contests.Contest
		decode(request("GET", path, owner, nil, 200), &c)
		return c
	}
	type cell struct{ points, wrong, pending, frozen int }
	standings := func(path, owner string) map[string]map[string]cell {
		t.Helper()
		var rows []contests.Standing
		decode(request("GET", path, owner, nil, 200), &rows)
		table := map[string]map[string]cell{}
		for _, row := range rows {
			table[row.Handle] = map[string]cell{"total": {points: row.Points}}
			for id, s := range row.Problems {
				table[row.Handle][id] = cell{s.Points, s.Wrong, s.Pending, s.Frozen}
			}
		}
		return table
	}
	ids := func(path, owner string) []string {
		t.Helper()
		var page submissions.ContestSubmissionList
		decode(request("GET", path, owner, nil, 200), &page)
		list := []string{}
		for _, item := range page.Items {
			list = append(list, item.ID)
		}
		slices.Sort(list)
		return list
	}

	// Settings: the freeze is null or 1..duration minutes, for drafts and published contests alike.
	penalty, zero, long, freeze := 5, 0, 61, 30
	start := time.Now().Add(time.Hour)
	in := contests.Input{Title: "Frozen", StartsAt: start, EndsAt: start.Add(time.Hour), PenaltyMinutes: &penalty, Problems: []contests.Problem{{ID: p1, Points: 100}, {ID: p2, Points: 200}}}
	for _, bad := range []*int{&zero, &long} {
		in.FreezeMinutes = bad
		request("PUT", "/my/contests/"+cid, "alice", in, 400)
	}
	duration := 60
	var saved contests.DraftResult
	decode(request("PUT", "/my/contests/"+did+"/draft", "alice", map[string]any{"version": 0, "draft": contests.Draft{Title: "Draft", StartsAt: &start, DurationMinutes: &duration, PenaltyMinutes: &penalty, FreezeMinutes: &long, Problems: []contests.DraftProblem{{ID: p1, Points: &duration}}}}, 200), &saved)
	if !slices.Contains(saved.Issues, contests.IssueFreeze) || saved.Draft.FreezeMinutes == nil || *saved.Draft.FreezeMinutes != 61 {
		t.Fatalf("draft issues: %+v", saved)
	}
	in.FreezeMinutes = &freeze
	request("PUT", "/my/contests/"+cid, "alice", in, 200)
	c := view("")
	if c.FreezeMinutes == nil || *c.FreezeMinutes != 30 || c.FreezeAt == nil || !c.FreezeAt.Equal(c.EndsAt.Add(-30*time.Minute)) || c.Revealed || c.RevealedAt != nil || c.ResultsFinal || c.CanRevealResults || c.CanViewLiveStandings {
		t.Fatalf("public view: %+v", c)
	}
	var link struct {
		Token string `json:"token"`
	}
	decode(request("POST", "/my/problems/"+p1+"/tester-invitation", "alice", nil, 200), &link)
	request("POST", "/my/tester-invitations/"+link.Token, "tess", nil, 200)
	request("POST", "/my/contests/"+cid+"/participation", "bob", nil, 200)
	request("POST", "/my/contests/"+cid+"/participation", "carol", nil, 200)
	if tester := view("tess"); !tester.CanViewLiveStandings || tester.CanRevealResults {
		t.Fatalf("tester view: %+v", tester)
	}

	// The contest runs for two hours and ends in ten minutes; the last 30 minutes are frozen.
	end := time.Now().Add(10 * time.Minute).Truncate(time.Millisecond)
	exec(`UPDATE contests SET starts_at=$2,ends_at=$3 WHERE id=$1`, cid, end.Add(-2*time.Hour), end)
	exec(`UPDATE problem_testers SET created_at=$1`, end.Add(-3*time.Hour))
	const (
		bobWrong   = "a0000000-0000-4000-8000-000000000001"
		bobAC      = "a0000000-0000-4000-8000-000000000002"
		bobLate    = "a0000000-0000-4000-8000-000000000003" // frozen, after an accepted pre-freeze run
		bobCE      = "a0000000-0000-4000-8000-000000000004" // frozen compile error
		carolSlow  = "a0000000-0000-4000-8000-000000000005" // pre-freeze, judged after the freeze
		carolAC    = "a0000000-0000-4000-8000-000000000006" // frozen
		carolQueue = "a0000000-0000-4000-8000-000000000007" // frozen, still queued
	)
	insert := func(id, owner, problem, status, verdict string, created, finished time.Duration) {
		t.Helper()
		var result any
		if verdict != "" {
			result = fmt.Sprintf(`{"verdict":%q,"passed":1,"total":2}`, verdict)
		}
		exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at,finished_at)
 VALUES($1,$2,$3,1,'Secret','cpp17','code','{}'::jsonb,$4,$5::jsonb,$6,$7,$8)`, id, owner, problem, status, result, cid, end.Add(created), end.Add(finished))
	}
	insert(bobWrong, "bob", p1, "DONE", "WA", -100*time.Minute, -99*time.Minute)
	insert(bobAC, "bob", p1, "DONE", "AC", -90*time.Minute, -89*time.Minute)
	insert(bobLate, "bob", p1, "DONE", "WA", -20*time.Minute, -19*time.Minute)
	insert(bobCE, "bob", p2, "DONE", "CE", -15*time.Minute, -14*time.Minute)
	insert(carolSlow, "carol", p2, "DONE", "AC", -31*time.Minute, -5*time.Minute)
	insert(carolAC, "carol", p1, "DONE", "AC", -10*time.Minute, -9*time.Minute)
	insert(carolQueue, "carol", p1, "QUEUED", "", -5*time.Minute, -5*time.Minute)

	frozen := map[string]map[string]cell{
		"bob":   {"total": {points: 100}, p1: {points: 100, wrong: 1}, p2: {frozen: 1}},
		"carol": {"total": {points: 200}, p1: {frozen: 2}, p2: {points: 200}},
	}
	live := map[string]map[string]cell{
		"bob":   {"total": {points: 100}, p1: {points: 100, wrong: 1}, p2: {}},
		"carol": {"total": {points: 300}, p1: {points: 100}, p2: {points: 200}},
	}
	checkTable := func(label string, got, want map[string]map[string]cell) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s standings:\n got %v\nwant %v", label, got, want)
		}
	}
	// The public route and every non-staff viewer get the frozen table; staff may ask for the live one.
	checkTable("public", standings("/contests/"+cid+"/standings?live=1", ""), frozen)
	checkTable("participant", standings("/my/contests/"+cid+"/standings?live=1", "bob"), frozen)
	checkTable("stranger", standings("/my/contests/"+cid+"/standings?live=1", "sam"), frozen)
	checkTable("staff default", standings("/my/contests/"+cid+"/standings", "alice"), frozen)
	checkTable("setter live", standings("/my/contests/"+cid+"/standings?live=1", "alice"), live)
	checkTable("tester live", standings("/my/contests/"+cid+"/standings?live=1", "tess"), live)
	request("GET", "/contests/"+cid+"/submissions", "", nil, 404)
	if body := request("POST", "/my/contests/"+cid+"/reveal", "alice", nil, 409); !strings.Contains(body, "contest_conflict") {
		t.Fatal(body)
	}

	// After the end, pre-freeze submissions are public; frozen ones only to their submitter and staff.
	shift := func(d time.Duration) {
		t.Helper()
		exec(`UPDATE contests SET starts_at=starts_at+$2::interval,ends_at=ends_at+$2::interval WHERE id=$1`, cid, fmt.Sprintf("%d milliseconds", d.Milliseconds()))
		exec(`UPDATE submissions SET created_at=created_at+$2::interval,finished_at=finished_at+$2::interval WHERE contest_id=$1`, cid, fmt.Sprintf("%d milliseconds", d.Milliseconds()))
		end = end.Add(d)
	}
	shift(-20 * time.Minute)
	if c := view(""); c.Status != "ended" || !c.CanViewSubmissions || c.ResultsFinal || c.Revealed || c.CanRevealResults {
		t.Fatalf("ended public view: %+v", c)
	}
	if c := view("alice"); !c.CanRevealResults || !c.CanViewLiveStandings {
		t.Fatalf("ended setter view: %+v", c)
	}
	if c := view("ops"); !c.CanRevealResults || c.CanViewLiveStandings {
		t.Fatalf("ended operator view: %+v", c)
	}
	for _, viewer := range []string{"bob", "tess", "sam"} {
		if c := view(viewer); c.CanRevealResults {
			t.Fatalf("%s may reveal: %+v", viewer, c)
		}
	}
	public := []string{bobWrong, bobAC, carolSlow}
	all := []string{bobWrong, bobAC, bobLate, bobCE, carolSlow, carolAC, carolQueue}
	for path, want := range map[string][]string{
		"/contests/" + cid + "/submissions":                     public,
		"/contests/" + cid + "/problems/" + p1 + "/submissions": {bobWrong, bobAC},
		"/problems/" + p1 + "/submissions":                      {bobWrong, bobAC},
	} {
		if got := ids(path, ""); !slices.Equal(got, want) {
			t.Fatalf("%s: got %v want %v", path, got, want)
		}
	}
	if got := ids("/my/contests/"+cid+"/submissions", "sam"); !slices.Equal(got, public) {
		t.Fatalf("stranger list: %v", got)
	}
	if got := ids("/my/contests/"+cid+"/submissions", "carol"); !slices.Equal(got, []string{bobWrong, bobAC, carolSlow, carolAC, carolQueue}) {
		t.Fatalf("submitter list: %v", got)
	}
	if got := ids("/my/problems/"+p1+"/submissions", "bob"); !slices.Equal(got, []string{bobWrong, bobAC, bobLate}) {
		t.Fatalf("submitter problem list: %v", got)
	}
	if got := ids("/my/contests/"+cid+"/submissions", "tess"); !slices.Equal(got, all) {
		t.Fatalf("staff list: %v", got)
	}
	for _, path := range []string{"/contests/" + cid + "/submissions/" + carolAC, "/submissions/" + carolAC, "/problems/" + p1 + "/submissions/" + carolAC} {
		request("GET", path, "", nil, 404)
	}
	request("GET", "/my/contests/"+cid+"/submissions/"+carolAC, "bob", nil, 404)
	request("GET", "/my/submissions/"+carolAC+"/public", "bob", nil, 404)
	request("GET", "/my/contests/"+cid+"/submissions/"+carolAC, "carol", nil, 200)
	request("GET", "/my/contests/"+cid+"/submissions/"+carolAC, "tess", nil, 200)
	request("GET", "/submissions/"+carolSlow, "", nil, 200)
	solvers := func() int64 {
		t.Helper()
		var p problems.PublicProblem
		decode(request("GET", "/problems/"+p1, "", nil, 200), &p)
		return p.SolverCount
	}
	if n := solvers(); n != 1 {
		t.Fatalf("solvers before the reveal: %d", n)
	}
	// The submitter's own solved marks do not wait for the reveal.
	if body := request("GET", "/my/solved-problems", "carol", nil, 200); !strings.Contains(body, p1) || !strings.Contains(body, p2) {
		t.Fatalf("carol solved: %s", body)
	}
	checkTable("ended public", standings("/contests/"+cid+"/standings", ""), frozen)
	checkTable("ended setter live", standings("/my/contests/"+cid+"/standings?live=1", "alice"), live)

	// Participants stay out of the staff until the reveal, so they cannot see the live table through a tester link.
	if body := request("POST", "/my/tester-invitations/"+link.Token, "bob", nil, 409); !strings.Contains(body, "contest_participant_locked") {
		t.Fatal(body)
	}
	// The last setter of an unrevealed frozen contest cannot delete the account.
	deleteAccount := func(owner string) error {
		return (&accounts.Store{Pool: store.Pool()}).Delete(ctx, owner, owner, func(context.Context) error { return nil })
	}
	if err := deleteAccount("alice"); !errors.Is(err, accounts.ErrActiveContest) {
		t.Fatalf("delete before the reveal: %v", err)
	}
	// Until the reveal the contest still takes co-setters, who derive rights over its problems.
	var invite struct {
		Token string `json:"token"`
	}
	decode(request("POST", "/my/contests/"+cid+"/setter-invitation", "alice", nil, 200), &invite)
	request("POST", "/my/setter-invitations/"+invite.Token, "cos", nil, 200)
	manages := func(owner string) bool {
		t.Helper()
		var ok bool
		if err := store.Pool().QueryRow(ctx, `SELECT can_manage_problem($1,$2)`, p1, owner).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !manages("cos") {
		t.Fatal("co-setter lost rights before the reveal")
	}

	// Only setters and operators reveal, and not while a contest submission waits for a verdict.
	request("POST", "/my/contests/"+cid+"/reveal", "tess", nil, 404)
	request("POST", "/my/contests/"+cid+"/reveal", "bob", nil, 404)
	request("POST", "/my/contests/"+cid+"/reveal", "", nil, 401)
	if body := request("POST", "/my/contests/"+cid+"/reveal", "ops", nil, 409); !strings.Contains(body, "contest_results_pending") {
		t.Fatal(body)
	}
	exec(`UPDATE submissions SET status='RUNNING' WHERE id=$1`, carolQueue)
	request("POST", "/my/contests/"+cid+"/reveal", "alice", nil, 409)
	exec(`UPDATE submissions SET status='DONE',result='{"verdict":"WA","passed":0,"total":2}' WHERE id=$1`, carolQueue)
	var revealed contests.Contest
	decode(request("POST", "/my/contests/"+cid+"/reveal", "ops", nil, 200), &revealed)
	if !revealed.Revealed || revealed.RevealedAt == nil || !revealed.ResultsFinal || revealed.CanRevealResults {
		t.Fatalf("revealed: %+v", revealed)
	}
	var again contests.Contest
	decode(request("POST", "/my/contests/"+cid+"/reveal", "alice", nil, 200), &again)
	if again.RevealedAt == nil || !again.RevealedAt.Equal(*revealed.RevealedAt) {
		t.Fatalf("reveal again: %+v", again)
	}

	// After the reveal everything is public, and the public table counts every submission.
	if got := ids("/contests/"+cid+"/submissions", ""); !slices.Equal(got, all) {
		t.Fatalf("revealed list: %v", got)
	}
	request("GET", "/submissions/"+carolAC, "", nil, 200)
	request("GET", "/my/submissions/"+carolAC+"/public", "bob", nil, 200)
	if n := solvers(); n != 2 {
		t.Fatalf("solvers after the reveal: %d", n)
	}
	final := map[string]map[string]cell{
		"bob":   {"total": {points: 100}, p1: {points: 100, wrong: 1}, p2: {}},
		"carol": {"total": {points: 300}, p1: {points: 100}, p2: {points: 200}},
	}
	checkTable("revealed public", standings("/contests/"+cid+"/standings", ""), final)
	// A participant who tests only after the reveal keeps the row.
	request("POST", "/my/tester-invitations/"+link.Token, "bob", nil, 200)
	checkTable("revealed public with a late tester", standings("/contests/"+cid+"/standings", ""), final)
	if c := view("alice"); c.CanRevealResults || !c.ResultsFinal {
		t.Fatalf("revealed setter view: %+v", c)
	}
	// The reveal ends co-setter rights and invitations, and lets the last setter leave.
	if manages("cos") {
		t.Fatal("co-setter keeps rights after the reveal")
	}
	if body := request("POST", "/my/setter-invitations/"+invite.Token, "dan", nil, 409); !strings.Contains(body, "contest_invitation_closed") {
		t.Fatal(body)
	}
	// Without a freeze the results are final at the end and there is nothing to reveal.
	const plain = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	if _, err = store.Save(ctx, "alice", p3, 0, draft); err != nil {
		t.Fatal(err)
	}
	request("PUT", "/my/contests/"+plain, "alice", contests.Input{Title: "Plain", StartsAt: start, EndsAt: start.Add(time.Hour), PenaltyMinutes: &penalty, Problems: []contests.Problem{{ID: p3, Points: 100}}}, 200)
	exec(`UPDATE contests SET starts_at=$2,ends_at=$3 WHERE id=$1`, plain, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	var ended contests.Contest
	decode(request("GET", "/my/contests/"+plain, "alice", nil, 200), &ended)
	if ended.FreezeMinutes != nil || ended.FreezeAt != nil || !ended.ResultsFinal || ended.Revealed || ended.CanRevealResults || ended.CanViewLiveStandings {
		t.Fatalf("contest without a freeze: %+v", ended)
	}
	if body := request("POST", "/my/contests/"+plain+"/reveal", "ops", nil, 409); !strings.Contains(body, "contest_conflict") {
		t.Fatal(body)
	}
	if err := deleteAccount("alice"); err != nil {
		t.Fatalf("delete after the reveal: %v", err)
	}
}
