package problems

import (
	"strings"
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

func TestNextFeaturedFromQueue(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	entry := func(id, slot, preference string, hours int) featuredEntry {
		return featuredEntry{featuredCandidate: featuredCandidate{id: id, preference: preference, entered: base.Add(time.Duration(hours) * time.Hour)}, slot: slot}
	}
	queue := []featuredEntry{entry("a", "easy", "later", 0), entry("b", "easy", "soon", 1), entry("c", "hard", "soon", 2), entry("d", "easy", "soon", 3), entry("late", "easy", "soon", 100)}
	at := base.Add(10 * time.Hour)
	var got []string
	for count := 0; count < 4; count++ {
		var e *featuredEntry
		queue, e = nextFeatured(queue, "easy", at, featuredPreference(count))
		if e == nil {
			got = append(got, "-")
			continue
		}
		got = append(got, e.id)
	}
	// soon, soon, later; the fourth turn finds only an application entered after the round.
	if strings.Join(got, ",") != "b,d,a,-" {
		t.Fatal(got)
	}
	if len(queue) != 2 || queue[0].id != "c" || queue[1].id != "late" {
		t.Fatal("queue changed unexpectedly", queue)
	}
	// With the favored preference empty, the other preference is used in entry order.
	queue = []featuredEntry{entry("x", "hard", "later", 5), entry("y", "hard", "later", 4)}
	if _, e := nextFeatured(queue, "hard", at, "soon"); e == nil || e.id != "y" {
		t.Fatal("fallback ignored entry order", e)
	}
}
