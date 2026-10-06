package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func example(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "problem")
	if err := initProblem(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPushLifecycle(t *testing.T) {
	dir := example(t)
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "test-token")
	ctx := context.Background()
	var saved problem
	var pending testFile
	var uploaded string
	requests, uploads, puts := 0, 0, 0
	failUpload, raceSave, lostResponse := false, false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/object" {
			if r.Header.Get("Authorization") != "" {
				t.Error("access token leaked to storage")
			}
			if failUpload {
				w.WriteHeader(503)
				return
			}
			data, _ := io.ReadAll(r.Body)
			uploaded = string(data)
			sum := sha256.Sum256(data)
			if len(data) != pending.Size || fmt.Sprintf("%x", sum) != pending.SHA256 || r.Header.Get("X-Amz-Checksum-Sha256") != base64.StdEncoding.EncodeToString(sum[:]) {
				t.Error("upload/checksum mismatch")
			}
			w.WriteHeader(200)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing API token")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/test-files"):
			uploads++
			if err := json.NewDecoder(r.Body).Decode(&pending); err != nil {
				t.Error(err)
			}
			pending.ID = "00000000-0000-4000-8000-000000000001"
			checksum, _ := hex.DecodeString(pending.SHA256)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": pending.ID, "url": "http://" + r.Host + "/object", "headers": map[string]string{"x-amz-checksum-sha256": base64.StdEncoding.EncodeToString(checksum)}})
		case strings.HasSuffix(r.URL.Path, "/complete"):
			_ = json.NewEncoder(w).Encode(pending)
		case r.Method == "GET":
			if saved.ID == "" {
				w.WriteHeader(404)
				fmt.Fprint(w, `{"error":"problem_not_found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
		case r.Method == "PUT":
			puts++
			var input problem
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if input.Version != saved.Version || raceSave {
				w.WriteHeader(409)
				fmt.Fprint(w, `{"error":"version_conflict"}`)
				return
			}
			saved = problem{ID: strings.TrimPrefix(r.URL.Path, "/my/problems/"), Version: input.Version + 1, Draft: input.Draft}
			if lostResponse {
				w.WriteHeader(503)
				fmt.Fprint(w, `{"error":"database_unavailable"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	client, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	s, err := push(ctx, dir, client, -1)
	if err != nil || s.Version != 1 || !problemID.MatchString(s.ID) {
		t.Fatalf("create: %+v %v", s, err)
	}
	var cases []testCase
	_ = json.Unmarshal(saved.Draft["testCases"], &cases)
	if len(cases) != 1 || cases[0].Input != "3 5\n" || !cases[0].IsSample {
		t.Fatalf("sample not preserved: %+v", cases)
	}
	var generators map[string]program
	_ = json.Unmarshal(saved.Draft["generators"], &generators)
	if len(generators) != 3 || generators["input"].Source == "" {
		t.Fatal("missing generators")
	}
	saved.Draft.set("editorial", "web editorial")
	large := strings.Repeat("12345\r\n", 10000)
	write(t, filepath.Join(dir, "tests/sample1.in"), large)
	s, err = push(ctx, dir, client, -1)
	if err != nil || s.Version != 2 || uploads != 1 || uploaded != large {
		t.Fatalf("large upload: %+v %v uploads=%d", s, err, uploads)
	}
	_ = json.Unmarshal(saved.Draft["testCases"], &cases)
	if cases[0].Input != "" || cases[0].InputFile == nil || string(saved.Draft["editorial"]) != `"web editorial"` {
		t.Fatal("file reference or omitted field was lost")
	}
	// Upload failures must not advance the draft or the local version.
	failUpload = true
	beforePuts := puts
	if _, err := push(ctx, dir, client, -1); err == nil {
		t.Fatal("upload failure succeeded")
	}
	var local state
	_ = readJSON(filepath.Join(dir, stateName), &local)
	if puts != beforePuts || local.Version != 2 {
		t.Fatal("failed upload advanced draft")
	}
	failUpload = false
	// Omitting tests must preserve the remote references without re-uploading.
	write(t, filepath.Join(dir, "problem.toml"), strings.Split(template, "[tests]")[0])
	previousCases := string(saved.Draft["testCases"])
	s, err = push(ctx, dir, client, -1)
	if err != nil || s.Version != 3 || string(saved.Draft["testCases"]) != previousCases {
		t.Fatalf("preserve tests: %v", err)
	}
	saved.Version++
	if _, err := push(ctx, dir, client, -1); err == nil || !strings.Contains(err.Error(), "remote 4") {
		t.Fatalf("missing conflict: %v", err)
	}
	s, err = push(ctx, dir, client, 4)
	if err != nil || s.Version != 5 {
		t.Fatalf("reviewed update: %v", err)
	}
	raceSave = true
	if _, err := push(ctx, dir, client, -1); err == nil {
		t.Fatal("save race succeeded")
	}
	_ = readJSON(filepath.Join(dir, stateName), &local)
	if local.Version != 5 {
		t.Fatal("409 advanced local state")
	}
	before := requests
	write(t, filepath.Join(dir, "input.cpp"), "\x00")
	if _, err := push(ctx, dir, client, -1); err == nil || requests != before {
		t.Fatal("invalid local data reached server")
	}
	// If the first save succeeds remotely but its response fails, retry the same ID.
	dir = example(t)
	saved, raceSave, lostResponse = problem{}, false, true
	if _, err := push(ctx, dir, client, -1); err == nil {
		t.Fatal("lost response reported success")
	}
	if err := readJSON(filepath.Join(dir, stateName), &local); err != nil {
		t.Fatal(err)
	}
	if local.Version != 0 || local.ID != saved.ID {
		t.Fatal("lost response discarded identity")
	}
	lostResponse = false
	beforePuts = puts
	if _, err := push(ctx, dir, client, -1); err == nil || puts != beforePuts {
		t.Fatal("ambiguous save was repeated")
	}
	s, err = push(ctx, dir, client, 1)
	if err != nil || s.ID != local.ID || s.Version != 2 {
		t.Fatalf("recovery: %+v %v", s, err)
	}
	// Five 64 KiB sides exceed the 256 KiB inline budget; exact boundaries stay inline.
	for _, name := range []string{"sample1.in", "sample1.out", "second.in", "second.out", "third.in"} {
		write(t, filepath.Join(dir, "tests", name), strings.Repeat("a", 64<<10))
	}
	write(t, filepath.Join(dir, "tests/third.out"), "")
	beforeUploads := uploads
	if _, err := push(ctx, dir, client, -1); err != nil {
		t.Fatal(err)
	}
	if uploads != beforeUploads+1 {
		t.Fatalf("inline budget: got %d uploads", uploads-beforeUploads)
	}
}

func TestProblemValidationAndRime(t *testing.T) {
	dir := example(t)
	if err := initProblem(dir); err == nil {
		t.Fatal("init overwrote existing directory")
	}
	if err := os.Rename(filepath.Join(dir, "tests/sample1.out"), filepath.Join(dir, "tests/sample1.diff")); err != nil {
		t.Fatal(err)
	}
	rimeConfig := strings.Replace(template, `".out"`, `".diff"`, 1)
	write(t, filepath.Join(dir, "problem.toml"), rimeConfig)
	if _, err := loadProblem(dir); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"\n[unknown]\nx = 1\n", "\n[generators.typo]\nsource = 'input.cpp'\nruntime = 'cpp17'\n"} {
		write(t, filepath.Join(dir, "problem.toml"), rimeConfig+extra)
		if _, err := loadProblem(dir); err == nil {
			t.Fatal("unknown key accepted")
		}
	}
	write(t, filepath.Join(dir, "problem.toml"), strings.Replace(rimeConfig, `statement = "statement.md"`, `statement = "../outside.md"`, 1))
	write(t, filepath.Join(filepath.Dir(dir), "outside.md"), "private")
	if _, err := loadProblem(dir); err == nil {
		t.Fatal("path traversal accepted")
	}
	if runtime.GOOS != "windows" {
		write(t, filepath.Join(dir, "problem.toml"), rimeConfig)
		if err := os.Remove(filepath.Join(dir, "statement.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../outside.md", filepath.Join(dir, "statement.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := loadProblem(dir); err == nil {
			t.Fatal("symlink escape accepted")
		}
	}
}

func TestLoginRefreshAndRedirect(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/auth/") && r.Header.Get("Authorization") != "" {
			t.Error("token sent to auth endpoint")
		}
		switch r.URL.Path {
		case "/auth/login":
			fmt.Fprint(w, `{"challenge_name":"SOFTWARE_TOKEN_MFA","session":"session","challenge_parameters":{"USER_ID_FOR_SRP":"canonical"}}`)
		case "/auth/challenge":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["username"] != "canonical" || body["session"] != "session" || body["responses"].(map[string]any)["SOFTWARE_TOKEN_MFA_CODE"] != "secret" {
				t.Error("invalid MFA request")
			}
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"refresh","expires_in":3600}`)
		case "/auth/refresh":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["refresh_token"] != "refresh" {
				t.Error("invalid refresh token")
			}
			fmt.Fprint(w, `{"access_token":"renewed","refresh_token":"rotated","expires_in":3600}`)
		case "/redirect":
			http.Redirect(w, r, "/leak?secret=never-print", 302)
		default:
			t.Error("unexpected request or followed redirect")
		}
	}))
	defer server.Close()
	client, _ := newClient(server.URL)
	var prompts []string
	err := client.login(context.Background(), "email", func(label string, secret bool) (string, error) {
		prompts = append(prompts, label)
		return "secret", nil
	})
	if err != nil || !reflect.DeepEqual(prompts, []string{"Password", "SOFTWARE_TOKEN_MFA_CODE"}) {
		t.Fatalf("login: %v %v", prompts, err)
	}
	info, _ := os.Stat(client.credential)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("credentials permissions")
	}
	if err := atomicJSON(client.credential, tokens{"old", "refresh", time.Now().Unix() - 1}); err != nil {
		t.Fatal(err)
	}
	client.token = ""
	if err := client.authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var stored tokens
	_ = readJSON(client.credential, &stored)
	if stored.Refresh != "rotated" || client.token != "renewed" {
		t.Fatal("refresh rotation not saved")
	}
	err = client.request(context.Background(), "GET", server.URL+"/redirect", nil, map[string]string{"Authorization": "Bearer token"}, nil)
	var status *httpError
	if !errors.As(err, &status) || status.status != 302 || strings.Contains(err.Error(), "never-print") {
		t.Fatalf("redirect handling: %v", err)
	}
	for _, target := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/#fragment"} {
		if validateURL(target) == nil {
			t.Errorf("unsafe URL accepted: %s", target)
		}
	}
}
