package problems

import (
	"reflect"
	"slices"
	"testing"
)

func TestWhitespaceJudgeSourcesAreNotEmpty(t *testing.T) {
	for _, runtime := range []string{"whitespace13", "whitespace13-isolate", "cpp23-gcc"} {
		if HasSource(runtime, "") || HasSource(runtime, " \t\n") != (runtime != "cpp23-gcc") {
			t.Fatal(runtime)
		}
	}
	for _, runtime := range []string{"whitespace13", "cpp23-gcc"} {
		code := &Generator{Runtime: runtime, Source: " \t\n"}
		for _, draft := range []Draft{{Checker: code}, {Interactor: code}, {Scoring: &Scoring{Mode: ScoringAggregate, Aggregator: code}}} {
			issues := ContentIssues(draft, []string{runtime}, []string{runtime})
			missing := slices.Contains(issues, IssueCheckerSource) || slices.Contains(issues, IssueInteractorSource) || slices.Contains(issues, IssueAggregatorSource)
			if missing != (runtime != "whitespace13") {
				t.Fatalf("%s: %v", runtime, issues)
			}
		}
	}
}

func TestAssessListsEveryReason(t *testing.T) {
	known := []string{"cpp17", "python314"}
	enabled := []string{"cpp17"}
	difficulty := 3
	ready := Draft{
		Difficulty: &difficulty, Title: "A", Markdown: "本文", Editorial: "解説", TimeLimitMS: "1000", MemoryLimitMB: "256",
		TestCases: []TestCase{{Name: "1", Input: "1", Output: "1"}},
	}
	with := func(change func(*Draft)) Draft {
		d := ready
		d.TestCases = append([]TestCase(nil), ready.TestCases...)
		change(&d)
		return d
	}
	for _, tc := range []struct {
		name  string
		draft Draft
		state ProblemState
		want  Readiness
	}{
		{"ready everywhere", ready, ProblemState{}, Readiness{Publish: []Issue{}, Contest: []Issue{}, Featured: []Issue{}}},
		{
			"only samples warns without blocking", with(func(d *Draft) { d.TestCases[0].IsSample = true }),
			ProblemState{},
			Readiness{Publish: []Issue{}, Contest: []Issue{}, Featured: []Issue{}, Warnings: []Issue{IssueOnlySampleTestCases, IssueSampleNotInStatement}},
		},
		{
			"a sample the statement does not show warns without blocking", with(func(d *Draft) {
				d.TestCases = append(d.TestCases, TestCase{Name: "sample1", IsSample: true, Input: "1 2\n", Output: "3\n"})
			}),
			ProblemState{},
			Readiness{Publish: []Issue{}, Contest: []Issue{}, Featured: []Issue{}, Warnings: []Issue{IssueSampleNotInStatement}},
		},
		{
			"a sample the statement shows is fine", with(func(d *Draft) {
				d.Markdown = "本文\n\n```text\n1 2\n```\n\n```text\n3\n```\n"
				d.TestCases = append(d.TestCases, TestCase{Name: "sample1", IsSample: true, Input: "1 2\n", Output: "3\n"})
			}),
			ProblemState{},
			Readiness{Publish: []Issue{}, Contest: []Issue{}, Featured: []Issue{}},
		},
		{
			"blank statement and title", with(func(d *Draft) { d.Title, d.Markdown = " ", "" }),
			ProblemState{},
			Readiness{
				Publish:  []Issue{IssueTitle, IssueStatement},
				Contest:  []Issue{IssueTitle, IssueStatement},
				Featured: []Issue{IssueTitle, IssueStatement},
			},
		},
		{
			"every destination needs difficulty, tests and an editorial", with(func(d *Draft) { d.Difficulty, d.Editorial, d.TestCases = nil, " ", nil }),
			ProblemState{},
			Readiness{
				Publish:  []Issue{IssueDifficulty, IssueTestCases, IssueEditorial},
				Contest:  []Issue{IssueDifficulty, IssueTestCases, IssueEditorial},
				Featured: []Issue{IssueDifficulty, IssueTestCases, IssueEditorial},
			},
		},
		{
			"judge code in a disabled runtime without source", with(func(d *Draft) { d.Checker = &Generator{Runtime: "python314"} }),
			ProblemState{},
			Readiness{
				Publish:  []Issue{IssueCheckerSource, IssueCheckerRuntime},
				Contest:  []Issue{IssueCheckerSource, IssueCheckerRuntime},
				Featured: []Issue{IssueCheckerSource, IssueCheckerRuntime},
			},
		},
		{
			"interactor is reported separately", with(func(d *Draft) { d.Interactor = &Generator{Runtime: "python314", Source: "print(1)"} }),
			ProblemState{},
			Readiness{
				Publish:  []Issue{IssueInteractorRuntime},
				Contest:  []Issue{IssueInteractorRuntime},
				Featured: []Issue{IssueInteractorRuntime},
			},
		},
		{
			"saved draft broken by tightened rules", with(func(d *Draft) { d.MemoryLimitMB = "4096" }),
			ProblemState{},
			Readiness{Publish: []Issue{IssueInvalidDraft}, Contest: []Issue{IssueInvalidDraft}, Featured: []Issue{IssueInvalidDraft}},
		},
		{
			"published problem", ready,
			ProblemState{Published: true, EverPublished: true},
			Readiness{Publish: []Issue{}, Contest: []Issue{IssuePublished}, Featured: []Issue{IssueEverPublished}},
		},
		{
			"scheduled contest locks publication", ready,
			ProblemState{InContest: true, ContestScheduled: true},
			Readiness{Publish: []Issue{IssueInContest}, Contest: []Issue{IssueInContest}, Featured: []Issue{IssueInContest}},
		},
		{
			"application limit only blocks new applications", ready,
			ProblemState{OtherApplications: 3},
			Readiness{Publish: []Issue{}, Contest: []Issue{}, Featured: []Issue{IssueFeaturedLimit}},
		},
		{
			"applied problem keeps its place and stays out of contests", ready,
			ProblemState{OtherApplications: 3, FeaturedPreference: "soon"},
			Readiness{Publish: []Issue{}, Contest: []Issue{IssueFeaturedApplied}, Featured: []Issue{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Assess(tc.draft, tc.state, known, enabled); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBooleanChecksFollowTheIssueLists(t *testing.T) {
	difficulty := 5
	d := Draft{Difficulty: &difficulty, Title: "A", Markdown: "本文", Editorial: "解説", TimeLimitMS: "1000", MemoryLimitMB: "256"}
	if Publishable(d, []string{"cpp17"}, nil) || featuredEligible(d, []string{"cpp17"}, nil) {
		t.Fatal("publication and 定期便 both need at least one test case")
	}
	d.TestCases = []TestCase{{Name: "1", Input: "1", Output: "1"}}
	if !Publishable(d, []string{"cpp17"}, nil) || !featuredEligible(d, []string{"cpp17"}, nil) {
		t.Fatal("a complete problem without judge code needs no enabled runtime")
	}
	d.Editorial = " "
	if Publishable(d, []string{"cpp17"}, nil) || featuredEligible(d, []string{"cpp17"}, nil) {
		t.Fatal("publication and 定期便 both need an editorial")
	}
	d.Editorial = "解説"
	d.Checker = &Generator{Runtime: "cpp17", Source: "int main(){}"}
	if Publishable(d, []string{"cpp17"}, nil) || featuredEligible(d, []string{"cpp17"}, nil) {
		t.Fatal("judge code waits while its runtime is disabled")
	}
}

func TestSampleInStatement(t *testing.T) {
	sample := TestCase{Name: "sample1", IsSample: true, Input: "1 2\n", Output: "3\n"}
	stored := sample
	stored.Input, stored.InputFile = "", &TestFile{ID: "f", Size: 100_000}
	for _, tc := range []struct {
		name, markdown string
		c              TestCase
		want           bool
	}{
		{"nothing", "## 問題文\n1 2 を足すと 3 です。", sample, false},
		{"between the markers", "<!-- sample-begin: sample1 -->\n古い内容\n<!-- sample-end: sample1 -->\n", sample, true},
		{"the markers of another sample", "<!-- sample-begin: sample2 -->\n<!-- sample-end: sample2 -->\n", sample, false},
		{"a start marker alone", "<!-- sample-begin: sample1 -->\n", sample, false},
		{"written by hand", "### 入力\n```\n1 2\n```\n### 出力\n```text\n3\n```\n", sample, true},
		{"tilde fences and CRLF", "~~~\r\n1 2\r\n~~~\r\n\r\n~~~~text\r\n3\r\n~~~~\r\n", sample, true},
		{"the input alone", "```\n1 2\n```\n", sample, false},
		{"another output", "```\n1 2\n```\n```\n4\n```\n", sample, false},
		{"outside a code block", "入力は\n1 2\nで、出力は\n3\nです。", sample, false},
		{"a longer fence holds a shorter one", "````\n```\n1 2\n```\n````\n```\n3\n```\n", sample, false},
		{"an empty input needs no block", "```\n3\n```\n", TestCase{Name: "s", IsSample: true, Output: "3\n"}, true},
		{"a stored file cannot be compared", "```\n1 2\n```\n```\n3\n```\n", stored, false},
		{"a stored file between the markers", "<!-- sample-begin: sample1 -->\n<!-- sample-end: sample1 -->", stored, true},
	} {
		if got := sampleInStatement(tc.markdown, tc.c); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestSampleWarningSkipsDialogueProblems(t *testing.T) {
	code := &Generator{Runtime: "cpp17", Source: "int main(){}"}
	cases := []TestCase{{Name: "sample1", IsSample: true, Input: "42\n"}, {Name: "1", Input: "7\n"}}
	for _, tc := range []struct {
		name  string
		draft Draft
		want  []Issue
	}{
		{"an ordinary problem", Draft{Markdown: "本文", TestCases: cases}, []Issue{IssueSampleNotInStatement}},
		{"a checker problem", Draft{Markdown: "本文", TestCases: cases, Checker: code}, []Issue{IssueSampleNotInStatement}},
		// The input goes to the judge code, not to the submission, so the statement has no reason to show it.
		{"an interactive problem", Draft{Markdown: "本文", TestCases: cases, Interactor: code}, nil},
		{"a run-twice problem", Draft{Markdown: "本文", TestCases: cases, RunTwice: code}, nil},
	} {
		if got := ContentWarnings(tc.draft); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}
