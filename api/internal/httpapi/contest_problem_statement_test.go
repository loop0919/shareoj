package httpapi

import (
	"maps"
	"reflect"
	"testing"

	"judge/api/internal/problems"
)

// TestContestProblemStatementPostgres pins what a contest problem page and the problem page published after the
// contest show from the same draft. The contest pages never show the tags (ADR 0018).
func TestContestProblemStatementPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		plain       = "d5000000-0000-4000-8000-000000000001"
		checker     = "d5000000-0000-4000-8000-000000000002"
		interactive = "d5000000-0000-4000-8000-000000000003"
		cid         = "d6000000-0000-4000-8000-000000000001"
	)
	four, two := 4, 2
	judge := &problems.Generator{Runtime: "cpp17", Source: "int main(){}"}
	for id, d := range map[string]problems.Draft{
		plain: {Difficulty: &four, Title: "Plain", Markdown: "plain statement", Editorial: "plain editorial", Tags: []string{"DP"}, TimeLimitMS: "1500", MemoryLimitMB: "512",
			TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}, {Input: "3", Output: "4"}}},
		checker: {Difficulty: &two, Title: "Checker", Markdown: "checker statement", Editorial: "checker editorial", TimeLimitMS: "2000", MemoryLimitMB: "256", Checker: judge,
			TestCases: []problems.TestCase{{Input: "1", Output: "2"}}},
		interactive: {Difficulty: &two, Title: "Interactive", Markdown: "interactive statement", Editorial: "interactive editorial", TimeLimitMS: "1000", MemoryLimitMB: "128", Interactor: judge,
			TestCases: []problems.TestCase{{Input: "1", Output: ""}}},
	} {
		if _, err := f.store.Save(f.ctx, "alice", id, 0, d); err != nil {
			t.Fatal(err)
		}
	}
	// Both pages show these fields of the draft, and leave out the optional keys that it does not set.
	want := map[string]map[string]any{
		plain:       {"title": "Plain", "markdown": "plain statement", "difficulty": 4.0, "timeLimitMs": "1500", "memoryLimitMb": "512", "hasSamples": true},
		checker:     {"title": "Checker", "markdown": "checker statement", "difficulty": 2.0, "timeLimitMs": "2000", "memoryLimitMb": "256", "hasSamples": false, "specialJudge": true},
		interactive: {"title": "Interactive", "markdown": "interactive statement", "difficulty": 2.0, "timeLimitMs": "1000", "memoryLimitMb": "128", "hasSamples": false, "interactive": true},
	}
	check := func(prefix, viewer, id string, extra map[string]any, absent ...string) {
		t.Helper()
		path := prefix + id
		var got map[string]any
		f.decode(f.request("GET", path, viewer, nil, 200), &got)
		fields := maps.Clone(want[id])
		maps.Copy(fields, extra)
		for k, v := range fields {
			if !reflect.DeepEqual(got[k], v) {
				t.Errorf("%s %s: got %#v want %#v", path, k, got[k], v)
			}
		}
		for _, k := range append([]string{"scoring", "specialJudge", "interactive"}, absent...) {
			if _, wanted := fields[k]; wanted {
				continue
			}
			if v, ok := got[k]; ok {
				t.Errorf("%s: unexpected %s %#v", path, k, v)
			}
		}
	}
	f.create(cid, plain, map[string]any{"problems": []map[string]any{{"id": plain, "points": 100}, {"id": checker, "points": 100}, {"id": interactive, "points": 100}}})
	f.schedule(cid, "running")
	for id := range want {
		check("/contests/"+cid+"/problems/", "", id, nil, "tags", "editorial")
	}
	// The contest owner reads the editorial during the contest, but not the tags.
	check("/my/contests/"+cid+"/problems/", "alice", plain, map[string]any{"editorial": "plain editorial"}, "tags")

	f.schedule(cid, "ended")
	check("/contests/"+cid+"/problems/", "", plain, map[string]any{"editorial": "plain editorial"}, "tags")
	// The problem published at the end shows the same statement, now with its tags.
	check("/problems/", "", plain, map[string]any{"editorial": "plain editorial", "tags": []any{"DP"}})
	check("/problems/", "", checker, nil)
	check("/problems/", "", interactive, nil)
}
