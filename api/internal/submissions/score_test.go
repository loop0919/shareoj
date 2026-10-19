package submissions

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"judge/api/internal/problems"
	"judge/api/internal/testfiles"
)

// scoredResult builds a worker result from case verdicts ("AC", "TLE", "SKIPPED", ...) and group scores in hundredths.
func scoredResult(groups []problems.ScoringGroup, scores []int64, verdicts ...string) Result {
	one, memory := 1.0, int64(1024)
	r := Result{Verdict: "AC", Total: len(verdicts), Scoring: &SubmissionScore{}}
	for _, v := range verdicts {
		c := CaseResult{Name: "case", Verdict: v}
		if v != "SKIPPED" {
			c.CPUTimeMS, c.WallTimeMS, c.MemoryBytes = &one, &one, &memory
			if v == "AC" {
				r.Passed++
			} else if r.Verdict == "AC" {
				r.Verdict = v
			}
		}
		r.Cases = append(r.Cases, c)
	}
	for i, g := range groups {
		r.Scoring.Groups = append(r.Scoring.Groups, GroupScore{Score: scores[i], MaxScore: 100 * int64(g.Points)})
		r.Scoring.Score += scores[i]
		r.Scoring.MaxScore += 100 * int64(g.Points)
	}
	return r
}

func scoringJob(mode string, groups []problems.ScoringGroup, membership ...[]string) Job {
	job := Job{Scoring: &problems.Scoring{Mode: mode, Groups: groups}}
	if mode == "aggregate" {
		job.Scoring.Aggregator = &problems.Generator{Runtime: "cpp23-gcc", Source: "int main(){}"}
	}
	for _, m := range membership {
		job.Cases = append(job.Cases, Case{Groups: m})
	}
	return job
}

func TestScoreResultKeepsAllOrNothingJobs(t *testing.T) {
	job := Job{Cases: []Case{{}, {}, {}}}
	old := scoredResult(nil, nil, "TLE", "TLE", "SKIPPED")
	old.Scoring = nil
	if err := scoreResult(job, &old); err != nil || old.Scoring != nil {
		t.Fatalf("old result rejected or changed: %v %+v", err, old.Scoring)
	}
	for _, r := range []Result{{Verdict: "JE"}, {Verdict: "CE", Total: 3}} {
		if err := scoreResult(job, &r); err != nil {
			t.Fatal(r.Verdict, err)
		}
	}
	scored := scoredResult([]problems.ScoringGroup{{Name: "g", Points: 1}}, []int64{0}, "WA", "AC", "AC")
	if err := scoreResult(job, &scored); !errors.Is(err, testfiles.ErrInvalid) {
		t.Fatal("a score for an all-or-nothing job", err)
	}
	withCaseScore := old
	withCaseScore.Cases = slices.Clone(old.Cases)
	score := int64(3)
	withCaseScore.Cases[0].Score = &score
	if err := scoreResult(job, &withCaseScore); !errors.Is(err, testfiles.ErrInvalid) {
		t.Fatal("a case score for an all-or-nothing job", err)
	}
}

