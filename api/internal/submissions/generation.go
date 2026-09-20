package submissions

import (
	"strconv"

	"judge/api/internal/problems"
	"judge/api/internal/testfiles"
)

type Generation struct {
	Mode  string `json:"mode"`
	Start int64  `json:"start"`
	Count int    `json:"count"`
}

func (g Generation) valid() bool {
	return (g.Mode == "input" || g.Mode == "output" || g.Mode == "validation") && g.Count >= 1 && g.Count <= 100 && g.Start >= -2147483648 && g.Start <= 2147483647-int64(g.Count-1)
}

// generationJob uses only cases from the authorized, versioned draft.
func generationJob(owner, problemID, image string, problem problems.Problem, g Generation) (Job, error) {
	if !g.valid() {
		return Job{}, ErrInvalid
	}
	job := Job{Generate: g.Mode != "validation", Validate: g.Mode == "validation", GenerationPrefix: testfiles.GenerationPrefix(owner, problemID), Image: image, TimeLimitMS: 5000, MemoryLimitMB: 512}
	if g.Mode == "input" {
		if len(problem.Draft.TestCases)+g.Count > 100 {
			return Job{}, ErrInvalid
		}
		for i := 0; i < g.Count; i++ {
			job.Cases = append(job.Cases, problems.TestCase{Input: strconv.FormatInt(g.Start+int64(i), 10) + "\n"})
		}
	} else {
		// Inputs come only from this owner's saved draft, never client-supplied file references.
		if g.Start < 1 || g.Start+int64(g.Count)-1 > int64(len(problem.Draft.TestCases)) {
			return Job{}, ErrInvalid
		}
		for _, c := range problem.Draft.TestCases[int(g.Start)-1 : int(g.Start)-1+g.Count] {
			c.Output = ""
			c.OutputFile = nil
			job.Cases = append(job.Cases, c)
		}
	}
	for i, c := range problem.Draft.TestCases {
		if c.InputFile != nil {
			job.GenerationBaseBytes += c.InputFile.Size
		} else {
			job.GenerationBaseBytes += int64(len(c.Input))
		}
		if g.Mode == "output" && i >= int(g.Start)-1 && i < int(g.Start)-1+g.Count {
			continue
		}
		if c.OutputFile != nil {
			job.GenerationBaseBytes += c.OutputFile.Size
		} else {
			job.GenerationBaseBytes += int64(len(c.Output))
		}
	}
	if job.GenerationBaseBytes > GenerationOutputLimit {
		return Job{}, ErrInvalid
	}
	return job, nil
}
