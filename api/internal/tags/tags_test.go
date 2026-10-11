package tags

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"ＤＰ":                    "DP",
		"  segment\u3000 tree ": "segment tree",
		"ｾｸﾞﾒﾝﾄ木":               "セグメント木",
		"C#":                    "C#",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	ok := [][]string{nil, {}, {"DP", "グラフ", "C#", "segment tree", strings.Repeat("あ", MaxLength)}}
	for _, list := range ok {
		if !Valid(list) {
			t.Errorf("Valid(%q) = false", list)
		}
	}
	bad := [][]string{
		{""}, {" DP"}, {"ＤＰ"}, {"segment  tree"}, {"a,b"}, {"a\nb"}, {strings.Repeat("あ", MaxLength+1)},
		{"DP", "dp"}, {"1", "2", "3", "4", "5", "6"},
	}
	for _, list := range bad {
		if Valid(list) {
			t.Errorf("Valid(%q) = true", list)
		}
	}
}
