package problems

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func scoredDraft(mode string, groups ...ScoringGroup) Draft {
	difficulty := 3
	return Draft{
		Difficulty: &difficulty, Title: "A", Markdown: "本文", Editorial: "解説", TimeLimitMS: "1000", MemoryLimitMB: "256",
		Scoring: &Scoring{Mode: mode, Groups: groups},
		TestCases: []TestCase{
			{IsSample: true, Name: "sample", Input: "1", Output: "1"},
			{Name: "01", Input: "1", Output: "1", Groups: []string{"小課題1", "小課題2"}},
			{Name: "02", Input: "1", Output: "1", Groups: []string{"小課題2"}},
		},
	}
}

// The structure follows the judge's limits (ADR 0025), so that a saved draft never turns into a JE job.
func TestValidDraftChecksScoringStructure(t *testing.T) {
	known := []string{"cpp17", "cpp23-gcc", "python314"}
	groups := []ScoringGroup{{Name: "小課題1", Points: 30}, {Name: "小課題2", Points: 70}}
	aggregator := &Generator{Runtime: "python314", Source: "print(1)"}
	with := func(change func(*Draft)) Draft {
		d := scoredDraft(ScoringGroups, groups...)
		d.Scoring.Groups = append([]ScoringGroup(nil), groups...)
		change(&d)
		return d
	}
	many := make([]ScoringGroup, MaxScoringGroups+1)
	for i := range many {
		many[i] = ScoringGroup{Name: "小課題" + strconv.Itoa(i+1), Points: 1}
	}
	for _, tc := range []struct {
		name  string
		draft Draft
		valid bool
	}{
		{"groups", with(func(*Draft) {}), true},
		{"aggregate with its aggregator", with(func(d *Draft) { d.Scoring.Mode, d.Scoring.Aggregator = ScoringAggregate, aggregator }), true},
		// What the editor saves on the way is reported as content issues instead.
		{"aggregate without an aggregator", with(func(d *Draft) { d.Scoring.Mode = ScoringAggregate }), true},
		{"no groups yet", with(func(d *Draft) {
			d.Scoring.Groups = nil
			for i := range d.TestCases {
				d.TestCases[i].Groups = nil
			}
		}), true},
		{"a sample in a group, a group without cases and a case without groups", with(func(d *Draft) {
			d.TestCases[0].Groups, d.TestCases[2].Groups = []string{"小課題1"}, []string{}
		}), true},
		{"a group worth nothing beside others", with(func(d *Draft) { d.Scoring.Groups = append(d.Scoring.Groups, ScoringGroup{Name: "おまけ"}) }), true},
		{"the most groups and points", with(func(d *Draft) {
			d.Scoring.Groups = slices.Clone(many[:MaxScoringGroups])
			d.Scoring.Groups[0].Points = MaxFullScore - MaxScoringGroups + 1
			d.TestCases = d.TestCases[:1]
		}), true},
		{"unknown mode", with(func(d *Draft) { d.Scoring.Mode = "custom" }), false},
		{"empty mode", with(func(d *Draft) { d.Scoring.Mode = "" }), false},
		{"aggregator outside aggregate mode", with(func(d *Draft) { d.Scoring.Aggregator = aggregator }), false},
		{"too many groups", with(func(d *Draft) { d.Scoring.Groups = many; d.TestCases = d.TestCases[:1] }), false},
		{"duplicate name", with(func(d *Draft) { d.Scoring.Groups[1].Name = "小課題1" }), false},
		{"duplicate name with spaces", with(func(d *Draft) {
			d.Scoring.Groups = append(d.Scoring.Groups, ScoringGroup{Name: " 小課題1 ", Points: 1})
		}), false},
		{"blank name", with(func(d *Draft) { d.Scoring.Groups = append(d.Scoring.Groups, ScoringGroup{Name: " ", Points: 1}) }), false},
		{"long name", with(func(d *Draft) {
			d.Scoring.Groups = append(d.Scoring.Groups, ScoringGroup{Name: strings.Repeat("長", 65), Points: 1})
		}), false},
		{"control character", with(func(d *Draft) { d.Scoring.Groups = append(d.Scoring.Groups, ScoringGroup{Name: "a\tb", Points: 1}) }), false},
		{"negative points", with(func(d *Draft) { d.Scoring.Groups[0].Points = -1 }), false},
		{"group over the full score", with(func(d *Draft) { d.Scoring.Groups[0].Points = MaxFullScore + 1 }), false},
		{"full score over the bound", with(func(d *Draft) { d.Scoring.Groups[0].Points, d.Scoring.Groups[1].Points = 600000, 400001 }), false},
		{"full score of 0", with(func(d *Draft) { d.Scoring.Groups[0].Points, d.Scoring.Groups[1].Points = 0, 0 }), false},
		{"unknown group", with(func(d *Draft) { d.TestCases[1].Groups = []string{"小課題3"} }), false},
		{"group named with other spaces", with(func(d *Draft) { d.TestCases[1].Groups = []string{"小課題1 "} }), false},
		{"group listed twice", with(func(d *Draft) { d.TestCases[1].Groups = []string{"小課題1", "小課題1"} }), false},
		{"aggregator in an unknown runtime", with(func(d *Draft) {
			d.Scoring.Mode, d.Scoring.Aggregator = ScoringAggregate, &Generator{Runtime: "cobol", Source: "x"}
		}), false},
		{"aggregator with a protocol", with(func(d *Draft) {
			d.Scoring.Mode, d.Scoring.Aggregator = ScoringAggregate, &Generator{Runtime: "cpp23-gcc", Source: "x", Protocol: "testlib"}
		}), false},
		{"aggregator over 64 KiB", with(func(d *Draft) {
			d.Scoring.Mode, d.Scoring.Aggregator = ScoringAggregate, &Generator{Runtime: "cpp17", Source: strings.Repeat("x", 65537)}
		}), false},
		{"aggregator with NUL", with(func(d *Draft) {
			d.Scoring.Mode, d.Scoring.Aggregator = ScoringAggregate, &Generator{Runtime: "cpp17", Source: "a\x00"}
		}), false},
	} {
		if got := ValidDraft(tc.draft, known); got != tc.valid {
			t.Errorf("%s: valid=%v, want %v", tc.name, got, tc.valid)
		}
	}
	// Membership needs scoring; an empty list is no membership.
	plain := with(func(d *Draft) { d.Scoring = nil })
	if ValidDraft(plain, known) {
		t.Error("membership without scoring")
	}
	for i := range plain.TestCases {
		plain.TestCases[i].Groups = []string{}
	}
	if !ValidDraft(plain, known) {
		t.Error("empty membership without scoring")
	}
}

