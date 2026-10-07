package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateNotice(t *testing.T) {
	originalVersion, originalTransport := version, http.DefaultTransport
	t.Cleanup(func() { version, http.DefaultTransport = originalVersion, originalTransport })
	for _, tc := range []struct {
		name, current, remote, cached string
		args                          []string
		fresh, disabled, timeout      bool
		status, requests              int
		want                          string
	}{
		{name: "new-minor", current: "cli-v0.9.9", remote: "cli-v0.10.0", requests: 1, want: "cli-v0.10.0"},
		{name: "new-major", current: "cli-v0.99.99", remote: "cli-v1.0.0", requests: 1, want: "cli-v1.0.0"},
		{name: "new-patch", remote: "cli-v0.9.1", requests: 1, want: "cli-v0.9.1"},
		{name: "same", remote: "cli-v0.9.0", requests: 1},
		{name: "older", remote: "cli-v0.8.99", requests: 1},
		{name: "prerelease", remote: "cli-v0.10.0-rc1", requests: 1},
		{name: "wrong-tag", remote: "v0.10.0", requests: 1},
		{name: "overflow", remote: "cli-v9999999999999999999999999.0.0", requests: 1},
		{name: "external-redirect", remote: "https://example.org/cli-v0.10.0", requests: 1},
		{name: "http-error", status: http.StatusServiceUnavailable, requests: 1},
		{name: "timeout", timeout: true, requests: 1},
		{name: "cached", cached: "cli-v0.10.0", fresh: true, want: "cli-v0.10.0"},
		{name: "refresh", cached: "cli-v0.9.1", remote: "cli-v0.10.0", requests: 1, want: "cli-v0.10.0"},
		{name: "failed-refresh", cached: "cli-v0.10.0", status: http.StatusServiceUnavailable, requests: 1, want: "cli-v0.10.0"},
		{name: "offline-guide", args: []string{"guide"}, cached: "cli-v0.10.0", want: "cli-v0.10.0"},
		{name: "offline-init", args: []string{"init", "problem"}},
		{name: "offline-check", args: []string{"check", "problem"}},
		{name: "offline-logout", args: []string{"logout"}},
		{name: "help", args: []string{"push", "--help"}, cached: "cli-v0.10.0"},
		{name: "help-single-dash", args: []string{"push", "-help"}, cached: "cli-v0.10.0"},
		{name: "version", args: []string{"version"}, cached: "cli-v0.10.0"},
		{name: "disabled", disabled: true, cached: "cli-v0.10.0"},
		{name: "development", current: "dev", cached: "cli-v0.10.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", dir)
			t.Setenv("HOME", dir)
			t.Setenv("LocalAppData", dir)
			t.Setenv("SHAREOJ_NO_UPDATE_CHECK", "")
			if tc.disabled {
				t.Setenv("SHAREOJ_NO_UPDATE_CHECK", "1")
			}
			version = tc.current
			if version == "" {
				version = "cli-v0.9.0"
			}
			cacheDir, err := os.UserCacheDir()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cacheDir, "shareoj", "update.json")
			if tc.cached != "" {
				checkedAt := time.Now().Add(-25 * time.Hour)
				if tc.fresh {
					checkedAt = time.Now()
				}
				if err := atomicJSON(path, updateCache{checkedAt, tc.cached}); err != nil {
					t.Fatal(err)
				}
			}
			requests := 0
			http.DefaultTransport = updateTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.Method != http.MethodHead || req.URL.String() != cliReleases+"/latest" || req.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected update request: %s %s", req.Method, req.URL)
				}
				deadline, ok := req.Context().Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Fatal("update request must have a one-second deadline")
				}
				if tc.timeout {
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				status := tc.status
				if status == 0 {
					status = http.StatusFound
				}
				location := cliReleases + "/tag/" + tc.remote
				if strings.HasPrefix(tc.remote, "https://") {
					location = tc.remote
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": {location}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
			})
			args := tc.args
			if args == nil {
				args = []string{"push", "problem"}
			}
			// A second command reuses even a failed check, without another request.
			for range 2 {
				var out strings.Builder
				notifyUpdate(context.Background(), args, &out)
				if tc.want == "" {
					if out.Len() != 0 {
						t.Fatalf("unexpected notice: %s", &out)
					}
				} else if !strings.Contains(out.String(), "Update available: "+tc.want) || !strings.Contains(out.String(), "https://") {
					t.Fatalf("missing version or update instructions: %s", &out)
				}
			}
			if requests != tc.requests {
				t.Fatalf("requests = %d, want %d", requests, tc.requests)
			}
		})
	}
}
