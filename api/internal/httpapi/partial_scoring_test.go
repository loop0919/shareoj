package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

// scoredDraft is a groups-mode problem that every destination takes: 小課題1 (30 points) holds case 01 and 小課題2
// (70 points) cases 01 and 02; the sample is in no group.
func scoredDraft() problems.Draft {
	difficulty := 3
	return problems.Draft{
		Difficulty: &difficulty, Title: "部分点", Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256",
		Scoring: &problems.Scoring{Mode: problems.ScoringGroups, Groups: []problems.ScoringGroup{{Name: "小課題1", Points: 30}, {Name: "小課題2", Points: 70}}},
		TestCases: []problems.TestCase{
			{IsSample: true, Name: "sample", Input: "1", Output: "1"},
			{Name: "01", Input: "2", Output: "2", Groups: []string{"小課題1", "小課題2"}},
			{Name: "02", Input: "3", Output: "3", Groups: []string{"小課題2"}},
		},
	}
}

// ungrouped is what an old CLI pushes: the cases lose their groups and the scoring stays.
func ungrouped(d problems.Draft) problems.Draft {
	d.TestCases = slices.Clone(d.TestCases)
	for i := range d.TestCases {
		d.TestCases[i].Groups = nil
	}
	return d
}

// allOrNothing is scoredDraft without scoring.
func allOrNothing() problems.Draft {
	d := ungrouped(scoredDraft())
	d.Scoring = nil
	return d
}

// scoredResult is a stored result of scoredDraft, with its groups' scores in hundredths.
func scoredResult(verdict string, first, second int64) string {
	return fmt.Sprintf(`{"verdict":%q,"passed":1,"total":3,"scoring":{"score":%d,"maxScore":10000,"groups":[
 {"name":"小課題1","score":%d,"maxScore":3000,"cases":[1]},{"name":"小課題2","score":%d,"maxScore":7000,"cases":[1,2]}]}}`, verdict, first+second, first, second)
}

const acceptedResult = `{"verdict":"AC","passed":3,"total":3}`

// save writes the draft as owner through the API at the problem's current version and returns the saved problem, or
// the response body when the save is refused. Like the editor, it always names scoring, as null when there is none.
func (f teamTest) save(owner, id string, d problems.Draft, want int) (problems.Problem, string) {
	f.t.Helper()
	return f.saveDraft(owner, id, sentScoring(f.t, d, true), want)
}

