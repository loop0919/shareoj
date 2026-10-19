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

// ADR 0024 (changed 2026-10-19): a team contest also takes participants who enter alone. Before the start they move
// into a team and members who leave are unregistered; after the start nobody who entered alone joins a team. The
// standings mix team rows and individual rows under the same rules, and an individual's division and submissions stay
// their own.
func TestContestTeamIndividualsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan", "erin", "frank", "gina", "hank", "ivy", "judy", "kate", "leo", "mia")
	const (
		tc     = "c2000000-0000-4000-8000-000000000001"
		pa     = "a2000000-0000-4000-8000-000000000001"
		pb     = "a2000000-0000-4000-8000-000000000002"
		phrase = "Venue_2026"
	)
	f.problem(pa)
	f.problem(pb)
	start := time.Now().Add(time.Hour)
	f.request("PUT", "/my/contests/"+tc, "alice", map[string]any{"title": "Mixed", "description": "", "startsAt": start, "endsAt": start.Add(2 * time.Hour),
		"penaltyMinutes": 5, "version": 0, "problems": []map[string]any{{"id": pa, "points": 100}, {"id": pb, "points": 200}}, "teamMode": true, "maxTeamSize": 3,
		"freezeMinutes": 30, "onsiteDivision": true, "onsitePassphrase": phrase}, 200)
	path := "/my/contests/" + tc + "/participation"
	members := "/my/contests/" + tc + "/team/members/"

	// An empty body enters alone; sending it again changes nothing.
	for range 2 {
		if bob := f.join(tc, "bob", nil); !bob.Participating || bob.Team != nil || bob.Onsite {
			t.Fatalf("alone: %+v", bob)
		}
	}
	f.fail("GET", "/my/contests/"+tc+"/team", "bob", nil, 404, "team_not_found")
	// Before the start someone who entered alone joins a team; the team stays open, and its members are listed in the
	// order they joined.
	alpha := f.join(tc, "dan", create("Alpha")).Team
	if joined := f.join(tc, "bob", joinCode(f.team(tc, "dan").Code)); joined.Team == nil || joined.Team.ID != alpha.ID ||
		!slices.Equal(joined.Team.Members, []string{"dan", "bob"}) || joined.Onsite {
		t.Fatalf("moved into a team: %+v", joined.Team)
	}
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE contest_id=$1 AND owner_id='bob'`, tc); n != 1 {
		t.Fatalf("bob's rows: %d", n)
	}
	// The passphrase alone makes a newcomer an onsite individual. A team they form carries their division: nobody turns
	// from onsite to open.
	if carol := f.join(tc, "carol", map[string]any{"onsitePassphrase": phrase}); carol.Team != nil || !carol.Onsite {
		t.Fatalf("onsite individual: %+v", carol)
	}
	if beta := f.join(tc, "carol", create("Beta")); beta.Team == nil || !beta.Team.Onsite || !beta.Onsite {
		t.Fatalf("formed onsite: %+v", beta.Team)
	}
	if judy := f.join(tc, "judy", map[string]any{"onsitePassphrase": " Ｖｅｎｕｅ＿２０２６ "}); !judy.Onsite {
		t.Fatalf("onsite individual: %+v", judy)
	}
	gamma := f.join(tc, "ivy", create("Gamma")).Team
	if gamma.Onsite {
		t.Fatalf("open team: %+v", gamma)
	}
	if joined := f.join(tc, "judy", joinCode(f.team(tc, "ivy").Code)); !joined.Onsite || !f.contest(tc, "ivy").Onsite ||
		!slices.Equal(joined.Team.Members, []string{"ivy", "judy"}) {
		t.Fatalf("joined onsite: %+v", joined.Team)
	}
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE contest_id=$1 AND team_id IS NOT NULL AND (onsite_at IS NOT NULL OR onsite_via IS NOT NULL)`, tc); n != 0 {
		t.Fatalf("members keep %d divisions of their own", n)
	}
	// A member who leaves before the start is unregistered, and may then join another team or enter alone.
	f.join(tc, "frank", joinCode(f.team(tc, "carol").Code))
	f.request("DELETE", members+"frank", "frank", nil, 204)
	if left := f.contest(tc, "frank"); left.Participating || left.Team != nil || left.Onsite {
		t.Fatalf("left: %+v", left)
	}
	f.join(tc, "frank", joinCode(f.team(tc, "dan").Code))
	f.join(tc, "gina", create("Solo"))
	f.request("DELETE", members+"gina", "gina", nil, 204)
	if gina := f.join(tc, "gina", nil); !gina.Participating || gina.Team != nil {
		t.Fatalf("alone after leaving: %+v", gina)
	}
	// A registration without a team, as the API from before teams made it, is someone who entered alone.
	f.join(tc, "erin", nil)
	f.exec(`INSERT INTO contest_participants(contest_id,owner_id) VALUES($1,'hank')`, tc)
	if hank := f.join(tc, "hank", nil); !hank.Participating || hank.Team != nil {
		t.Fatalf("legacy registration: %+v", hank)
	}

	// Setters list teams and individuals together, oldest first, and make an individual onsite by handle.
	var e contests.Entrant
	f.decode(f.request("PUT", "/my/contests/"+tc+"/participants/erin/onsite", "alice", map[string]any{"onsite": true}, 200), &e)
	if e.Handle != "erin" || e.Team != nil || !e.Onsite || e.OnsiteVia != "setter" || !f.contest(tc, "erin").Onsite {
		t.Fatalf("individual switched onsite: %+v", e)
	}
	f.fail("PUT", "/my/contests/"+tc+"/participants/erin/onsite", "alice", map[string]any{"onsite": false}, 400, "onsite_irreversible")
	f.fail("PUT", "/my/contests/"+tc+"/participants/bob/onsite", "alice", map[string]any{"onsite": true}, 404, "participant_not_found")
	entrant := func(e contests.Entrant) string {
		name := e.Handle
		if e.Team != nil {
			if e.Handle != "" || e.Team.Onsite != e.Onsite {
				t.Fatalf("team entrant: %+v", e)
			}
			name = e.Team.Name + "(" + strings.Join(e.Team.Members, ",") + ")"
		}
		return fmt.Sprintf("%s %t %s %t", name, e.Onsite, e.OnsiteVia, e.OnsiteAt != nil)
	}
	got := []string{}
	for _, e := range f.entrants(tc) {
		got = append(got, entrant(e))
	}
	if want := []string{"Alpha(dan,bob,frank) false  false", "Beta(carol) true passphrase true", "Gamma(ivy,judy) true passphrase true", "gina false  false",
		"erin true setter true", "hank false  false"}; !slices.Equal(got, want) {
		t.Fatalf("entrants:\n got %q\nwant %q", got, want)
	}

	// The contest runs for two hours, ends in 20 minutes and froze 10 minutes ago.
	end := time.Now().Add(20 * time.Minute).Truncate(time.Millisecond)
	f.exec(`UPDATE contests SET starts_at=$1,ends_at=$2 WHERE id=$3`, end.Add(-2*time.Hour), end, tc)
	at := func(minutes int) time.Time { return end.Add(time.Duration(minutes-120) * time.Minute) }

	// After the start someone who entered alone cannot form or join a team, with or without a secret, and nothing is
	// counted as a guess.
	failures := f.count(`SELECT count(*) FROM contest_attempt_failures`)
	for _, body := range []map[string]any{create("Late Solo"), joinCode(f.team(tc, "ivy").Code), joinCode("WRONGCODE2"), withOnsite(create("Late Solo"), "wrong")} {
		f.fail("POST", path, "gina", body, 409, "contest_individual_locked")
	}
	if gina := f.contest(tc, "gina"); gina.Team != nil || gina.Onsite || f.count(`SELECT count(*) FROM contest_attempt_failures`) != failures ||
		f.count(`SELECT count(*) FROM contest_teams WHERE contest_id=$1`, tc) != 3 {
		t.Fatalf("refused requests changed something: %+v", gina)
	}
	// They still become onsite with the passphrase until the end.
	if hank := f.join(tc, "hank", map[string]any{"onsitePassphrase": phrase}); !hank.Onsite || hank.Team != nil {
		t.Fatalf("upgraded individual: %+v", hank)
	}

	insert := func(id, owner, problem, verdict string, created time.Time) {
		t.Helper()
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at,finished_at)
 VALUES($1,$2,$3,1,'Mixed','cpp17','code of '||$2,'{}'::jsonb,'DONE',jsonb_build_object('verdict',$4::text,'passed',0,'total',1),$5,$6,$6)`, id, owner, problem, verdict, tc, created)
	}
	const (
		bobAC     = "d2000000-0000-4000-8000-000000000001"
		danWA     = "d2000000-0000-4000-8000-000000000002"
		carolWA   = "d2000000-0000-4000-8000-000000000003"
		carolAC   = "d2000000-0000-4000-8000-000000000004"
		erinAC    = "d2000000-0000-4000-8000-000000000005"
		erinFroze = "d2000000-0000-4000-8000-000000000006"
		ginaAC    = "d2000000-0000-4000-8000-000000000007"
		judyAC    = "d2000000-0000-4000-8000-000000000008"
		hankWA    = "d2000000-0000-4000-8000-000000000009"
	)
	insert(bobAC, "bob", pa, "AC", at(10))
	insert(danWA, "dan", pb, "WA", at(20))
	insert(carolWA, "carol", pa, "WA", at(5))
	insert(carolAC, "carol", pa, "AC", at(15))
	insert(erinAC, "erin", pa, "AC", at(8))
	insert(erinFroze, "erin", pb, "AC", at(95))
	insert(ginaAC, "gina", pa, "AC", at(10))
	insert(judyAC, "judy", pb, "AC", at(30))
	insert(hankWA, "hank", pa, "WA", at(12))

	// Teams and individuals share one table, one scoring, one freeze and one onsite filter; a team and an individual
	// with the same score share the rank.
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
			for _, p := range []string{pa, pb} {
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
	const (
		gammaRow    = "Gamma(ivy,judy) onsite=true 200 30m - 200/0/0"
		frozenErin  = "erin onsite=true 100 8m 100/0/0 0/0/1"
		liveErin    = "erin onsite=true 300 95m 100/0/0 200/0/0"
		alphaRow    = "Alpha(dan,bob,frank) onsite=false 100 10m 100/0/0 0/1/0"
		ginaRow     = "gina onsite=false 100 10m 100/0/0 -"
		betaRow     = "Beta(carol) onsite=true 100 20m 100/1/0 -"
		hankRow     = "hank onsite=true 0 0m 0/1/0 -"
		standingsOf = "/contests/" + tc + "/standings"
	)
	check(standingsOf, "", "1/1 "+gammaRow, "2/2 "+frozenErin, "3/3 "+alphaRow, "3/3 "+ginaRow, "5/5 "+betaRow, "6/6 "+hankRow)
	check(standingsOf+"?scope=onsite", "", "1/1 "+gammaRow, "2/2 "+frozenErin, "3/5 "+betaRow, "4/6 "+hankRow)
	check("/my/contests/"+tc+"/standings?live=1", "alice", "1/1 "+liveErin, "2/2 "+gammaRow, "3/3 "+alphaRow, "3/3 "+ginaRow, "5/5 "+betaRow, "6/6 "+hankRow)
	check("/my/contests/"+tc+"/standings?live=1&scope=onsite", "alice", "1/1 "+liveErin, "2/2 "+gammaRow, "3/5 "+betaRow, "4/6 "+hankRow)

	// An individual sees only their own submissions and solved marks; members see their team's, never an individual's.
	sorted := func(list ...string) []string {
		list = slices.Clone(list)
		slices.Sort(list)
		return list
	}
	expect := func(path, viewer string, want ...string) {
		t.Helper()
		var list submissions.ContestSubmissionList
		f.decode(f.request("GET", path, viewer, nil, 200), &list)
		got := []string{}
		for _, item := range list.Items {
			got = append(got, item.ID)
		}
		if !slices.Equal(sorted(got...), sorted(want...)) {
			t.Fatalf("%s (%s):\n got %v\nwant %v", path, viewer, sorted(got...), sorted(want...))
		}
	}
	problem := func(p string) string { return "/my/contests/" + tc + "/problems/" + p + "/submissions?mine=team" }
	expect(problem(pa), "erin", erinAC)
	expect(problem(pb), "erin", erinFroze)
	expect(problem(pa), "frank", bobAC)
	expect(problem(pa), "gina", ginaAC)
	list := "/my/contests/" + tc + "/submissions"
	for _, individual := range []string{"erin", "gina", "hank"} {
		if c := f.contest(tc, individual); c.CanViewSubmissions || c.Team != nil {
			t.Fatalf("%s may view submissions: %+v", individual, c)
		}
		f.fail("GET", list, individual, nil, 404, "contest_not_found")
	}
	expect(list, "dan", bobAC, danWA)
	f.fail("GET", list+"/"+erinAC, "bob", nil, 404, "contest_not_found")
	f.fail("GET", list+"/"+bobAC, "erin", nil, 404, "contest_not_found")
	f.fail("GET", "/my/submissions/"+erinAC+"/public", "gina", nil, 404, "submission_not_found")
	solved := func(viewer string) []bool {
		t.Helper()
		var marks []bool
		for _, p := range f.contest(tc, viewer).Problems {
			marks = append(marks, p.Solved)
		}
		return marks
	}
	for viewer, want := range map[string][]bool{"erin": {true, true}, "gina": {true, false}, "hank": {false, false}, "frank": {true, false}, "ivy": {false, true}} {
		if got := solved(viewer); !slices.Equal(got, want) {
			t.Fatalf("%s solved: %v", viewer, got)
		}
	}
	// The onsite filter keeps onsite individuals' submissions next to onsite teams'.
	expect(list+"?scope=onsite", "alice", carolWA, carolAC, erinAC, erinFroze, judyAC, hankWA)

	// Newcomers still enter alone or with a team until the end; members still cannot leave.
	if kate := f.join(tc, "kate", nil); !kate.Participating || kate.Team != nil {
		t.Fatalf("late individual: %+v", kate)
	}
	if leo := f.join(tc, "leo", create("Late")); leo.Team == nil {
		t.Fatalf("late team: %+v", leo)
	}
	f.fail("DELETE", members+"bob", "bob", nil, 409, "team_locked")
	// After the end nobody enters, and someone who entered alone still gets the reason that applies to them.
	f.exec(`UPDATE contests SET starts_at=starts_at-interval '1 hour',ends_at=now()-interval '1 minute' WHERE id=$1`, tc)
	f.fail("POST", path, "gina", create("After"), 409, "contest_individual_locked")
	f.fail("POST", path, "mia", nil, 409, "contest_participation_unavailable")
	f.fail("POST", path, "kate", map[string]any{"onsitePassphrase": phrase}, 409, "contest_participation_unavailable")
}

// ADR 0024: someone who entered open moves into a team with the venue passphrase in the same request, as the team page
// sends it. A wrong passphrase, a full team or a taken name leaves them alone with nothing created; a right one makes
// the team onsite, and the division lives on the team alone.
func TestContestTeamSwitchWithPassphrasePostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan", "erin", "kate")
	const (
		tc     = "c3000000-0000-4000-8000-000000000001"
		pa     = "a3000000-0000-4000-8000-000000000001"
		phrase = "Venue_2026"
	)
	f.problem(pa)
	f.create(tc, pa, map[string]any{"teamMode": true, "maxTeamSize": 2, "onsiteDivision": true, "onsitePassphrase": phrase})
	path := "/my/contests/" + tc + "/participation"
	f.join(tc, "bob", create("Alpha"))
	alpha := f.team(tc, "bob").Code
	f.join(tc, "carol", joinCode(alpha))
	f.join(tc, "kate", nil)
	failures := func() int { return f.count(`SELECT count(*) FROM contest_attempt_failures WHERE owner_id='kate'`) }
	alone := func(step string, guesses int) {
		t.Helper()
		if kate := f.contest(tc, "kate"); !kate.Participating || kate.Team != nil || kate.Onsite {
			t.Fatalf("%s: %+v", step, kate)
		}
		if n := f.count(`SELECT count(*) FROM contest_teams WHERE name_key='delta'`); n != 0 {
			t.Fatalf("%s: a team was left behind", step)
		}
		if n := failures(); n != guesses {
			t.Fatalf("%s: %d guesses counted, want %d", step, n, guesses)
		}
	}
	// A wrong passphrase, or one with other characters, is a counted guess, and nothing moves or is created.
	f.fail("POST", path, "kate", withOnsite(create("Delta"), "Venue_2025"), 400, "incorrect_venue_passphrase")
	alone("wrong passphrase", 1)
	f.fail("POST", path, "kate", withOnsite(create("Delta"), "Venue-2026"), 400, "incorrect_venue_passphrase")
	alone("other characters", 2)
	f.fail("POST", path, "kate", withOnsite(joinCode(alpha), "Venue_2025"), 400, "incorrect_venue_passphrase")
	alone("wrong passphrase on a join", 3)
	// A full team or a taken name refuses the move even with the right passphrase, which then makes nobody onsite.
	f.fail("POST", path, "kate", withOnsite(joinCode(alpha), phrase), 409, "team_full")
	alone("full team", 3)
	f.fail("POST", path, "kate", withOnsite(create("ALPHA"), phrase), 409, "team_name_taken")
	alone("taken name", 3)
	// The right passphrase, as typed, moves them into a new team that becomes onsite through it.
	kate := f.join(tc, "kate", withOnsite(create("Delta"), " Ｖｅｎｕｅ＿２０２６ "))
	if kate.Team == nil || !kate.Team.Onsite || !kate.Onsite || !slices.Equal(kate.Team.Members, []string{"kate"}) {
		t.Fatalf("moved onsite: %+v %+v", kate, kate.Team)
	}
	// Joining an open team with it turns that team onsite too.
	f.join(tc, "erin", create("Gamma"))
	f.join(tc, "dan", nil)
	dan := f.join(tc, "dan", withOnsite(joinCode(f.team(tc, "erin").Code), phrase))
	if dan.Team == nil || !dan.Team.Onsite || !slices.Equal(dan.Team.Members, []string{"erin", "dan"}) || !f.contest(tc, "erin").Onsite {
		t.Fatalf("joined onsite: %+v", dan.Team)
	}
	if n := f.count(`SELECT count(*) FROM contest_teams WHERE name IN ('Delta','Gamma') AND onsite_via='passphrase' AND onsite_at IS NOT NULL`); n != 2 {
		t.Fatalf("onsite teams through the passphrase: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE onsite_at IS NOT NULL OR onsite_via IS NOT NULL`); n != 0 || failures() != 3 {
		t.Fatalf("members keep %d divisions of their own, %d guesses", n, failures())
	}
}
