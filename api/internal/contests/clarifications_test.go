package contests

import (
	"strings"
	"testing"
)

func TestValidClarificationText(t *testing.T) {
	for _, tc := range []struct {
		text  string
		valid bool
	}{
		{"", false},
		{" \n\t　", false},
		{"質問\n本文", true},
		{strings.Repeat("質", 4000), true},
		{strings.Repeat("質", 4001), false},
		{"bad\x00text", false},
		{string([]byte{0xff}), false},
	} {
		if got := ValidClarificationText(tc.text); got != tc.valid {
			t.Errorf("%d bytes: got %v, want %v", len(tc.text), got, tc.valid)
		}
	}
}
