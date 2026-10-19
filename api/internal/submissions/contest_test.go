package submissions

import (
	"encoding/json"
	"strings"
	"testing"

	"judge/api/internal/problems"
)

func TestPublicSubmissionCaseResults(t *testing.T) {
	cpu, wall, memory := 1.5, 3.0, int64(1048576)
	output := "private output"
	original := Submission{Result: &Result{
		Verdict: "WA", Passed: 1, Total: 2, CompileLog: "private compile log", CheckerLog: "private checker log",
		Cases: []CaseResult{{
			Name: "sample_1", Verdict: "WA", CPUTimeMS: &cpu, WallTimeMS: &wall, MemoryBytes: &memory,
			Output: &output, OutputFile: &problems.TestFile{},
			CheckerLog: &TextPreview{Text: "private diagnostic"}, SampleDetails: &SampleDetails{},
		}, {Name: "test_2", Verdict: "SKIPPED"}},
	}}
	original.Result.summarizeUsage()
	got := publicSubmission(original)
	if got.Result.CPUTimeMS == nil || *got.Result.CPUTimeMS != cpu || got.Result.MemoryBytes == nil || *got.Result.MemoryBytes != memory {
		t.Fatal("lost public usage summary")
	}
	if got.Result.Verdict != "WA" || got.Result.Passed != 1 || got.Result.Total != 2 || len(got.Result.Cases) != 2 {
		t.Fatalf("lost result summary: %+v", got.Result)
	}
	c := got.Result.Cases[0]
	if c.Name != "sample_1" || c.Verdict != "WA" || c.CPUTimeMS == nil || *c.CPUTimeMS != cpu || c.WallTimeMS == nil || *c.WallTimeMS != wall || c.MemoryBytes == nil || *c.MemoryBytes != memory {
		t.Fatalf("lost case metrics: %+v", c)
	}
	if got.Result.Cases[1].Verdict != "SKIPPED" || got.Result.Cases[1].CPUTimeMS != nil {
		t.Fatalf("changed skipped case: %+v", got.Result.Cases[1])
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"private", "checkerLog", "compileLog", "sampleDetails", "output", "outputFile"} {
		if strings.Contains(string(data), field) {
			t.Fatalf("exposed %s: %s", field, data)
		}
	}
	if original.Result.Cases[0].Output == nil || original.Result.CheckerLog == "" {
		t.Fatal("modified original private result")
	}
	if publicSubmission(Submission{}).Result != nil {
		t.Fatal("created a result for a pending submission")
	}
}

func TestResultUsageSummary(t *testing.T) {
	zero, cpu := 0.0, 12.25
	small, large := int64(1000000), int64(2500000)
	r := Result{Cases: []CaseResult{
		{CPUTimeMS: &cpu, MemoryBytes: &small},
		{CPUTimeMS: &zero, MemoryBytes: &large},
		{Verdict: "SKIPPED"},
	}}
	r.summarizeUsage()
	if r.CPUTimeMS == nil || *r.CPUTimeMS != cpu || r.MemoryBytes == nil || *r.MemoryBytes != large {
		t.Fatalf("expected independent maxima, got %+v", r)
	}
	r.Cases = []CaseResult{{CPUTimeMS: &zero}}
	r.summarizeUsage()
	if r.CPUTimeMS == nil || *r.CPUTimeMS != 0 || r.MemoryBytes != nil {
		t.Fatal("zero is measured; missing memory is not zero")
	}
	r.Cases = nil
	r.summarizeUsage()
	if r.CPUTimeMS != nil || r.MemoryBytes != nil {
		t.Fatal("unmeasured result must stay empty")
	}
}

func TestPublicSubmissionScores(t *testing.T) {
	cpu, memory, score := 1.0, int64(1024), int64(MaxCaseScore)
	original := Submission{Result: &Result{
		Verdict: "WA", Passed: 1, Total: 2, CheckerLog: "集計コードの標準エラー:\nprivate aggregator log",
		Cases: []CaseResult{
			{Name: "01", Verdict: "AC", CPUTimeMS: &cpu, WallTimeMS: &cpu, MemoryBytes: &memory, Score: &score},
			{Name: "02", Verdict: "WA", CPUTimeMS: &cpu, WallTimeMS: &cpu, MemoryBytes: &memory},
		},
		Scoring: &SubmissionScore{Score: 3750, MaxScore: 10000, Groups: []GroupScore{
			{Name: "小課題1", Score: 3000, MaxScore: 3000, Cases: []int{0}},
			{Name: "小課題2", Score: 750, MaxScore: 7000, Cases: []int{0, 1}},
		}},
	}}
	got := publicSubmission(original)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"score":1000000000000000000`, `"scoring":{"score":3750,"maxScore":10000`, `{"name":"小課題2","score":750,"maxScore":7000,"cases":[0,1]}`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s: %s", want, data)
		}
	}
	if strings.Contains(string(data), "private") || strings.Contains(string(data), "checkerLog") {
		t.Fatalf("exposed the aggregator log: %s", data)
	}
	// Lists show the score without the group table, and leave the detail alone.
	got.Result.Scoring.Groups = nil
	if len(original.Result.Scoring.Groups) != 2 {
		t.Fatal("modified original groups")
	}
	if team := teamSubmission(original, true); team.Result.Scoring == nil || team.Result.Cases[0].Score == nil {
		t.Fatal("teammates lost the scores")
	}
}
