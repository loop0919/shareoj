package problems

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRunTwiceIsExclusiveJudgeCode(t *testing.T) {
	known := []string{"cpp23-gcc", "python314", "text"}
	code := func(runtime, protocol string) *Generator {
		return &Generator{Runtime: runtime, Source: "x", Protocol: protocol}
	}
	base := Draft{TimeLimitMS: "1000", MemoryLimitMB: "256"}
	for _, tc := range []struct {
		name  string
		edit  func(*Draft)
		valid bool
	}{
		{"run twice alone", func(d *Draft) { d.RunTwice = code("python314", "") }, true},
		{"empty source in draft", func(d *Draft) { d.RunTwice = &Generator{Runtime: "python314"} }, true},
		{"testlib on cpp", func(d *Draft) { d.RunTwice = code("cpp23-gcc", "testlib") }, true},
		{"checker and run twice", func(d *Draft) { d.Checker, d.RunTwice = code("python314", ""), code("python314", "") }, false},
		{"interactor and run twice", func(d *Draft) { d.Interactor, d.RunTwice = code("python314", ""), code("python314", "") }, false},
		{"all three", func(d *Draft) {
			d.Checker, d.Interactor, d.RunTwice = code("python314", ""), code("python314", ""), code("python314", "")
		}, false},
		{"text runtime", func(d *Draft) { d.RunTwice = code("text", "") }, false},
		{"testlib on python", func(d *Draft) { d.RunTwice = code("python314", "testlib") }, false},
		{"unknown runtime", func(d *Draft) { d.RunTwice = code("nope", "") }, false},
		{"too large", func(d *Draft) { d.RunTwice = &Generator{Runtime: "python314", Source: strings.Repeat("a", 65537)} }, false},
	} {
		d := base
		tc.edit(&d)
		if got := ValidDraft(d, known); got != tc.valid {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestRunTwiceReadinessIssues(t *testing.T) {
	d := Draft{RunTwice: &Generator{Runtime: "python314", Source: " \n"}, TimeLimitMS: "1000"}
	issues := ContentIssues(d, []string{"python314"}, []string{"cpp23-gcc"})
	if !slices.Contains(issues, IssueRunTwiceSource) || !slices.Contains(issues, IssueRunTwiceRuntime) {
		t.Fatal(issues)
	}
	if string(IssueRunTwiceSource) != "run_twice_source_missing" || string(IssueRunTwiceRuntime) != "run_twice_runtime_unavailable" {
		t.Fatal("issue codes")
	}
	d.RunTwice.Source = "print(1)"
	issues = ContentIssues(d, []string{"python314"}, []string{"python314"})
	if slices.Contains(issues, IssueRunTwiceSource) || slices.Contains(issues, IssueRunTwiceRuntime) {
		t.Fatal(issues)
	}
}

func TestRunTwicePublicProblemHasNoSource(t *testing.T) {
	var p PublicProblem
	p.SetStatement(Draft{RunTwice: &Generator{Runtime: "python314", Source: "SECRET"}})
	if !p.RunTwice || p.Interactive || p.SpecialJudge {
		t.Fatalf("%+v", p)
	}
	b, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(b), `"runTwice":true`) || strings.Contains(string(b), "SECRET") {
		t.Fatal(string(b), err)
	}
	b, _ = json.Marshal(PublicProblem{})
	if strings.Contains(string(b), "runTwice") {
		t.Fatal(string(b))
	}
}
