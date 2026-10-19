package contests

import (
	"reflect"
	"testing"
	"time"
)

func TestRankPenalizesOnlyWrongSubmissionsBeforeFirstAC(t *testing.T) {
	start := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	attempt := func(user, problem, verdict string, minute, points int) Attempt {
		return Attempt{Handle: user, ProblemID: problem, Verdict: verdict, CreatedAt: start.Add(time.Duration(minute) * time.Minute), Points: points}
	}
	attempts := []Attempt{
		attempt("alice", "A", "CE", 1, 100), attempt("alice", "A", "WA", 2, 100), attempt("alice", "B", "WA", 3, 200),
		attempt("alice", "A", "RE", 4, 100), attempt("alice", "A", "AC", 10, 100), attempt("alice", "A", "WA", 11, 100), attempt("alice", "A", "AC", 12, 100),
		attempt("bob", "A", "AC", 20, 100), attempt("carol", "B", "AC", 25, 200), attempt("dave", "A", "JE", 26, 100),
	}
	rows := Rank(nil, attempts, start, 5)
	if len(rows) != 4 || rows[0].Handle != "carol" || rows[1].Handle != "alice" || rows[2].Handle != "bob" || rows[1].Rank != 2 || rows[2].Rank != 2 || rows[3].Rank != 4 {
		t.Fatalf("rank: %+v", rows)
	}
	alice := rows[1]
	if alice.Points != 100 || alice.TimeMS != 20*60000 || alice.Problems["A"].Wrong != 2 || alice.Problems["B"].Points != 0 {
		t.Fatalf("score: %+v", alice)
	}
	if zero := Rank(nil, attempts, start, 0); zero[1].TimeMS != 10*60000 {
		t.Fatalf("zero penalty: %+v", zero)
	}
	// An earlier queued submission resolving to AC must replace the later first AC.
	pending := []Attempt{attempt("alice", "A", "", 1, 100), attempt("alice", "A", "WA", 2, 100), attempt("alice", "A", "AC", 10, 100)}
	if r := Rank(nil, pending, start, 5)[0]; r.TimeMS != 15*60000 || r.Problems["A"].Pending != 1 {
		t.Fatal(r)
	}
	pending[0].Verdict = "AC"
	if r := Rank(nil, pending, start, 5)[0]; r.TimeMS != 60000 || r.Problems["A"].Wrong != 0 {
		t.Fatal(r)
	}
	// Penalties across solved problems accumulate, while elapsed time is counted once.
	multi := []Attempt{attempt("alice", "A", "WA", 1, 100), attempt("alice", "A", "AC", 2, 100), attempt("alice", "B", "WA", 3, 200), attempt("alice", "B", "AC", 30, 200)}
	if r := Rank(nil, multi, start, 7)[0]; r.Points != 300 || r.TimeMS != 44*60000 {
		t.Fatal(r)
	}
}

func TestRankIncludesParticipantsWithoutAttempts(t *testing.T) {
	rows := Rank([]string{"bob", "alice"}, nil, time.Now(), 5)
	if len(rows) != 2 || rows[0].Handle != "alice" || rows[1].Handle != "bob" {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row.Rank != 1 || row.Points != 0 || row.TimeMS != 0 || len(row.Problems) != 0 {
			t.Fatal(row)
		}
	}
}

func TestRankFrozenHidesSubmissionsFromTheFreeze(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return start.Add(time.Duration(minutes) * time.Minute) }
	attempts := []Attempt{
		{Handle: "a", ProblemID: "A", Verdict: "WA", Points: 100, CreatedAt: at(10)},
		{Handle: "a", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(50)},
		{Handle: "a", ProblemID: "A", Verdict: "WA", Points: 100, CreatedAt: at(70)},
		{Handle: "a", ProblemID: "B", Verdict: "CE", Points: 200, CreatedAt: at(61)},
		{Handle: "a", ProblemID: "B", Verdict: "", Points: 200, CreatedAt: at(62)},
		{Handle: "b", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(60)},
	}
	rows := RankFrozen([]string{"a", "b"}, attempts, start, 5, at(60))
	a, b := rows[0], rows[1]
	if a.Handle != "a" || a.Points != 100 || a.TimeMS != 55*60000 || a.Problems["A"].Frozen != 0 || a.Problems["A"].Wrong != 1 ||
		a.Problems["B"].Frozen != 2 || a.Problems["B"].Pending != 0 {
		t.Fatalf("frozen a: %+v %+v %+v", a, a.Problems["A"], a.Problems["B"])
	}
	// The freeze starts at the minute itself, so an acceptance then is hidden.
	if b.Handle != "b" || b.Points != 0 || b.Problems["A"].Frozen != 1 || b.Problems["A"].AcceptedAt != nil {
		t.Fatalf("frozen b: %+v %+v", b, b.Problems["A"])
	}
	live := RankFrozen([]string{"a", "b"}, attempts, start, 5, time.Time{})
	if !reflect.DeepEqual(live, Rank([]string{"a", "b"}, attempts, start, 5)) || live[1].Handle != "b" || live[1].Points != 100 || live[0].Problems["B"].Pending != 1 {
		t.Fatalf("live: %+v", live)
	}
}

