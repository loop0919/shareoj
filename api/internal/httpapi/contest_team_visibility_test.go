package httpapi

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"judge/api/internal/contests"
	"judge/api/internal/submissions"
)

// ADR 0024: members share their team's submissions, a team contest ranks teams, and the onsite division filters the
// standings and the submission lists.
func TestContestTeamVisibilityPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "tess", "bob", "carol", "erin", "dan", "frank", "sam", "hank", "ivy")
	const (
		tc = "c1000000-0000-4000-8000-000000000001" // team contest with a freeze and an onsite division
		ic = "c1000000-0000-4000-8000-000000000002" // individual contest with an onsite division
		nc = "c1000000-0000-4000-8000-000000000003" // individual contest without a division
		pa = "a1000000-0000-4000-8000-000000000001"
		pb = "a1000000-0000-4000-8000-000000000002"
		pc = "a1000000-0000-4000-8000-000000000003"
		pd = "a1000000-0000-4000-8000-000000000004"
	)
	for _, p := range []string{pa, pb, pc, pd} {
		f.problem(p)
	}
	start := time.Now().Add(time.Hour)
	body := map[string]any{"title": "Teams", "description": "", "startsAt": start, "endsAt": start.Add(2 * time.Hour), "penaltyMinutes": 5, "version": 0,
		"problems": []map[string]any{{"id": pa, "points": 100}, {"id": pb, "points": 200}}, "teamMode": true, "freezeMinutes": 30,
		"onsiteDivision": true, "onsitePassphrase": "Venue_2026"}
	f.request("PUT", "/my/contests/"+tc, "alice", body, 200)
	f.create(ic, pc, map[string]any{"onsiteDivision": true, "onsitePassphrase": "open_sesame"})
	f.create(nc, pd, nil)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tess')`, pa)

	// Alpha enters the onsite division by the passphrase, Gamma by a setter; Beta stays open.
	f.join(tc, "bob", map[string]any{"team": map[string]any{"action": "create", "name": "Alpha"}, "onsitePassphrase": "Venue_2026"})
	f.join(tc, "carol", joinCode(f.team(tc, "bob").Code))
	beta := f.join(tc, "dan", create("Beta")).Team
	gamma := f.join(tc, "frank", create("Gamma")).Team
	f.request("PUT", "/my/contests/"+tc+"/teams/"+gamma.ID+"/onsite", "alice", map[string]any{"onsite": true}, 200)
	f.join(ic, "hank", map[string]any{"onsitePassphrase": "open_sesame"})
	f.join(ic, "ivy", nil)
	f.join(nc, "ivy", nil)
	if c := f.contest(tc, "bob"); c.CanViewSubmissions {
		t.Fatalf("member before the start: %+v", c)
	}
	f.fail("GET", "/my/contests/"+tc+"/submissions", "bob", nil, 404, "contest_not_found")

	// Both contests run for two hours and end in 20 minutes; the team contest froze 10 minutes ago.
	end := time.Now().Add(20 * time.Minute).Truncate(time.Millisecond)
	f.exec(`UPDATE contests SET starts_at=$1,ends_at=$2 WHERE id IN ($3,$4,$5)`, end.Add(-2*time.Hour), end, tc, ic, nc)
	at := func(minutes int) time.Time { return end.Add(time.Duration(minutes-120) * time.Minute) }
	insert := func(id, owner, problem, contest, verdict, job string, created time.Time) {
		t.Helper()
		result := fmt.Sprintf(`{"verdict":%q,"passed":0,"total":1,"checkerLog":"CHECKER SECRET"}`, verdict)
		if verdict == "CE" {
			result = `{"verdict":"CE","passed":0,"total":1,"compileLog":"COMPILE SECRET","checkerLog":"CHECKER SECRET"}`
		}
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at,finished_at)
 VALUES($1,$2,$3,1,'Team','cpp17','code of '||$2,$4::jsonb,'DONE',$5::jsonb,$6,$7,$7)`, id, owner, problem, job, result, contest, created)
	}
	const (
		carolWA     = "d1000000-0000-4000-8000-000000000001"
		bobAC       = "d1000000-0000-4000-8000-000000000002"
		tessRun     = "d1000000-0000-4000-8000-000000000003" // staff
		carolCE     = "d1000000-0000-4000-8000-000000000004"
		danAC       = "d1000000-0000-4000-8000-000000000005"
		bobSample   = "d1000000-0000-4000-8000-000000000006"
		carolFrozen = "d1000000-0000-4000-8000-000000000007"
		danFrozen   = "d1000000-0000-4000-8000-000000000008"
		hankWA      = "d1000000-0000-4000-8000-000000000009"
		ivyAC       = "d1000000-0000-4000-8000-00000000000a"
		bobPractice = "d1000000-0000-4000-8000-00000000000b"
		samPractice = "d1000000-0000-4000-8000-00000000000c"
		ivyNC       = "d1000000-0000-4000-8000-00000000000d"
	)
	insert(carolWA, "carol", pa, tc, "WA", "{}", at(5))
	insert(bobAC, "bob", pa, tc, "AC", "{}", at(10))
	insert(tessRun, "tess", pa, tc, "AC", "{}", at(15))
	insert(carolCE, "carol", pb, tc, "CE", "{}", at(20))
	insert(danAC, "dan", pb, tc, "AC", "{}", at(30))
	insert(bobSample, "bob", pb, tc, "AC", `{"easyTest":true}`, at(40))
	insert(carolFrozen, "carol", pb, tc, "WA", "{}", at(115))
	insert(danFrozen, "dan", pa, tc, "AC", "{}", at(117))
	insert(hankWA, "hank", pc, ic, "WA", "{}", at(10))
	insert(ivyAC, "ivy", pc, ic, "AC", "{}", at(20))
	insert(ivyNC, "ivy", pd, nc, "AC", "{}", at(20))

	sorted := func(list ...string) []string {
		list = slices.Clone(list)
		slices.Sort(list)
		return list
	}
	ids := func(path, viewer string) []string {
		t.Helper()
		var list submissions.ContestSubmissionList
		f.decode(f.request("GET", path, viewer, nil, 200), &list)
		got := []string{}
		for _, item := range list.Items {
			if item.Source != "" || (item.Result != nil && item.Result.CompileLog != "") {
				t.Fatalf("%s lists source or compile errors: %+v", path, item)
			}
			got = append(got, item.ID)
		}
		return sorted(got...)
	}
	expect := func(path, viewer string, want ...string) {
		t.Helper()
		if got := ids(path, viewer); !slices.Equal(got, sorted(want...)) {
			t.Fatalf("%s (%s):\n got %v\nwant %v", path, viewer, got, sorted(want...))
		}
	}
	// detail opens a submission; shared tells whether the viewer also reads the compile errors as a teammate.
	detail := func(path, viewer string, shared bool) {
		t.Helper()
		got := f.request("GET", path, viewer, nil, 200)
		var item submissions.Submission
		f.decode(got, &item)
		if !strings.HasPrefix(item.Source, "code of ") || strings.Contains(got, "CHECKER SECRET") || strings.Contains(got, "COMPILE SECRET") != shared {
			t.Fatalf("%s (%s): %s", path, viewer, got)
		}
	}
	// missing expects a 404: contest routes answer that the contest is not found, the others the submission.
	missing := func(path, viewer string) {
		t.Helper()
		code := "submission_not_found"
		if strings.Contains(path, "/contests/") {
			code = "contest_not_found"
		}
		f.fail("GET", path, viewer, nil, 404, code)
	}
	solved := func(id, viewer string) []bool {
		t.Helper()
		var marks []bool
		for _, p := range f.contest(id, viewer).Problems {
			marks = append(marks, p.Solved)
		}
		return marks
	}

	// During the contest members list and open their team's submissions, frozen ones and compile errors included;
	// other teams, strangers and individual contests stay as before.
	for viewer, want := range map[string]bool{"bob": true, "carol": true, "dan": true, "frank": true, "alice": true, "sam": false} {
		if c := f.contest(tc, viewer); c.CanViewSubmissions != want {
			t.Fatalf("%s may view submissions: %+v", viewer, c)
		}
	}
	if c := f.contest(ic, "hank"); c.CanViewSubmissions {
		t.Fatalf("individual participant: %+v", c)
	}
	list := "/my/contests/" + tc + "/submissions"
	expect(list, "bob", carolWA, bobAC, carolCE, carolFrozen)
	expect(list, "dan", danAC, danFrozen)
	expect(list, "frank")
	expect(list, "alice", carolWA, bobAC, tessRun, carolCE, danAC, carolFrozen, danFrozen)
	missing(list, "sam")
	missing("/contests/"+tc+"/submissions", "")
	missing("/my/contests/"+ic+"/submissions", "hank")
	for _, path := range []string{list + "/" + carolCE, "/my/submissions/" + carolCE + "/public"} {
		detail(path, "bob", true)
		detail(path, "carol", true)
		missing(path, "dan")
		missing(path, "sam")
		missing(path, "hank")
	}
	detail(list+"/"+carolCE, "alice", false)
	detail(list+"/"+carolFrozen, "bob", false)
	missing("/submissions/"+carolCE, "")
	missing("/contests/"+tc+"/submissions/"+carolCE, "")
	// Sample runs stay with their submitter.
	missing(list+"/"+bobSample, "carol")
	missing("/my/submissions/"+bobSample+"/public", "carol")
	f.request("GET", "/my/submissions/"+carolCE, "bob", nil, 404)

	// A contest problem lists the team's submissions next to the viewer's own.
	problem := func(contest, p, query string) string {
		return "/my/contests/" + contest + "/problems/" + p + "/submissions" + query
	}
	expect(problem(tc, pa, "?mine=team"), "bob", carolWA, bobAC)
	expect(problem(tc, pb, "?mine=team"), "bob", carolCE, carolFrozen)
	expect(problem(tc, pa, "?mine=1"), "bob", bobAC)
	expect(problem(tc, pa, "?mine=team"), "dan", danFrozen)
	expect(problem(tc, pa, "?mine=team"), "sam")
	expect(problem(ic, pc, "?mine=team"), "hank", hankWA)
	missing(problem(tc, pa, ""), "bob")
	f.fail("GET", "/contests/"+tc+"/problems/"+pa+"/submissions?mine=team", "", nil, 401, "authentication_required")
	f.fail("GET", problem(tc, pa, "?mine=teams"), "bob", nil, 400, "invalid_request")
	f.fail("GET", "/my/problems/"+pa+"/submissions?mine=team", "alice", nil, 400, "invalid_request")

	// A problem is solved for the team when any member solved it.
	for viewer, want := range map[string][]bool{"bob": {true, false}, "carol": {true, false}, "dan": {true, true}, "frank": {false, false}, "sam": {false, false}} {
		if got := solved(tc, viewer); !slices.Equal(got, want) {
			t.Fatalf("%s solved: %v", viewer, got)
		}
	}
	// A late member sees what the team did before they joined.
	f.join(tc, "erin", joinCode(f.team(tc, "carol").Code))
	expect(list, "erin", carolWA, bobAC, carolCE, carolFrozen)
	if got := solved(tc, "erin"); !slices.Equal(got, []bool{true, false}) || !f.contest(tc, "erin").Onsite {
		t.Fatalf("late member: %v", got)
	}

	// A team contest ranks teams on all their members' submissions, with the freeze and penalty of individual rows.
	table := func(path, viewer string) []string {
		t.Helper()
		var rows []contests.Standing
		f.decode(f.request("GET", path, viewer, nil, 200), &rows)
		lines := []string{}
		for _, r := range rows {
			name := r.Handle
			if r.Team != nil {
				if r.Handle != "" || r.Team.Onsite != r.Onsite {
					t.Fatalf("team row: %+v", r)
				}
				name = r.Team.Name + "(" + strings.Join(r.Team.Members, ",") + ")"
			}
			cells := []string{}
			for _, p := range []string{pa, pb, pc} {
				cell := "-"
				if s := r.Problems[p]; s != nil {
					cell = fmt.Sprintf("%d/%d/%d", s.Points, s.Wrong, s.Frozen)
				}
				cells = append(cells, cell)
			}
			lines = append(lines, fmt.Sprintf("%d/%d %s onsite=%t %d %dm %s", r.Rank, r.OverallRank, name, r.Onsite, r.Points, r.TimeMS/60000, strings.Join(cells, " ")))
		}
		return lines
	}
	check := func(path, viewer string, want ...string) {
		t.Helper()
		if got := table(path, viewer); !slices.Equal(got, want) {
			t.Fatalf("%s (%s):\n got %q\nwant %q", path, viewer, got, want)
		}
	}
	var (
		frozenBeta  = "Beta(dan) onsite=false 200 30m 0/0/1 200/0/0 -"
		frozenAlpha = "Alpha(bob,carol,erin) onsite=true 100 15m 100/1/0 0/0/1 -"
		liveBeta    = "Beta(dan) onsite=false 300 117m 100/0/0 200/0/0 -"
		liveAlpha   = "Alpha(bob,carol,erin) onsite=true 100 15m 100/1/0 0/1/0 -"
		gammaRow    = "Gamma(frank) onsite=true 0 0m - - -"
	)
	standings := "/contests/" + tc + "/standings"
	check(standings, "", "1/1 "+frozenBeta, "2/2 "+frozenAlpha, "3/3 "+gammaRow)
	check(standings+"?scope=onsite", "", "1/2 "+frozenAlpha, "2/3 "+gammaRow)
	check("/my/contests/"+tc+"/standings?scope=onsite&live=1", "bob", "1/2 "+frozenAlpha, "2/3 "+gammaRow)
	check("/my/contests/"+tc+"/standings?live=1", "alice", "1/1 "+liveBeta, "2/2 "+liveAlpha, "3/3 "+gammaRow)
	check("/my/contests/"+tc+"/standings?live=1&scope=onsite", "tess", "1/2 "+liveAlpha, "2/3 "+gammaRow)
	for _, bad := range []string{"all", "Onsite", "open"} {
		f.fail("GET", standings+"?scope="+bad, "", nil, 400, "invalid_request")
		f.fail("GET", list+"?scope="+bad, "alice", nil, 400, "invalid_request")
		f.fail("GET", problem(tc, pa, "?scope="+bad), "alice", nil, 400, "invalid_request")
	}
	// Individual rows keep the handle and gain the onsite mark; a contest without the division has no onsite rows.
	check("/contests/"+ic+"/standings", "", "1/1 ivy onsite=false 100 20m - - 100/0/0", "2/2 hank onsite=true 0 0m - - 0/1/0")
	check("/contests/"+ic+"/standings?scope=onsite", "", "1/2 hank onsite=true 0 0m - - 0/1/0")
	check("/contests/"+nc+"/standings?scope=onsite", "")
	if len(table("/contests/"+nc+"/standings", "")) != 1 {
		t.Fatal("standings without the division")
	}
	// Staff list the onsite division's submissions as they happen.
	expect(list+"?scope=onsite", "alice", carolWA, bobAC, carolCE, carolFrozen)
	expect(list+"?scope=onsite", "dan")

	// After the end, before the reveal, members still see their team's frozen submissions, and others do not.
	shift := -30 * time.Minute
	f.exec(`UPDATE contests SET starts_at=starts_at+$1::interval,ends_at=ends_at+$1::interval WHERE id IN ($2,$3,$4)`, fmt.Sprintf("%d minutes", int(shift.Minutes())), tc, ic, nc)
	f.exec(`UPDATE submissions SET created_at=created_at+$1::interval`, fmt.Sprintf("%d minutes", int(shift.Minutes())))
	end = end.Add(shift)
	insert(bobPractice, "bob", pa, tc, "WA", "{}", end.Add(time.Minute))
	insert(samPractice, "sam", pa, tc, "AC", "{}", end.Add(2*time.Minute))
	public := []string{carolWA, bobAC, tessRun, carolCE, danAC, bobPractice, samPractice}
	expect(list, "sam", public...)
	expect(list, "bob", append(public, carolFrozen)...)
	expect(list, "dan", append(public, danFrozen)...)
	detail(list+"/"+carolFrozen, "erin", false)
	detail(list+"/"+carolCE, "bob", true)
	detail(list+"/"+carolCE, "sam", false)
	missing(list+"/"+carolFrozen, "dan")
	// The problems are public now, and their pages follow the same rules.
	expect("/my/problems/"+pb+"/submissions", "bob", carolCE, danAC, carolFrozen)
	expect("/my/problems/"+pb+"/submissions", "sam", carolCE, danAC)
	detail("/my/problems/"+pb+"/submissions/"+carolCE, "carol", true)
	detail("/my/problems/"+pb+"/submissions/"+carolCE, "bob", true)
	detail("/my/problems/"+pb+"/submissions/"+carolCE, "sam", false)
	detail("/my/problems/"+pb+"/submissions/"+carolFrozen, "bob", false)
	missing("/my/problems/"+pb+"/submissions/"+carolFrozen, "sam")

	// The onsite filter keeps the contest submissions, practice included, of participants onsite now.
	onsiteRows := []string{carolWA, bobAC, carolCE, bobPractice}
	expect("/contests/"+tc+"/submissions?scope=onsite", "", onsiteRows...)
	expect(list+"?scope=onsite", "bob", append(onsiteRows, carolFrozen)...)
	expect("/contests/"+tc+"/problems/"+pa+"/submissions?scope=onsite", "", carolWA, bobAC, bobPractice)
	expect("/contests/"+tc+"/problems/"+pa+"/submissions", "", carolWA, bobAC, tessRun, bobPractice, samPractice)
	expect("/contests/"+ic+"/submissions?scope=onsite", "", hankWA)
	expect("/contests/"+nc+"/submissions?scope=onsite", "")
	// It ignores the viewer's own and team's lists.
	expect(problem(tc, pa, "?mine=1&scope=onsite"), "dan", danFrozen)
	expect(problem(tc, pa, "?mine=team&scope=onsite"), "dan", danFrozen)
	// A setter's switch also moves the earlier submissions.
	f.request("PUT", "/my/contests/"+tc+"/teams/"+beta.ID+"/onsite", "alice", map[string]any{"onsite": true}, 200)
	expect("/contests/"+tc+"/submissions?scope=onsite", "", append(onsiteRows, danAC)...)
	check(standings+"?scope=onsite", "", "1/1 "+strings.Replace(frozenBeta, "onsite=false", "onsite=true", 1), "2/2 "+frozenAlpha, "3/3 "+gammaRow)

	// Once revealed, frozen submissions are public, and compile errors still reach teammates only.
	f.request("POST", "/my/contests/"+tc+"/reveal", "alice", nil, 200)
	expect(list, "sam", append(public, carolFrozen, danFrozen)...)
	detail("/contests/"+tc+"/submissions/"+carolCE, "", false)
	detail("/submissions/"+carolCE, "", false)
	detail("/my/submissions/"+carolCE+"/public", "dan", false)
	detail("/my/submissions/"+carolCE+"/public", "erin", true)
	onsiteBeta := strings.Replace(liveBeta, "onsite=false", "onsite=true", 1)
	check(standings, "", "1/1 "+onsiteBeta, "2/2 "+liveAlpha, "3/3 "+gammaRow)
	check(standings+"?scope=onsite", "", "1/1 "+onsiteBeta, "2/2 "+liveAlpha, "3/3 "+gammaRow)

	// A participant who became a setter after the results were final leaves the standings, and the onsite filter too.
	f.exec(`INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,'hank')`, ic)
	expect("/contests/"+ic+"/submissions?scope=onsite", "")
	check("/contests/"+ic+"/standings", "", "1/1 ivy onsite=false 100 20m - - 100/0/0")
}
