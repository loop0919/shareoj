package problems

import (
	"slices"
	"strings"
)

// Issue names one reason a saved problem cannot be published, put in a contest or
// applied to 定期便. Codes are stable; the web client translates them.
type Issue string

const (
	IssueInvalidDraft      Issue = "invalid_draft"
	IssueTitle             Issue = "title_missing"
	IssueStatement         Issue = "statement_missing"
	IssueCheckerSource     Issue = "checker_source_missing"
	IssueCheckerRuntime    Issue = "checker_runtime_unavailable"
	IssueInteractorSource  Issue = "interactor_source_missing"
	IssueInteractorRuntime Issue = "interactor_runtime_unavailable"
	IssueTestCases         Issue = "test_cases_missing"
	IssueDifficulty        Issue = "difficulty_missing"
	IssueEditorial         Issue = "editorial_missing"
	IssuePublished         Issue = "published"
	IssueEverPublished     Issue = "ever_published"
	IssueInContest         Issue = "in_contest"
	IssueFeaturedLimit     Issue = "featured_limit"
)

const featuredApplicationsPerAuthor = 3

// ProblemState is what the database knows about a problem beyond its draft.
type ProblemState struct {
	Published, EverPublished bool
	// InContest covers any contest, including released ones; ContestScheduled only unreleased ones.
	InContest, ContestScheduled bool
	FeaturedPreference          string
	// OtherApplications counts the author's 定期便 applications for other problems.
	OtherApplications int
}

// Readiness lists, per destination, why the saved problem cannot go there. Empty means it can.
type Readiness struct {
	Publish  []Issue `json:"publish"`
	Contest  []Issue `json:"contest"`
	Featured []Issue `json:"featured"`
}

// PublishIssues lists every publication requirement the draft misses.
func PublishIssues(d Draft, knownRuntimes, enabledRuntimes []string) []Issue {
	issues := []Issue{}
	if !ValidDraft(d, knownRuntimes) {
		issues = append(issues, IssueInvalidDraft)
	}
	if strings.TrimSpace(d.Title) == "" {
		issues = append(issues, IssueTitle)
	}
	if strings.TrimSpace(d.Markdown) == "" {
		issues = append(issues, IssueStatement)
	}
	for _, judge := range []struct {
		code             *Generator
		source, language Issue
	}{{d.Checker, IssueCheckerSource, IssueCheckerRuntime}, {d.Interactor, IssueInteractorSource, IssueInteractorRuntime}} {
		if judge.code == nil {
			continue
		}
		if strings.TrimSpace(judge.code.Source) == "" {
			issues = append(issues, judge.source)
		}
		// During judge maintenance no runtime is enabled, so every judge-code problem waits.
		if !slices.Contains(enabledRuntimes, judge.code.Runtime) {
			issues = append(issues, judge.language)
		}
	}
	return issues
}

// ContestContentIssues adds what a contest needs from the draft itself.
func ContestContentIssues(d Draft, knownRuntimes, enabledRuntimes []string) []Issue {
	issues := PublishIssues(d, knownRuntimes, enabledRuntimes)
	if len(d.TestCases) == 0 {
		issues = append(issues, IssueTestCases)
	}
	return issues
}

// FeaturedContentIssues adds what 定期便 needs from the draft itself.
func FeaturedContentIssues(d Draft, knownRuntimes, enabledRuntimes []string) []Issue {
	issues := ContestContentIssues(d, knownRuntimes, enabledRuntimes)
	if d.Difficulty == nil {
		issues = append(issues, IssueDifficulty)
	}
	if strings.TrimSpace(d.Editorial) == "" {
		issues = append(issues, IssueEditorial)
	}
	return issues
}

// Assess mirrors the checks of publication, contest saving and ApplyFeatured.
func Assess(d Draft, s ProblemState, knownRuntimes, enabledRuntimes []string) Readiness {
	r := Readiness{
		Publish:  PublishIssues(d, knownRuntimes, enabledRuntimes),
		Contest:  ContestContentIssues(d, knownRuntimes, enabledRuntimes),
		Featured: FeaturedContentIssues(d, knownRuntimes, enabledRuntimes),
	}
	if s.ContestScheduled {
		r.Publish = append(r.Publish, IssueInContest)
	}
	if s.Published {
		r.Contest = append(r.Contest, IssuePublished)
	}
	if s.InContest {
		r.Contest = append(r.Contest, IssueInContest)
	}
	if s.EverPublished {
		r.Featured = append(r.Featured, IssueEverPublished)
	}
	if s.InContest {
		r.Featured = append(r.Featured, IssueInContest)
	}
	if s.FeaturedPreference == "" && s.OtherApplications >= featuredApplicationsPerAuthor {
		r.Featured = append(r.Featured, IssueFeaturedLimit)
	}
	return r
}
