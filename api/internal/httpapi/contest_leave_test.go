package httpapi

import (
	"slices"
	"strings"
	"testing"

	"judge/api/internal/contests"
)

// A participant cancels their own registration before the start, and may then join the staff.
func TestContestLeavePostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan")
	const (
		p1  = "11111111-1111-4111-8111-111111111111"
		p2  = "22222222-2222-4222-8222-222222222222"
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		tc  = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	f.problem(p1)
	f.problem(p2)
	f.create(cid, p1, nil)
	path := "/my/contests/" + cid + "/participation"
	leave := func(id, owner string) contests.Contest {
		t.Helper()
		var c contests.Contest
		f.decode(f.request("DELETE", "/my/contests/"+id+"/participation", owner, nil, 200), &c)
		return c
	}
	registered := func(id, owner string) int {
		t.Helper()
		return f.count(`SELECT count(*) FROM contest_participants WHERE contest_id=$1 AND owner_id=$2`, id, owner)
	}

	f.fail("DELETE", "/my/contests/bad/participation", "bob", nil, 404, "contest_not_found")
	f.fail("DELETE", "/my/contests/"+tc+"/participation", "bob", nil, 404, "contest_not_found")
	f.request("DELETE", path, "", nil, 401)

	// Someone who never registered has nothing to cancel.
	if c := leave(cid, "bob"); c.Participating {
		t.Fatalf("never registered: %+v", c)
	}

	// The registration goes, with its row on the standings, and the same person may register again.
	f.join(cid, "bob", nil)
	f.join(cid, "carol", nil)
	if c := leave(cid, "bob"); c.Participating || c.ID != cid {
		t.Fatalf("after leaving: %+v", c)
	}
	if registered(cid, "bob") != 0 || registered(cid, "carol") != 1 {
		t.Fatal("registration kept, or another one removed")
	}
	if table := f.request("GET", "/contests/"+cid+"/standings", "", nil, 200); strings.Contains(table, `"bob"`) || !strings.Contains(table, `"carol"`) {
		t.Fatalf("standings: %s", table)
	}
	f.join(cid, "bob", nil)

	// A participant cannot join the staff (ADR 0022). Having cancelled, they can, and staff do not register.
	var link struct{ Token string }
	f.decode(f.request("POST", "/my/problems/"+p1+"/tester-invitation", "alice", nil, 200), &link)
	invite := "/my/tester-invitations/" + link.Token
	f.fail("POST", invite, "bob", nil, 409, "contest_participant_locked")
	leave(cid, "bob")
	f.request("POST", invite, "bob", nil, 200)
	f.fail("POST", path, "bob", nil, 409, "contest_participation_unavailable")
	if c := leave(cid, "bob"); !c.Staff || c.Participating {
		t.Fatalf("staff: %+v", c)
	}

	// Once the contest starts the registration stays.
	f.schedule(cid, "running")
	f.fail("DELETE", path, "carol", nil, 409, "contest_participation_locked")
	if registered(cid, "carol") != 1 {
		t.Fatal("registration removed after the start")
	}
	if c := leave(cid, "dan"); c.Participating {
		t.Fatalf("never registered, running: %+v", c)
	}
	f.schedule(cid, "ended")
	f.fail("DELETE", path, "carol", nil, 409, "contest_participation_locked")

	// A team member who cancels leaves the team as 「チームを抜ける」 does: the code is renewed, and the last one out
	// takes the team along.
	f.create(tc, p2, map[string]any{"teamMode": true})
	f.join(tc, "bob", create("Team Alpha"))
	alpha := f.team(tc, "bob")
	f.join(tc, "carol", joinCode(alpha.Code))
	f.join(tc, "dan", nil)
	if c := leave(tc, "carol"); c.Participating || c.Team != nil {
		t.Fatalf("member after leaving: %+v", c)
	}
	after := f.team(tc, "bob")
	if after.Code == alpha.Code || !slices.Equal(after.Members, []string{"bob"}) {
		t.Fatalf("team after a member left: %+v", after)
	}
	leave(tc, "bob")
	if n := f.count(`SELECT count(*) FROM contest_teams WHERE id=$1`, alpha.ID); n != 0 {
		t.Fatal("empty team kept")
	}
	leave(tc, "dan")
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE contest_id=$1`, tc); n != 0 {
		t.Fatalf("registrations left: %d", n)
	}
	f.join(tc, "carol", create("Team Alpha"))
	f.schedule(tc, "running")
	f.fail("DELETE", "/my/contests/"+tc+"/participation", "carol", nil, 409, "contest_participation_locked")
	f.team(tc, "carol")
}