func TestContentIssuesForPartialScoring(t *testing.T) {
	known := []string{"cpp17", "python314"}
	enabled := []string{"cpp17"}
	with := func(mode string, change func(*Draft)) Draft {
		d := scoredDraft(mode, ScoringGroup{Name: "小課題1", Points: 30}, ScoringGroup{Name: "小課題2", Points: 70})
		change(&d)
		return d
	}
	for _, tc := range []struct {
		name  string
		draft Draft
		want  []Issue
	}{
		{"groups ready", with(ScoringGroups, func(*Draft) {}), []Issue{}},
		{"an ungrouped sample is fine", with(ScoringGroups, func(d *Draft) { d.TestCases[0].Groups = nil }), []Issue{}},
		{"aggregate ready", with(ScoringAggregate, func(d *Draft) { d.Scoring.Aggregator = &Generator{Runtime: "cpp17", Source: "int main(){}"} }), []Issue{}},
		{"no groups", with(ScoringGroups, func(d *Draft) {
			d.Scoring.Groups = nil
			d.TestCases[1].Groups, d.TestCases[2].Groups = nil, nil
		}), []Issue{IssueScoringGroups, IssueTestCaseUngrouped}},
		{"a group without cases", with(ScoringGroups, func(d *Draft) { d.TestCases[1].Groups = []string{"小課題2"} }), []Issue{IssueScoringGroupEmpty}},
		{"a group of samples only is not empty", with(ScoringGroups, func(d *Draft) {
			d.TestCases[0].Groups, d.TestCases[1].Groups = []string{"小課題1"}, []string{"小課題2"}
		}), []Issue{}},
		{"a case in no group", with(ScoringGroups, func(d *Draft) {
			d.TestCases = append(d.TestCases, TestCase{Name: "03", Input: "1", Output: "1"})
		}), []Issue{IssueTestCaseUngrouped}},
		{"aggregator missing", with(ScoringAggregate, func(*Draft) {}), []Issue{IssueAggregatorSource}},
		{"aggregator blank in a disabled runtime", with(ScoringAggregate, func(d *Draft) {
			d.Scoring.Aggregator = &Generator{Runtime: "python314", Source: " \n"}
		}), []Issue{IssueAggregatorSource, IssueAggregatorRuntime}},
	} {
		if got := ContentIssues(tc.draft, known, enabled); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestScoringDefinition(t *testing.T) {
	a := &Scoring{Mode: ScoringGroups, Groups: []ScoringGroup{{Name: "1", Points: 30}, {Name: "2", Points: 70}}}
	same := &Scoring{Mode: ScoringGroups, Groups: []ScoringGroup{{Name: "1", Points: 30}, {Name: "2", Points: 70}}, Aggregator: &Generator{Runtime: "cpp17"}}
	if !a.SameDefinition(same) || !(*Scoring)(nil).SameDefinition(nil) || a.FullScore() != 100 || (*Scoring)(nil).FullScore() != 0 {
		t.Fatal("same definition")
	}
	for _, other := range []*Scoring{
		nil,
		{Mode: ScoringAggregate, Groups: a.Groups},
		{Mode: ScoringGroups, Groups: []ScoringGroup{{Name: "2", Points: 70}, {Name: "1", Points: 30}}},
		{Mode: ScoringGroups, Groups: []ScoringGroup{{Name: "1", Points: 30}, {Name: "2", Points: 71}}},
		{Mode: ScoringGroups, Groups: []ScoringGroup{{Name: "1", Points: 30}, {Name: "二", Points: 70}}},
		{Mode: ScoringGroups, Groups: a.Groups[:1]},
	} {
		if a.SameDefinition(other) || other.SameDefinition(a) {
			t.Fatalf("changed definition %+v", other)
		}
	}
	d := Draft{Scoring: &Scoring{Mode: ScoringAggregate, Groups: a.Groups, Aggregator: &Generator{Runtime: "cpp17", Source: "secret"}}}
	if got := d.PublicScoring(); got == nil || got.FullScore != 100 || !reflect.DeepEqual(got.Groups, a.Groups) || (Draft{}).PublicScoring() != nil {
		t.Fatalf("public scoring %+v", got)
	}
	if empty := (Draft{Scoring: &Scoring{Mode: ScoringGroups}}).PublicScoring(); empty.Groups == nil {
		t.Fatal("groups must be a list")
	}
}
