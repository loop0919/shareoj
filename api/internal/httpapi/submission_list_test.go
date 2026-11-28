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
		{"user=alice&sort=shortest", []string{s1, s6}},
		{"verdict=CE", []string{s5}},
	} {
		if got := listed(c.query); !slices.Equal(got, c.want) {
			t.Errorf("?%s: got %v want %v", c.query, got, c.want)
		}
	}
	for _, bad := range []string{"sort=longest", "verdict=OK", "verdict=ac", "language=cobol", "runtime=cpp23-gcc-isolate", "runtime=nope", "language=cpp&runtime=cpp17", "user=a%20b", "user=", "user=" + strings.Repeat("a", 100)} {
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
