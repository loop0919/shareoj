package httpapi

import (
	"testing"

	"judge/api/internal/problems"
)

// TestContestFavoritesPostgres pins ADR 0040: whoever may open a contest problem may favorite it, the contest problem
// shows the count to everyone, and the favorites stay with the problem published at the end.
func TestContestFavoritesPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol")
	const (
		pid     = "f1000000-0000-4000-8000-000000000001"
		outside = "f1000000-0000-4000-8000-000000000002"
		cid     = "f2000000-0000-4000-8000-000000000001"
	)
	f.problem(pid)
	f.problem(outside)
	f.create(cid, pid, nil)
	favorite := "/my/favorites/" + pid
	on := map[string]any{"favorited": true}
	state := func(method, viewer string, body any) problems.Favorite {
		t.Helper()
		var got problems.Favorite
		f.decode(f.request(method, favorite, viewer, body, 200), &got)
		return got
	}
	shown := func(path, viewer string) int64 {
		t.Helper()
		var got struct {
			FavoriteCount *int64 `json:"favoriteCount"`
		}
		f.decode(f.request("GET", path, viewer, nil, 200), &got)
		if got.FavoriteCount == nil {
			t.Fatalf("%s: no favoriteCount", path)
		}
		return *got.FavoriteCount
	}

	// Before the start only those who may open the problem reach it; the owner is one of them.
	f.fail("GET", favorite, "bob", nil, 404, "problem_not_found")
	f.fail("PUT", favorite, "bob", on, 404, "problem_not_found")
	if got := state("PUT", "alice", on); !got.Favorited || got.Count != 1 {
		t.Fatalf("owner before the start: %+v", got)
	}
	if n := shown("/my/contests/"+cid+"/problems/"+pid, "alice"); n != 1 {
		t.Fatalf("owner reads %d before the start", n)
	}

	f.schedule(cid, "running")
	// Signed-in viewers favorite without joining, and everyone reads the count.
	if got := state("GET", "bob", nil); got.Favorited || got.Count != 1 {
		t.Fatalf("bob before favoriting: %+v", got)
	}
	if got := state("PUT", "bob", on); !got.Favorited || got.Count != 2 {
		t.Fatalf("bob favorites: %+v", got)
	}
	f.join(cid, "carol", nil)
	if got := state("PUT", "carol", on); !got.Favorited || got.Count != 3 {
		t.Fatalf("carol favorites: %+v", got)
	}
	if got := state("PUT", "carol", map[string]any{"favorited": false}); got.Favorited || got.Count != 2 {
		t.Fatalf("carol takes it back: %+v", got)
	}
	if n := shown("/contests/"+cid+"/problems/"+pid, ""); n != 2 {
		t.Fatalf("anonymous viewers read %d during the contest", n)
	}
	if n := shown("/my/contests/"+cid+"/problems/"+pid, "carol"); n != 2 {
		t.Fatalf("participants read %d during the contest", n)
	}
	// The owner hears of bob and carol, as for a published problem, and not of the owner's own favorite.
	if n := f.count(`SELECT count(*) FROM notifications WHERE owner_id='alice' AND problem_id=$1 AND kind='favorite'`, pid); n != 2 {
		t.Fatalf("%d favorite notifications", n)
	}
	// A running contest opens only its own problems.
	f.fail("PUT", "/my/favorites/"+outside, "bob", on, 404, "problem_not_found")

	f.schedule(cid, "ended")
	if n := shown("/contests/"+cid+"/problems/"+pid, ""); n != 2 {
		t.Fatalf("the contest problem reads %d after the end", n)
	}
	// The problem published at the end keeps the favorites and takes new ones.
	if n := shown("/problems/"+pid, ""); n != 2 {
		t.Fatalf("the published problem reads %d", n)
	}
	if got := state("PUT", "carol", on); !got.Favorited || got.Count != 3 {
		t.Fatalf("carol favorites after the end: %+v", got)
	}
}
