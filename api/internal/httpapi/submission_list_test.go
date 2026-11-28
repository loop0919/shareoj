package httpapi

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"judge/api/internal/submissions"
)

// listedRun inserts a submission of the problem with the runtime, source, state and result given, made at the time
// given. result may be empty for a run without one; draft marks it as a private draft run.
func (f teamTest) listedRun(owner, problem, runtime string, size int, status, result string, draft bool, at time.Time) string {
	f.t.Helper()
	var value any
	if result != "" {
		value = result
	}
	id := newSubmissionID()
	f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,created_at,finished_at)
 VALUES($1,$2,$3,1,'List',$4,$5,jsonb_build_object('privateDraft',$6::boolean),$7,$8::jsonb,$9,$9)`, id, owner, problem, runtime, strings.Repeat("x", size), draft, status, value, at)
	return id
}

// casesResult is a judged result whose cases took the times given in milliseconds.
func casesResult(verdict string, times ...int) string {
	cases := make([]string, len(times))
	for i, ms := range times {
		cases[i] = fmt.Sprintf(`{"name":"%02d","verdict":%q,"cpuTimeMs":%d}`, i+1, verdict, ms)
	}
	return fmt.Sprintf(`{"verdict":%q,"passed":1,"total":1,"cases":[%s]}`, verdict, strings.Join(cases, ","))
}

func TestSubmissionListConditionsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol")
	const (
		pid   = "f3000000-0000-4000-8000-000000000001"
		other = "f3000000-0000-4000-8000-000000000002"
	)
	f.problem(pid)
	f.publish("alice", pid, true, 200)
	f.problem(other)
	f.publish("alice", other, true, 200)

	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Minute) }
	s1 := f.listedRun("alice", pid, "cpp17", 40, "DONE", casesResult("AC", 12, 30), false, at(1))
	s2 := f.listedRun("bob", pid, "cpp23-gcc-isolate", 25, "DONE", casesResult("AC", 5, 8), false, at(2))
	s3 := f.listedRun("bob", pid, "python314-isolate", 25, "DONE", casesResult("WA", 100), false, at(3))
	s4 := f.listedRun("carol", pid, "pypy311-local", 300, "DONE", casesResult("TLE", 2000), false, at(4))
	s5 := f.listedRun("carol", pid, "cpp23-clang-isolate", 10, "DONE", `{"verdict":"CE","passed":0,"total":1}`, false, at(5))
	s6 := f.listedRun("alice", pid, "python314-isolate", 60, "DONE", `{"verdict":"AC","passed":1,"total":1,"cases":[{"name":"01","verdict":"AC"}]}`, false, at(6))
	s7 := f.listedRun("bob", pid, "rust2024-isolate", 70, "QUEUED", "", false, at(7))
	s8 := f.listedRun("alice", pid, "cpp23-gcc-isolate", 5, "DONE", casesResult("AC", 1), true, at(8))

	page := func(path, owner string) submissions.ContestSubmissionList {
		var list submissions.ContestSubmissionList
		f.decode(f.request("GET", path, owner, nil, 200), &list)
		return list
	}
	ids := func(list submissions.ContestSubmissionList) []string {
		out := []string{}
		for _, item := range list.Items {
			out = append(out, item.ID)
		}
		return out
	}
	listed := func(query string) []string {
		f.t.Helper()
		return ids(page("/problems/"+pid+"/submissions?"+query, ""))
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"", []string{s7, s6, s5, s4, s3, s2, s1}},
		{"sort=newest", []string{s7, s6, s5, s4, s3, s2, s1}},
		{"sort=oldest", []string{s1, s2, s3, s4, s5, s6, s7}},
		{"sort=shortest", []string{s5, s3, s2, s1, s6, s7, s4}},
		{"sort=fastest", []string{s2, s1, s3, s4, s7, s6, s5}},
		{"verdict=AC", []string{s6, s2, s1}},
		{"verdict=AC&sort=fastest", []string{s2, s1, s6}},
		{"verdict=AC&sort=shortest", []string{s2, s1, s6}},
		{"language=cpp", []string{s5, s2, s1}},
		{"language=python", []string{s6, s4, s3}},
		{"runtime=python314", []string{s6, s3}},
		{"runtime=cpp17", []string{s1}},
		{"user=bob", []string{s7, s3, s2}},
		{"user=bob&verdict=AC&language=cpp", []string{s2}},
		{"user=nobody", []string{}},
		{"user=", []string{s7, s6, s5, s4, s3, s2, s1}}, // an empty value reads as absent, as for sort, verdict, language and runtime
		{"user=alice&sort=shortest", []string{s1, s6}},
		{"verdict=CE", []string{s5}},
	} {
		if got := listed(c.query); !slices.Equal(got, c.want) {
			t.Errorf("?%s: got %v want %v", c.query, got, c.want)
		}
	}
	for _, bad := range []string{"sort=longest", "verdict=OK", "verdict=ac", "language=cobol", "runtime=cpp23-gcc-isolate", "runtime=nope", "language=cpp&runtime=cpp17", "user=a%20b", "user=" + strings.Repeat("a", 100)} {
		f.fail("GET", "/problems/"+pid+"/submissions?"+bad, "", nil, 400, "invalid_request")
		f.fail("GET", "/my/problems/"+pid+"/submissions?"+bad, "alice", nil, 400, "invalid_request")
	}

	// The author sees the draft run, with the same conditions; mine keeps working beside them.
	mine := "/my/problems/" + pid + "/submissions?"
	if got := ids(page(mine+"user=alice&sort=shortest", "alice")); !slices.Equal(got, []string{s8, s1, s6}) {
		t.Errorf("author: got %v", got)
	}
	if got := ids(page(mine+"mine=1&verdict=AC", "bob")); !slices.Equal(got, []string{s2}) {
		t.Errorf("mine: got %v", got)
	}

	// The value sorted by is the value the row shows.
	fastest := page("/problems/"+pid+"/submissions?sort=fastest", "").Items
	var last float64
	timed := true
	for _, item := range fastest {
		switch {
		case item.Result == nil || item.Result.CPUTimeMS == nil:
			timed = false
		case !timed:
			t.Errorf("fastest: %s has a time after a row without one", item.ID)
		case *item.Result.CPUTimeMS < last:
			t.Errorf("fastest: %s took %v after %v", item.ID, *item.Result.CPUTimeMS, last)
		default:
			last = *item.Result.CPUTimeMS
		}
	}
	shortest := page("/problems/"+pid+"/submissions?sort=shortest", "").Items
	for i := 1; i < len(shortest); i++ {
		if shortest[i].SourceBytes < shortest[i-1].SourceBytes {
			t.Errorf("shortest: %s has %d bytes after %d", shortest[i].ID, shortest[i].SourceBytes, shortest[i-1].SourceBytes)
		}
	}

	// Rows that tie on the value, even on the time, are listed once across pages.
	tied := at(10)
	want := map[string]bool{}
	for i := 0; i < 60; i++ {
		want[f.listedRun("carol", other, "cpp23-gcc", 10, "DONE", `{"verdict":"CE","passed":0,"total":1}`, false, tied)] = true
	}
	for _, sort := range []string{"newest", "oldest", "shortest", "fastest"} {
		seen := map[string]bool{}
		for _, offset := range []int{0, 50} {
			list := page(fmt.Sprintf("/problems/%s/submissions?sort=%s&offset=%d", other, sort, offset), "")
			if list.HasMore != (offset == 0) {
				t.Errorf("%s offset %d: hasMore %v", sort, offset, list.HasMore)
			}
			for _, id := range ids(list) {
				if seen[id] || !want[id] {
					t.Errorf("%s: %s listed twice or unknown", sort, id)
				}
				seen[id] = true
			}
		}
		if len(seen) != len(want) {
			t.Errorf("%s: listed %d of %d rows", sort, len(seen), len(want))
		}
	}

	// The SQL function reads what the row shows, and survives odd results; none stands for NULL.
	const none = -1.0
	for result, want := range map[string]float64{
		`{"cases":[{"cpuTimeMs":3},{"cpuTimeMs":7.5},{}]}`:      7.5,
		`{"cases":[{"cpuTimeMs":"fast"},{"cpuTimeMs":4}]}`:      4,
		`{"cases":[{"cpuTimeMs":"fast"},{"cpuTimeMs":null},1]}`: none,
		`{"cases":[]}`:              none,
		`{"cases":{"cpuTimeMs":5}}`: none,
		`{"verdict":"AC"}`:          none,
		`[1,2]`:                     none,
		`null`:                      none,
	} {
		var got *float64
		if err := f.store.Pool().QueryRow(f.ctx, `SELECT submission_cpu_time_ms($1::jsonb)`, result).Scan(&got); err != nil {
			t.Fatal(result, err)
		}
		if (want == none) != (got == nil) || (got != nil && *got != want) {
			t.Errorf("submission_cpu_time_ms(%s) = %v, want %v", result, got, want)
		}
	}
	var got *float64
	if err := f.store.Pool().QueryRow(f.ctx, `SELECT submission_cpu_time_ms(NULL)`).Scan(&got); err != nil || got != nil {
		t.Errorf("submission_cpu_time_ms(NULL) = %v, %v", got, err)
	}
}

func TestContestSubmissionListConditionsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan")
	const (
		cid = "f4000000-0000-4000-8000-000000000001" // ended, with an onsite division
		fz  = "f4000000-0000-4000-8000-000000000002" // ended, with a freeze
		tm  = "f4000000-0000-4000-8000-000000000003" // running team contest
		pa  = "f4000000-0000-4000-8000-0000000000a1"
		pb  = "f4000000-0000-4000-8000-0000000000a2"
		pc  = "f4000000-0000-4000-8000-0000000000a3" // a problem sits in one contest at a time
		pd  = "f4000000-0000-4000-8000-0000000000a4"
		pe  = "f4000000-0000-4000-8000-0000000000a5"
	)
	for _, p := range []string{pa, pb, pc, pd, pe} {
		f.problem(p)
	}
	two := []map[string]any{{"id": pa, "points": 100}, {"id": pb, "points": 100}}
	f.create(cid, pa, map[string]any{"problems": two, "onsiteDivision": true, "onsitePassphrase": "open_sesame"})
	f.create(fz, pc, map[string]any{"problems": []map[string]any{{"id": pc, "points": 100}, {"id": pd, "points": 100}}, "freezeMinutes": 30})
	f.create(tm, pe, map[string]any{"teamMode": true})
	// bob competes onsite and carol does not; the teams of the team contest are formed before it starts.
	f.join(cid, "bob", map[string]any{"onsitePassphrase": "open_sesame"})
	f.join(cid, "carol", nil)
	f.join(tm, "bob", create("Alpha"))
	f.join(tm, "carol", joinCode(f.team(tm, "bob").Code))
	f.join(tm, "dan", create("Beta"))
	f.schedule(cid, "ended")
	f.schedule(fz, "ended")
	f.schedule(tm, "running")

	now := time.Now().Truncate(time.Second)
	run := func(contest, owner, problem, runtime string, size int, status, result string, at time.Time) string {
		f.t.Helper()
		id := f.listedRun(owner, problem, runtime, size, status, result, false, at)
		f.exec(`UPDATE submissions SET contest_id=$1 WHERE id=$2`, contest, id)
		return id
	}
	// The rows of the problem list's table, split over two problems, made while the contest ran.
	at := func(i int) time.Time { return now.Add(-115*time.Minute + time.Duration(i)*time.Minute) }
	c1 := run(cid, "alice", pa, "cpp17", 40, "DONE", casesResult("AC", 12, 30), at(1))
	c2 := run(cid, "bob", pa, "cpp23-gcc-isolate", 25, "DONE", casesResult("AC", 5, 8), at(2))
	c3 := run(cid, "bob", pb, "python314-isolate", 25, "DONE", casesResult("WA", 100), at(3))
	c4 := run(cid, "carol", pb, "pypy311-local", 300, "DONE", casesResult("TLE", 2000), at(4))
	c5 := run(cid, "carol", pa, "cpp23-clang-isolate", 10, "DONE", `{"verdict":"CE","passed":0,"total":1}`, at(5))
	c6 := run(cid, "alice", pb, "python314-isolate", 60, "DONE", `{"verdict":"AC","passed":1,"total":1,"cases":[{"name":"01","verdict":"AC"}]}`, at(6))
	c7 := run(cid, "bob", pa, "rust2024-isolate", 70, "QUEUED", "", at(7))

	// check lists path as viewer and expects the rows in order and, for the lists that count, the total.
	check := func(path, viewer string, wantTotal int, want ...string) {
		f.t.Helper()
		var list submissions.CountedSubmissionList
		f.decode(f.request("GET", path, viewer, nil, 200), &list)
		got := []string{}
		for _, item := range list.Items {
			got = append(got, item.ID)
		}
		counted := !strings.Contains(path, "/problems/")
		if !slices.Equal(got, want) || (counted && list.Total != wantTotal) {
			f.t.Errorf("%s (%q): got %v total %d, want %v total %d", path, viewer, got, list.Total, want, wantTotal)
		}
	}
	all := "/contests/" + cid + "/submissions"
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"", []string{c7, c6, c5, c4, c3, c2, c1}},
		{"sort=newest", []string{c7, c6, c5, c4, c3, c2, c1}},
		{"sort=oldest", []string{c1, c2, c3, c4, c5, c6, c7}},
		{"sort=shortest", []string{c5, c3, c2, c1, c6, c7, c4}},
		{"sort=fastest", []string{c2, c1, c3, c4, c7, c6, c5}},
		{"verdict=AC", []string{c6, c2, c1}},
		{"verdict=AC&sort=fastest", []string{c2, c1, c6}},
		{"verdict=AC&sort=shortest", []string{c2, c1, c6}},
		{"verdict=CE", []string{c5}},
		{"language=python&problem=" + pb, []string{c6, c4, c3}},
		{"language=cpp&problem=" + pb, []string{}},
		{"language=cpp", []string{c5, c2, c1}},
		{"runtime=python314", []string{c6, c3}},
		{"user=bob", []string{c7, c3, c2}},
		{"user=bob&verdict=AC&language=cpp", []string{c2}},
		{"user=nobody", []string{}},
		{"user=", []string{c7, c6, c5, c4, c3, c2, c1}}, // an empty value reads as absent
		{"scope=onsite", []string{c7, c3, c2}},
		{"scope=onsite&verdict=AC", []string{c2}},
		{"scope=onsite&sort=shortest", []string{c3, c2, c7}},
		{"scope=onsite&user=carol", []string{}},
	} {
		check(all+"?"+c.query, "", len(c.want), c.want...)
	}
	// The page size does not change the total: the second of bob's three rows, with hasMore.
	var page submissions.CountedSubmissionList
	f.decode(f.request("GET", all+"?user=bob&limit=1&offset=1", "", nil, 200), &page)
	if len(page.Items) != 1 || page.Items[0].ID != c3 || page.Total != 3 || !page.HasMore {
		t.Errorf("user=bob&limit=1&offset=1: %+v", page)
	}
	f.decode(f.request("GET", all+"?user=bob&limit=1&offset=2", "", nil, 200), &page)
	if len(page.Items) != 1 || page.Items[0].ID != c2 || page.Total != 3 || page.HasMore {
		t.Errorf("user=bob&limit=1&offset=2: %+v", page)
	}
	// The signed-in routes take the same conditions; mine keeps working beside them.
	check("/my"+all+"?sort=shortest&verdict=AC", "carol", 3, c2, c1, c6)
	check("/my"+all+"?mine=1&verdict=WA", "bob", 1, c3)
	check("/my"+all+"?mine=1&sort=fastest", "bob", 3, c2, c3, c7)
	check("/my"+all+"?mine=1&user=carol", "bob", 0)
	// A contest problem lists that problem's rows with the same conditions.
	byProblem := func(problem string) string { return "/contests/" + cid + "/problems/" + problem + "/submissions" }
	check(byProblem(pa)+"?verdict=AC&sort=shortest", "", 0, c2, c1)
	check("/my"+byProblem(pa)+"?sort=fastest&language=cpp", "alice", 0, c2, c1, c5)
	check("/my"+byProblem(pa)+"?mine=1&verdict=AC", "bob", 0, c2)
	check(byProblem(pa)+"?scope=onsite&user=bob&sort=oldest", "", 0, c2, c7)
	check(byProblem(pb)+"?user=carol&sort=oldest", "", 0, c4)

	for _, bad := range []string{"sort=longest", "verdict=OK", "verdict=ac", "language=cobol", "runtime=cpp23-gcc-isolate", "runtime=nope", "language=cpp&runtime=cpp17", "user=a%20b", "user=" + strings.Repeat("a", 100)} {
		for _, path := range []string{all, byProblem(pa)} {
			f.fail("GET", path+"?"+bad, "", nil, 400, "invalid_request")
			f.fail("GET", "/my"+path+"?"+bad, "alice", nil, 400, "invalid_request")
		}
	}

	// A frozen run stays hidden when its submitter or its verdict is named, and is not counted.
	f1 := run(fz, "carol", pc, "cpp17", 20, "DONE", casesResult("WA", 7), now.Add(-100*time.Minute))
	f2 := run(fz, "carol", pc, "cpp17", 30, "DONE", casesResult("AC", 7), now.Add(-70*time.Minute))
	f3 := run(fz, "dan", pd, "cpp17", 10, "DONE", casesResult("AC", 9), now.Add(-65*time.Minute))
	frozen := "/contests/" + fz + "/submissions"
	check(frozen, "", 1, f1)
	check(frozen+"?user=carol", "", 1, f1)
	check(frozen+"?user=carol&verdict=AC", "", 0)
	check(frozen+"?user=dan", "", 0)
	check(frozen+"?verdict=AC&sort=fastest", "", 0)
	check(frozen+"?sort=shortest&language=cpp", "", 1, f1)
	check("/contests/"+fz+"/problems/"+pc+"/submissions?user=carol&verdict=AC", "", 0)
	check("/my"+frozen+"?user=carol&verdict=AC", "bob", 0)
	// The submitter sees their own frozen run with the same conditions, and staff see every run.
	check("/my"+frozen+"?user=carol&verdict=AC", "carol", 1, f2)
	check("/my"+frozen+"?user=carol&sort=shortest", "carol", 2, f1, f2)
	check("/my"+frozen+"?user=dan&verdict=AC", "carol", 0)
	check("/my"+frozen+"?verdict=AC&sort=shortest", "alice", 2, f3, f2)
	check("/my"+frozen+"?user=dan", "alice", 1, f3)

	// In a team contest, the team's list keeps working with the conditions.
	b := run(tm, "bob", pe, "cpp17", 30, "DONE", casesResult("AC", 4), now.Add(-30*time.Minute))
	cw := run(tm, "carol", pe, "cpp17", 20, "DONE", casesResult("WA", 4), now.Add(-20*time.Minute))
	d := run(tm, "dan", pe, "cpp17", 10, "DONE", casesResult("AC", 4), now.Add(-10*time.Minute))
	team := "/my/contests/" + tm + "/submissions?mine=team"
	check(team+"&sort=shortest", "bob", 2, cw, b)
	check(team+"&sort=shortest&verdict=WA", "bob", 1, cw)
	check(team+"&sort=oldest&user=carol", "bob", 1, cw)
	check(team+"&user=dan", "bob", 0)
	check(team+"&sort=shortest", "dan", 1, d)
	check("/my/contests/"+tm+"/problems/"+pe+"/submissions?mine=team&sort=shortest&verdict=AC", "bob", 0, b)
}
