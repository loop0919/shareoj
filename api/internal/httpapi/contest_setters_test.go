package httpapi

import (
	"context"
	"encoding/base64"
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

// ADR 0022: co-setters join through the contest's link and work on it like the owner.
func TestContestSettersPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_contest_setters_%d", time.Now().UnixNano())
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
	pool := store.Pool()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if e := pool.QueryRow(ctx, query, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	people := []string{"olive", "cora", "carl", "rex", "sam", "pat", "ann", "late", "dan", "eve"}
	for _, handle := range people {
		exec(`INSERT INTO user_profiles(owner_id,handle) VALUES($1,$1)`, handle)
	}
	const (
		pO  = "10000000-0000-4000-8000-000000000001" // by olive
		pC  = "10000000-0000-4000-8000-000000000002" // by cora
		pA  = "10000000-0000-4000-8000-000000000003" // by ann, tested by cora
		pY  = "10000000-0000-4000-8000-000000000004" // by ann, tested by carl
		pX  = "10000000-0000-4000-8000-000000000005" // by sam
		pL  = "10000000-0000-4000-8000-000000000007" // by ann, tested by olive
		pK2 = "10000000-0000-4000-8000-000000000008" // by olive
		pZ  = "10000000-0000-4000-8000-000000000009" // never created
		pO2 = "10000000-0000-4000-8000-00000000000a" // by olive, outside any contest at first
		pD  = "10000000-0000-4000-8000-00000000000b" // by dan
		cid = "d0000000-0000-4000-8000-000000000001"
		d2  = "d0000000-0000-4000-8000-000000000002"
		hid = "d0000000-0000-4000-8000-000000000003"
		kid = "d0000000-0000-4000-8000-000000000004"
		d3  = "d0000000-0000-4000-8000-000000000005"
		d4  = "d0000000-0000-4000-8000-000000000006"
		k2  = "d0000000-0000-4000-8000-000000000007"
	)
	difficulty := 3
	for id, owner := range map[string]string{pO: "olive", pC: "cora", pA: "ann", pY: "ann", pX: "sam", pL: "ann", pK2: "olive", pO2: "olive", pD: "dan"} {
		draft := problems.Draft{Difficulty: &difficulty, Title: "Title of " + id[len(id)-2:], Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
		if _, err = store.Save(ctx, owner, id, 0, draft); err != nil {
			t.Fatal(err)
		}
	}
	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: pool}, Images: &images.Store{Pool: pool}, Notifications: &notifications.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(f.server.URL, "client")})
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
			t.Fatalf("%s %s (%s): got %d want %d: %s", method, path, owner, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable response", method, path)
		}
		return w.Body.String()
	}
	decode := func(body string, value any) {
		t.Helper()
		if err := json.Unmarshal([]byte(body), value); err != nil {
			t.Fatal(err, body)
		}
	}
	link := func(id, actor, method string) string {
		t.Helper()
		var result struct {
			Token string `json:"token"`
		}
		decode(request(method, "/my/contests/"+id+"/setter-invitation", actor, nil, 200), &result)
		if !testerToken.MatchString(result.Token) {
			t.Fatal("token", result.Token)
		}
		return "/my/setter-invitations/" + result.Token
	}
	accept := func(invitation, actor string, want int) contests.SetterInvitation {
		t.Helper()
		var result contests.SetterInvitation
		body := request("POST", invitation, actor, nil, want)
		if want == 200 {
			decode(body, &result)
		}
		return result
	}
	testerLink := func(problem, issuer string) string {
		t.Helper()
		var result struct {
			Token string `json:"token"`
		}
		decode(request("POST", "/my/problems/"+problem+"/tester-invitation", issuer, nil, 200), &result)
		return "/my/tester-invitations/" + result.Token
	}
	draftOf := func(id, actor string) contests.DraftResult {
		t.Helper()
		var result contests.DraftResult
		decode(request("GET", "/my/contests/"+id+"/draft", actor, nil, 200), &result)
		return result
	}
	saveDraft := func(id, actor string, version int64, draft map[string]any, want int) contests.DraftResult {
		t.Helper()
		var result contests.DraftResult
		body := request("PUT", "/my/contests/"+id+"/draft", actor, map[string]any{"version": version, "draft": draft}, want)
		if want == 200 {
			decode(body, &result)
		}
		return result
	}
	contestOf := func(id, actor string) contests.Contest {
		t.Helper()
		path := "/contests/" + id
		if actor != "" {
			path = "/my/contests/" + id
		}
		var result contests.Contest
		decode(request("GET", path, actor, nil, 200), &result)
		return result
	}
	problemList := func(ids ...string) []map[string]any {
		list := []map[string]any{}
		for _, pid := range ids {
			list = append(list, map[string]any{"id": pid, "points": 100})
		}
		return list
	}
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	draftBody := func(title string, ids ...string) map[string]any {
		return map[string]any{"title": title, "description": "", "startsAt": start, "durationMinutes": 60, "penaltyMinutes": 5, "problems": problemList(ids...)}
	}
	upload := func(owner string, size int) string {
		t.Helper()
		body := `{"data":"` + base64.StdEncoding.EncodeToString(imagePNG(t, size)) + `"}`
		r := httptest.NewRequest("POST", "/my/images", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+f.token(t, owner, nil))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var uploaded struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		}
		if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &uploaded) != nil {
			t.Fatal("upload", w.Code, w.Body.String())
		}
		return uploaded.URL
	}
	image := func(path, viewer string, want int) {
		t.Helper()
		id := strings.TrimPrefix(path, "/api/images/")
		r := httptest.NewRequest("GET", "/my/images/"+id, nil)
		if viewer == "" {
			r = httptest.NewRequest("GET", "/images/"+id, nil)
		} else {
			r.Header.Set("Authorization", "Bearer "+f.token(t, viewer, nil))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("image %s for %q: got %d want %d", id, viewer, w.Code, want)
		}
	}
	tested := func(actor string) map[string]string {
		t.Helper()
		var page struct {
			Items []problems.Summary `json:"items"`
		}
		decode(request("GET", "/my/problems?role=tester", actor, nil, 200), &page)
		marks := map[string]string{}
		for _, item := range page.Items {
			marks[item.ID] = ""
			if item.ViaContest != nil {
				marks[item.ID] = item.ViaContest.ID + " " + item.ViaContest.Title
			}
		}
		return marks
	}
	excluded := func(id string) []string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT owner_id FROM contest_exclusions WHERE contest_id=$1 ORDER BY owner_id`, id)
		if err != nil {
			t.Fatal(err)
		}
		owners, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return owners
	}
	request("POST", testerLink(pA, "ann"), "cora", nil, 200)
	request("POST", testerLink(pY, "ann"), "carl", nil, 200)
	request("POST", testerLink(pL, "ann"), "olive", nil, 200)

	// The owner issues one link per contest, already for the draft; strangers get none.
	saveDraft(cid, "olive", 0, draftBody("Shared", pO), 200)
	invite := link(cid, "olive", "POST")
	if again := link(cid, "olive", "POST"); again != invite {
		t.Fatal("link changed on retry", invite, again)
	}
	request("POST", "/my/contests/"+cid+"/setter-invitation", "sam", nil, 404)
	request("PUT", "/my/contests/"+cid+"/setter-invitation", "cora", nil, 404)
	request("GET", "/my/setter-invitations/ABC", "cora", nil, 404)
	request("GET", "/my/setter-invitations/"+strings.Repeat("A", 32), "cora", nil, 404)
	request("GET", invite, "", nil, 401)
	var preview contests.SetterInvitation
	decode(request("GET", invite, "cora", nil, 200), &preview)
	if preview != (contests.SetterInvitation{ID: cid, Title: "Shared", Author: "olive"}) {
		t.Fatalf("preview: %+v", preview)
	}

	// Accepting joins once and tells the owner; the owner's own acceptance changes nothing.
	if joined := accept(invite, "cora", 200); !joined.Joined || joined.Published {
		t.Fatalf("joined: %+v", joined)
	}
	accept(invite, "cora", 200)
	accept(invite, "olive", 200)
	if n := count(`SELECT count(*) FROM contest_setters WHERE contest_id=$1`, cid); n != 1 {
		t.Fatal("setter rows", n)
	}
	var bell struct {
		Notifications []notifications.Notification `json:"notifications"`
	}
	decode(request("GET", "/my/notifications", "olive", nil, 200), &bell)
	if len(bell.Notifications) != 1 || bell.Notifications[0] != (notifications.Notification{ID: bell.Notifications[0].ID, ContestID: cid, Title: "Shared", Actor: "cora", Kind: "contest_setter", CreatedAt: bell.Notifications[0].CreatedAt}) {
		t.Fatalf("owner notifications: %+v", bell)
	}
	if body := request("GET", "/my/notifications", "olive", nil, 200); strings.Contains(body, "problemId") {
		t.Fatal("contest notification names a problem", body)
	}

	// A co-setter reads the draft with the other setters' problems, but gets no rights to them from it.
	shared := draftOf(cid, "cora")
	if !slices.Equal(shared.Authors, []string{"olive", "cora"}) || shared.IsOwner || len(shared.Problems) != 1 || shared.Problems[0].ID != pO ||
		shared.Problems[0].Title != "Title of 01" || shared.Problems[0].Author != "olive" || shared.Problems[0].Issues == nil || len(shared.Problems[0].Issues) != 0 {
		t.Fatalf("co-setter draft: %+v", shared)
	}
	if !draftOf(cid, "olive").IsOwner {
		t.Fatal("owner flag")
	}
	request("GET", "/my/problems/"+pO, "cora", nil, 404)
	var drafts offsetResponse[contests.DraftSummary]
	decode(request("GET", "/my/contests/drafts", "cora", nil, 200), &drafts)
	if len(drafts.Items) != 1 || drafts.Items[0].ID != cid || drafts.Items[0].Author != "olive" {
		t.Fatalf("co-setter drafts: %+v", drafts)
	}
	accept(invite, "carl", 200)

	// Each setter adds only problems they author or test themselves; the problems already listed stay.
	saved := saveDraft(cid, "cora", 1, draftBody("Shared", pO, pC, pA), 200)
	if len(saved.Problems) != 3 || saved.Problems[2].ID != pA || saved.Problems[2].Author != "ann" || len(saved.Issues) != 0 {
		t.Fatalf("co-setter save: %+v", saved)
	}
	for _, extra := range []string{pY, pX} {
		if body := request("PUT", "/my/contests/"+cid+"/draft", "cora", map[string]any{"version": 2, "draft": draftBody("Shared", pO, pC, pA, extra)}, 409); !strings.Contains(body, "contest_conflict") {
			t.Fatal(body)
		}
	}
	if current := draftOf(cid, "olive"); current.Version != 2 || len(current.Draft.Problems) != 3 {
		t.Fatalf("refused save changed the draft: %+v", current)
	}
	saveDraft(cid, "carl", 2, draftBody("Shared", pO, pC, pA, pY), 200)
	request("GET", "/my/contests/"+cid+"/draft", "sam", nil, 404)
	request("PUT", "/my/contests/"+cid+"/draft", "sam", map[string]any{"version": 3, "draft": draftBody("Mine", pX)}, 404)
	request("PUT", "/my/contests/"+cid+"/publication", "sam", map[string]any{"version": 3}, 404)
	request("DELETE", "/my/contests/"+cid+"/draft?version=3", "cora", nil, 404)

	// Naming someone else's problem in a draft is refused and, even if stored, grants nothing.
	saveDraft(hid, "sam", 0, draftBody("Hijack", pA), 409)
	saveDraft(hid, "sam", 0, draftBody("Hijack", pZ), 200)
	exec(`UPDATE contest_drafts SET draft=jsonb_set(draft,'{problems}',$2::jsonb) WHERE id=$1`, hid, `[{"id":"`+pA+`","points":100}]`)
	if body := request("GET", "/my/contests/"+hid+"/draft", "sam", nil, 200); strings.Contains(body, "Title of 03") || !strings.Contains(body, "problem_unavailable") {
		t.Fatal("hijacked draft", body)
	}
	if body := request("PUT", "/my/contests/"+hid+"/publication", "sam", map[string]any{"version": 1}, 409); !strings.Contains(body, "contest_not_ready") {
		t.Fatal(body)
	}
	request("GET", "/my/problems/"+pA, "sam", nil, 404)
	// Nor can a contest be created over a draft's id to borrow its setters.
	penalty := 5
	squat := contests.Input{Title: "Squat", StartsAt: start, EndsAt: start.Add(time.Hour), PenaltyMinutes: &penalty, Problems: []contests.Problem{{ID: pX, Points: 100}}}
	request("PUT", "/my/contests/"+cid, "sam", squat, 409)
	request("GET", "/my/contests/"+cid+"/draft", "sam", nil, 404)

	// Deleting a draft takes its co-setters, link and notifications with it.
	saveDraft(d2, "olive", 0, draftBody("Doomed"), 200)
	doomed := link(d2, "olive", "POST")
	accept(doomed, "rex", 200)
	request("DELETE", "/my/contests/"+d2+"/draft?version=1", "olive", nil, 204)
	for _, table := range []string{"contest_setters", "contest_setter_invitations", "notifications", "contest_exclusions"} {
		if n := count(`SELECT count(*) FROM `+table+` WHERE contest_id=$1`, d2); n != 0 {
			t.Fatal("left over after deleting the draft", table, n)
		}
	}
	request("GET", doomed, "rex", nil, 404)

	// Images in the draft description are shown to every setter and nobody else.
	draftImage := upload("olive", 5)
	described := draftBody("Shared", pO, pC, pA, pY)
	described["description"] = "![](" + draftImage + ")"
	saveDraft(cid, "olive", 3, described, 200)
	image(draftImage, "cora", 200)
	image(draftImage, "rex", 404)
	image(draftImage, "", 404)

	// A co-setter who leaves the draft loses it and its private images, and their problem in it becomes unavailable.
	accept(invite, "dan", 200)
	image(draftImage, "dan", 200)
	withLeaver := draftBody("Shared", pO, pC, pA, pY, pD)
	withLeaver["description"] = described["description"]
	saveDraft(cid, "dan", 4, withLeaver, 200)
	request("DELETE", "/my/contests/"+cid+"/setters/dan", "dan", nil, 204)
	image(draftImage, "dan", 404)
	request("GET", "/my/contests/"+cid+"/draft", "dan", nil, 404)
	request("PUT", "/my/contests/"+cid+"/draft", "dan", map[string]any{"version": 5, "draft": withLeaver}, 404)
	request("PUT", "/my/contests/"+cid+"/publication", "dan", map[string]any{"version": 5}, 404)
	var leftDrafts offsetResponse[contests.DraftSummary]
	decode(request("GET", "/my/contests/drafts", "dan", nil, 200), &leftDrafts)
	if len(leftDrafts.Items) != 0 {
		t.Fatalf("drafts after leaving: %+v", leftDrafts)
	}
	if body := request("GET", "/my/contests/"+cid+"/draft", "olive", nil, 200); strings.Contains(body, "Title of 0b") || !strings.Contains(body, "problem_unavailable") {
		t.Fatal("draft after its setter left", body)
	}
	if body := request("PUT", "/my/contests/"+cid+"/publication", "olive", map[string]any{"version": 5}, 409); !strings.Contains(body, "contest_not_ready") {
		t.Fatal(body)
	}
	saveDraft(cid, "olive", 5, described, 200)
	// The owner removes a co-setter from the draft; the link is renewed, as after publication.
	accept(invite, "eve", 200)
	request("DELETE", "/my/contests/"+cid+"/setters/eve", "eve", nil, 204)
	accept(invite, "eve", 200)
	request("DELETE", "/my/contests/"+cid+"/setters/eve", "cora", nil, 404)
	request("DELETE", "/my/contests/"+cid+"/setters/eve", "olive", nil, 204)
	request("GET", invite, "sam", nil, 404)
	request("GET", "/my/contests/"+cid+"/draft", "eve", nil, 404)
	image(draftImage, "eve", 404)
	invite = link(cid, "olive", "POST")
	if !slices.Equal(draftOf(cid, "cora").Authors, []string{"olive", "cora", "carl"}) {
		t.Fatal("authors after leaving the draft")
	}

	// A co-setter publishes, and the draft's owner stays the owner; the setters and link carry over.
	var published contests.Contest
	decode(request("PUT", "/my/contests/"+cid+"/publication", "cora", map[string]any{"version": 6}, 200), &published)
	if published.Author != "olive" || !slices.Equal(published.Authors, []string{"olive", "cora", "carl"}) || !published.IsSetter || published.IsOwner || !published.CanEdit {
		t.Fatalf("published by a co-setter: %+v", published)
	}
	if n := count(`SELECT count(*) FROM contests WHERE id=$1 AND owner_id='olive'`, cid); n != 1 {
		t.Fatal("owner changed on publication")
	}
	image(draftImage, "", 200)
	// Those who left or were removed at the draft stage stay out of the published contest.
	for _, gone := range []string{"dan", "eve"} {
		if body := request("POST", "/my/contests/"+cid+"/participation", gone, nil, 409); !strings.Contains(body, "contest_participation_unavailable") {
			t.Fatal(gone, body)
		}
		if c := contestOf(cid, gone); c.IsSetter || c.Staff || c.Official {
			t.Fatalf("contest view for %s: %+v", gone, c)
		}
	}
	if got := excluded(cid); !slices.Equal(got, []string{"dan", "eve"}) {
		t.Fatal("exclusions after publication", got)
	}
	if joined := accept(invite, "rex", 200); !joined.Published || !joined.Joined {
		t.Fatalf("published invitation: %+v", joined)
	}
	request("POST", "/my/contests/"+cid+"/participation", "pat", nil, 200)
	if body := request("POST", invite, "pat", nil, 409); !strings.Contains(body, "contest_participant_locked") {
		t.Fatal(body)
	}
	public := request("GET", "/contests/"+cid, "", nil, 200)
	if strings.Contains(public, strings.TrimPrefix(invite, "/my/setter-invitations/")) {
		t.Fatal("public contest exposes the link")
	}
	if c := contestOf(cid, ""); !slices.Equal(c.Authors, []string{"olive", "cora", "carl", "rex"}) || c.IsSetter || c.IsOwner || slices.Contains(c.Testers, "olive") {
		t.Fatalf("public contest: %+v", c)
	}
	var mine offsetResponse[contests.Contest]
	decode(request("GET", "/my/contests", "cora", nil, 200), &mine)
	if len(mine.Items) != 1 || mine.Items[0].ID != cid {
		t.Fatalf("co-setter contests: %+v", mine)
	}

	// Removing a co-setter renews the link, so the old one no longer brings them back.
	request("DELETE", "/my/contests/"+cid+"/setters/rex", "cora", nil, 404)
	request("DELETE", "/my/contests/"+cid+"/setters/rex", "sam", nil, 404)
	request("DELETE", "/my/contests/"+cid+"/setters/olive", "olive", nil, 404)
	request("DELETE", "/my/contests/"+cid+"/setters/Bad", "olive", nil, 404)
	request("DELETE", "/my/contests/"+cid+"/setters/rex", "olive", nil, 204)
	request("DELETE", "/my/contests/"+cid+"/setters/rex", "olive", nil, 404)
	request("POST", invite, "rex", nil, 404)
	renewed := link(cid, "cora", "POST")
	if renewed == invite {
		t.Fatal("removal kept the link")
	}
	// Any current setter may renew the link on purpose, and the old one then stops working.
	put := link(cid, "cora", "PUT")
	if put == renewed {
		t.Fatal("renewal kept the link")
	}
	request("GET", renewed, "sam", nil, 404)
	request("POST", renewed, "sam", nil, 404)
	request("GET", put, "sam", nil, 200)
	request("PUT", "/my/contests/"+cid+"/setter-invitation", "rex", nil, 404)
	if again := link(cid, "olive", "POST"); again != put {
		t.Fatal("link changed without renewal", put, again)
	}
	renewed = put

	// Permissions by role: stranger, participant, co-setter, removed co-setter, owner.
	roles := []string{"sam", "pat", "cora", "rex", "olive"}
	for _, tc := range []struct {
		method, path string
		body         any
		want         []int
	}{
		{"POST", "/my/contests/" + cid + "/setter-invitation", nil, []int{404, 404, 200, 404, 200}},
		{"GET", "/my/problems/" + pO, nil, []int{404, 404, 200, 404, 200}},
		{"GET", "/my/problems/" + pA, nil, []int{404, 404, 200, 404, 200}},
		{"GET", "/my/problems/" + pA + "/rejudge", nil, []int{404, 404, 200, 404, 200}},
		{"POST", "/my/problems/" + pA + "/rejudge", map[string]any{"submissionIds": []string{}}, []int{404, 404, 202, 404, 202}},
		{"GET", "/my/contests/" + cid + "/problems/" + pO, nil, []int{404, 404, 200, 404, 200}},
		{"GET", "/my/contests/" + cid + "/submissions", nil, []int{404, 404, 200, 404, 200}},
		{"POST", "/my/contests/" + cid + "/participation", nil, []int{200, 200, 409, 409, 409}},
	} {
		for i, actor := range roles {
			request(tc.method, tc.path, actor, tc.body, tc.want[i])
		}
	}
	for i, actor := range roles {
		c := contestOf(cid, actor)
		setter := i == 2 || i == 4
		if c.IsSetter != setter || c.Staff != setter || c.CanEdit != setter || c.Official != (i < 2) || c.IsOwner != (i == 4) {
			t.Fatalf("contest flags for %s: %+v", actor, c)
		}
	}
	exec(`DELETE FROM contest_participants WHERE contest_id=$1 AND owner_id='sam'`, cid)
	// Before the start, every current setter may try any contest problem, but a removed one may not.
	submit := func(actor, problem string, want int) string {
		t.Helper()
		return request("POST", "/my/submissions", actor, map[string]any{"problemId": problem, "contestId": cid, "runtime": "cpp17", "source": "code of " + actor}, want)
	}
	submit("cora", pO, 202)
	submit("olive", pA, 202)
	if body := submit("rex", pO, 409); !strings.Contains(body, "contest_participation_required") {
		t.Fatal(body)
	}
	submit("pat", pO, 409)

	// A co-setter edits another setter's problem in the published contest, like a tester.
	var owned problems.Problem
	decode(request("GET", "/my/problems/"+pO, "cora", nil, 200), &owned)
	if owned.Readiness != nil || owned.ContestEditors == nil || owned.ContestEditors.ContestID != cid {
		t.Fatalf("co-setter problem view: %+v", owned)
	}
	owned.Draft.Markdown = "fixed by cora"
	request("PUT", "/my/problems/"+pO, "cora", map[string]any{"version": owned.Version, "draft": owned.Draft}, 200)
	if n := count(`SELECT count(*) FROM contest_problems WHERE problem_id=$1 AND draft->>'markdown'='fixed by cora'`, pO); n != 1 {
		t.Fatal("contest copy not updated")
	}

	// Rights through the contest never mint tester links, which nobody could revoke.
	request("POST", "/my/problems/"+pO+"/tester-invitation", "cora", nil, 404)
	request("POST", "/my/problems/"+pA+"/tester-invitation", "olive", nil, 404)
	request("POST", "/my/problems/"+pA+"/tester-invitation", "cora", nil, 200)
	var managed problems.Problem
	decode(request("GET", "/my/problems/"+pA, "ann", nil, 200), &managed)
	if !slices.Equal(managed.Testers, []string{"cora"}) || managed.ContestEditors == nil || managed.ContestEditors.ContestID != cid ||
		managed.ContestEditors.Title != "Shared" || !slices.Equal(managed.ContestEditors.Setters, []string{"olive", "cora", "carl"}) {
		t.Fatalf("author view of contest editors: %+v", managed)
	}
	via := cid + " Shared"
	if got := tested("olive"); len(got) != 4 || got[pA] != via || got[pC] != via || got[pY] != via || got[pL] != "" {
		t.Fatalf("owner tests: %v", got)
	}
	if got := tested("cora"); len(got) != 3 || got[pA] != "" || got[pO] != via || got[pY] != via {
		t.Fatalf("co-setter tests: %v", got)
	}
	var detail contests.ContestProblem
	decode(request("GET", "/my/contests/"+cid+"/problems/"+pO, "cora", nil, 200), &detail)
	if detail.Author != "olive" || !slices.Equal(detail.Authors, []string{"olive", "cora", "carl"}) || !detail.CanReadEditorial || detail.Editorial == "" || len(detail.Testers) != 0 {
		t.Fatalf("co-setter contest problem: %+v", detail)
	}

	// A co-setter saves the published contest; images any setter puts in the description are public.
	contestImage := upload("cora", 6)
	edit := contests.Input{Title: "Shared", Description: "![](" + contestImage + ")", StartsAt: published.StartsAt, EndsAt: published.EndsAt, PenaltyMinutes: &published.PenaltyMinutes, Version: published.Version}
	for _, p := range published.Problems {
		edit.Problems = append(edit.Problems, contests.Problem{ID: p.ID, Points: p.Points})
	}
	refused := edit
	refused.Problems = append(slices.Clone(edit.Problems), contests.Problem{ID: pX, Points: 100})
	request("PUT", "/my/contests/"+cid, "cora", refused, 409)
	decode(request("PUT", "/my/contests/"+cid, "cora", edit, 200), &published)
	if published.Author != "olive" {
		t.Fatal("save changed the owner", published.Author)
	}
	image(contestImage, "", 200)
	// A setter adds only problems they manage themselves, even when another setter manages the problem.
	attempt := edit
	attempt.Version = published.Version
	attempt.Problems = append(slices.Clone(edit.Problems), contests.Problem{ID: pO2, Points: 100})
	if body := request("PUT", "/my/contests/"+cid, "cora", attempt, 409); !strings.Contains(body, "contest_conflict") {
		t.Fatal(body)
	}
	if c := contestOf(cid, "cora"); c.Version != published.Version || len(c.Problems) != len(edit.Problems) {
		t.Fatalf("refused save changed the contest: %+v", c)
	}
	request("GET", "/my/problems/"+pO2, "cora", nil, 404)
	decode(request("PUT", "/my/contests/"+cid, "olive", attempt, 200), &published)
	request("GET", "/my/problems/"+pO2, "cora", nil, 200)
	edit = attempt

	// A problem that no current setter authors or tests blocks saving until it is removed.
	request("DELETE", "/my/contests/"+cid+"/setters/carl", "carl", nil, 204)
	request("POST", "/my/contests/"+cid+"/participation", "carl", nil, 409)
	// Someone who left may rejoin through the current link; the owner hears of it again, and they still cannot compete.
	request("GET", "/my/problems/"+pO, "carl", nil, 404)
	if rejoined := accept(renewed, "carl", 200); !rejoined.Joined || !rejoined.Published {
		t.Fatalf("rejoined: %+v", rejoined)
	}
	if c := contestOf(cid, "carl"); !c.IsSetter || !c.Staff || !slices.Equal(c.Authors, []string{"olive", "cora", "carl"}) {
		t.Fatalf("rejoined setter view: %+v", c)
	}
	request("GET", "/my/problems/"+pO, "carl", nil, 200)
	if n := count(`SELECT count(*) FROM notifications WHERE owner_id='olive' AND contest_id=$1 AND actor_id='carl' AND kind='contest_setter'`, cid); n != 2 {
		t.Fatal("rejoin notifications", n)
	}
	request("POST", "/my/contests/"+cid+"/participation", "carl", nil, 409)
	request("DELETE", "/my/contests/"+cid+"/setters/carl", "carl", nil, 204)
	unmanaged := []string{}
	for _, p := range contestOf(cid, "cora").Problems {
		if p.Unmanaged {
			unmanaged = append(unmanaged, p.ID)
		}
	}
	if !slices.Equal(unmanaged, []string{pY}) || slices.ContainsFunc(contestOf(cid, "").Problems, func(p contests.Problem) bool { return p.Unmanaged }) {
		t.Fatal("unmanaged problems", unmanaged)
	}
	edit.Version = published.Version
	request("PUT", "/my/contests/"+cid, "cora", edit, 409)
	edit.Problems = slices.DeleteFunc(edit.Problems, func(p contests.Problem) bool { return p.ID == pY })
	decode(request("PUT", "/my/contests/"+cid, "cora", edit, 200), &published)
	if got := excluded(cid); !slices.Equal(got, []string{"ann", "carl", "dan", "eve", "rex"}) {
		t.Fatal("exclusions", got)
	}

	// The editorial link follows what the server returns; contest rights end when the results are final.
	exec(`UPDATE contests SET starts_at=now()-interval '1 hour' WHERE id=$1`, cid)
	detail = contests.ContestProblem{}
	decode(request("GET", "/my/contests/"+cid+"/problems/"+pO, "pat", nil, 200), &detail)
	if detail.CanReadEditorial || detail.Editorial != "" {
		t.Fatalf("participant editorial while running: %+v", detail)
	}
	// While the contest runs, a registered participant still cannot join the setters.
	if body := request("POST", renewed, "pat", nil, 409); !strings.Contains(body, "contest_participant_locked") {
		t.Fatal(body)
	}
	if c := contestOf(cid, "pat"); c.IsSetter || c.Staff || !c.Participating {
		t.Fatalf("participant view while running: %+v", c)
	}
	// Until the results are final, setters keep their rights through the contest, and helpers may still join.
	request("POST", "/my/problems/"+pA+"/rejudge", "olive", map[string]any{"submissionIds": []string{}}, 202)
	request("GET", "/my/problems/"+pO, "cora", nil, 200)
	if joined := accept(renewed, "late", 200); !joined.Joined || !joined.Published {
		t.Fatalf("joined while running: %+v", joined)
	}
	if c := contestOf(cid, "late"); !c.IsSetter || !c.Staff || !c.CanViewSubmissions {
		t.Fatalf("helper view while running: %+v", c)
	}
	request("GET", "/my/problems/"+pO, "late", nil, 200)
	// Leaving on their own keeps the link, which the end then closes.
	request("DELETE", "/my/contests/"+cid+"/setters/late", "late", nil, 204)
	request("GET", "/my/problems/"+pO, "late", nil, 404)
	exec(`UPDATE contests SET ends_at=now()-interval '1 minute' WHERE id=$1`, cid)
	decode(request("GET", "/my/contests/"+cid+"/problems/"+pO, "pat", nil, 200), &detail)
	if !detail.CanReadEditorial {
		t.Fatal("editorial hidden after the end")
	}
	request("GET", "/my/problems/"+pA, "olive", nil, 404)
	request("GET", "/my/problems/"+pO, "cora", nil, 404)
	request("POST", "/my/problems/"+pA+"/rejudge", "olive", map[string]any{"submissionIds": []string{}}, 404)
	request("GET", "/my/problems/"+pA, "cora", nil, 200)
	managed = problems.Problem{}
	decode(request("GET", "/my/problems/"+pA, "ann", nil, 200), &managed)
	if managed.ContestEditors != nil {
		t.Fatalf("contest editors after the results: %+v", managed.ContestEditors)
	}
	if got := tested("olive"); len(got) != 1 || got[pL] != "" {
		t.Fatalf("owner tests after the results: %v", got)
	}
	if body := request("POST", renewed, "late", nil, 409); !strings.Contains(body, "contest_invitation_closed") {
		t.Fatal(body)
	}

	// Account deletion: the owner's open contests and shared drafts go to the co-setter who joined first.
	deleteAccount := func(owner string) error {
		return (&accounts.Store{Pool: pool}).Delete(ctx, owner, owner, func(context.Context) error { return nil })
	}
	// The owner tests the contest's only problem, so the problem's author waits for the contest and the owner does not.
	saveDraft(kid, "olive", 0, draftBody("Kept", pL), 200)
	kept := link(kid, "olive", "POST")
	accept(kept, "cora", 200)
	accept(kept, "carl", 200)
	request("PUT", "/my/contests/"+kid+"/publication", "olive", map[string]any{"version": 1}, 200)
	saveDraft(d3, "olive", 0, draftBody("Shared draft"), 200)
	accept(link(d3, "olive", "POST"), "carl", 200)
	saveDraft(d4, "olive", 0, draftBody("Private draft"), 200)
	saveDraft(k2, "olive", 0, draftBody("Alone", pK2), 200)
	request("PUT", "/my/contests/"+k2+"/publication", "olive", map[string]any{"version": 1}, 200)
	// The author of an unpublished problem in a contest that has not ended waits, as do owners without co-setters.
	if err = deleteAccount("ann"); !errors.Is(err, accounts.ErrActiveContest) {
		t.Fatal("author deletion", err)
	}
	if err = deleteAccount("olive"); !errors.Is(err, accounts.ErrActiveContest) {
		t.Fatal("owner deletion", err)
	}
	if n := count(`SELECT count(*) FROM contests WHERE id=$1 AND owner_id='olive'`, kid); n != 1 {
		t.Fatal("refused deletion handed over")
	}
	exec(`UPDATE contests SET starts_at=now()-interval '2 hours',ends_at=now()-interval '1 hour' WHERE id=$1`, k2)
	if err = deleteAccount("olive"); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM contests WHERE id='` + kid + `' AND owner_id='cora'`:                                         1,
		`SELECT count(*) FROM contest_setters WHERE contest_id='` + kid + `' AND left_at IS NULL`:                          1,
		`SELECT count(*) FROM contest_setters WHERE contest_id='` + kid + `' AND owner_id='olive' AND left_at IS NOT NULL`: 1,
		`SELECT count(*) FROM contest_drafts WHERE id='` + d3 + `' AND owner_id='carl'`:                                    1,
		`SELECT count(*) FROM contest_drafts WHERE id='` + d4 + `'`:                                                        0,
		`SELECT count(*) FROM contests WHERE id='` + cid + `' AND owner_id='olive'`:                                        1,
		`SELECT count(*) FROM problem_drafts WHERE id='` + pK2 + `'`:                                                       1,
	} {
		if got := count(query); got != want {
			t.Fatal(query, got, want)
		}
	}
	if got := excluded(kid); !slices.Equal(got, []string{"olive"}) {
		t.Fatal("former owner exclusion", got)
	}
	if heir := contestOf(kid, "cora"); !heir.IsOwner || !slices.Equal(heir.Authors, []string{"cora", "carl"}) {
		t.Fatalf("heir view: %+v", heir)
	}
	// A co-setter's deletion is leaving; their images stay in the contest.
	if err = deleteAccount("cora"); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT count(*) FROM contests WHERE id=$1 AND owner_id='carl'`, kid); n != 1 {
		t.Fatal("second hand-over")
	}
	if c := contestOf(cid, ""); len(c.Authors) != 1 {
		t.Fatalf("deleted co-setter still credited: %+v", c.Authors)
	}
	image(contestImage, "", 200)
}
