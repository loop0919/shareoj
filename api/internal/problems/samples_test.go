package problems

import "testing"

func TestHasSamples(t *testing.T) {
	for _, tt := range []struct {
		name  string
		cases []TestCase
		want  bool
	}{
		{"no cases", nil, false},
		{"non-sample cases", []TestCase{{Input: "1", Output: "2"}}, false},
		{"empty sample", []TestCase{{IsSample: true}}, true},
		{"mixed cases", []TestCase{{}, {IsSample: true}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Draft{TestCases: tt.cases}).HasSamples(); got != tt.want {
				t.Fatalf("HasSamples() = %v, want %v", got, tt.want)
			}
		})
	}
}
