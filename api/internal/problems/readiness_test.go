package problems

import (
	"reflect"
	"testing"
)

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