func TestRankRowsCountsMembersTowardTheirTeam(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return start.Add(time.Duration(minutes) * time.Minute) }
	teams := []Standing{
		{Team: &TeamSummary{ID: "t1", Name: "Zeta", Members: []string{"a", "b"}, Onsite: true}, Onsite: true},
		{Team: &TeamSummary{ID: "t2", Name: "Alpha", Members: []string{"c"}}},
		{Team: &TeamSummary{ID: "t3", Name: "Beta", Members: []string{"d"}, Onsite: true}, Onsite: true},
	}
	attempts := []Attempt{
		{Handle: "a", Team: "t1", ProblemID: "A", Verdict: "WA", Points: 100, CreatedAt: at(1)},
		{Handle: "b", Team: "t1", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(4)},
		{Handle: "a", Team: "t1", ProblemID: "A", Verdict: "WA", Points: 100, CreatedAt: at(5)},
		{Handle: "c", Team: "t2", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(9)},
		{Handle: "a", Team: "t1", ProblemID: "B", Verdict: "AC", Points: 200, CreatedAt: at(70)},
	}
	rows := RankRows(teams, attempts, start, 5, at(60))
	// One member's wrong answer costs the team the penalty another member's acceptance then pays; ties go by name.
	if len(rows) != 3 || rows[0].Team.Name != "Alpha" || rows[1].Team.Name != "Zeta" || rows[2].Team.Name != "Beta" ||
		rows[0].Rank != 1 || rows[1].Rank != 1 || rows[2].Rank != 3 || rows[2].OverallRank != 3 {
		t.Fatalf("teams: %+v", rows)
	}
	zeta := rows[1]
	if zeta.Handle != "" || zeta.Points != 100 || zeta.TimeMS != 9*60000 || zeta.Problems["A"].Wrong != 1 || zeta.Problems["B"].Frozen != 1 ||
		!zeta.Onsite || len(zeta.Team.Members) != 2 {
		t.Fatalf("zeta: %+v %+v", zeta, zeta.Problems)
	}
	// The onsite rows are ranked again among themselves and keep the overall rank.
	onsite := OnsiteRows(rows)
	if len(onsite) != 2 || onsite[0].Team.Name != "Zeta" || onsite[0].Rank != 1 || onsite[0].OverallRank != 1 ||
		onsite[1].Team.Name != "Beta" || onsite[1].Rank != 2 || onsite[1].OverallRank != 3 || rows[2].Rank != 3 {
		t.Fatalf("onsite: %+v", onsite)
	}
	// Tied onsite rows share the onsite rank too, whatever open row ties with them.
	tied := RankRows([]Standing{
		{Team: &TeamSummary{ID: "t1", Name: "Alpha", Onsite: true}, Onsite: true},
		{Team: &TeamSummary{ID: "t2", Name: "Beta"}},
		{Team: &TeamSummary{ID: "t3", Name: "Gamma", Onsite: true}, Onsite: true},
	}, []Attempt{
		{Team: "t1", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(5)},
		{Team: "t2", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(5)},
		{Team: "t3", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(5)},
	}, start, 5, time.Time{})
	if len(tied) != 3 || tied[0].Rank != 1 || tied[1].Rank != 1 || tied[2].Rank != 1 {
		t.Fatalf("tied: %+v", tied)
	}
	if onsiteTied := OnsiteRows(tied); len(onsiteTied) != 2 || onsiteTied[0].Team.Name != "Alpha" || onsiteTied[1].Team.Name != "Gamma" ||
		onsiteTied[0].Rank != 1 || onsiteTied[1].Rank != 1 || onsiteTied[0].OverallRank != 1 || onsiteTied[1].OverallRank != 1 {
		t.Fatalf("tied onsite: %+v", onsiteTied)
	}
	if none := OnsiteRows(Rank([]string{"a"}, nil, start, 5)); none == nil || len(none) != 0 {
		t.Fatalf("no onsite rows: %#v", none)
	}
	// An attempt of a team not listed still gets a team row.
	if extra := RankRows(nil, []Attempt{{Handle: "e", Team: "t9", ProblemID: "A", Verdict: "AC", Points: 100, CreatedAt: at(3)}}, start, 5, time.Time{}); len(extra) != 1 ||
		extra[0].Team == nil || extra[0].Team.ID != "t9" || extra[0].Handle != "" || extra[0].Points != 100 {
		t.Fatalf("unlisted team: %+v", extra)
	}
}
