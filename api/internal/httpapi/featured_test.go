package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

type featuredTest struct {
	t       *testing.T
	store   *problems.Store
	request func(string, string, string, any, int) string
}

func newFeaturedTest(t *testing.T) featuredTest {
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
	schema := fmt.Sprintf("test_featured_%d", time.Now().UnixNano())
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
	f := featuredTest{t: t, store: store}
	f.exec(`INSERT INTO user_profiles(owner_id,handle) VALUES('alice','alice'),('bob','bob'),('tester','tester')`)
	sign := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(sign.server.URL, "client")})
	f.request = func(method, path, owner string, body any, want int) string {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		if owner != "" {
			r.Header.Set("Authorization", "Bearer "+sign.token(t, owner, nil))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s (%s): %d want %d: %s", method, path, owner, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable featured response")
		}
		return w.Body.String()
	}
	return f
}

func (f featuredTest) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool().Exec(context.Background(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f featuredTest) draft(owner string, level int) problems.Problem {
	f.t.Helper()
	d := problems.Draft{Title: "Featured problem", Markdown: "Statement", Editorial: "SECRET EDITORIAL", TimeLimitMS: "1000", MemoryLimitMB: "256", Difficulty: &level, TestCases: []problems.TestCase{{Input: "1", Output: "2"}}}
	p, err := f.store.Save(context.Background(), owner, newSubmissionID(), 0, d)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f featuredTest) apply(p problems.Problem, preference string, want int) {
	f.t.Helper()
	f.request("PUT", "/my/problems/"+p.ID+"/featured", p.Author, map[string]any{"version": p.Version, "preference": preference}, want)
}

func (f featuredTest) due() time.Time {
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	f.exec(`UPDATE featured_schedule SET next_at=$1`, at)
	f.exec(`UPDATE featured_applications SET entered_at=$1`, at.Add(-time.Hour))
	return at
}

func TestFeaturedApplicationsPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	a := f.draft("alice", 4)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, a.ID)
	f.request("PUT", "/my/problems/"+a.ID+"/featured", "", map[string]any{"version": 1, "preference": "soon"}, 401)
	f.request("PUT", "/my/problems/"+a.ID+"/featured", "tester", map[string]any{"version": 1, "preference": "soon"}, 404)
	f.apply(a, "bad", 400)
	f.request("PUT", "/my/problems/"+a.ID+"/featured", "alice", map[string]any{"version": 2, "preference": "soon"}, 409)
	f.exec(`UPDATE problem_drafts SET draft=draft-'editorial' WHERE id=$1`, a.ID)
	f.apply(a, "soon", 400)
	f.exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{editorial}','"explanation"') WHERE id=$1`, a.ID)
	f.apply(a, "soon", 204)
	f.apply(f.draft("alice", 2), "later", 204)
	f.apply(f.draft("alice", 3), "later", 204)
	fourth := f.draft("alice", 1)
	f.apply(fourth, "soon", 409)
	f.apply(a, "later", 204) // Updating a preference does not consume another place.
	f.apply(a, "", 204)
	f.apply(fourth, "soon", 204)
	f.request("GET", "/problems/"+fourth.ID, "", nil, 404)
	f.request("GET", "/my/problems/"+a.ID, "tester", nil, 200)
	f.request("PUT", "/my/problems/"+fourth.ID+"/publication", "alice", map[string]any{"version": 1, "publish": true}, 200)
	listed := f.request("GET", "/my/featured", "alice", nil, 200)
	if strings.Contains(listed, fourth.ID) {
		t.Fatal("normal publication retained application")
	}
	f.request("PUT", "/my/problems/"+fourth.ID+"/publication", "alice", map[string]any{"version": 2, "publish": false}, 200)
	fourth.Version = 3
	f.apply(fourth, "soon", 400) // Previously public remains ineligible after unpublishing.
	f.apply(a, "soon", 204)
	cid := newSubmissionID()
	f.request("PUT", "/my/contests/"+cid, "alice", map[string]any{"version": 0, "title": "Contest", "description": "", "startsAt": time.Now().Add(time.Hour), "endsAt": time.Now().Add(2 * time.Hour), "penaltyMinutes": 5, "problems": []map[string]any{{"id": a.ID, "points": 100}}}, 200)
	if strings.Contains(f.request("GET", "/my/featured", "alice", nil, 200), a.ID) {
		t.Fatal("contest registration retained application")
	}
	f.apply(a, "soon", 400)
}

func TestFeaturedReleaseAndEmbargoPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	a, b := f.draft("alice", 4), f.draft("bob", 9)
	imageID := newSubmissionID()
	f.exec(`INSERT INTO content_images(id,owner_id,digest,media_type,data,size) VALUES($1,'alice','x','image/png','x',1)`, imageID)
	f.exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{editorial}',to_jsonb('SECRET EDITORIAL ![](/api/images/'||$2::text||')')) WHERE id=$1`, a.ID, imageID)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, a.ID)
	f.apply(a, "soon", 204)
	f.apply(b, "later", 204)
	at := f.due()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			errs <- f.store.ReleaseFeatured(context.Background(), submissions.RuntimeIDs(), []string{"cpp17"})
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	page := f.request("GET", "/featured", "", nil, 200)
	var list problems.FeaturedPage
	if err := json.Unmarshal([]byte(page), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || len(list.Items[0].Slots) != 2 || list.Items[0].Slots[0].ProblemID != a.ID || list.Items[0].Slots[1].Difficulty == nil || *list.Items[0].Slots[1].Difficulty != 9 {
		t.Fatal(page)
	}
	if !list.Items[0].Slots[0].RevealAt.Equal(at.Add(23 * time.Hour)) {
		t.Fatal("wrong reveal boundary", page)
	}
	if again := f.request("GET", "/featured", "", nil, 200); again != page {
		t.Fatal("replay changed selection")
	}
	for _, viewer := range []string{"", "alice", "bob", "tester"} {
		body := f.request("GET", "/problems/"+a.ID, viewer, nil, 200)
		if strings.Contains(body, "SECRET") || strings.Contains(body, imageID) || !strings.Contains(body, `"editorialHidden":true`) {
			t.Fatal("editorial leaked", body)
		}
	}
	f.request("GET", "/images/"+imageID, "", nil, 404)
	f.request("GET", "/my/images/"+imageID, "bob", nil, 404)
	f.request("GET", "/my/images/"+imageID, "tester", nil, 200)
	if !strings.Contains(f.request("GET", "/my/problems/"+a.ID, "tester", nil, 200), "SECRET") {
		t.Fatal("tester lost draft access")
	}
	// A normal public submission is accepted while the editorial is hidden.
	f.request("POST", "/my/submissions", "bob", map[string]any{"problemId": a.ID, "runtime": "cpp17", "source": "int main(){}"}, 202)
	sid := newSubmissionID()
	f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result) VALUES($1,'bob',$2,2,'Featured','cpp17','SECRET SOURCE','{"privateDraft":false}','DONE','{"verdict":"AC","passed":1,"total":1}')`, sid, a.ID)
	for _, viewer := range []string{"", "alice", "tester"} {
		path := "/problems/" + a.ID + "/submissions"
		if viewer != "" {
			path = "/my" + path
		}
		body := f.request("GET", path, viewer, nil, 200)
		if strings.Contains(body, sid) {
			t.Fatal("submission list leaked", body)
		}
		f.request("GET", path+"/"+sid, viewer, nil, 404)
	}
	f.request("GET", "/my/problems/"+a.ID+"/submissions/"+sid, "bob", nil, 200)
	f.request("GET", "/my/submissions/"+sid, "bob", nil, 200)
	if !strings.Contains(f.request("GET", "/my/problems/"+a.ID+"/submissions?mine=1", "bob", nil, 200), sid) {
		t.Fatal("own result unavailable")
	}
	// A difficulty edit or vote never moves the already recorded slot.
	f.exec(`UPDATE problem_drafts SET published_draft=jsonb_set(published_draft,'{difficulty}','9') WHERE id=$1`, a.ID)
	var edited problems.FeaturedPage
	if err := json.Unmarshal([]byte(f.request("GET", "/featured", "", nil, 200)), &edited); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(edited.Items, list.Items) {
		t.Fatal("slot changed after difficulty edit", edited.Items)
	}
	f.exec(`UPDATE featured_slots SET reveal_at=statement_timestamp() WHERE problem_id=$1`, a.ID)
	if !strings.Contains(f.request("GET", "/problems/"+a.ID, "", nil, 200), "SECRET EDITORIAL") {
		t.Fatal("editorial did not unlock")
	}
	f.request("GET", "/images/"+imageID, "", nil, 200)
	f.request("GET", "/problems/"+a.ID+"/submissions/"+sid, "", nil, 200)
	if !strings.Contains(f.request("GET", "/problems/"+a.ID+"/submissions", "", nil, 200), sid) {
		t.Fatal("list did not unlock")
	}
}

func TestFeaturedFallbackAndFairnessPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	a, b, c := f.draft("alice", 1), f.draft("bob", 2), f.draft("alice", 3)
	f.apply(a, "soon", 204)
	f.apply(b, "later", 204)
	f.apply(c, "soon", 204)
	at := f.due()
	// Two earlier new Easy slots make this the guaranteed 'later' turn.
	f.exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,difficulty,reveal_at) VALUES($1,'easy','new',1,$1),($2,'easy','new',2,$2)`, at.Add(-7*24*time.Hour), at.Add(-4*24*time.Hour))
	f.request("GET", "/featured", "", nil, 200)
	var selected, kind string
	if err := f.store.Pool().QueryRow(context.Background(), `SELECT problem_id::text,kind FROM featured_slots WHERE scheduled_at=$1 AND slot='easy'`, at).Scan(&selected, &kind); err != nil || selected != b.ID || kind != "new" {
		t.Fatal("later queue starved", selected, kind, err)
	}
	// Revalidate changed drafts. With no valid new work, choose a revival in the same slot.
	f.exec(`DELETE FROM featured_slots`)
	f.exec(`UPDATE problem_drafts SET draft=draft-'editorial' WHERE id=$1`, a.ID)
	f.exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{difficulty}','8') WHERE id=$1`, c.ID)
	at = f.due()
	f.request("GET", "/featured", "", nil, 200)
	if err := f.store.Pool().QueryRow(context.Background(), `SELECT problem_id::text,kind FROM featured_slots WHERE scheduled_at=$1 AND slot='easy'`, at).Scan(&selected, &kind); err != nil || selected != b.ID || kind != "revival" {
		t.Fatal("wrong revival", selected, kind, err)
	}
	if err := f.store.Pool().QueryRow(context.Background(), `SELECT problem_id::text,kind FROM featured_slots WHERE scheduled_at=$1 AND slot='hard'`, at).Scan(&selected, &kind); err != nil || selected != c.ID || kind != "new" {
		t.Fatal("difficulty not revalidated", selected, kind, err)
	}
	f.exec(`DELETE FROM featured_slots`)
	f.exec(`DELETE FROM featured_applications`)
	f.exec(`UPDATE problem_drafts SET published_draft=NULL WHERE id=$1`, c.ID)
	at = f.due()
	f.request("GET", "/featured", "", nil, 200)
	if err := f.store.Pool().QueryRow(context.Background(), `SELECT kind FROM featured_slots WHERE scheduled_at=$1 AND slot='hard'`, at).Scan(&kind); err != nil || kind != "missing" {
		t.Fatal("wrong difficulty filled empty slot", kind, err)
	}
	// Recent-only inventory is still usable, and revival never hides the editorial.
	f.exec(`UPDATE featured_schedule SET next_at=$1`, at.Add(time.Second))
	f.request("GET", "/featured", "", nil, 200)
	if !strings.Contains(f.request("GET", "/problems/"+b.ID, "", nil, 200), "SECRET EDITORIAL") {
		t.Fatal("revival hid editorial")
	}
	// An outage past the reveal deadline records missing slots without consuming inventory.
	f.exec(`DELETE FROM featured_slots`)
	f.exec(`UPDATE featured_schedule SET next_at=$1`, time.Now().Add(-24*time.Hour))
	f.request("GET", "/featured", "", nil, 200)
	var nonmissing int
	if err := f.store.Pool().QueryRow(context.Background(), `SELECT count(*) FROM featured_slots WHERE kind<>'missing'`).Scan(&nonmissing); err != nil || nonmissing != 0 {
		t.Fatal("late recovery consumed inventory", nonmissing, err)
	}
}

