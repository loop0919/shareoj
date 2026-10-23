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
