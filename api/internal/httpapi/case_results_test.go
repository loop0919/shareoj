package httpapi

import (
	"strings"
	"testing"
)

// The case names hint at the tests, so a contest and a 定期便 round show only the verdict counts until they end.
func TestCaseResultsHiddenWhileOngoingPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	const result = `{"verdict":"WA","passed":1,"total":3,"scoring":{"score":0,"maxScore":10000,"groups":[{"name":"all","score":0,"maxScore":10000,"cases":[0,1,2]}]},
 "cases":[{"name":"CASE_small","verdict":"AC","cpuTimeMs":5,"memoryBytes":1000,"score":4},{"name":"CASE_max","verdict":"WA","cpuTimeMs":7,"memoryBytes":3000},{"name":"CASE_rest","verdict":"SKIPPED"}]}`
	submit := func(owner, problem, contest, job string) string {
		t.Helper()
		sid := newSubmissionID()
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,contest_id,problem_version,problem_title,runtime,source,job,status,result)
          VALUES($1,$2,$3,NULLIF($4,'')::uuid,1,'Cases','cpp17','source',$5,'DONE',$6)`, sid, owner, problem, contest, job, result)
		return sid
	}
	// shown reads the submission and tells whether it lists the cases; a hidden one must give the counts instead.
	shown := func(path, viewer string) bool {
		t.Helper()
		body := f.request("GET", path, viewer, nil, 200)
		named := strings.Contains(body, "CASE_")
		counted := strings.Contains(body, `"caseVerdicts":{"AC":1,"SKIPPED":1,"WA":1}`)
		if named == counted || !strings.Contains(body, `"cpuTimeMs":7`) || !strings.Contains(body, `"maxScore":10000`) {
			t.Fatalf("%s (%s): %s", path, viewer, body)
		}
		if !named && (strings.Contains(body, `"cases":[{`) || strings.Contains(body, `"cases":[0`) || strings.Contains(body, `"score":4`)) {
			t.Fatalf("%s (%s) leaves case results: %s", path, viewer, body)
		}
		return named
	}
	check := func(path, viewer string, want bool) {
		t.Helper()
		if got := shown(path, viewer); got != want {
			t.Fatalf("%s (%s): cases shown=%v, want %v", path, viewer, got, want)
		}
	}

	// A running team contest: bob and carol compete as a team, alice sets it, and tester tests its problem.
	p := f.draft("alice", 3)
	const contest, team = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	f.exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,team_mode) VALUES($1,'alice','c','d',now()-interval '1 hour',now()+interval '1 hour',true)`, contest)
	f.exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) SELECT $1,id,0,100,draft,version FROM problem_drafts WHERE id=$2`, contest, p.ID)
	f.exec(`INSERT INTO contest_teams(id,contest_id,name,name_key,join_code) VALUES($1,$2,'team','team','code')`, team, contest)
	f.exec(`INSERT INTO contest_participants(contest_id,owner_id,team_id) VALUES($1,'bob',$2),($1,'carol',$2)`, contest, team)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, p.ID)
	inContest := submit("bob", p.ID, contest, `{}`)
	check("/my/submissions/"+inContest, "bob", false)
	check("/my/contests/"+contest+"/submissions/"+inContest, "carol", false)
	check("/my/submissions/"+inContest+"/public", "carol", false)
	check("/my/contests/"+contest+"/submissions/"+inContest, "alice", true)
	check("/my/contests/"+contest+"/submissions/"+inContest, "tester", true)
	// The sample check lists the samples, which the statement shows anyway.
	sample := submit("bob", p.ID, contest, `{"easyTest":true}`)
	check("/my/submissions/"+sample, "bob", true)
	f.exec(`UPDATE contests SET starts_at=now()-interval '2 hours',ends_at=now()-interval '1 minute' WHERE id=$1`, contest)
	check("/my/submissions/"+inContest, "bob", true)
	check("/my/contests/"+contest+"/submissions/"+inContest, "carol", true)
	check("/contests/"+contest+"/submissions/"+inContest, "", true)

	// A public problem that a running 定期便 round revives: its earlier submissions are covered as well.
	q := f.draft("alice", 3)
	f.request("PUT", "/my/problems/"+q.ID+"/publication", "alice", map[string]any{"version": q.Version, "publish": true}, 200)
	revived := submit("bob", q.ID, "", `{"privateDraft":false}`)
	check("/submissions/"+revived, "", true)
	f.exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES(now()-interval '1 hour','easy','revival',$1,3,now()+interval '1 day')`, q.ID)
	check("/submissions/"+revived, "", false)
	check("/problems/"+q.ID+"/submissions/"+revived, "", false)
	check("/my/problems/"+q.ID+"/submissions/"+revived, "carol", false)
	check("/my/submissions/"+revived, "bob", false)
	check("/my/problems/"+q.ID+"/submissions/"+revived, "alice", true)
	f.exec(`UPDATE featured_slots SET reveal_at=now()-interval '1 minute' WHERE problem_id=$1`, q.ID)
	check("/submissions/"+revived, "", true)
	check("/my/submissions/"+revived, "bob", true)
}
