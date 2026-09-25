package problems

import (
	"testing"
	"time"
)

func TestNextFeatured(t *testing.T) {
	for _, pair := range [][2]string{
		{"2026-09-28T13:59:59Z", "2026-09-28T14:00:00Z"},
		{"2026-09-28T14:00:00Z", "2026-10-01T14:00:00Z"},
		{"2026-10-01T14:00:00Z", "2026-10-05T14:00:00Z"},
		{"2026-12-31T14:00:00Z", "2027-01-04T14:00:00Z"},
	} {
		from, _ := time.Parse(time.RFC3339, pair[0])
		want, _ := time.Parse(time.RFC3339, pair[1])
		got := NextFeatured(from)
		if !got.Equal(want) {
			t.Fatalf("%s: got %s want %s", from, got, want)
		}
		if got.Add(23*time.Hour).In(time.FixedZone("JST", 9*3600)).Hour() != 22 {
			t.Fatal("wrong reveal time")
		}
	}
}
