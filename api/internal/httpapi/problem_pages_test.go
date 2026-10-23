package httpapi

import (
	"net/url"
	"testing"
	"time"

	"judge/api/internal/problems"
)

// The public problem list takes an offset and a page size, and counts every problem it may show.
func TestPublicProblemPagesPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	start := time.Now().Add(-time.Hour)
	for i, id := range ids {
		f.problem(id)
		f.publish("alice", id, true, 200)
		f.exec(`UPDATE problem_drafts SET published_at=$2 WHERE id=$1`, id, start.Add(time.Duration(i)*time.Minute))
	}

	type page struct {
		Items      []problems.PublicProblem
		NextCursor string
		Total      int
	}
	list := func(query string) page {
		t.Helper()
		var p page
		f.decode(f.request("GET", "/problems?author=alice"+query, "", nil, 200), &p)
		return p
	}
	check := func(name string, p page, want []string, more bool, total int) {
		t.Helper()
		var got []string
		for _, item := range p.Items {
			got = append(got, item.ID)
		}
		if len(got) != len(want) || (len(want) > 0 && (got[0] != want[0] || got[len(got)-1] != want[len(want)-1])) || (p.NextCursor != "") != more || p.Total != total {
			t.Fatalf("%s: got %v next=%q total=%d, want %v more=%v total=%d", name, got, p.NextCursor, p.Total, want, more, total)
		}
	}
	newest := []string{ids[2], ids[1], ids[0]}
	check("default", list(""), newest, false, 3)
	first := list("&limit=2")
	check("limit", first, newest[:2], true, 3)
	check("cursor", list("&limit=2&cursor="+url.QueryEscape(first.NextCursor)), newest[2:], false, 3)
	check("offset", list("&offset=1&limit=1"), newest[1:2], true, 3)
	check("past the end", list("&offset=3"), nil, false, 3)
	var other page
	f.decode(f.request("GET", "/problems?author=bob", "", nil, 200), &other)
	check("other author", other, nil, false, 0)

	f.fail("GET", "/problems?author=alice&offset=1&cursor="+url.QueryEscape(first.NextCursor), "", nil, 400, "invalid_request")
	f.fail("GET", "/problems?author=alice&limit=0", "", nil, 400, "invalid_request")
	f.fail("GET", "/problems?author=alice&limit=101", "", nil, 400, "invalid_request")
}
