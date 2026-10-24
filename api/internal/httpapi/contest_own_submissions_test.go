package httpapi

import (
	"testing"
	"time"

	"judge/api/internal/submissions"
)

// A contest's own list gathers the viewer's submissions to all its problems at any time, like each problem's own list.
func TestContestOwnSubmissionsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dave", "erin")
	const (
		p1   = "11111111-1111-4111-8111-111111111111"
		p2   = "22222222-2222-4222-8222-222222222222"
		p3   = "33333333-3333-4333-8333-333333333333"
		cid  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		team = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	for _, id := range []string{p1, p2, p3} {
		f.problem(id)
	}
	f.create(cid, p1, map[string]any{"problems": []map[string]any{{"id": p1, "points": 100}, {"id": p2, "points": 100}}})
	f.create(team, p3, map[string]any{"teamMode": true})
	f.join(cid, "bob", nil)
	f.join(cid, "carol", nil)
	f.join(team, "dave", create("Team"))
	f.join(team, "erin", joinCode(f.team(team, "dave").Code))
	base := "/my/contests/" + cid + "/submissions"
	list := func(path, viewer string) submissions.CountedSubmissionList {
		t.Helper()
		var page submissions.CountedSubmissionList
		f.decode(f.request("GET", path, viewer, nil, 200), &page)
		return page
	}
	authors := func(page submissions.CountedSubmissionList) []string {
		names := []string{}
		for _, item := range page.Items {
			names = append(names, item.ProblemID[:1]+":"+item.Author)
		}
		return names
	}
	// Before the start the list is empty rather than closed, since the menu offers it from the first visit.
	if page := list(base+"?mine=1", "bob"); len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("before the start: %+v", page)
	}
	f.schedule(cid, "running")
	f.schedule(team, "running")
	start := time.Now().Add(-30 * time.Minute)
	f.run(cid, "bob", p1, "DONE", acceptedResult, start)
	f.run(cid, "carol", p1, "DONE", acceptedResult, start.Add(time.Minute))
	f.run(cid, "bob", p2, "DONE", acceptedResult, start.Add(2*time.Minute))
	f.run("", "bob", p1, "DONE", acceptedResult, start.Add(3*time.Minute))
	f.run(team, "dave", p3, "DONE", acceptedResult, start)
	f.run(team, "erin", p3, "DONE", acceptedResult, start.Add(time.Minute))

	// While running, a participant lists their own submissions to every problem, but not the whole contest.
	page := list(base+"?mine=1", "bob")
	if got := authors(page); len(got) != 2 || got[0] != "2:bob" || got[1] != "1:bob" || page.Total != 2 || page.HasMore {
		t.Fatalf("bob: %v %+v", got, page)
	}
	if got := authors(list(base+"?mine=1&limit=1", "bob")); len(got) != 1 || got[0] != "2:bob" {
		t.Fatalf("page size: %v", got)
	}
	if got := authors(list(base+"?mine=1", "carol")); len(got) != 1 || got[0] != "1:carol" {
		t.Fatalf("carol: %v", got)
	}
	f.request("GET", base, "bob", nil, 404)
	// The team's list holds the teammates' submissions too; the own list does not.
	teamBase := "/my/contests/" + team + "/submissions"
	if got := authors(list(teamBase+"?mine=team", "dave")); len(got) != 2 || got[0] != "3:erin" || got[1] != "3:dave" {
		t.Fatalf("team: %v", got)
	}
	if got := authors(list(teamBase+"?mine=1", "dave")); len(got) != 1 || got[0] != "3:dave" {
		t.Fatalf("dave: %v", got)
	}
	f.fail("GET", "/contests/"+cid+"/submissions?mine=1", "", nil, 401, "authentication_required")
	f.fail("GET", base+"?mine=all", "bob", nil, 400, "invalid_request")
	f.fail("GET", "/my/contests/eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee/submissions?mine=1", "bob", nil, 404, "contest_not_found")

	// After the end practice joins the list.
	f.schedule(cid, "ended")
	f.run(cid, "bob", p1, "DONE", acceptedResult, time.Now())
	if got := authors(list(base+"?mine=1", "bob")); len(got) != 3 || got[0] != "1:bob" {
		t.Fatalf("practice: %v", got)
	}
	// Staff see their own runs of the contest's problems outside it, as on each problem's own list.
	f.run("", "alice", p2, "DONE", acceptedResult, start)
	if got := authors(list(base+"?mine=1", "alice")); len(got) != 1 || got[0] != "2:alice" {
		t.Fatalf("staff: %v", got)
	}
}
