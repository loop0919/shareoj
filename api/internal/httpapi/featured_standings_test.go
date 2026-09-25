package httpapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"judge/api/internal/problems"
)

func TestFeaturedStandingsPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	easy, hard := f.draft("alice", 4), f.draft("bob", 9)
	for _, p := range []problems.Problem{easy, hard} {
		f.request("PUT", "/my/problems/"+p.ID+"/publication", p.Author, map[string]any{"version": p.Version, "publish": true}, 200)
	}
	at := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	f.exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES($1,'easy','revival',$2,4,$1::timestamptz+interval '23 hours'),($1,'hard','new',$3,9,$1::timestamptz+interval '23 hours')`, at, easy.ID, hard.ID)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, easy.ID)
	for _, owner := range []string{"z_fast", "a_slow", "hard_only", "easy_only", "z_zero", "a_zero", "excluded"} {
		f.exec(`INSERT INTO user_profiles(owner_id,handle) VALUES($1,$1)`, owner)
	}
	submit := func(owner, problem, verdict string, elapsed time.Duration, job string, status string) string {
		t.Helper()
		id := newSubmissionID()
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,created_at) VALUES($1,$2,$3,2,'PRIVATE TITLE','cpp17','SECRET SOURCE',$4::jsonb,$5,jsonb_build_object('verdict',$6::text),$7)`, id, owner, problem, job, status, verdict, at.Add(elapsed))
		return id
	}
	const public = `{"privateDraft":false}`
	for _, v := range []struct {
		verdict string
		seconds int
	}{{"WA", 10}, {"CE", 20}, {"JE", 30}, {"AC", 60}, {"WA", 70}, {"AC", 300}} {
		submit("z_fast", easy.ID, v.verdict, time.Duration(v.seconds)*time.Second, public, "DONE")
	}
	submit("z_fast", hard.ID, "TLE", 20*time.Second, public, "DONE")
	submit("z_fast", hard.ID, "AC", 120*time.Second, public, "DONE")
	submit("a_slow", easy.ID, "AC", 10*time.Second, public, "DONE")
	submit("a_slow", hard.ID, "AC", 180*time.Second, public, "DONE")
	submit("hard_only", hard.ID, "AC", 5*time.Minute, public, "DONE")
	submit("hard_only", easy.ID, "RE", time.Minute, public, "DONE")
	submit("easy_only", easy.ID, "AC", 0, public, "DONE")                      // The start boundary is inclusive.
	pending := submit("z_zero", easy.ID, "AC", time.Second, public, "RUNNING") // A stale result must not count before DONE.
	submit("a_zero", hard.ID, "WA", 2*time.Second, public, "DONE")
	submit("a_zero", easy.ID, "AC", 23*time.Hour, public, "DONE") // Deadline is exclusive.
	submit("excluded", easy.ID, "AC", -time.Microsecond, public, "DONE")
	submit("excluded", hard.ID, "AC", 23*time.Hour, public, "DONE")
	for _, job := range []string{`{}`, `{"privateDraft":true}`, `{"privateDraft":false,"easyTest":true}`, `{"privateDraft":false,"generate":true}`, `{"privateDraft":false,"validate":true}`} {
		submit("excluded", easy.ID, "AC", time.Minute, job, "DONE")
	}
	for _, owner := range []string{"alice", "bob", "tester"} {
		submit(owner, hard.ID, "AC", time.Minute, public, "DONE")
	}
	contest := newSubmissionID()
	f.exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,released) VALUES($1,'alice','Contest','',$2,$2::timestamptz+interval '1 hour',true)`, contest, at)
	contestSubmission := submit("excluded", easy.ID, "AC", time.Minute, public, "DONE")
	f.exec(`UPDATE submissions SET contest_id=$1 WHERE id=$2`, contest, contestSubmission)
	path := "/featured/standings?at=" + url.QueryEscape(at.Format(time.RFC3339Nano))
	get := func(offset int) problems.FeaturedStandings {
		t.Helper()
		raw := f.request("GET", fmt.Sprintf("%s&offset=%d", path, offset), "", nil, 200)
		for _, secret := range []string{"SECRET", "PRIVATE TITLE", "source", "submissionId", "points", pending} {
			if strings.Contains(raw, secret) {
				t.Fatal("submission information leaked", raw)
			}
		}
		var page problems.FeaturedStandings
		if err := json.Unmarshal([]byte(raw), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	page := get(0)
	var handles []string
	var ranks []int64
	for _, row := range page.Items {
		handles = append(handles, row.Handle)
		ranks = append(ranks, row.Rank)
	}
	if !reflect.DeepEqual(handles, []string{"z_fast", "a_slow", "hard_only", "easy_only", "z_zero", "a_zero"}) || !reflect.DeepEqual(ranks, []int64{1, 1, 3, 4, 5, 5}) {
		t.Fatal(page.Items)
	}
	if !page.Closed || !page.ClosesAt.Equal(at.Add(23*time.Hour)) || page.HasMore {
		t.Fatal(page)
	}
	fast := page.Items[0]
	if !fast.Easy.Accepted || !fast.Hard.Accepted || *fast.TimeMS != 120000 || *fast.Easy.TimeMS != 60000 || fast.Easy.Wrong != 1 || fast.Hard.Wrong != 1 {
		t.Fatal(fast)
	}
	if *page.Items[3].TimeMS != 0 || page.Items[4].TimeMS != nil || page.Items[4].Easy.Accepted || page.Items[5].Hard.Wrong != 1 {
		t.Fatal(page.Items)
	}
	if tail := get(2); len(tail.Items) != 4 || tail.Items[0].Rank != 3 {
		t.Fatal(tail)
	}
	// Delayed judging changes the solved set after closing, never the submission time.
	f.exec(`UPDATE submissions SET status='DONE' WHERE id=$1`, pending)
	page = get(0)
	if page.Items[4].Handle != "z_zero" || page.Items[4].Rank != 4 || *page.Items[4].TimeMS != 1000 {
		t.Fatal(page.Items)
	}
	// Page boundaries preserve global tied ranks, and time wins over handle order.
	for i := 0; i < 51; i++ {
		owner := fmt.Sprintf("tail_%02d", i)
		f.exec(`INSERT INTO user_profiles(owner_id,handle) VALUES($1,$1)`, owner)
		submit(owner, easy.ID, "CE", time.Duration(i+10)*time.Minute, public, "DONE")
	}
	if first := get(0); !first.HasMore || len(first.Items) != 50 {
		t.Fatal(first)
	}
	if tail := get(50); tail.HasMore || len(tail.Items) != 7 || tail.Items[0].Rank != 6 {
		t.Fatal(tail)
	}
	for _, bad := range []string{"", "?at=bad", "?at=0000-01-01T00:00:00Z", "?at=" + url.QueryEscape(at.Format(time.RFC3339)) + "&offset=-1"} {
		f.request("GET", "/featured/standings"+bad, "", nil, 400)
	}
	f.request("GET", "/featured/standings?at="+url.QueryEscape(at.Add(time.Second).Format(time.RFC3339)), "", nil, 404)
}

func TestFeaturedStandingsMissingAndFuturePostgres(t *testing.T) {
	f := newFeaturedTest(t)
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	future := at.Add(48 * time.Hour)
	f.exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,reveal_at) VALUES($1,'easy','missing',$1::timestamptz+interval '23 hours'),($1,'hard','missing',$1::timestamptz+interval '23 hours'),($2,'easy','missing',$2::timestamptz+interval '23 hours')`, at, future)
	raw := f.request("GET", "/featured/standings?at="+url.QueryEscape(at.Format(time.RFC3339)), "", nil, 200)
	var page problems.FeaturedStandings
	if err := json.Unmarshal([]byte(raw), &page); err != nil || page.Closed || len(page.Round.Slots) != 2 || page.Items == nil || len(page.Items) != 0 {
		t.Fatal(raw, err)
	}
	f.request("GET", "/featured/standings?at="+url.QueryEscape(future.Format(time.RFC3339)), "", nil, 404)
}
