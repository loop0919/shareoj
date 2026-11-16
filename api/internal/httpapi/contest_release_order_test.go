package httpapi

import (
	"slices"
	"testing"
	"time"

	"judge/api/internal/problems"
)

// TestContestReleaseOrderPostgres pins the order in which a contest's problems are published at its end: A first,
// a millisecond apart, so the newest-first problem list shows the last problem on top instead of following the IDs.
func TestContestReleaseOrderPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	// The IDs sort differently from the labels.
	const (
		a   = "f3000000-0000-4000-8000-000000000002"
		b   = "f3000000-0000-4000-8000-000000000003"
		c   = "f3000000-0000-4000-8000-000000000001"
		cid = "f4000000-0000-4000-8000-000000000001"
	)
	for _, id := range []string{a, b, c} {
		f.problem(id)
	}
	f.create(cid, a, map[string]any{"problems": []map[string]any{{"id": a, "points": 100}, {"id": b, "points": 100}, {"id": c, "points": 100}}})
	f.schedule(cid, "ended")
	var list struct {
		Items []problems.PublicProblem `json:"items"`
	}
	f.decode(f.request("GET", "/problems", "", nil, 200), &list)
	var got []string
	for _, p := range list.Items {
		got = append(got, p.ID)
	}
	if want := []string{c, b, a}; !slices.Equal(got, want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
	// A is published at the end itself, and each later problem a millisecond after the one before.
	end := f.contest(cid, "").EndsAt
	for i, id := range []string{a, b, c} {
		var p problems.PublicProblem
		f.decode(f.request("GET", "/problems/"+id, "", nil, 200), &p)
		if want := end.Add(time.Duration(i) * time.Millisecond); !p.PublishedAt.Equal(want) {
			t.Errorf("%s published at %v, want %v", id, p.PublishedAt, want)
		}
	}
}
