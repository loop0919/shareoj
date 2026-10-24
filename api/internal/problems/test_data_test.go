package problems

import "testing"

func TestNormalizeTestData(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"1 2\n", "1 2\n"},
		{"1 2", "1 2\n"},
		{"1 2\r\n3\r\n", "1 2\n3\n"},
		{"a\rb\r", "a\nb\n"},
		{"a\r\r\nb", "a\n\nb\n"},
		{"8\n\n", "8\n\n"},
	} {
		if got := NormalizeTestData(tt.in); got != tt.want {
			t.Errorf("NormalizeTestData(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
