package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"judge/api/internal/contests"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
)

func TestCreationQuotasPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	ctx := context.Background()
	sign := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{
		Store: f.store, Posts: posts.New(f.store.Pool()), Contests: &contests.Store{Pool: f.store.Pool()},
		Verifier: newCognitoVerifier(sign.server.URL, "client"),
	})
	tokens := map[string]string{"alice": sign.token(t, "alice", nil), "bob": sign.token(t, "bob", nil)}
	request := func(method, path, owner string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+tokens[owner])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	difficulty := 3
	draft := problems.Draft{Difficulty: &difficulty, Title: "Quota test", Markdown: "Statement", Editorial: "Editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2"}}}
	for _, kind := range []string{"problem", "contest", "post"} {
		t.Run(kind, func(t *testing.T) {
			check := func(w *httptest.ResponseRecorder, want int) {
				t.Helper()
				if w.Code != want {
					t.Fatalf("got %d, want %d: %s", w.Code, want, w.Body.String())
				}
				if want == 429 {
					seconds, err := strconv.Atoi(w.Header().Get("Retry-After"))
					if err != nil || seconds < 1 || seconds > 7200 || !strings.Contains(w.Body.String(), "creation_quota_exceeded") {
						t.Fatalf("missing quota response: %v %s", w.Header(), w.Body.String())
					}
				}
			}
			path := "/my/" + kind + "s/"
			input := func(owner string) map[string]any {
				switch kind {
				case "problem":
					return map[string]any{"version": 0, "draft": draft}
				case "contest":
					// Independent eligible problems avoid consuming the problem quota.
					pid := newSubmissionID()
					data, _ := json.Marshal(draft)
					f.exec(`INSERT INTO problem_drafts(id,owner_id,draft) VALUES($1,$2,$3)`, pid, owner, data)
					return map[string]any{"version": 0, "title": "Quota test", "startsAt": time.Now().Add(time.Hour), "endsAt": time.Now().Add(2 * time.Hour), "problems": []contests.Problem{{ID: pid, Points: 100}}}
				default:
					return map[string]any{"version": 0, "title": "Quota test", "markdown": "Body"}
				}
			}
			first, firstInput := newSubmissionID(), input("alice")
			check(request("PUT", path+first, "alice", firstInput), 200)
			// A conflicting create must roll back its charge.
			check(request("PUT", path+first, "alice", firstInput), 409)
			var wg sync.WaitGroup
			results := make(chan *httptest.ResponseRecorder, 24)
			for range 24 {
				id, body := newSubmissionID(), input("alice")
				wg.Go(func() { results <- request("PUT", path+id, "alice", body) })
			}
			wg.Wait()
			close(results)
			success := 0
			for w := range results {
				if w.Code == 200 {
					success++
				} else {
					check(w, 429)
				}
			}
			if success != 19 {
				t.Fatalf("concurrent creations: got %d successes after first, want 19", success)
			}
			firstInput["version"] = 1
			check(request("PUT", path+first, "alice", firstInput), 200)
			if kind != "contest" {
				check(request("DELETE", path+first+"?version=2", "alice", nil), 204)
			}
			next, nextInput := newSubmissionID(), input("alice")
			check(request("PUT", path+next, "alice", nextInput), 429)
			check(request("GET", path+next, "alice", nil), 404)
			// Accounts and content kinds have independent buckets.
			check(request("PUT", path+newSubmissionID(), "bob", input("bob")), 200)
			// Exhaustion lasts until the next slot, and rejection must not delay it.
			f.exec(`UPDATE creation_quotas SET full_at=clock_timestamp()+interval '38 hours 30 seconds' WHERE owner_id='alice' AND kind=$1`, kind)
			var before, after time.Time
			if err := f.store.Pool().QueryRow(ctx, `SELECT full_at FROM creation_quotas WHERE owner_id='alice' AND kind=$1`, kind).Scan(&before); err != nil {
				t.Fatal(err)
			}
			w := request("PUT", path+next, "alice", nextInput)
			check(w, 429)
			seconds, _ := strconv.Atoi(w.Header().Get("Retry-After"))
			if seconds > 30 {
				t.Fatalf("retry after = %d, want at most 30", seconds)
			}
			if err := f.store.Pool().QueryRow(ctx, `SELECT full_at FROM creation_quotas WHERE owner_id='alice' AND kind=$1`, kind).Scan(&after); err != nil || !before.Equal(after) {
				t.Fatalf("rejection changed refill time: %v -> %v: %v", before, after, err)
			}
			f.exec(`UPDATE creation_quotas SET full_at=clock_timestamp()+interval '38 hours' WHERE owner_id='alice' AND kind=$1`, kind)
			check(request("PUT", path+next, "alice", nextInput), 200)
			check(request("PUT", path+newSubmissionID(), "alice", input("alice")), 429)
			// Long inactivity refills to 20, never beyond it.
			f.exec(`UPDATE creation_quotas SET full_at=clock_timestamp()-interval '10 days' WHERE owner_id='alice' AND kind=$1`, kind)
			for range 20 {
				check(request("PUT", path+newSubmissionID(), "alice", input("alice")), 200)
			}
			check(request("PUT", path+newSubmissionID(), "alice", input("alice")), 429)
		})
	}
}
