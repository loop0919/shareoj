package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"judge/api/internal/httpapi"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	httpapi.NewHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	if got := recorder.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q, want %q", got, "{\"status\":\"ok\"}\n")
	}
}

func TestHealthRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/health", nil)

	httpapi.NewHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestHealthHead(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	httpapi.NewHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	httpapi.NewHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/auth/login", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD /auth/login: status code = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}
