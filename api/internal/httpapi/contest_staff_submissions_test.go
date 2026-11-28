package httpapi

import (
	"slices"
	"strings"
	"testing"
	"time"

	"judge/api/internal/submissions"
)

// Before the start, the author and testers of a problem review the submissions to the problems they can open, and the
// setters review every problem's. From the start, staff review every submission of the contest.
func TestContestStaffSubmissionsBeforeStartPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "tess")
	const (
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		pa  = "a0000000-0000-4000-8000-000000000001" // tested by tess
		pb  = "a0000000-0000-4000-8000-000000000002" // tess cannot open it before the start
	)
	f.problem(pa)
	f.problem(pb)
	f.create(cid, pa, map[string]any{"problems": []map[string]any{{"id": pa, "points": 100}, {"id": pb, "points": 100}}})
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tess')`, pa)

	// The setter tries both problems, in the contest and from the problem's own page.
	run := func(problem string, inContest bool) string {
		t.Helper()
		id := newSubmissionID()
		var contest any
		if inContest {
			contest = cid
		}
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at,finished_at)
 VALUES($1,'alice',$2,1,'Team','cpp17','code',jsonb_build_object('privateDraft',$3::boolean),'DONE',$4::jsonb,$5,$6,$6)`,
			id, problem, !inContest, acceptedResult, contest, time.Now())
		return id
	}
	contestA, contestB := run(pa, true), run(pb, true)
	draftA, draftB := run(pa, false), run(pb, false)
	own, others := []string{contestA, draftA}, []string{contestB, draftB}

	listed := func(viewer, path string) []string {
		t.Helper()
		var page submissions.CountedSubmissionList
		f.decode(f.request("GET", path, viewer, nil, 200), &page)
		ids := []string{}
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		if strings.HasPrefix(path, "/my/contests/"+cid+"/submissions") && page.Total != len(ids) {
			t.Fatalf("%s counts %d of %v for %s", path, page.Total, ids, viewer)
		}
		slices.Sort(ids)
		return ids
	}
	sorted := func(ids ...string) []string {
		slices.Sort(ids)
		return ids
	}
	all := "/my/contests/" + cid + "/submissions"
	byProblem := func(problem string) string { return "/my/contests/" + cid + "/problems/" + problem + "/submissions" }

	if got := listed("alice", all); !slices.Equal(got, sorted(contestA, contestB, draftA, draftB)) {
		t.Fatalf("setter before the start: %v", got)
	}
	if got := listed("tess", all); !slices.Equal(got, sorted(own...)) {
		t.Fatalf("tester before the start: got %v, want %v", got, sorted(own...))
	}
	if got := listed("tess", byProblem(pa)); !slices.Equal(got, sorted(own...)) {
		t.Fatalf("tester's own problem before the start: %v", got)
	}
	// Naming the setter and the verdict does not open the problem the tester cannot open.
	if got := listed("tess", all+"?user=alice&verdict=AC&sort=shortest"); !slices.Equal(got, sorted(own...)) {
		t.Fatalf("tester's conditions before the start: %v", got)
	}
	f.request("GET", byProblem(pb), "tess", nil, 404)
	for _, id := range own {
		f.request("GET", all+"/"+id, "tess", nil, 200)
		f.request("GET", "/my/submissions/"+id+"/public", "tess", nil, 200)
	}
	for _, id := range others {
		f.request("GET", all+"/"+id, "tess", nil, 404)
		f.request("GET", "/my/submissions/"+id+"/public", "tess", nil, 404)
	}

	f.schedule(cid, "running")
	if got := listed("tess", all); !slices.Equal(got, sorted(contestA, contestB, draftA, draftB)) {
		t.Fatalf("tester after the start: %v", got)
	}
	if got := listed("tess", byProblem(pb)); !slices.Equal(got, sorted(others...)) {
		t.Fatalf("another problem after the start: %v", got)
	}
	for _, id := range others {
		f.request("GET", all+"/"+id, "tess", nil, 200)
	}
}
