package httpapi

import (
	"testing"
	"time"

	"judge/api/internal/contests"
)

// A cell of the standings names the submission that set its score, so the score can link to it. The ID opens nothing
// that the submission routes keep closed, and the frozen table names no submission made after the freeze.
func TestContestStandingsNameTheScoringSubmissionPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol")
	const (
		pid = "11111111-1111-4111-8111-111111111111"
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	f.problem(pid)
	f.create(cid, pid, map[string]any{"freezeMinutes": 30})
	f.join(cid, "bob", nil)
	f.join(cid, "carol", nil)
	f.schedule(cid, "running")
	// The contest started an hour ago and ends in ten minutes, so the freeze began twenty minutes ago.
	f.exec(`UPDATE contests SET ends_at=now()+interval '10 minutes' WHERE id=$1`, cid)
	start := time.Now().Add(-time.Hour)
	const wrong = `{"verdict":"WA","passed":1,"total":3}`
	f.run(cid, "bob", pid, "DONE", wrong, start.Add(time.Minute))
	f.run(cid, "bob", pid, "DONE", acceptedResult, start.Add(2*time.Minute))
	f.run(cid, "bob", pid, "DONE", acceptedResult, start.Add(3*time.Minute))
	f.run(cid, "carol", pid, "DONE", wrong, start.Add(4*time.Minute))
	// carol's AC comes after the freeze.
	f.run(cid, "carol", pid, "DONE", acceptedResult, start.Add(50*time.Minute))
	id := func(owner string) string {
		t.Helper()
		var id string
		if err := f.store.Pool().QueryRow(f.ctx, `SELECT id FROM submissions WHERE owner_id=$1 AND result->>'verdict'='AC' ORDER BY created_at LIMIT 1`, owner).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	cells := func(path, owner string) map[string]string {
		t.Helper()
		var rows []contests.Standing
		f.decode(f.request("GET", path, owner, nil, 200), &rows)
		named := map[string]string{}
		for _, row := range rows {
			if cell := row.Problems[pid]; cell != nil {
				named[row.Handle] = cell.SubmissionID
			}
		}
		return named
	}
	if got := cells("/contests/"+cid+"/standings", ""); got["bob"] != id("bob") || got["carol"] != "" {
		t.Fatalf("frozen table: %v", got)
	}
	if got := cells("/my/contests/"+cid+"/standings?live=1", "alice"); got["bob"] != id("bob") || got["carol"] != id("carol") {
		t.Fatalf("staff's live table: %v", got)
	}
	// While the contest runs the ID opens the submission only for its owner and the staff.
	f.fail("GET", "/contests/"+cid+"/submissions/"+id("bob"), "", nil, 404, "contest_not_found")
	f.fail("GET", "/my/contests/"+cid+"/submissions/"+id("bob"), "carol", nil, 404, "contest_not_found")
	f.request("GET", "/my/submissions/"+id("bob"), "bob", nil, 200)
	f.request("GET", "/my/contests/"+cid+"/submissions/"+id("bob"), "alice", nil, 200)
	f.schedule(cid, "ended")
	f.request("GET", "/contests/"+cid+"/submissions/"+id("bob"), "", nil, 200)
}