func TestScoreResultTurnsUnscoredResultsIntoJudgeErrors(t *testing.T) {
	groups := []problems.ScoringGroup{{Name: "小課題1", Points: 30}, {Name: "小課題2", Points: 70}}
	job := scoringJob("groups", groups, nil, []string{"小課題1"}, []string{"小課題2"})
	// Old workers send their own shapes, such as the cutoff after 2 TLEs in the whole submission.
	old := scoredResult(groups, []int64{0, 0}, "AC", "TLE", "TLE")
	old.Scoring = nil
	compileError := Result{Verdict: "CE", Total: 3}
	for _, r := range []Result{old, compileError} {
		if err := scoreResult(job, &r); !errors.Is(err, ErrUnscored) {
			t.Fatal(r.Verdict, err)
		}
	}
	// A judge error has no score; one with a score was not sent by the worker.
	je := Result{Verdict: "JE", Total: 3, CheckerLog: "集計コードの実行失敗: TLE"}
	if err := scoreResult(job, &je); err != nil {
		t.Fatal(err)
	}
	je.Scoring = &SubmissionScore{Score: 0, MaxScore: 10000, Groups: []GroupScore{{MaxScore: 3000}, {MaxScore: 7000}}}
	if err := scoreResult(job, &je); !errors.Is(err, testfiles.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestScoreResultRederivesGroupScores(t *testing.T) {
	groups := []problems.ScoringGroup{{Name: "小課題1", Points: 30}, {Name: "小課題2", Points: 70}, {Name: "空", Points: 0}}
	job := scoringJob("groups", groups, nil, []string{"小課題1", "小課題2"}, []string{"小課題2"})
	// The sample in no group is WA, and the score is still full.
	r := scoredResult(groups, []int64{3000, 7000, 0}, "WA", "AC", "AC")
	scoring := r.Scoring
	if err := scoreResult(job, &r); err != nil {
		t.Fatal(err)
	}
	want := []GroupScore{{Name: "小課題1", Score: 3000, MaxScore: 3000, Cases: []int{1}}, {Name: "小課題2", Score: 7000, MaxScore: 7000, Cases: []int{1, 2}}, {Name: "空", Cases: []int{}}}
	for i, g := range r.Scoring.Groups {
		if g.Name != want[i].Name || g.Score != want[i].Score || g.MaxScore != want[i].MaxScore || !slices.Equal(g.Cases, want[i].Cases) || g.Cases == nil {
			t.Fatalf("group %d: %+v", i, g)
		}
	}
	if scoring.Groups[0].Name != "" || r.Scoring == scoring {
		t.Fatal("changed the caller's result")
	}
	for name, tc := range map[string]struct {
		scores   []int64
		verdicts []string
	}{
		"partial points in groups mode": {[]int64{1500, 7000, 0}, []string{"AC", "AC", "AC"}},
		"a failed group with points":    {[]int64{3000, 7000, 0}, []string{"AC", "AC", "WA"}},
		"points for a group of no case": {[]int64{3000, 7000, 0}, []string{"AC", "AC", "AC"}},
	} {
		t.Run(name, func(t *testing.T) {
			g := slices.Clone(groups)
			if name == "points for a group of no case" {
				g[2].Points = 10
				tc.scores[2] = 1000
			}
			r := scoredResult(g, tc.scores, tc.verdicts...)
			if err := scoreResult(scoringJob("groups", g, nil, []string{"小課題1", "小課題2"}, []string{"小課題2"}), &r); !errors.Is(err, testfiles.ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	// A skipped case is not accepted, so its groups score nothing.
	skipped := scoredResult(groups, []int64{0, 0, 0}, "AC", "TLE", "SKIPPED")
	skipped.Verdict = "TLE"
	if err := scoreResult(scoringJob("groups", groups, nil, []string{"小課題1", "小課題2"}, []string{"小課題2"}), &skipped); err != nil {
		t.Fatal(err)
	}
}

func TestScoreResultChecksDefinitionAndUnits(t *testing.T) {
	groups := []problems.ScoringGroup{{Name: "a", Points: 30}, {Name: "b", Points: 70}}
	valid := func() (Job, Result) {
		return scoringJob("aggregate", groups, []string{"a"}, []string{"b"}), scoredResult(groups, []int64{1250, 0}, "AC", "WA")
	}
	job, r := valid()
	if err := scoreResult(job, &r); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Job, *Result){
		"missing group":       func(_ *Job, r *Result) { r.Scoring.Groups = r.Scoring.Groups[:1] },
		"group maximum":       func(_ *Job, r *Result) { r.Scoring.Groups[1].MaxScore = 70 },
		"full score":          func(_ *Job, r *Result) { r.Scoring.MaxScore = 7000 },
		"unknown mode":        func(j *Job, _ *Result) { j.Scoring.Mode = "best" },
		"unknown group name":  func(j *Job, _ *Result) { j.Cases[0].Groups = []string{"c"} },
		"repeated membership": func(j *Job, _ *Result) { j.Cases[0].Groups = []string{"a", "a"} },
		"repeated group name": func(j *Job, _ *Result) {
			j.Scoring.Groups = []problems.ScoringGroup{{Name: "a", Points: 30}, {Name: "a", Points: 70}}
		},
		"case count": func(_ *Job, r *Result) { r.Cases, r.Total, r.Passed, r.Verdict = r.Cases[:1], 1, 1, "AC" },
		"compile error score": func(_ *Job, r *Result) {
			*r = Result{Verdict: "CE", Total: 2, Scoring: &SubmissionScore{Score: 1250, MaxScore: 10000, Groups: []GroupScore{{Score: 1250, MaxScore: 3000}, {MaxScore: 7000}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			job, r := valid()
			job.Scoring.Groups = slices.Clone(job.Scoring.Groups)
			change(&job, &r)
			if err := scoreResult(job, &r); !errors.Is(err, testfiles.ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	// A compile error scores 0 in every group and still names them.
	ce := Result{Verdict: "CE", Total: 2, Scoring: &SubmissionScore{MaxScore: 10000, Groups: []GroupScore{{MaxScore: 3000}, {MaxScore: 7000}}}}
	if err := scoreResult(job, &ce); err != nil || ce.Scoring.Groups[1].Name != "b" || !slices.Equal(ce.Scoring.Groups[1].Cases, []int{1}) {
		t.Fatalf("%v %+v", err, ce.Scoring)
	}
}

func TestScoreResultCaseScoresOnlyInAggregateMode(t *testing.T) {
	groups := []problems.ScoringGroup{{Name: "a", Points: 100}}
	score := int64(MaxCaseScore)
	for mode, want := range map[string]error{"aggregate": nil, "groups": testfiles.ErrInvalid} {
		r := scoredResult(groups, []int64{0}, "WA")
		if mode == "aggregate" {
			r.Scoring.Groups[0].Score, r.Scoring.Score = 4250, 4250
		}
		r.Cases[0].Score = &score
		if err := scoreResult(scoringJob(mode, groups, []string{"a"}), &r); !errors.Is(err, want) {
			t.Fatal(mode, err)
		}
	}
}

func TestScoreResultSkippedCases(t *testing.T) {
	groups := []problems.ScoringGroup{{Name: "a", Points: 50}, {Name: "b", Points: 50}}
	for _, tc := range []struct {
		name       string
		membership string // per case: "a", "b", "ab" or "-" for no group
		verdicts   string
		valid      bool
	}{
		{"group a stops, group b runs", "a a a b b", "TLE TLE SKIPPED AC AC", true},
		{"a stopped group must not run", "a a a b", "TLE TLE AC AC", false},
		{"one TLE does not stop", "a a a", "TLE AC AC", true},
		{"a TLE in two groups counts in each", "ab ab a b", "TLE TLE SKIPPED SKIPPED", true},
		{"a case runs while one of its groups runs", "a a ab", "TLE TLE AC", true},
		{"its skip is where the budget ran out", "a a ab b", "TLE TLE SKIPPED SKIPPED", true},
		{"after the budget everything is skipped", "a a ab b", "TLE TLE SKIPPED AC", false},
		{"a case in no group always runs", "a a - b", "TLE TLE AC AC", true},
		{"a case in no group is skipped only by the budget", "a a - b", "TLE TLE SKIPPED AC", false},
		{"budget after accepted cases", "- a b", "AC SKIPPED SKIPPED", true},
		{"budget before the first case", "- a", "SKIPPED SKIPPED", true},
		{"WA does not stop a group", "a a a", "WA WA AC", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var membership [][]string
			for _, m := range strings.Fields(tc.membership) {
				var names []string
				for _, g := range strings.Trim(m, "-") {
					names = append(names, string(g))
				}
				membership = append(membership, names)
			}
			r := scoredResult(groups, []int64{0, 0}, strings.Fields(tc.verdicts)...)
			if err := scoreResult(scoringJob("aggregate", groups, membership...), &r); (err == nil) != tc.valid {
				t.Fatalf("%+v: %v", tc, err)
			}
		})
	}
}
