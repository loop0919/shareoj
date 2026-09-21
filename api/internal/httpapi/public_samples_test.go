package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"judge/api/internal/problems"
	"judge/api/internal/testfiles"
)

type sampleTestFiles struct{ fakeTestFiles }

func (f *sampleTestFiles) PublicSampleDownload(ctx context.Context, problem, file string) (testfiles.Download, error) {
	return f.Download(ctx, "", problem, file)
}

func testPublicSamples(t *testing.T, store *problems.Store) {
	t.Helper()
	ctx := context.Background()
	const id = "12345678-1234-4234-8234-123456789012"
	files := &sampleTestFiles{}
	handler := newHandler(AuthConfig{}, handlerDependencies{Store: store, Files: files})
	request := func(path string, status int) []publicSample {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("samples: %d %s", w.Code, w.Body.String())
		}
		if status != 200 {
			return nil
		}
		var result struct {
			Items []publicSample `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Items == nil {
			t.Fatalf("invalid sample response: %s (%v)", w.Body.String(), err)
		}
		for _, secret := range []string{"hidden", "draft-only", "inputFileID", "versionId"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("leaked %s: %s", secret, w.Body.String())
			}
		}
		return result.Items
	}
	path := "/problems/" + id + "/samples"
	request(path, 404)
	request("/problems/invalid/samples", 404)
	draft := problems.Draft{Title: "samples", TestCases: []problems.TestCase{
		{Name: "hidden", Input: "hidden input", Output: "hidden output"},
		{Name: "example", IsSample: true, Input: " 3 5\r\n\n", Output: "8\n"},
		{Name: "empty", IsSample: true},
	}}
	p, err := store.Save(ctx, "alice", id, 0, draft)
	if err != nil {
		t.Fatal(err)
	}
	request(path, 404)
	p, err = store.Publish(ctx, "alice", id, p.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []publicSample{{Name: "example", Input: " 3 5\r\n\n", Output: "8\n"}, {Name: "empty"}}
	if got := request(path, 200); !reflect.DeepEqual(got, want) {
		t.Fatalf("samples = %#v", got)
	}
	draft.TestCases = []problems.TestCase{{Name: "draft-only", Input: "draft-only", IsSample: true}}
	p, err = store.Save(ctx, "alice", id, p.Version, draft)
	if err != nil {
		t.Fatal(err)
	}
	if got := request(path, 200); !reflect.DeepEqual(got, want) {
		t.Fatalf("draft exposed: %#v", got)
	}
	p, err = store.Publish(ctx, "alice", id, p.Version, false)
	if err != nil {
		t.Fatal(err)
	}
	request(path, 404)
	draft.TestCases = nil
	p, err = store.Save(ctx, "alice", id, p.Version, draft)
	if err != nil {
		t.Fatal(err)
	}
	p, err = store.Publish(ctx, "alice", id, p.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := request(path, 200); len(got) != 0 {
		t.Fatal(got)
	}
	// External samples expose download metadata, never internal file references.
	const file = "22345678-1234-4234-8234-123456789012"
	_, err = store.Pool().Exec(ctx, `UPDATE problem_drafts SET published_draft=jsonb_set(published_draft,'{testCases}',$2::jsonb) WHERE id=$1`, id,
		`[{"name":"external","isSample":true,"inputFile":{"id":"`+file+`"},"outputFile":{"id":"`+file+`"}}]`)
	if err != nil {
		t.Fatal(err)
	}
	got := request(path, 200)
	if len(got) != 1 || got[0].InputFile == nil || got[0].OutputFile == nil || got[0].InputFile.URL != "https://download.example/file" || files.problem != id || files.file != file {
		t.Fatalf("file samples: %#v", got)
	}
	handler = newHandler(AuthConfig{}, handlerDependencies{Store: store})
	request(path, 503)
	if err := store.Delete(ctx, "alice", id, p.Version); err != nil {
		t.Fatal(err)
	}
}
