package problems

import "testing"

func TestNormalizeNewlines(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"1 2\n", "1 2\n"},
		{"1 2\r\n3\r\n", "1 2\n3\n"},
		{"a\rb\r", "a\nb\n"},
		{"a\r\r\nb", "a\n\nb"},
	} {
		if got := NormalizeNewlines(tt.in); got != tt.want {
			t.Errorf("NormalizeNewlines(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