func (f featuredTest) page(path string) problems.FeaturedPage {
	f.t.Helper()
	var result problems.FeaturedPage
	if err := json.Unmarshal([]byte(f.request("GET", path, "", nil, 200)), &result); err != nil {
		f.t.Fatal(err)
	}
	return result
}

func TestFeaturedPreviewPrivacyAndReleasePostgres(t *testing.T) {
	f := newFeaturedTest(t)
	a, b := f.draft("alice", 4), f.draft("bob", 9)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, a.ID)
	f.apply(a, "later", 204)
	f.apply(b, "soon", 204)
	first := f.page("/featured")
	if first.Current != nil || first.Waiting.Easy != 1 || first.Waiting.Hard != 1 || len(first.NextSlots) != 2 || first.NextSlots[0].Writer != "alice" || !reflect.DeepEqual(first.NextSlots[0].Testers, []string{"tester"}) || *first.NextSlots[1].Difficulty != 9 {
		t.Fatal(first)
	}
	for _, viewer := range []string{"", "alice", "tester"} {
		raw := f.request("GET", "/featured", viewer, nil, 200)
		for _, secret := range []string{a.ID, b.ID, "Featured problem", "Statement", "SECRET", "problemId", "title"} {
			if strings.Contains(raw, secret) {
				t.Fatalf("preview leaked %q: %s", secret, raw)
			}
		}
	}
	c := f.draft("bob", 2)
	f.apply(c, "soon", 204)
	// New applications do not reshuffle a valid reserved new problem.
	next := f.page("/featured")
	if !reflect.DeepEqual(first.NextSlots, next.NextSlots) || next.Waiting.Easy != 2 {
		t.Fatal(next)
	}
	at := f.due()
	f.exec(`UPDATE featured_previews SET scheduled_at=$1`, at)
	current := f.page("/featured")
	if current.Current == nil || current.Current.Slots[0].ProblemID != a.ID || current.Current.Slots[1].ProblemID != b.ID || current.Waiting.Easy != 1 || current.Waiting.Hard != 0 || current.Current.Slots[0].Title != "Featured problem" {
		t.Fatal(current)
	}
	older := f.page("/featured?offset=20")
	if len(older.Items) != 0 || !reflect.DeepEqual(current.Current, older.Current) || !reflect.DeepEqual(current.NextSlots, older.NextSlots) {
		t.Fatal(older)
	}
	// Immediately before the 23-hour boundary the published edition is still primary.
	f.exec(`UPDATE featured_slots SET scheduled_at=clock_timestamp()-interval '23 hours'+interval '10 seconds'`)
	if f.page("/featured").Current == nil {
		t.Fatal("current edition ended early")
	}
	f.exec(`UPDATE featured_slots SET scheduled_at=clock_timestamp()-interval '23 hours'`)
	if f.page("/featured").Current != nil {
		t.Fatal("current edition remained after the reveal boundary")
	}
}

func TestFeaturedPreviewRevalidationPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	a, b, c := f.draft("alice", 4), f.draft("bob", 2), f.draft("alice", 8)
	f.apply(a, "soon", 204)
	f.apply(b, "later", 204)
	f.apply(c, "soon", 204)
	if f.page("/featured").NextSlots[0].Writer != "alice" {
		t.Fatal("wrong initial selection")
	}
	f.apply(a, "", 204)
	next := f.page("/featured")
	if next.NextSlots[0].Writer != "bob" || next.Waiting.Easy != 1 {
		t.Fatal("withdrawal retained preview", next)
	}
	f.exec(`UPDATE problem_drafts SET draft=draft-'editorial' WHERE id=$1`, b.ID)
	f.exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{difficulty}','3') WHERE id=$1`, c.ID)
	next = f.page("/featured")
	if next.NextSlots[0].Writer != "alice" || next.NextSlots[1].Kind != "missing" || next.Waiting.Easy != 1 || next.Waiting.Hard != 0 {
		t.Fatal("draft edits not reflected", next)
	}
	f.request("PUT", "/my/problems/"+c.ID+"/publication", "alice", map[string]any{"version": 1, "publish": true}, 200)
	next = f.page("/featured")
	if next.NextSlots[0].Kind != "revival" || next.Waiting.Easy != 0 {
		t.Fatal("public problem counted as waiting", next)
	}
	for range 4 {
		if got := f.page("/featured"); !reflect.DeepEqual(got.NextSlots, next.NextSlots) {
			t.Fatal("unstable revival", got)
		}
	}
	// A new application replaces a revival, and deletion removes the reservation.
	f.apply(a, "soon", 204)
	if f.page("/featured").NextSlots[0].Kind != "new" {
		t.Fatal("revival displaced new work")
	}
	f.exec(`DELETE FROM problem_drafts WHERE id=$1`, a.ID)
	if f.page("/featured").NextSlots[0].Kind != "revival" {
		t.Fatal("deleted draft retained preview")
	}
}

func TestProblemListPublicationPlansPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	a, b, c := f.draft("alice", 3), f.draft("alice", 6), f.draft("alice", 2)
	f.apply(a, "soon", 204)
	f.apply(b, "later", 204)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, a.ID)
	cid := newSubmissionID()
	f.request("PUT", "/my/contests/"+cid, "alice", map[string]any{"version": 0, "title": "Contest", "description": "", "startsAt": time.Now().Add(time.Hour), "endsAt": time.Now().Add(2 * time.Hour), "penaltyMinutes": 5, "problems": []map[string]any{{"id": c.ID, "points": 100}}}, 200)
	list := func(owner, path string) map[string]problems.Summary {
		t.Helper()
		var result struct {
			Items []problems.Summary `json:"items"`
		}
		if err := json.Unmarshal([]byte(f.request("GET", path, owner, nil, 200)), &result); err != nil {
			t.Fatal(err)
		}
		items := make(map[string]problems.Summary)
		for _, item := range result.Items {
			items[item.ID] = item
		}
		return items
	}
	items := list("alice", "/my/problems")
	if items[a.ID].FeaturedPreference != "soon" || items[b.ID].FeaturedPreference != "later" || !items[c.ID].ContestScheduled || items[c.ID].ContestID != cid {
		t.Fatal(items)
	}
	if tested := list("tester", "/my/problems?role=tester"); len(tested) != 1 || tested[a.ID].FeaturedPreference != "soon" {
		t.Fatal(tested)
	}
	if other := list("bob", "/my/problems"); len(other) != 0 {
		t.Fatal("other author's plans leaked", other)
	}
	f.apply(a, "", 204)
	f.request("PUT", "/my/problems/"+b.ID+"/publication", "alice", map[string]any{"version": b.Version, "publish": true}, 200)
	f.exec(`UPDATE contests SET released=true WHERE id=$1`, cid)
	items = list("alice", "/my/problems")
	if items[a.ID].FeaturedPreference != "" || items[b.ID].FeaturedPreference != "" || items[b.ID].PublishedVersion == 0 || items[c.ID].ContestScheduled || items[c.ID].ContestID != cid {
		t.Fatal(items)
	}
}
