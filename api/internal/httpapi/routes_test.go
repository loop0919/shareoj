package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"judge/api/internal/contests"
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
