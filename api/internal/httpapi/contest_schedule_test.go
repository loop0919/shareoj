package httpapi

import (
	"slices"
	"strings"
	"testing"

	"judge/api/internal/contests"
)

func TestContestSchedulePostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol")
	const (
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		p1  = "11111111-1111-4111-8111-111111111111"
	)
	f.problem(p1)
	c := f.create(cid, p1, map[string]any{"teamMode": true})
	f.join(cid, "bob", create("Team"))
	const path = "/my/contest-schedule"
	list := func(actor string) []contests.ScheduledContest {
		t.Helper()
		var out itemsResponse[contests.ScheduledContest]
		w := f.do(f.newRequest("GET", path, actor, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"items":[`) {
			t.Fatalf("%s: %d %s", actor, w.Code, w.Body.String())
		}
		f.decode(w.Body.String(), &out)
		return out.Items
	}
	f.request("GET", path, "", nil, 401)
	items := list("bob")
	if len(items) != 1 || items[0].ID != cid || items[0].Title != c.Title || !items[0].StartsAt.Equal(c.StartsAt) || !items[0].EndsAt.Equal(c.EndsAt) {
		t.Fatalf("bob: %+v", items)
	}
	if got := list("carol"); len(got) != 0 {
		t.Fatalf("carol: %+v", got)
	}
	f.schedule(cid, "running")
	if got := list("bob"); len(got) != 1 {
		t.Fatalf("running: %+v", got)
	}
	f.schedule(cid, "ended")
	if got := list("bob"); len(got) != 0 {
		t.Fatalf("ended: %+v", got)
	}
}

func TestUpcomingContestsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const path = "/contests/upcoming"
	add := func(id, startsIn string) {
		t.Helper()
		problem := strings.ReplaceAll(id, "c", "1")
		f.problem(problem)
		f.create(id, problem, nil)
		f.exec(`UPDATE contests SET starts_at=now()+$2::interval,ends_at=now()+$2::interval+interval '1 hour' WHERE id=$1`, id, startsIn)
	}
	ids := func() []string {
		t.Helper()
		var out itemsResponse[contests.ScheduledContest]
		f.decode(f.request("GET", path, "", nil, 200), &out)
		list := make([]string, len(out.Items))
		for i, c := range out.Items {
			list[i] = c.ID
		}
		return list
	}
	const (
		first  = "cccccccc-cccc-4ccc-8ccc-cccccccccc01"
		second = "cccccccc-cccc-4ccc-8ccc-cccccccccc02"
		third  = "cccccccc-cccc-4ccc-8ccc-cccccccccc03"
		fourth = "cccccccc-cccc-4ccc-8ccc-cccccccccc04"
		fifth  = "cccccccc-cccc-4ccc-8ccc-cccccccccc05"
		sixth  = "cccccccc-cccc-4ccc-8ccc-cccccccccc06"
		beyond = "cccccccc-cccc-4ccc-8ccc-cccccccccc07"
	)
	if got := ids(); len(got) != 0 {
		t.Fatalf("no contests: %v", got)
	}
	add(second, "6 days 23 hours")
	add(first, "1 hour")
	add(beyond, "7 days 1 hour")
	if got := ids(); !slices.Equal(got, []string{first, second}) {
		t.Fatalf("within a week: %v", got)
	}
	f.schedule(first, "running")
	if got := ids(); !slices.Equal(got, []string{second}) {
		t.Fatalf("running: %v", got)
	}
	f.schedule(first, "ended")
	if got := ids(); !slices.Equal(got, []string{second}) {
		t.Fatalf("ended: %v", got)
	}
	add(third, "1 day")
	add(fourth, "2 days")
	add(fifth, "3 days")
	add(sixth, "4 days")
	f.exec(`UPDATE contests SET starts_at=now()+interval '5 days',ends_at=now()+interval '5 days 1 hour' WHERE id=$1`, first)
	if got := ids(); !slices.Equal(got, []string{third, fourth, fifth, sixth, first}) {
		t.Fatalf("limit: %v", got)
	}
}
