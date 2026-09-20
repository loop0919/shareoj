package submissions

import (
	"errors"
	"testing"

	"judge/api/internal/problems"
)

func TestGenerationJobPreservesDraftAndCountsReplacedOutputs(t *testing.T) {
	file := &problems.TestFile{ID: "saved-output", Size: 100}
	problem := problems.Problem{Draft: problems.Draft{TestCases: []problems.TestCase{
		{Input: "12", OutputFile: file},
		{Input: "345", Output: "6789"},
	}}}
	for _, tc := range []struct {
		mode               string
		base               int64
		generate, validate bool
	}{
		{"input", 109, true, false},
		{"output", 9, true, false},
		{"validation", 109, false, true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			job, err := generationJob("owner", "problem", "digest", problem, Generation{Mode: tc.mode, Start: 1, Count: 1})
			if err != nil {
				t.Fatal(err)
			}
			if job.GenerationBaseBytes != tc.base || job.Generate != tc.generate || job.Validate != tc.validate || len(job.Cases) != 1 {
				t.Fatalf("unexpected job: %+v", job)
			}
			if tc.mode == "input" && job.Cases[0].Input != "1\n" {
				t.Fatal("seed was not serialized")
			}
			if tc.mode != "input" && (job.Cases[0].Input != "12" || job.Cases[0].OutputFile != nil || job.Cases[0].Output != "") {
				t.Fatalf("output leaked into generation input: %+v", job.Cases[0])
			}
			if problem.Draft.TestCases[0].OutputFile != file {
				t.Fatal("draft was mutated")
			}
		})
	}
}

func TestGenerationJobRejectsInvalidRangesAndCapacity(t *testing.T) {
	problem := problems.Problem{Draft: problems.Draft{TestCases: []problems.TestCase{{Input: "1"}}}}
	for _, g := range []Generation{
		{Mode: "unknown", Count: 1},
		{Mode: "input", Count: 0},
		{Mode: "input", Start: 2147483647, Count: 2},
		{Mode: "input", Start: -2147483649, Count: 1},
		{Mode: "input", Count: 100},
		{Mode: "output", Start: 0, Count: 1},
		{Mode: "validation", Start: 1, Count: 2},
	} {
		if _, err := generationJob("owner", "problem", "digest", problem, g); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v: %v", g, err)
		}
	}
	problem.Draft.TestCases[0].InputFile = &problems.TestFile{Size: GenerationOutputLimit + 1}
	if _, err := generationJob("owner", "problem", "digest", problem, Generation{Mode: "output", Start: 1, Count: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted oversized draft: %v", err)
	}
}
