package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"judge/api/internal/contests"
	"judge/api/internal/submissions"
)

type staticVerifier struct{}

func (staticVerifier) Verify(context.Context, string) (string, error) { return "alice", nil }

func TestUnrelatedRoutesDoNotReleaseContests(t *testing.T) {
	// This store panics if used: unrelated endpoints must not touch contest storage.
	h := newHandler(AuthConfig{}, handlerDependencies{Contests: &contests.Store{}, Verifier: staticVerifier{}})
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/auth/me", `"id":"alice"`, 200},
		{"GET", "/my/profile", "database_unavailable", 503},
		{"GET", "/my/posts", "database_unavailable", 503},
		{"GET", "/my/images", "database_unavailable", 503},
		{"GET", "/my/notifications", "database_unavailable", 503},
		{"GET", "/my/submissions", "judging_unavailable", 503},
		{"GET", "/my/unknown", "404", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	// Authorization runs before maintenance, even on release-dependent routes.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/my/problems", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated request: %d", w.Code)
	}
}

// TestProblemRoutesWithoutDatabase pins which input checks come before database_unavailable on the problem and
// profile routes when no store is configured.
func TestProblemRoutesWithoutDatabase(t *testing.T) {
	// Without a contest store, release-dependent routes skip the release.
	h := newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}})
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		method, path string
		status       int
		code         string
	}{
		{"GET", "/featured", 503, "database_unavailable"},
		{"GET", "/featured?offset=-1", 503, "database_unavailable"},
		{"GET", "/featured/schedule", 503, "database_unavailable"},
		{"GET", "/featured/standings?at=bad", 400, "invalid_request"},
		{"GET", "/featured/standings?at=2026-10-19T14:00:00Z&offset=-1", 400, "invalid_request"},
		{"GET", "/featured/standings?at=2026-10-19T14:00:00Z", 503, "database_unavailable"},
		{"GET", "/problems?author=BAD", 400, "invalid_author"},
		{"GET", "/problems/bad?author=BAD", 400, "invalid_author"},
		{"GET", "/problems/bad", 404, "not_found"},
		{"GET", "/problems?cursor=bad", 503, "database_unavailable"},
		{"GET", "/problems/" + id, 503, "database_unavailable"},
		{"GET", "/problems/bad/samples", 404, "not_found"},
		{"GET", "/problems/" + id + "/samples", 503, "database_unavailable"},
		{"GET", "/users/BAD", 404, "not_found"},
		{"GET", "/users/alice", 503, "database_unavailable"},
		{"GET", "/my/solved-problems", 503, "database_unavailable"},
		{"GET", "/my/favorites/bad", 404, "not_found"},
		{"PUT", "/my/favorites/" + id, 503, "database_unavailable"},
		{"PUT", "/my/difficulty-votes/bad", 404, "problem_not_found"},
		{"PUT", "/my/difficulty-votes/" + id, 503, "database_unavailable"},
		{"GET", "/my/featured", 503, "database_unavailable"},
		{"PUT", "/my/problems/bad/featured", 503, "database_unavailable"},
		{"GET", "/my/featured/schedule?problem=bad", 404, "problem_not_found"},
		{"GET", "/my/featured/schedule?preference=x", 400, "invalid_request"},
		{"GET", "/my/featured/schedule", 503, "database_unavailable"},
		{"GET", "/my/tester-invitations/bad", 503, "database_unavailable"},
		{"POST", "/my/problems/bad/tester-invitation", 503, "database_unavailable"},
		{"GET", "/my/problems?role=tester", 503, "database_unavailable"},
		{"GET", "/my/problems?role=x", 503, "database_unavailable"},
		{"GET", "/my/problems?cursor=bad", 503, "database_unavailable"},
		{"GET", "/my/problems/bad", 503, "database_unavailable"},
		{"GET", "/my/problems/bad/publication", 404, "not_found"},
		{"GET", "/my/problems/" + id + "/publication", 503, "database_unavailable"},
		{"PUT", "/my/problems/bad/publication", 404, "problem_not_found"},
		{"PUT", "/my/problems/" + id + "/publication", 503, "database_unavailable"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			// No body is sent: where a route reads one, the store check comes first.
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expect(t, w, tc.status, tc.code)
		})
	}
}

// TestGenerationWithoutProblemStore pins that a generation is refused, not run against a nil problem store.
func TestGenerationWithoutProblemStore(t *testing.T) {
	// The submission store is never used: the generation stops before it.
	h := newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}, Submissions: &submissions.Store{}, JudgeImage: "sha256:" + strings.Repeat("a", 64)})
	r := httptest.NewRequest("POST", "/my/submissions", strings.NewReader(`{"problemId":"11111111-1111-4111-8111-111111111111","runtime":"cpp23-gcc-local","source":"x","generation":{"mode":"input","start":1,"count":1}}`))
	r.Header.Set("Authorization", "Bearer token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	expect(t, w, 503, "database_unavailable")
}

// A judge that is not ready refuses a submission before its content type is checked.
func TestSubmissionReadinessBeforeContentType(t *testing.T) {
	h := newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}, Submissions: &submissions.Store{}})
	r := httptest.NewRequest("POST", "/my/submissions", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	expect(t, w, 503, "judging_unavailable")
}

func TestAuthenticationPreservesHandlerDeadline(t *testing.T) {
	for _, timeout := range []time.Duration{10 * time.Second, 25 * time.Second} {
		called := false
		auth := authentication{Verifier: staticVerifier{}}
		h := auth.require(func(w http.ResponseWriter, r *http.Request, owner string) {
			called = true
			deadline, ok := r.Context().Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining > timeout || remaining < timeout-time.Second || owner != "alice" {
				t.Fatalf("deadline=%v owner=%s", remaining, owner)
			}
		}, true, timeout)
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer token")
		h(httptest.NewRecorder(), r)
		if !called {
			t.Fatal("handler not called")
		}
	}
}
