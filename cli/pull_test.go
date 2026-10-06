package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPullLifecycle(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "author")
	ctx := context.Background()
	d, err := loadProblem(example(t))
	if err != nil {
		t.Fatal(err)
	}
	d.set("editorial", "解説\r\n")
	d.set("difficulty", 3)
	d.set("checker", program{Runtime: "cpp23-gcc", Source: "// checker\n", Protocol: "testlib"})
	const id = "00000000-0000-4000-8000-000000000001"
	large := strings.Repeat("12345\r\n", 700000) // Larger than the JSON response limit.
	ref := testFile{ID: "00000000-0000-4000-8000-000000000002", Size: len(large), SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(large)))}
	cases := []testCase{
		{Name: "../日本語", IsSample: true, Input: "3 5\r\n", Output: "8\r\n"},
		{Name: "", InputFile: &ref, Output: ""},
		{Name: "", Input: "", Output: ""},
	}
	d.set("testCases", cases)
	remote := problem{ID: id, Version: 7, Draft: d}
	badStorage := ""
	requests, puts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/object" {
			if r.Header.Get("Authorization") != "" {
				t.Error("API token leaked to storage")
			}
			switch badStorage {
			case "redirect":
				http.Redirect(w, r, "/secret?credential=hidden", 302)
			case "checksum":
				fmt.Fprint(w, strings.Repeat("x", len(large)))
			case "oversize":
				fmt.Fprint(w, large+"x")
			case "http":
				w.WriteHeader(503)
			default:
				fmt.Fprint(w, large)
			}
			return
		}
		if r.Header.Get("Authorization") != "Bearer author" && r.Header.Get("Authorization") != "Bearer tester" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Path {
		case "/my/problems/" + id:
			if r.Method == http.MethodPut {
				puts++
				var input problem
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Version != remote.Version {
					t.Errorf("push version: %v %+v", err, input)
				}
				remote.Draft, remote.Version = input.Draft, remote.Version+1
			}
			_ = json.NewEncoder(w).Encode(remote)
		case "/my/problems/" + id + "/test-files/" + ref.ID:
			_ = json.NewEncoder(w).Encode(map[string]any{"url": "http://" + r.Host + "/object", "size": ref.Size, "sha256": ref.SHA256})
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
	for _, role := range []string{"author", "tester"} {
		t.Run(role, func(t *testing.T) {
			t.Setenv("SHAREOJ_ACCESS_TOKEN", role)
			dir := filepath.Join(t.TempDir(), "pulled")
			if err := run(ctx, []string{"pull", "--api", server.URL, id, dir}, io.Discard); err != nil {
				t.Fatal(err)
			}
			got, err := loadProblem(dir)
			if err != nil {
				t.Fatal(err)
			}
			var actual []testCase
			_ = json.Unmarshal(got["testCases"], &actual)
			expected := append([]testCase(nil), cases...)
			expected[1].Input, expected[1].InputFile = large, nil
			if !reflect.DeepEqual(actual, expected) {
				t.Fatal("test content, names, ordering or samples changed")
			}
			for _, key := range []string{"title", "markdown", "editorial", "difficulty", "timeLimitMs", "memoryLimitMb", "generators", "checker"} {
				var a, b any
				_ = json.Unmarshal(got[key], &a)
				_ = json.Unmarshal(d[key], &b)
				if !reflect.DeepEqual(a, b) {
					t.Errorf("%s changed", key)
				}
			}
			var saved state
			if err := readJSON(filepath.Join(dir, stateName), &saved); err != nil || saved != (state{API: server.URL, ID: id, Version: 7}) {
				t.Fatalf("state: %+v %v", saved, err)
			}
			before := requests
			if _, err := pull(ctx, id, dir, client); err == nil || requests != before {
				t.Fatal("existing directory was touched")
			}
			if _, err := loadProblem(dir); err != nil {
				t.Fatal("existing directory damaged", err)
			}
		})
	}
	if puts != 0 {
		t.Fatal("pull modified the server")
	}
	// Optional programs and empty test sets must also produce loadable TOML.
	for _, minimal := range []bool{false, true} {
		variant := draft{}
		for key, value := range d {
			variant[key] = value
		}
		delete(variant, "checker")
		variant.set("testCases", []testCase{})
		variant.set("generators", map[string]program{"validation": {}})
		if !minimal {
			variant.set("interactor", program{Runtime: "python3", Source: "print('hello')", Protocol: "legacy"})
		}
		remote.Draft = variant
		dir := filepath.Join(t.TempDir(), "optional")
		if _, err := pull(ctx, id, dir, client); err != nil {
			t.Fatal(err)
		}
		got, err := loadProblem(dir)
		if err != nil || string(got["testCases"]) != "[]" || (!minimal && string(got["interactor"]) != string(variant["interactor"])) {
			t.Fatal("optional fields changed", err)
		}
	}
	remote.Draft = d
	for _, invalid := range []struct {
		key   string
		value any
	}{
		{"generators", map[string]program{"../escape": {Runtime: "cpp17"}}},
		{"testCases", []testCase{{Name: "a"}, {Name: " a "}}},
		{"testCases", []testCase{{InputFile: &testFile{ID: "../escape", Size: 1, SHA256: ref.SHA256}}}},
	} {
		original := d[invalid.key]
		d.set(invalid.key, invalid.value)
		dir := filepath.Join(t.TempDir(), "invalid")
		if _, err := pull(ctx, id, dir, client); err == nil {
			t.Fatalf("invalid %s accepted", invalid.key)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("invalid draft left output")
		}
		d[invalid.key] = original
	}
	for _, mode := range []string{"checksum", "oversize", "redirect", "http"} {
		badStorage = mode
		dir := filepath.Join(t.TempDir(), "failed")
		if _, err := pull(ctx, id, dir, client); err == nil || strings.Contains(err.Error(), "credential") {
			t.Fatalf("%s: %v", mode, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("partial download left behind")
		}
	}
	badStorage = ""
	client.token = "unauthorized"
	dir := filepath.Join(t.TempDir(), "denied")
	if _, err := pull(ctx, id, dir, client); err == nil {
		t.Fatal("unauthorized pull succeeded")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("denied pull left output")
	}
	client.token = "tester"
	// Small cases exercise pull -> edit -> push without a storage upload mock.
	d.set("testCases", cases[:1])
	dir = filepath.Join(t.TempDir(), "editable")
	if _, err := pull(ctx, id, dir, client); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "statement.md"), "edited")
	saved, err := push(ctx, dir, client, -1)
	if err != nil || saved.ID != id || saved.Version != 8 || string(remote.Draft["markdown"]) != `"edited"` {
		t.Fatalf("push pulled draft: %+v %v", saved, err)
	}
	if _, err := push(ctx, dir, client, -1); err != nil {
		t.Fatal(err)
	}
	remote.Version++
	if _, err := push(ctx, dir, client, -1); err == nil {
		t.Fatal("pulled draft bypassed version conflict")
	}
	for _, args := range [][]string{{"pull"}, {"pull", id}, {"pull", "../bad", filepath.Join(t.TempDir(), "bad")}} {
		if err := run(ctx, args, io.Discard); err == nil {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
}
