package submissions

import (
	"judge/api/internal/testfiles"
)

// scoreResult checks a partial-scoring result against the job and adds each group's name and cases from the job, so
// that the stored result shows its group table by itself. The bridge has checked the result's own sums and bounds
// before; these rules need the job (ADR 0025).
func scoreResult(job Job, r *Result) error {
	if job.Scoring == nil {
		for _, c := range r.Cases {
			if c.Score != nil {
				return testfiles.ErrInvalid
			}
		}
		if r.Scoring != nil {
			return testfiles.ErrInvalid
		}
		return nil
	}
	if r.Verdict == "JE" {
		// A judge error has no score, for example when the aggregator failed.
		if r.Scoring != nil {
			return testfiles.ErrInvalid
		}
		return nil
	}
	if r.Scoring == nil {
		return ErrUnscored
	}
	definition := job.Scoring
	if (definition.Mode != "groups" && definition.Mode != "aggregate") || len(r.Scoring.Groups) != len(definition.Groups) {
		return testfiles.ErrInvalid
	}
	index := make(map[string]int, len(definition.Groups))
	var maxScore int64
	for i, g := range definition.Groups {
		index[g.Name] = i
		if g.Points < 0 || g.Points > 1_000_000 || r.Scoring.Groups[i].MaxScore != 100*int64(g.Points) {
			return testfiles.ErrInvalid
		}
		maxScore += 100 * int64(g.Points)
	}
	if len(index) != len(definition.Groups) || r.Scoring.MaxScore != maxScore {
		return testfiles.ErrInvalid
	}
	// The worker refers to groups by name on each case and judges the job as JE when a name is unknown or repeated.
	caseGroups := make([][]int, len(job.Cases))
	members := make([][]int, len(definition.Groups))
	for i, c := range job.Cases {
		for _, name := range c.Groups {
			g, ok := index[name]
			if !ok || (len(members[g]) > 0 && members[g][len(members[g])-1] == i) {
				return testfiles.ErrInvalid
			}
			caseGroups[i] = append(caseGroups[i], g)
			members[g] = append(members[g], i)
		}
	}
	if r.Verdict == "CE" {
		for _, g := range r.Scoring.Groups {
			if g.Score != 0 {
				return testfiles.ErrInvalid
			}
		}
	} else {
		if err := judgedAsScored(definition.Mode, len(definition.Groups), caseGroups, r.Cases); err != nil {
			return err
		}
		if definition.Mode == "groups" {
			// The score follows from the verdicts, so the stored score never rests on the worker's sums.
			for g, cases := range members {
				want := int64(0)
				if len(cases) > 0 && allAccepted(r.Cases, cases) {
					want = 100 * int64(definition.Groups[g].Points)
				}
				if r.Scoring.Groups[g].Score != want {
					return testfiles.ErrInvalid
				}
			}
		}
	}
	score := *r.Scoring
	score.Groups = append([]GroupScore(nil), score.Groups...)
	for g := range score.Groups {
		score.Groups[g].Name = definition.Groups[g].Name
		score.Groups[g].Cases = append([]int{}, members[g]...)
	}
	r.Scoring = &score
	return nil
}

// judgedAsScored checks which cases were skipped. A group stops after 2 TLEs, and a case runs unless all of its groups
// have stopped, so a case without groups always runs. Any other skipped case is where the time budget ran out, and
// every case after it is skipped too.
func judgedAsScored(mode string, groups int, caseGroups [][]int, cases []CaseResult) error {
	if len(cases) != len(caseGroups) {
		return testfiles.ErrInvalid
	}
	tles := make([]int, groups)
	budget := false
	for i, c := range cases {
		stopped := len(caseGroups[i]) > 0
		for _, g := range caseGroups[i] {
			stopped = stopped && tles[g] >= 2
		}
		switch {
		case budget || stopped:
			if c.Verdict != "SKIPPED" {
				return testfiles.ErrInvalid
			}
		case c.Verdict == "SKIPPED":
			budget = true
		case c.Verdict == "TLE":
			// A case in several groups counts toward each of them.
			for _, g := range caseGroups[i] {
				tles[g]++
			}
		}
		if c.Score != nil && mode != "aggregate" {
			return testfiles.ErrInvalid
		}
	}
	return nil
}

func allAccepted(results []CaseResult, cases []int) bool {
	for _, i := range cases {
		if results[i].Verdict != "AC" {
			return false
		}
	}
	return true
}
