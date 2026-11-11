package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRewriteImagesLeavesCodeAndURLsAlone(t *testing.T) {
	const text = "![図](images/a.png) and ![b](<images/my fig.png> \"title\") ![c](images/a%20b.png)\n" +
		"![web](https://example.com/a.png) ![site](/api/images/x) `![code](images/a.png)`\n" +
		"```md\n![fenced](images/a.png)\n```\n" +
		"~~~\n```\n![still fenced](images/a.png)\n~~~\n" +
		"[link](images/a.png) ![last](images/skip.png)"
	var seen []string
	got, err := rewriteImages(text, func(path string) (string, error) {
		seen = append(seen, path)
		if path == "images/skip.png" {
			return "", nil
		}
		return "/api/images/1", nil
	})
	const want = "![図](/api/images/1) and ![b](/api/images/1 \"title\") ![c](/api/images/1)\n" +
		"![web](https://example.com/a.png) ![site](/api/images/x) `![code](images/a.png)`\n" +
		"```md\n![fenced](images/a.png)\n```\n" +
		"~~~\n```\n![still fenced](images/a.png)\n~~~\n" +
		"[link](images/a.png) ![last](images/skip.png)"
	if err != nil || got != want {
		t.Fatalf("%v\n%s", err, got)
	}
	if strings.Join(seen, "|") != "images/a.png|images/my fig.png|images/a b.png|images/skip.png" {
		t.Fatal(seen)
	}
}

// Issue #19: push uploads the local images of the statement and saves their URLs, without touching the local file.
func TestPushUploadsLocalImages(t *testing.T) {
	dir := example(t)
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "test-token")
	if err := os.Mkdir(filepath.Join(dir, "images"), 0700); err != nil {
		t.Fatal(err)
	}
	figure := pngBytes(t, 4, 4)
	write(t, filepath.Join(dir, "images", "fig.png"), string(figure))
	write(t, filepath.Join(dir, "images", "copy.png"), string(figure))
	const statement = "## 問題文\n\n![図](images/fig.png)\n![同じ図](images/copy.png)\n![なし](images/missing.png)\n![外](https://example.com/x.png)\n"
	write(t, filepath.Join(dir, "statement.md"), statement)

	// check names the image without a file and accepts the rest.
	var out strings.Builder
	if err := run(context.Background(), []string{"check", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `warning: statement.md: image "images/missing.png" is not a file`) || strings.Count(out.String(), "statement.md") != 1 {
		t.Fatal(out.String())
	}

	const url = "/api/images/00000000-0000-4000-8000-0000000000a1"
	var saved problem
	uploads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/my/images":
			uploads++
			var input struct{ Data string }
			data, err := base64.StdEncoding.DecodeString(func() string { _ = json.NewDecoder(r.Body).Decode(&input); return input.Data }())
			if err != nil || !bytes.Equal(data, figure) || r.Header.Get("Authorization") != "Bearer test-token" {
				t.Error("image upload differs from the file")
			}
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"id":"00000000-0000-4000-8000-0000000000a1","url":%q,"size":%d}`, url, len(data))
		case r.Method == "GET":
			if saved.ID == "" {
				w.WriteHeader(404)
				fmt.Fprint(w, `{"error":"problem_not_found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
		case r.Method == "PUT":
			var input problem
			_ = json.NewDecoder(r.Body).Decode(&input)
			saved = problem{ID: strings.TrimPrefix(r.URL.Path, "/my/problems/"), Version: input.Version + 1, Draft: input.Draft}
			_ = json.NewEncoder(w).Encode(saved)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	pushed := func() string {
		t.Helper()
		if _, err := push(context.Background(), dir, client, -1); err != nil {
			t.Fatal(err)
		}
		var markdown string
		if err := json.Unmarshal(saved.Draft["markdown"], &markdown); err != nil {
			t.Fatal(err)
		}
		return markdown
	}
	want := "## 問題文\n\n![図](" + url + ")\n![同じ図](" + url + ")\n![なし](images/missing.png)\n![外](https://example.com/x.png)\n"
	// Two files with the same bytes are one upload.
	if got := pushed(); got != want || uploads != 1 {
		t.Fatalf("uploads=%d\n%s", uploads, got)
	}
	if local, _ := os.ReadFile(filepath.Join(dir, "statement.md")); string(local) != statement {
		t.Fatal("push rewrote the local statement")
	}
	// The state remembers the upload, so the next push sends nothing.
	var s state
	if err := readJSON(filepath.Join(dir, stateName), &s); err != nil || len(s.Images) != 1 {
		t.Fatal(err, s.Images)
	}
	if got := pushed(); got != want || uploads != 1 {
		t.Fatalf("second push: uploads=%d\n%s", uploads, got)
	}

	// A file the server would refuse stops check and push before anything is sent.
	for name, data := range map[string]string{"text.png": "not an image", "wide.png": string(pngBytes(t, 2049, 1)), "big.png": string(figure) + strings.Repeat("x", 512<<10)} {
		write(t, filepath.Join(dir, "statement.md"), "![x](images/"+name+")\n")
		write(t, filepath.Join(dir, "images", name), data)
		if _, err := push(context.Background(), dir, client, -1); err == nil || !strings.Contains(err.Error(), "requires a PNG or JPEG") {
			t.Fatal(name, err)
		}
	}
	write(t, filepath.Join(dir, "statement.md"), "![x](../outside.png)\n")
	if _, err := push(context.Background(), dir, client, -1); err == nil || !strings.Contains(err.Error(), "must stay inside") {
		t.Fatal(err)
	}
	if uploads != 1 || saved.Version != 2 {
		t.Fatal("a refused image reached the server", uploads, saved.Version)
	}
}
