package contests

import (
	"testing"

	"judge/api/internal/problems"
)

func TestContestProblemMemoryLimits(t *testing.T) {
	policy := JudgePolicy{}
	for memory, want := range map[string]bool{
		"63": false, "64": true, "128": true, "256": true, "315": true,
		"512": true, "513": false, "1024": false, "invalid": false,
	} {
		t.Run(memory, func(t *testing.T) {
			difficulty := 1
			draft := problems.Draft{
				Difficulty: &difficulty, Title: "Welcome to ShareOJ!", Markdown: "Print a greeting.",
				TimeLimitMS: "2000", MemoryLimitMB: memory,
				TestCases: []problems.TestCase{{Output: "Hello"}},
			}
			if got := policy.validProblem(draft); got != want {
				t.Fatalf("memory=%s: validProblem=%t, want %t", memory, got, want)
			}
		})
	}
}
