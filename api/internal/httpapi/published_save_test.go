package httpapi

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"judge/api/internal/problems"
)

func TestPublishedProblemSavePostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	path := "/my/problems/" + p.ID
	if err := json.Unmarshal([]byte(f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true}, 200)), &p); err != nil {
		t.Fatal(err)
	}
	before, err := f.store.PublicGet(context.Background(), p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	submit := func() string {
		t.Helper()
		var result struct{ ID string }
		if err := json.Unmarshal([]byte(f.request("POST", "/my/submissions", "bob", map[string]any{"problemId": p.ID, "runtime": "cpp17", "source": "int main() {}"}, 202)), &result); err != nil {
			t.Fatal(err)
		}
		return result.ID
	}
	previousSubmission := submit()
	p.Draft.Title, p.Draft.Markdown, p.Draft.Editorial = "Saved title", "Saved statement", "Saved editorial"
	p.Draft.TimeLimitMS = "1500"
	p.Draft.TestCases = []problems.TestCase{{Input: "3", Output: "4", IsSample: true}}
	if err := json.Unmarshal([]byte(f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": p.Draft}, 200)), &p); err != nil {
		t.Fatal(err)
	}
	if p.Version != 3 || p.PublishedVersion != p.Version {
		t.Fatal("save did not update publication version atomically", p)
	}
	after, err := f.store.PublicGet(context.Background(), p.ID, "")
	if err != nil || after.Title != p.Draft.Title || after.Markdown != p.Draft.Markdown || after.Editorial != p.Draft.Editorial || after.TimeLimitMS != "1500" || !after.PublishedAt.Equal(before.PublishedAt) {
		t.Fatal("save did not update public content or changed publication time", after, err)
	}
	if !strings.Contains(f.request("GET", "/problems/"+p.ID+"/samples", "", nil, 200), `"output":"4"`) {
		t.Fatal("sample not updated")
	}
	for _, want := range []struct {
		id, output string
		version    int64
	}{{previousSubmission, "2", 2}, {submit(), "4", 3}} {
		var output string
		var version int64
		if err := f.store.Pool().QueryRow(context.Background(), `SELECT problem_version,job->'cases'->0->>'output' FROM submissions WHERE id=$1`, want.id).Scan(&version, &output); err != nil || version != want.version || output != want.output {
			t.Fatal("submission used the wrong saved version", version, output, err)
		}
	}
	for _, invalid := range []problems.Draft{
		{Title: "", Markdown: p.Draft.Markdown, Difficulty: p.Draft.Difficulty, TimeLimitMS: "1500", MemoryLimitMB: "256"},
		{Title: p.Draft.Title, Markdown: "", Difficulty: p.Draft.Difficulty, TimeLimitMS: "1500", MemoryLimitMB: "256"},
		{Title: p.Draft.Title, Markdown: p.Draft.Markdown, TimeLimitMS: "1500", MemoryLimitMB: "256"},
	} {
		f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": invalid}, 400)
	}
	f.request("PUT", path, "alice", map[string]any{"version": p.Version - 1, "draft": p.Draft}, 409)
	f.request("PUT", path, "alice", map[string]any{"version": p.Version + 1, "draft": p.Draft}, 409)
	current, err := f.store.Get(context.Background(), "alice", p.ID)
	if err != nil || current.Version != p.Version || !reflect.DeepEqual(current.Draft, p.Draft) {
		t.Fatal("rejected save changed the stored content", current, err)
	}
	public, err := f.store.PublicGet(context.Background(), p.ID, "")
	if err != nil || public.Markdown != after.Markdown {
		t.Fatal("rejected save changed public content", public, err)
	}
}
