package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"judge/api/internal/submissions"
)

// The contest's submission list takes a page size and counts every submission it may show.
func TestContestSubmissionPagesPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol")
	const (
		pid = "11111111-1111-4111-8111-111111111111"
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	f.problem(pid)
	f.create(cid, pid, nil)
	f.join(cid, "bob", nil)
	f.join(cid, "carol", nil)
	f.schedule(cid, "running")
	start := time.Now().Add(-30 * time.Minute)
	for i, owner := range []string{"bob", "carol", "bob", "carol", "bob"} {
		f.run(cid, owner, pid, "DONE", acceptedResult, start.Add(time.Duration(i)*time.Minute))
	}
	f.schedule(cid, "ended")

	list := func(query string) submissions.CountedSubmissionList {
		t.Helper()
		var page submissions.CountedSubmissionList
		f.decode(f.request("GET", "/contests/"+cid+"/submissions"+query, "", nil, 200), &page)
		return page
	}
	page := list("?limit=2")
	if len(page.Items) != 2 || !page.HasMore || page.Total != 5 || page.Items[0].Author != "bob" || !page.Items[0].CreatedAt.After(page.Items[1].CreatedAt) {
		t.Fatalf("first page: %+v", page)
	}
	if page = list("?offset=4&limit=2"); len(page.Items) != 1 || page.HasMore || page.Total != 5 {
		t.Fatalf("last page: %+v", page)
	}
	if page = list("?offset=10&limit=2"); len(page.Items) != 0 || page.HasMore || page.Total != 5 {
		t.Fatalf("past the end: %+v", page)
	}
	if page = list(""); len(page.Items) != 5 || page.HasMore || page.Total != 5 {
		t.Fatalf("default size: %+v", page)
	}
	for _, q := range []string{"limit=0", "limit=101", "limit=abc"} {
		f.fail("GET", "/contests/"+cid+"/submissions?"+q, "", nil, 400, "invalid_request")
	}
	// The list of a problem has no total.
	var problem map[string]json.RawMessage
	f.decode(f.request("GET", "/contests/"+cid+"/problems/"+pid+"/submissions", "", nil, 200), &problem)
	if _, ok := problem["total"]; ok {
		t.Fatalf("problem list counts: %v", problem)
	}
}

// A problem narrows the contest's list, the viewer's own included, and the total counts only that problem's.
func TestContestSubmissionProblemFilterPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol")
	const (
		first  = "11111111-1111-4111-8111-111111111111"
		second = "22222222-2222-4222-8222-222222222222"
		cid    = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	f.problem(first)
	f.problem(second)
	f.create(cid, first, map[string]any{"problems": []map[string]any{{"id": first, "points": 100}, {"id": second, "points": 200}}})
	f.join(cid, "bob", nil)
	f.join(cid, "carol", nil)
	f.schedule(cid, "running")
	start := time.Now().Add(-30 * time.Minute)
	for i, run := range []struct{ owner, problem string }{{"bob", first}, {"carol", second}, {"bob", second}, {"carol", first}, {"bob", second}} {
		f.run(cid, run.owner, run.problem, "DONE", acceptedResult, start.Add(time.Duration(i)*time.Minute))
	}

	list := func(query, owner string) submissions.CountedSubmissionList {
		t.Helper()
		var page submissions.CountedSubmissionList
		path := "/contests/" + cid + "/submissions" + query
		if owner != "" {
			path = "/my" + path
		}
		f.decode(f.request("GET", path, owner, nil, 200), &page)
		return page
	}
	// bob's own list is open while the contest runs.
	if page := list("?mine=1&problem="+second, "bob"); page.Total != 2 || len(page.Items) != 2 || page.Items[0].ProblemID != second || page.Items[1].ProblemID != second {
		t.Fatalf("own list of the second problem: %+v", page)
	}
	if page := list("?mine=1", "bob"); page.Total != 3 {
		t.Fatalf("own list of every problem: %+v", page)
	}
	f.schedule(cid, "ended")
	page := list("?problem="+second+"&limit=1", "")
	if page.Total != 3 || len(page.Items) != 1 || !page.HasMore || page.Items[0].ProblemID != second {
		t.Fatalf("all of the second problem: %+v", page)
	}
	if page = list("?problem="+first, ""); page.Total != 2 || page.Items[0].Author != "carol" || page.Items[1].Author != "bob" {
		t.Fatalf("all of the first problem: %+v", page)
	}
	// A problem outside the contest leaves nothing.
	if page = list("?problem=33333333-3333-4333-8333-333333333333", ""); page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("outside problem: %+v", page)
	}
	for _, q := range []string{"problem=A", "problem=1"} {
		f.fail("GET", "/contests/"+cid+"/submissions?"+q, "", nil, 400, "invalid_request")
	}
}
