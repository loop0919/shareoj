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

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

func TestContestDraftsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_contest_drafts_%d", time.Now().UnixNano())
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
	if _, err = store.Pool().Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob')`); err != nil {
		t.Fatal(err)
	}
	const ready = "11111111-1111-4111-8111-111111111111"
	const second = "22222222-2222-4222-8222-222222222222"
	const incomplete = "33333333-3333-4333-8333-333333333333"
	const missing = "44444444-4444-4444-8444-444444444444"
	difficulty := 3
	complete := problems.Draft{Difficulty: &difficulty, Title: "Ready", Markdown: "statement", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
	for _, pid := range []string{ready, second} {
		if _, err = store.Save(ctx, "alice", pid, 0, complete); err != nil {
			t.Fatal(err)
		}
	}
	unfinished := complete
	unfinished.Difficulty = nil
	if _, err = store.Save(ctx, "alice", incomplete, 0, unfinished); err != nil {
		t.Fatal(err)
	}
	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(f.server.URL, "client")})
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
	saveDraft := func(id string, version int64, draft map[string]any, want int) contests.DraftResult {
		t.Helper()
		var result contests.DraftResult
		body := request("PUT", "/my/contests/"+id+"/draft", "alice", map[string]any{"version": version, "draft": draft}, want)
		if want == 200 {
			if err := json.Unmarshal([]byte(body), &result); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	const id = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"

	// An empty draft saves, and the response lists everything publication still needs.
	saved := saveDraft(id, 0, map[string]any{"title": "", "description": "Secret plan"}, 200)
	if saved.Version != 1 || saved.Draft.Description != "Secret plan" || !slices.Equal(saved.Issues, []string{"title_missing", "start_missing", "duration_invalid", "penalty_invalid", "problems_missing"}) {
		t.Fatal("empty draft", saved)
	}
	request("PUT", "/my/contests/"+id+"/draft", "", map[string]any{"version": 0, "draft": map[string]any{}}, 401)
	request("PUT", "/my/contests/"+id+"/draft", "alice", map[string]any{"version": 1, "draft": map[string]any{"problems": []map[string]any{{"id": ready}, {"id": ready}}}}, 400)
	request("PUT", "/my/contests/"+id+"/draft", "alice", map[string]any{"version": 1, "draft": map[string]any{"title": strings.Repeat("a", 121)}}, 400)

	// Drafts never reach contest routes, and other users cannot read or take over the id.
	if body := request("GET", "/contests", "", nil, 200); strings.Contains(body, id) || strings.Contains(body, "Secret plan") {
		t.Fatal("draft listed publicly", body)
	}
	request("GET", "/contests/"+id, "", nil, 404)
	request("GET", "/contests/"+id+"/standings", "", nil, 404)
	request("GET", "/my/contests/"+id, "alice", nil, 404)
	request("POST", "/my/contests/"+id+"/participation", "bob", nil, 404)
	request("GET", "/my/contests/"+id+"/draft", "bob", nil, 404)
	request("PUT", "/my/contests/"+id+"/draft", "bob", map[string]any{"version": 0, "draft": map[string]any{}}, 409)
	request("PUT", "/my/contests/"+id+"/draft", "bob", map[string]any{"version": 1, "draft": map[string]any{}}, 404)
	request("DELETE", "/my/contests/"+id+"/draft?version=1", "bob", nil, 404)
	request("PUT", "/my/contests/"+id+"/publication", "bob", map[string]any{"version": 1}, 404)

	// Stale versions are rejected without overwriting the saved draft.
	saveDraft(id, 0, map[string]any{}, 409)
	saveDraft(id, 5, map[string]any{}, 409)

	// Out-of-range values and unusable problems are saved and reported, and block publication.
	past := time.Now().Add(-time.Hour)
	saved = saveDraft(id, 1, map[string]any{"title": "Draft", "startsAt": past, "durationMinutes": 0, "penaltyMinutes": 2000,
		"problems": []map[string]any{{"id": ready}, {"id": incomplete, "points": 100}, {"id": missing, "points": 100}}}, 200)
	if saved.Version != 2 || len(saved.Draft.Problems) != 3 || saved.Draft.Problems[0].Points != nil || !slices.Equal(saved.Issues, []string{"start_past", "duration_invalid", "penalty_invalid", "points_invalid", "problem_unavailable", "problem_incomplete"}) {
		t.Fatal("invalid draft", saved)
	}
	if body := request("GET", "/my/contests/"+id+"/draft", "alice", nil, 200); !strings.Contains(body, "problem_incomplete") {
		t.Fatal("reloaded draft lost its issues", body)
	}
	if body := request("PUT", "/my/contests/"+id+"/publication", "alice", map[string]any{"version": 2}, 409); !strings.Contains(body, "contest_not_ready") {
		t.Fatal(body)
	}

	// A complete draft publishes under the same id: start plus duration becomes the schedule.
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	saved = saveDraft(id, 2, map[string]any{"title": "Draft contest", "description": "Rules", "startsAt": start, "durationMinutes": 90, "penaltyMinutes": 0,
		"problems": []map[string]any{{"id": second, "points": 200}, {"id": ready, "points": 100}}}, 200)
	if len(saved.Issues) != 0 {
		t.Fatal("complete draft has issues", saved.Issues)
	}
	var drafts offsetResponse[contests.DraftSummary]
	if err := json.Unmarshal([]byte(request("GET", "/my/contests/drafts", "alice", nil, 200)), &drafts); err != nil {
		t.Fatal(err)
	}
	if len(drafts.Items) != 1 || drafts.Items[0].ID != id || drafts.Items[0].Title != "Draft contest" || drafts.Items[0].StartsAt == nil || !drafts.Items[0].StartsAt.Equal(start) {
		t.Fatal("draft list", drafts)
	}
	if body := request("GET", "/my/contests/drafts", "bob", nil, 200); strings.Contains(body, id) {
		t.Fatal("another user's draft listed", body)
	}
	request("PUT", "/my/contests/"+id+"/publication", "alice", map[string]any{"version": 2}, 409)
	var published contests.Contest
	if err := json.Unmarshal([]byte(request("PUT", "/my/contests/"+id+"/publication", "alice", map[string]any{"version": 3}, 200)), &published); err != nil {
		t.Fatal(err)
	}
	if published.Status != "scheduled" || published.Title != "Draft contest" || published.PenaltyMinutes != 0 || !published.StartsAt.Equal(start) ||
		!published.EndsAt.Equal(start.Add(90*time.Minute)) || len(published.Problems) != 2 || published.Problems[0].ID != second || published.Problems[0].Points != 200 {
		t.Fatal("published contest", published)
	}
	request("GET", "/contests/"+id, "", nil, 200)
	request("GET", "/my/contests/"+id+"/draft", "alice", nil, 404)
	if body := request("GET", "/my/contests/drafts", "alice", nil, 200); strings.Contains(body, id) {
		t.Fatal("published draft still listed", body)
	}
	// The published contest keeps its id; drafts cannot be created over it, and stale editors get 409.
	saveDraft(id, 0, map[string]any{}, 409)
	saveDraft(id, 3, map[string]any{}, 409)

	// Problems in a published contest are unavailable to other drafts.
	const another = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	saved = saveDraft(another, 0, map[string]any{"title": "Another", "problems": []map[string]any{{"id": ready, "points": 100}}}, 200)
	if !slices.Contains(saved.Issues, "problem_unavailable") {
		t.Fatal("reserved problem offered again", saved.Issues)
	}
	request("DELETE", "/my/contests/"+another+"/draft?version=2", "alice", nil, 409)
	request("DELETE", "/my/contests/"+another+"/draft", "alice", nil, 400)
	request("DELETE", "/my/contests/"+another+"/draft?version=1", "alice", nil, 204)
	request("DELETE", "/my/contests/"+another+"/draft?version=1", "alice", nil, 404)
}
