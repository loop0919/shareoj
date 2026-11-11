package problems

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func marginPtr(s string) *string { return &s }

func TestErrorJudgeShape(t *testing.T) {
	for _, tc := range []struct {
		name  string
		e     ErrorJudge
		valid bool
	}{
		{"absolute", ErrorJudge{Mode: "absolute", Epsilon: "1e-1"}, true},
		{"relative", ErrorJudge{Mode: "relative", Epsilon: "1e-12"}, true},
		{"either", ErrorJudge{Mode: "either", Epsilon: "1e-6"}, true},
		{"margin 0", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("0")}, true},
		{"margin 0.5", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("0.5")}, true},
		{"margin 100", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("100")}, true},
		{"margin 100.0", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("100.000000")}, true},
		{"margin 099", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("099")}, true},
		{"margin 000", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("000")}, true},
		{"margin 12.345678", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("12.345678")}, true},
		{"mode both", ErrorJudge{Mode: "both", Epsilon: "1e-6"}, false},
		{"mode empty", ErrorJudge{Epsilon: "1e-6"}, false},
		{"eps 1e-13", ErrorJudge{Mode: "either", Epsilon: "1e-13"}, false},
		{"eps 1e-0", ErrorJudge{Mode: "either", Epsilon: "1e-0"}, false},
		{"eps 1e-01", ErrorJudge{Mode: "either", Epsilon: "1e-01"}, false},
		{"eps decimal", ErrorJudge{Mode: "either", Epsilon: "0.000001"}, false},
		{"eps empty", ErrorJudge{Mode: "either"}, false},
		{"margin 100.000001", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("100.000001")}, false},
		{"margin 101", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("101")}, false},
		{"margin -1", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("-1")}, false},
		{"margin 1e1", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("1e1")}, false},
		{"margin .5", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr(".5")}, false},
		{"margin 7 decimals", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("0.1234567")}, false},
		{"margin 4 digits", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("1000")}, false},
		{"margin empty", ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("")}, false},
	} {
		if got := tc.e.Valid(); got != tc.valid {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
	for i := 1; i <= 12; i++ {
		if e := (ErrorJudge{Mode: "absolute", Epsilon: fmt.Sprintf("1e-%d", i)}); !e.Valid() {
			t.Errorf("epsilon %s", e.Epsilon)
		}
	}
	var none *ErrorJudge
	if none.Valid() {
		t.Error("nil is not valid")
	}
}

func TestErrorJudgeIsExclusive(t *testing.T) {
	known := []string{"python314"}
	code := &Generator{Runtime: "python314", Source: "x"}
	base := Draft{TimeLimitMS: "1000", MemoryLimitMB: "256"}
	ej := func() *ErrorJudge { return &ErrorJudge{Mode: "either", Epsilon: "1e-6"} }
	for _, tc := range []struct {
		name  string
		edit  func(*Draft)
		valid bool
	}{
		{"alone", func(d *Draft) { d.ErrorJudge = ej() }, true},
		{"with margin", func(d *Draft) {
			d.ErrorJudge = &ErrorJudge{Mode: "relative", Epsilon: "1e-9", MarginPercent: marginPtr("5")}
		}, true},
		{"with checker", func(d *Draft) { d.ErrorJudge, d.Checker = ej(), code }, false},
		{"with interactor", func(d *Draft) { d.ErrorJudge, d.Interactor = ej(), code }, false},
		{"with run twice", func(d *Draft) { d.ErrorJudge, d.RunTwice = ej(), code }, false},
		{"invalid settings", func(d *Draft) { d.ErrorJudge = &ErrorJudge{Mode: "both", Epsilon: "1e-6"} }, false},
	} {
		d := base
		tc.edit(&d)
		if got := ValidDraft(d, known); got != tc.valid {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestErrorJudgeReadinessHasNoJudgeCodeIssues(t *testing.T) {
	d := Draft{ErrorJudge: &ErrorJudge{Mode: "either", Epsilon: "1e-6"}, TimeLimitMS: "1000"}
	for _, issue := range ContentIssues(d, []string{"python314"}, []string{"cpp23-gcc"}) {
		if strings.HasSuffix(string(issue), "_source_missing") || strings.HasSuffix(string(issue), "_runtime_unavailable") {
			t.Fatal(issue)
		}
	}
}

func TestErrorJudgeDraftJSONKeepsMarginOmitted(t *testing.T) {
	var d Draft
	if err := json.Unmarshal([]byte(`{"errorJudge":{"mode":"either","epsilon":"1e-6"}}`), &d); err != nil || d.ErrorJudge == nil || d.ErrorJudge.MarginPercent != nil {
		t.Fatal(d, err)
	}
	b, _ := json.Marshal(d)
	if !strings.Contains(string(b), `"errorJudge":{"mode":"either","epsilon":"1e-6"}`) {
		t.Fatal(string(b))
	}
	if err := json.Unmarshal([]byte(`{"errorJudge":{"mode":"either","epsilon":"1e-6","marginPercent":null}}`), &d); err != nil || d.ErrorJudge.MarginPercent != nil || !d.ErrorJudge.Valid() {
		t.Fatal(d, err)
	}
	b, _ = json.Marshal(Draft{})
	if strings.Contains(string(b), "errorJudge") {
		t.Fatal(string(b))
	}
}

func TestErrorJudgePublicProblemHasNoMargin(t *testing.T) {
	var p PublicProblem
	p.SetStatement(Draft{ErrorJudge: &ErrorJudge{Mode: "either", Epsilon: "1e-6", MarginPercent: marginPtr("7")}})
	b, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(b), `"errorJudge":{"mode":"either","epsilon":"1e-6"}`) || strings.Contains(string(b), "marginPercent") {
		t.Fatal(string(b), err)
	}
	var q PublicProblem
	q.SetStatement(Draft{})
	b, _ = json.Marshal(q)
	if strings.Contains(string(b), "errorJudge") {
		t.Fatal(string(b))
	}
}
