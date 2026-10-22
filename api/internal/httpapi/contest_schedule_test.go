package httpapi

import (
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
