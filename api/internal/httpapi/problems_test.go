package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"judge/api/internal/httpapi"
)

func TestPublicProblemErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/problems/11111111-1111-4111-8111-111111111111", http.StatusServiceUnavailable},
		{http.MethodGet, "/problems/missing", http.StatusNotFound},
		{http.MethodGet, "/problems/11111111-1111-4111-8111-111111111111/samples", http.StatusServiceUnavailable},
		{http.MethodGet, "/problems/missing/samples", http.StatusNotFound},
		{http.MethodPost, "/problems/11111111-1111-4111-8111-111111111111/samples", http.StatusMethodNotAllowed},
		{http.MethodGet, "/problems/A-PLUS-B", http.StatusNotFound},
		{http.MethodPost, "/problems/a-plus-b", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRecorder()
			httpapi.NewHandler().ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, nil))
			if r.Code != tc.status {
				t.Fatalf("status = %d, want %d", r.Code, tc.status)
			}
		})
	}
}