// sentScoring is the draft as a JSON object, with or without the scoring key when it has no scoring.
func sentScoring(t *testing.T, d problems.Draft, named bool) map[string]any {
	t.Helper()
	data, err := json.Marshal(d)
	var body map[string]any
	if err == nil {
		err = json.Unmarshal(data, &body)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["scoring"]; named && !ok {
		body["scoring"] = nil
	}
	return body
}

func (f teamTest) saveDraft(owner, id string, d map[string]any, want int) (problems.Problem, string) {
	f.t.Helper()
	version := int64(0)
	if w := f.do(f.newRequest("GET", "/my/problems/"+id, owner, nil)); w.Code == 200 {
		var current problems.Problem
		f.decode(w.Body.String(), &current)
		version = current.Version
	}
	body := f.request("PUT", "/my/problems/"+id, owner, map[string]any{"version": version, "draft": d}, want)
	var saved problems.Problem
	if want == 200 {
		f.decode(body, &saved)
	}
	return saved, body
}

func (f teamTest) publish(owner, id string, publish bool, want int) {
	f.t.Helper()
	var current problems.Problem
	f.decode(f.request("GET", "/my/problems/"+id, owner, nil, 200), &current)
	f.request("PUT", "/my/problems/"+id+"/publication", owner, map[string]any{"version": current.Version, "publish": publish}, want)
}

// run inserts a public run, judged or waiting, made at the time given.
func (f teamTest) run(contest, owner, problem, status, result string, at time.Time) {
	f.t.Helper()
	var value, contestID any
	if result != "" {
		value = result
	}
	if contest != "" {
		contestID = contest
	}
	f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at,finished_at)
 VALUES($1,$2,$3,1,'部分点','cpp17','code','{"privateDraft":false}'::jsonb,$4,$5::jsonb,$6,$7,$7)`, newSubmissionID(), owner, problem, status, value, contestID, at)
}

func TestPartialScoringProblemsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		pub     = "f1000000-0000-4000-8000-000000000001"
		partial = "f1000000-0000-4000-8000-000000000002"
		binary  = "f1000000-0000-4000-8000-000000000003"
		late    = "f1000000-0000-4000-8000-000000000004"
		cid     = "f2000000-0000-4000-8000-000000000001"
		frozen  = "f2000000-0000-4000-8000-000000000002"
	)

	// Structure is checked at every save; what is missing shows in the readiness and holds publication back.
	broken := scoredDraft()
	broken.TestCases[2].Groups = []string{"小課題3"}
	f.fail("PUT", "/my/problems/"+pub, "alice", map[string]any{"version": 0, "draft": broken}, 400, "invalid_draft")
	waiting := scoredDraft()
	waiting.Scoring.Groups = append(waiting.Scoring.Groups, problems.ScoringGroup{Name: "小課題3", Points: 10})
	waiting.TestCases = append(waiting.TestCases, problems.TestCase{Name: "03", Input: "4", Output: "4"})
	saved, _ := f.save("alice", pub, waiting, 200)
	if saved.Readiness == nil || !slices.Equal(saved.Readiness.Publish, []problems.Issue{problems.IssueScoringGroupEmpty, problems.IssueTestCaseUngrouped}) {
		t.Fatalf("readiness: %+v", saved.Readiness)
	}
	f.publish("alice", pub, true, 400)
	if saved, _ = f.save("alice", pub, scoredDraft(), 200); len(saved.Readiness.Publish) != 0 {
		t.Fatalf("ready problem: %+v", saved.Readiness)
	}
	f.publish("alice", pub, true, 200)
	// The problem page shows the full score and the groups' points.
	var public problems.PublicProblem
	f.decode(f.request("GET", "/problems/"+pub, "", nil, 200), &public)
	if public.Scoring == nil || public.Scoring.FullScore != 100 || !slices.Equal(public.Scoring.Groups, scoredDraft().Scoring.Groups) {
		t.Fatalf("public scoring: %+v", public.Scoring)
	}
	// A published problem refuses what an old CLI pushes and keeps its draft.
	if _, body := f.save("alice", pub, ungrouped(scoredDraft()), 400); !strings.Contains(body, `"incomplete_problem"`) {
		t.Fatal(body)
	}
	var kept problems.Problem
	f.decode(f.request("GET", "/my/problems/"+pub, "alice", nil, 200), &kept)
	if !slices.Equal(kept.Draft.TestCases[2].Groups, []string{"小課題2"}) {
		t.Fatalf("draft changed: %+v", kept.Draft.TestCases)
	}

	// A contest draft asks no points of a partially scored problem: it is worth its full score.
	f.save("alice", partial, scoredDraft(), 200)
	f.save("alice", binary, allOrNothing(), 200)
	// The editor learns from the author's list which problems need no points.
	var own struct{ Items []problems.Summary }
	f.decode(f.request("GET", "/my/problems", "alice", nil, 200), &own)
	listed := map[string]problems.Summary{}
	for _, item := range own.Items {
		listed[item.ID] = item
	}
	if p, b := listed[partial], listed[binary]; !p.Partial || p.FullScore != 100 || b.Partial || b.FullScore != 0 {
		t.Fatalf("list: %+v %+v", p, b)
	}
	start, five := time.Now().Add(time.Hour), 5
	draft := func(version int64, points *int) map[string]any {
		return map[string]any{"version": version, "draft": map[string]any{"title": "部分点", "startsAt": start, "durationMinutes": 60, "penaltyMinutes": 5,
			"problems": []map[string]any{{"id": partial, "points": nil}, {"id": binary, "points": points}}}}
	}
	var d contests.DraftResult
	f.decode(f.request("PUT", "/my/contests/"+cid+"/draft", "alice", draft(0, nil), 200), &d)
	if !slices.Equal(d.Issues, []string{contests.IssuePoints}) || len(d.Problems) != 2 || !d.Problems[0].Partial || d.Problems[0].FullScore != 100 || d.Problems[1].Partial {
		t.Fatalf("draft: %+v", d)
	}
	f.decode(f.request("PUT", "/my/contests/"+cid+"/draft", "alice", draft(d.Version, &five), 200), &d)
	if len(d.Issues) != 0 {
		t.Fatalf("draft issues: %v", d.Issues)
	}
	var c contests.Contest
	f.decode(f.request("PUT", "/my/contests/"+cid+"/publication", "alice", map[string]any{"version": d.Version}, 200), &c)
	if !c.PartialScoring || c.Problems[0].Points != 100 || !c.Problems[0].Partial || c.Problems[1].Points != 5 || c.Problems[1].Partial {
		t.Fatalf("published: %+v", c)
	}
	// Before the start everyone learns that the contest has partial scores, though not which problem.
	if view := f.contest(cid, ""); !view.PartialScoring || view.Problems[0].Partial || view.Problems[0].Points != 100 {
		t.Fatalf("public view: %+v", view)
	}
	// The published contest reads points only for the all-or-nothing problem, which needs them.
	body := update(c, nil)
	body["problems"] = []map[string]any{{"id": partial, "points": 7}, {"id": binary, "points": 0}}
	f.fail("PUT", "/my/contests/"+cid, "alice", body, 400, "invalid_contest")
	body["problems"] = []map[string]any{{"id": partial, "points": 7}, {"id": binary, "points": 1000001}}
	f.fail("PUT", "/my/contests/"+cid, "alice", body, 400, "invalid_contest")
	body["problems"] = []map[string]any{{"id": partial, "points": 0}, {"id": binary, "points": 200}}
	f.decode(f.request("PUT", "/my/contests/"+cid, "alice", body, 200), &c)
	if c.Problems[0].Points != 100 || c.Problems[1].Points != 200 {
		t.Fatalf("saved points: %+v", c.Problems)
	}
	// Until the start a problem save brings the contest's points along, and the content rules hold.
	more := scoredDraft()
	more.Scoring.Groups[0].Points = 50
	f.save("alice", partial, more, 200)
	if c = f.contest(cid, "alice"); c.Problems[0].Points != 120 {
		t.Fatalf("points not synced: %+v", c.Problems)
	}
	if _, body := f.save("alice", partial, ungrouped(more), 400); !strings.Contains(body, `"incomplete_problem"`) {
		t.Fatal(body)
	}

	// From the start the scoring definition stays; membership, tests and the aggregator's code may change.
	f.schedule(cid, "running")
	reordered := scoredDraft()
	reordered.Scoring.Groups = []problems.ScoringGroup{{Name: "小課題2", Points: 70}, {Name: "小課題1", Points: 50}}
	aggregate := scoredDraft()
	aggregate.Scoring.Groups[0].Points = 50
	aggregate.Scoring.Mode, aggregate.Scoring.Aggregator = problems.ScoringAggregate, &problems.Generator{Runtime: "cpp17", Source: "int main(){}"}
	for name, changed := range map[string]problems.Draft{"points": scoredDraft(), "order": reordered, "mode": aggregate, "all-or-nothing": allOrNothing()} {
		if _, body := f.save("alice", partial, changed, 409); !strings.Contains(body, `"problem_scoring_locked"`) {
			t.Fatal(name, body)
		}
	}
	if _, body := f.save("alice", binary, scoredDraft(), 409); !strings.Contains(body, `"problem_scoring_locked"`) {
		t.Fatal(body)
	}
	moved := scoredDraft()
	moved.Scoring.Groups[0].Points = 50
	moved.TestCases[2].Groups = []string{"小課題1", "小課題2"}
	moved.TestCases[2].Output = "fixed"
	f.save("alice", partial, moved, 200)
	f.save("alice", partial, ungrouped(moved), 400)
	if c = f.contest(cid, "alice"); c.Problems[0].Points != 120 {
		t.Fatalf("points after the start: %+v", c.Problems)
	}
	var problem contests.ContestProblem
	f.decode(f.request("GET", "/contests/"+cid+"/problems/"+partial, "", nil, 200), &problem)
	if problem.Scoring == nil || problem.Scoring.FullScore != 120 || problem.Scoring.Groups[0].Points != 50 {
		t.Fatalf("contest problem: %+v", problem.Scoring)
	}

	// The content rules hold until the results are final, even for a problem its author takes down after the end.
	f.save("alice", late, scoredDraft(), 200)
	f.create(frozen, late, map[string]any{"freezeMinutes": 30})
	f.schedule(frozen, "ended")
	if err := (&contests.Store{Pool: f.store.Pool()}).Release(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.publish("alice", late, false, 200)
	f.save("alice", late, ungrouped(scoredDraft()), 400)
	f.request("POST", "/my/contests/"+frozen+"/reveal", "alice", nil, 200)
	f.save("alice", late, ungrouped(scoredDraft()), 200)
	f.save("alice", late, allOrNothing(), 409)
}

func TestPartialScoringSolvedPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan", "erin", "fay")
	const (
		scored     = "f3000000-0000-4000-8000-000000000001"
		allOr      = "f3000000-0000-4000-8000-000000000002"
		individual = "f3000000-0000-4000-8000-000000000003"
		teamed     = "f3000000-0000-4000-8000-000000000004"
		cid        = "f4000000-0000-4000-8000-000000000001"
		tid        = "f4000000-0000-4000-8000-000000000002"
	)
	f.save("alice", scored, scoredDraft(), 200)
	f.save("alice", allOr, allOrNothing(), 200)
	f.publish("alice", scored, true, 200)
	f.publish("alice", allOr, true, 200)
	// A run solves a problem with the full score: an AC short of it does not, a full score beside a wrong sample does,
	// and a result from before partial scores keeps its AC.
	now := time.Now()
	notified := func() string {
		t.Helper()
		var actor string
		if err := f.store.Pool().QueryRow(f.ctx, `SELECT COALESCE((SELECT actor_id FROM notifications WHERE kind='first_accept' AND problem_id=$1),'')`, scored).Scan(&actor); err != nil {
			t.Fatal(err)
		}
		return actor
	}
	f.run("", "bob", scored, "DONE", scoredResult("AC", 3000, 0), now.Add(-3*time.Minute))
	if actor := notified(); actor != "" {
		t.Fatalf("an AC short of the full score notified %q", actor)
	}
	f.run("", "carol", scored, "DONE", scoredResult("WA", 3000, 7000), now.Add(-2*time.Minute))
	f.run("", "dan", scored, "DONE", acceptedResult, now.Add(-time.Minute))
	f.run("", "erin", allOr, "DONE", acceptedResult, now.Add(-time.Minute))
	if actor := notified(); actor != "carol" {
		t.Fatalf("first full score from %q", actor)
	}
	var list struct {
		Items []problems.PublicProblem `json:"items"`
	}
	f.decode(f.request("GET", "/problems", "", nil, 200), &list)
	solvers := map[string]int64{}
	for _, p := range list.Items {
		solvers[p.ID] = p.SolverCount
	}
	var detail problems.PublicProblem
	f.decode(f.request("GET", "/problems/"+scored, "", nil, 200), &detail)
	if solvers[scored] != 2 || solvers[allOr] != 1 || detail.SolverCount != 2 {
		t.Fatalf("solvers: %v %d", solvers, detail.SolverCount)
	}
	for owner, want := range map[string]string{"bob": `{"items":[]}`, "carol": `{"items":["` + scored + `"]}`, "dan": `{"items":["` + scored + `"]}`} {
		if got := strings.TrimSpace(f.request("GET", "/my/solved-problems", owner, nil, 200)); got != want {
			t.Fatalf("%s solved %s", owner, got)
		}
	}

	// 定期便 solves with the full score too, whatever the verdict, and shows a single run's best score; a run that
	// raises the score is not wrong (ADR 0033). An all-or-nothing problem shows no score.
	at := now.UTC().Add(-48 * time.Hour).Truncate(time.Second)
	f.exec(`INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES($1,'easy','revival',$2,3,$1::timestamptz+interval '23 hours'),($1,'hard','revival',$3,7,$1::timestamptz+interval '23 hours')`, at, scored, allOr)
	f.run("", "erin", scored, "DONE", scoredResult("AC", 3000, 0), at.Add(time.Minute))
	f.run("", "erin", scored, "DONE", scoredResult("WA", 3000, 7000), at.Add(2*time.Minute))
	f.run("", "fay", scored, "DONE", scoredResult("AC", 0, 7000), at.Add(time.Minute))
	f.run("", "fay", allOr, "DONE", acceptedResult, at.Add(3*time.Minute))
	raw := f.request("GET", "/featured/standings?at="+url.QueryEscape(at.Format(time.RFC3339)), "", nil, 200)
	var standings problems.FeaturedStandings
	f.decode(raw, &standings)
	if len(standings.Items) != 2 {
		t.Fatalf("featured: %+v", standings.Items)
	}
	fay, erin := standings.Items[0], standings.Items[1]
	if fay.Handle != "fay" || fay.Rank != 1 || fay.Easy.Accepted || *fay.Easy.Score != 7000 || fay.Easy.Wrong != 0 || !fay.Hard.Accepted || fay.Hard.Score != nil ||
		erin.Handle != "erin" || erin.Rank != 2 || !erin.Easy.Accepted || *erin.Easy.Score != 10000 || erin.Easy.Wrong != 0 || *erin.Easy.TimeMS != 120000 || erin.Hard.Accepted {
		t.Fatalf("featured: %+v", standings.Items)
	}

	// In a contest the solved mark sums each group's best like the standings, over a team's runs in a team contest.
	f.save("alice", individual, scoredDraft(), 200)
	f.save("alice", teamed, scoredDraft(), 200)
	f.create(cid, individual, nil)
	f.join(cid, "bob", nil)
	f.join(cid, "carol", nil)
	f.create(tid, teamed, map[string]any{"teamMode": true})
	f.join(tid, "dan", create("Alpha"))
	f.join(tid, "erin", joinCode(f.team(tid, "dan").Code))
	f.join(tid, "fay", nil)
	f.schedule(cid, "running")
	f.schedule(tid, "running")
	started := time.Now().Add(-30 * time.Minute)
	f.run(cid, "bob", individual, "DONE", scoredResult("WA", 3000, 0), started)
	f.run(cid, "bob", individual, "DONE", scoredResult("WA", 0, 7000), started.Add(time.Minute))
	f.run(cid, "carol", individual, "DONE", scoredResult("AC", 3000, 0), started)
	f.run(tid, "dan", teamed, "DONE", scoredResult("WA", 3000, 0), started)
	f.run(tid, "erin", teamed, "DONE", scoredResult("WA", 0, 7000), started.Add(time.Minute))
	f.run(tid, "fay", teamed, "DONE", scoredResult("WA", 0, 7000), started.Add(time.Minute))
	for _, v := range []struct {
		contest, viewer string
		solved          bool
	}{{cid, "bob", true}, {cid, "carol", false}, {tid, "dan", true}, {tid, "erin", true}, {tid, "fay", false}} {
		if got := f.contest(v.contest, v.viewer).Problems[0].Solved; got != v.solved {
			t.Fatalf("%s solved %v", v.viewer, got)
		}
	}
}

func TestPartialScoringStandingsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan", "erin", "fay", "gus")
	const (
		scored = "f5000001-0000-4000-8000-000000000001"
		allOr  = "f5000002-0000-4000-8000-000000000001"
		teamed = "f5000003-0000-4000-8000-000000000001"
		cid    = "f6000000-0000-4000-8000-000000000001"
		tid    = "f6000000-0000-4000-8000-000000000002"
	)
	f.save("alice", scored, scoredDraft(), 200)
	f.save("alice", teamed, scoredDraft(), 200)
	f.problem(allOr)
	start := time.Now().Add(time.Hour)
	f.request("PUT", "/my/contests/"+cid, "alice", map[string]any{"title": "部分点", "startsAt": start, "endsAt": start.Add(2 * time.Hour), "penaltyMinutes": 5, "version": 0, "freezeMinutes": 30,
		"problems": []map[string]any{{"id": scored}, {"id": allOr, "points": 200}}}, 200)
	for _, owner := range []string{"bob", "carol", "dan"} {
		f.join(cid, owner, nil)
	}
	// The contest runs from 100 minutes ago for two hours and is frozen from minute 90.
	begin := time.Now().Add(-100 * time.Minute).Truncate(time.Millisecond)
	f.exec(`UPDATE contests SET starts_at=$2,ends_at=$3 WHERE id=$1`, cid, begin, begin.Add(2*time.Hour))
	at := func(minutes int) time.Time { return begin.Add(time.Duration(minutes) * time.Minute) }
	// bob: 30 points, a run that adds nothing although AC, then 小課題2 for the full score, and the other problem.
	f.run(cid, "bob", scored, "DONE", scoredResult("WA", 3000, 0), at(10))
	f.run(cid, "bob", scored, "DONE", scoredResult("AC", 0, 0), at(20))
	f.run(cid, "bob", scored, "DONE", scoredResult("WA", 0, 7000), at(30))
	f.run(cid, "bob", allOr, "DONE", acceptedResult, at(40))
	// carol: 15.50 points, a CE, a run that adds nothing, and the full score after the freeze.
	f.run(cid, "carol", scored, "DONE", scoredResult("WA", 1550, 0), at(5))
	f.run(cid, "carol", scored, "DONE", scoredResult("CE", 0, 0), at(6))
	f.run(cid, "carol", scored, "DONE", scoredResult("WA", 0, 0), at(7))
	f.run(cid, "carol", scored, "DONE", scoredResult("WA", 3000, 7000), at(95))
	// dan: a run waiting, a JE and a run without points.
	f.run(cid, "dan", scored, "QUEUED", "", at(15))
	f.run(cid, "dan", scored, "DONE", `{"verdict":"JE","passed":0,"total":0}`, at(16))
	f.run(cid, "dan", scored, "DONE", scoredResult("WA", 0, 0), at(17))

	minutes := func(at *time.Time) int {
		if at == nil {
			return -1
		}
		return int(at.Sub(begin) / time.Minute)
	}
	// table lists each row as "rank score scoreText minutes" and each cell as
	// "score scoreText wrong pending frozen scoredAt acceptedAt", times in minutes from the start.
	table := func(path, viewer string) map[string]string {
		t.Helper()
		raw := f.request("GET", path, viewer, nil, 200)
		for _, field := range []string{`"score":`, `"scoreText":`, `"scoredAt":`} {
			if !strings.Contains(raw, field) {
				t.Fatalf("missing %s in %s", field, raw)
			}
		}
		var rows []contests.Standing
		f.decode(raw, &rows)
		got := map[string]string{}
		for _, row := range rows {
			name := row.Handle
			if row.Team != nil {
				name = row.Team.Name
			}
			if row.Points != int(row.Score/100) {
				t.Fatalf("whole points %+v", row)
			}
			got[name] = fmt.Sprintf("%d %d %s %d", row.Rank, row.Score, row.ScoreText, row.TimeMS/60000)
			for id, s := range row.Problems {
				got[name+" "+id] = fmt.Sprintf("%d %s %d %d %d %d %d", s.Score, s.ScoreText, s.Wrong, s.Pending, s.Frozen, minutes(s.ScoredAt), minutes(s.AcceptedAt))
			}
		}
		return got
	}
	check := func(label string, got, want map[string]string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s:\n got %v\nwant %v", label, got, want)
		}
	}
	frozenTable := map[string]string{
		"bob": "1 30000 300 45", "bob " + scored: "10000 100 1 0 0 30 30", "bob " + allOr: "20000 200 0 0 0 40 40",
		// The run at 7 came after carol's last improvement, so neither the cell nor the penalty counts it.
		"carol": "2 1550 15.50 5", "carol " + scored: "1550 15.50 0 0 1 5 -1",
		"dan": "3 0 0 0", "dan " + scored: "0 0 1 1 0 -1 -1",
	}
	check("public", table("/contests/"+cid+"/standings", ""), frozenTable)
	check("participant", table("/my/contests/"+cid+"/standings?live=1", "carol"), frozenTable)
	live := map[string]string{}
	for k, v := range frozenTable {
		live[k] = v
	}
	live["carol"], live["carol "+scored] = "2 10000 100 100", "10000 100 1 0 0 95 95"
	check("live", table("/my/contests/"+cid+"/standings?live=1", "alice"), live)
	// Solved marks follow the viewer's own results, frozen or not.
	if !f.contest(cid, "carol").Problems[0].Solved || f.contest(cid, "dan").Problems[0].Solved {
		t.Fatal("solved marks")
	}

	// A team adds up each group's best over its members' runs; someone who entered alone keeps their own.
	f.create(tid, teamed, map[string]any{"teamMode": true})
	f.join(tid, "erin", create("Beta"))
	f.join(tid, "fay", joinCode(f.team(tid, "erin").Code))
	f.join(tid, "gus", nil)
	f.exec(`UPDATE contests SET starts_at=$2,ends_at=$3 WHERE id=$1`, tid, begin, begin.Add(2*time.Hour))
	f.run(tid, "erin", teamed, "DONE", scoredResult("WA", 3000, 0), at(10))
	f.run(tid, "fay", teamed, "DONE", scoredResult("WA", 0, 7000), at(20))
	f.run(tid, "gus", teamed, "DONE", scoredResult("WA", 3000, 0), at(5))
	check("teams", table("/contests/"+tid+"/standings", ""), map[string]string{
		"Beta": "1 10000 100 20", "Beta " + teamed: "10000 100 0 0 0 20 20",
		"gus": "2 3000 30 5", "gus " + teamed: "3000 30 0 0 0 5 -1",
	})
}

// A client that does not know partial scoring sends neither scoring nor the cases' groups. Its save is refused as
// outdated instead of making the problem all-or-nothing; switching back is an explicit null.
func TestPartialScoringNeedsScoringNamedPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		private = "f3000000-0000-4000-8000-000000000001"
		plain   = "f3000000-0000-4000-8000-000000000002"
	)
	f.save("alice", private, scoredDraft(), 200)
	old := sentScoring(t, allOrNothing(), false)
	if _, ok := old["scoring"]; ok {
		t.Fatal("the old body names scoring")
	}
	f.fail("PUT", "/my/problems/"+private, "alice", map[string]any{"version": 1, "draft": old}, 409, "version_conflict")
	var kept problems.Problem
	f.decode(f.request("GET", "/my/problems/"+private, "alice", nil, 200), &kept)
	if kept.Version != 1 || kept.Draft.Scoring == nil || !slices.Equal(kept.Draft.TestCases[1].Groups, []string{"小課題1", "小課題2"}) {
		t.Fatalf("draft changed: %+v", kept.Draft)
	}
	if saved, _ := f.saveDraft("alice", private, sentScoring(t, allOrNothing(), true), 200); saved.Draft.Scoring != nil {
		t.Fatalf("null kept scoring: %+v", saved.Draft.Scoring)
	}
	// A problem without scoring saves as before, with or without the key.
	f.saveDraft("alice", plain, old, 200)
	f.saveDraft("alice", plain, old, 200)
}

// The start is read once: a save that passed the lock just before the start brings the contest's points along even when
// it writes the contest's copy after the start, so the points always match the full score of that copy.
func TestPartialScoringSaveAtTheStartPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		partial = "f4000000-0000-4000-8000-000000000001"
		cid     = "f4000000-0000-4000-8000-000000000002"
	)
	f.save("alice", partial, scoredDraft(), 200)
	f.create(cid, partial, nil)
	version := func() int64 {
		var current problems.Problem
		f.decode(f.request("GET", "/my/problems/"+partial, "alice", nil, 200), &current)
		return current.Version
	}
	more := scoredDraft()
	more.Scoring.Groups[0].Points = 50
	// A submission being created holds the contest; the save waits for it rather than racing its judging.
	if w := f.waits(f.newRequest("PUT", "/my/problems/"+partial, "alice", map[string]any{"version": version(), "draft": sentScoring(t, more, true)}),
		`SELECT 1 FROM contests WHERE id='`+cid+`' FOR SHARE`); w.Code != 200 {
		t.Fatalf("save after a submission: %d %s", w.Code, w.Body.String())
	}
	if c := f.contest(cid, "alice"); c.Problems[0].Points != 120 {
		t.Fatalf("points before the start: %+v", c.Problems)
	}

	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	r := f.newRequest("PUT", "/my/problems/"+partial, "alice", map[string]any{"version": version(), "draft": sentScoring(t, scoredDraft(), true)})
	f.exec(`UPDATE contests SET starts_at=clock_timestamp()+interval '300 milliseconds' WHERE id=$1`, cid)
	// Hold the contest's copy, so that the save writes it only after the start. Writing the row makes the save
	// evaluate its update again once the lock is free.
	tx, err := f.store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `UPDATE contest_problems SET points=points WHERE problem_id=$1`, partial); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.do(r) }()
	time.Sleep(600 * time.Millisecond)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-done:
	case <-ctx.Done():
		t.Fatal("save did not finish")
	}
	if w.Code != 200 {
		t.Fatalf("save across the start: %d %s", w.Code, w.Body.String())
	}
	var points, full int
	if err = f.store.Pool().QueryRow(f.ctx, `SELECT cp.points,(SELECT sum((g->>'points')::int) FROM jsonb_array_elements(cp.draft->'scoring'->'groups') g)
 FROM contest_problems cp WHERE cp.problem_id=$1`, partial).Scan(&points, &full); err != nil {
		t.Fatal(err)
	}
	if points != 100 || full != 100 {
		t.Fatalf("points %d for a full score of %d", points, full)
	}
	// From now on the definition stays, and so do the points.
	f.save("alice", partial, more, 409)
}

// During judge maintenance no runtime is enabled, but problems with judge code in a contest, or published, still save
// (ADR 0010); the readiness tells that they wait for the judge.
func TestProblemSaveDuringMaintenancePostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		special = "f5000000-0000-4000-8000-000000000001"
		pub     = "f5000000-0000-4000-8000-000000000002"
		cid     = "f5000000-0000-4000-8000-000000000003"
	)
	checked := scoredDraft()
	checked.Scoring.Mode, checked.Scoring.Aggregator = problems.ScoringAggregate, &problems.Generator{Runtime: "cpp17", Source: "int main(){}"}
	checked.Checker = &problems.Generator{Runtime: "cpp17", Source: "int main(){}"}
	f.save("alice", special, checked, 200)
	f.save("alice", pub, checked, 200)
	f.publish("alice", pub, true, 200)
	f.create(cid, special, nil)
	pool := f.store.Pool()
	f.handler = newHandler(AuthConfig{}, handlerDependencies{Store: f.store, Contests: &contests.Store{Pool: pool}, Images: &images.Store{Pool: pool}, Notifications: &notifications.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "none", Verifier: newCognitoVerifier(f.sign.server.URL, "client")})
	checked.Markdown = "typo fixed"
	for _, id := range []string{special, pub} {
		saved, _ := f.save("alice", id, checked, 200)
		if saved.Readiness == nil || !slices.Contains(saved.Readiness.Contest, problems.IssueCheckerRuntime) || !slices.Contains(saved.Readiness.Contest, problems.IssueAggregatorRuntime) {
			t.Fatalf("readiness during maintenance: %+v", saved.Readiness)
		}
	}
	// What is really missing is still refused.
	f.save("alice", special, ungrouped(checked), 400)
	f.save("alice", pub, ungrouped(checked), 400)
}

// Lists carry each run's score without its groups; details carry the group table and the cases' scores, and neither
// shows the checker log, which holds the aggregator's stderr (ADR 0025).
func TestPartialScoringSubmissionViewsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	const (
		pub     = "f7000000-0000-4000-8000-000000000001"
		inside  = "f7000000-0000-4000-8000-000000000002"
		cid     = "f7000000-0000-4000-8000-000000000003"
		ownRun  = "f7100000-0000-4000-8000-000000000001"
		contRun = "f7100000-0000-4000-8000-000000000002"
	)
	f.save("alice", pub, scoredDraft(), 200)
	f.publish("alice", pub, true, 200)
	f.save("alice", inside, scoredDraft(), 200)
	f.create(cid, inside, nil)
	f.join(cid, "bob", nil)
	f.schedule(cid, "ended")
	const detail = `{"verdict":"WA","passed":2,"total":3,"checkerLog":"aggregator stderr","cases":[
 {"name":"sample","verdict":"AC","cpuTimeMs":1,"wallTimeMs":1,"memoryBytes":1024},
 {"name":"01","verdict":"AC","score":1000000000000000000,"cpuTimeMs":1,"wallTimeMs":1,"memoryBytes":1024},
 {"name":"02","verdict":"WA","score":3,"cpuTimeMs":1,"wallTimeMs":1,"memoryBytes":1024}],
 "scoring":{"score":3750,"maxScore":10000,"groups":[{"name":"小課題1","score":3000,"maxScore":3000,"cases":[1]},{"name":"小課題2","score":750,"maxScore":7000,"cases":[1,2]}]}}`
	insert := func(id, problem string, contest any, at time.Time) {
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result,contest_id,created_at,finished_at)
 VALUES($1,'bob',$2,1,'部分点','cpp17','code','{"privateDraft":false}'::jsonb,'DONE',$3::jsonb,$4,$5,$5)`, id, problem, detail, contest, at)
	}
	insert(ownRun, pub, nil, time.Now())
	insert(contRun, inside, cid, time.Now().Add(-90*time.Minute))
	for _, path := range []string{"/problems/" + pub + "/submissions", "/contests/" + cid + "/submissions"} {
		body := f.request("GET", path, "", nil, 200)
		if !strings.Contains(body, `"scoring":{"score":3750,"maxScore":10000}`) || strings.Contains(body, `"groups"`) || strings.Contains(body, "aggregator stderr") {
			t.Fatalf("list %s: %s", path, body)
		}
	}
	for _, path := range []string{"/problems/" + pub + "/submissions/" + ownRun, "/contests/" + cid + "/submissions/" + contRun, "/submissions/" + ownRun} {
		body := f.request("GET", path, "", nil, 200)
		for _, want := range []string{`"score":1000000000000000000`, `"score":3}`, `{"name":"小課題1","score":3000,"maxScore":3000,"cases":[1]}`, `{"name":"小課題2","score":750,"maxScore":7000,"cases":[1,2]}`} {
			if !strings.Contains(body, want) {
				t.Fatalf("detail %s lacks %s: %s", path, want, body)
			}
		}
		if strings.Contains(body, "aggregator stderr") || strings.Contains(body, "checkerLog") {
			t.Fatalf("detail %s shows the checker log: %s", path, body)
		}
	}
}
