package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/accounts"
	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

// teamTest runs the participation rules of ADR 0024 against PostgreSQL in a schema of its own.
type teamTest struct {
	t       *testing.T
	ctx     context.Context
	store   *problems.Store
	handler http.Handler
	sign    signingFixture
}

func newTeamTest(t *testing.T, users ...string) teamTest {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	schema := fmt.Sprintf("test_contest_teams_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	pool := store.Pool()
	for _, user := range users {
		if _, err = pool.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES($1,$1)`, user); err != nil {
			t.Fatal(err)
		}
	}
	sign := newSigningFixture(t)
	f := teamTest{t: t, ctx: ctx, store: store, sign: sign}
	f.handler = newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: pool}, Images: &images.Store{Pool: pool}, Notifications: &notifications.Store{Pool: pool}, Profiles: profiles.New(pool), Submissions: &submissions.Store{Pool: pool}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(sign.server.URL, "client")})
	return f
}

// newRequest builds the request in the test goroutine, so that it can be served from another one.
func (f teamTest) newRequest(method, path, owner string, body any) *http.Request {
	f.t.Helper()
	raw := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		raw = string(data)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if owner != "" {
		r.Header.Set("Authorization", "Bearer "+f.sign.token(f.t, owner, nil))
	}
	return r
}

func (f teamTest) do(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func (f teamTest) request(method, path, owner string, body any, want int) string {
	f.t.Helper()
	w := f.do(f.newRequest(method, path, owner, body))
	if w.Code != want {
		f.t.Fatalf("%s %s (%s): got %d want %d: %s", method, path, owner, w.Code, want, w.Body.String())
	}
	return w.Body.String()
}

// fail expects an error response with the code.
func (f teamTest) fail(method, path, owner string, body any, want int, code string) {
	f.t.Helper()
	if got := f.request(method, path, owner, body, want); !strings.Contains(got, `"error":"`+code+`"`) {
		f.t.Fatalf("%s %s (%s): want %s, got %s", method, path, owner, code, got)
	}
}

func (f teamTest) decode(body string, v any) {
	f.t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		f.t.Fatal(err)
	}
}

func (f teamTest) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool().Exec(f.ctx, query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f teamTest) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.store.Pool().QueryRow(f.ctx, query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// problem saves a problem by alice that a contest accepts.
func (f teamTest) problem(id string) {
	f.t.Helper()
	difficulty := 3
	draft := problems.Draft{Difficulty: &difficulty, Title: "Team", Markdown: "statement", Editorial: "editorial", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
	if _, err := f.store.Save(f.ctx, "alice", id, 0, draft); err != nil {
		f.t.Fatal(err)
	}
}

// create publishes a contest an hour ahead with the problem and the settings in extra.
func (f teamTest) create(id, problem string, extra map[string]any) contests.Contest {
	f.t.Helper()
	start := time.Now().Add(time.Hour)
	body := map[string]any{"title": "Teams", "description": "", "startsAt": start, "endsAt": start.Add(time.Hour), "penaltyMinutes": 5, "version": 0, "problems": []map[string]any{{"id": problem, "points": 100}}}
	for k, v := range extra {
		body[k] = v
	}
	var c contests.Contest
	f.decode(f.request("PUT", "/my/contests/"+id, "alice", body, 200), &c)
	return c
}

// update is the body that saves c again with the settings in extra; without extra it is what a client from before the
// participation settings sends.
func update(c contests.Contest, extra map[string]any) map[string]any {
	list := []map[string]any{}
	for _, p := range c.Problems {
		list = append(list, map[string]any{"id": p.ID, "points": p.Points})
	}
	body := map[string]any{"title": c.Title, "description": c.Description, "startsAt": c.StartsAt, "endsAt": c.EndsAt, "penaltyMinutes": c.PenaltyMinutes, "version": c.Version, "problems": list}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func (f teamTest) contest(id, viewer string) contests.Contest {
	f.t.Helper()
	path := "/contests/" + id
	if viewer != "" {
		path = "/my/contests/" + id
	}
	var c contests.Contest
	f.decode(f.request("GET", path, viewer, nil, 200), &c)
	return c
}

func (f teamTest) join(id, owner string, body any) contests.Contest {
	f.t.Helper()
	var c contests.Contest
	f.decode(f.request("POST", "/my/contests/"+id+"/participation", owner, body, 200), &c)
	return c
}

func (f teamTest) team(id, owner string) contests.Team {
	f.t.Helper()
	var t contests.Team
	f.decode(f.request("GET", "/my/contests/"+id+"/team", owner, nil, 200), &t)
	return t
}

func (f teamTest) entrants(id string) []contests.Entrant {
	f.t.Helper()
	var list itemsResponse[contests.Entrant]
	f.decode(f.request("GET", "/my/contests/"+id+"/participants", "alice", nil, 200), &list)
	return list.Items
}

// schedule moves a contest: "running" started an hour ago, "ended" ended an hour ago.
func (f teamTest) schedule(id, state string) {
	f.t.Helper()
	switch state {
	case "running":
		f.exec(`UPDATE contests SET starts_at=now()-interval '1 hour',ends_at=now()+interval '1 hour' WHERE id=$1`, id)
	case "ended":
		f.exec(`UPDATE contests SET starts_at=now()-interval '2 hours',ends_at=now()-interval '1 hour' WHERE id=$1`, id)
	}
}

func create(name string) map[string]any {
	return map[string]any{"team": map[string]any{"action": "create", "name": name}}
}

func joinCode(code string) map[string]any {
	return map[string]any{"team": map[string]any{"action": "join", "code": code}}
}

func TestContestTeamsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "cos", "tess", "bob", "carol", "dan", "erin", "judy")
	const (
		tc = "c0000000-0000-4000-8000-000000000001"
		ic = "c0000000-0000-4000-8000-000000000002"
		pa = "a0000000-0000-4000-8000-000000000001"
		pb = "a0000000-0000-4000-8000-000000000002"
	)
	f.problem(pa)
	f.problem(pb)

	// Settings: a team size is 2 to 50 or null for no limit of the contest's own, and only a team contest keeps one.
	for _, bad := range []int{1, 51} {
		f.fail("PUT", "/my/contests/"+tc, "alice", map[string]any{"title": "Teams", "startsAt": time.Now().Add(time.Hour), "endsAt": time.Now().Add(2 * time.Hour), "version": 0,
			"problems": []map[string]any{{"id": pa, "points": 100}}, "teamMode": true, "maxTeamSize": bad}, 400, "invalid_contest")
	}
	c := f.create(tc, pa, map[string]any{"teamMode": true, "maxTeamSize": 3})
	if !c.TeamMode || c.MaxTeamSize == nil || *c.MaxTeamSize != 3 || c.OnsiteDivision || c.Team != nil || c.Onsite {
		t.Fatalf("team contest: %+v", c)
	}
	if individual := f.create(ic, pb, map[string]any{"teamMode": false, "maxTeamSize": 4}); individual.TeamMode || individual.MaxTeamSize != nil {
		t.Fatalf("individual contest: %+v", individual)
	}
	if public := f.contest(tc, ""); !public.TeamMode || *public.MaxTeamSize != 3 || public.Team != nil || public.Participating {
		t.Fatalf("public view: %+v", public)
	}
	f.exec(`INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,'cos')`, tc)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tess')`, pa)

	// A team contest needs a team; names are normalized, and hidden characters are refused.
	path := "/my/contests/" + tc + "/participation"
	f.fail("POST", path, "bob", nil, 409, "contest_team_required")
	f.fail("POST", path, "bob", create("Team\u200bAlpha"), 400, "invalid_team_name")
	f.fail("POST", path, "bob", create(strings.Repeat("a", 41)), 400, "invalid_team_name")
	f.fail("POST", path, "bob", map[string]any{"team": map[string]any{"action": "leave"}}, 400, "invalid_request")
	f.fail("POST", path, "bob", map[string]any{"team": map[string]any{"action": "join", "code": " - "}}, 400, "invalid_request")
	f.fail("POST", path, "bob", map[string]any{"teams": nil}, 400, "invalid_request")
	if n := f.count(`SELECT count(*) FROM contest_participants`); n != 0 {
		t.Fatalf("refused requests registered %d", n)
	}
	joined := f.join(tc, "bob", create("  Ｔｅａｍ　 Alpha "))
	if !joined.Participating || joined.Team == nil || joined.Team.Name != "Team Alpha" || !slices.Equal(joined.Team.Members, []string{"bob"}) || joined.Onsite {
		t.Fatalf("created: %+v %+v", joined, joined.Team)
	}
	alpha := f.team(tc, "bob")
	if len(alpha.Code) != 10 || alpha.ID != joined.Team.ID || alpha.Name != "Team Alpha" {
		t.Fatalf("team: %+v", alpha)
	}
	// Names are unique regardless of case and width, and an invisible character cannot make a look-alike.
	f.fail("POST", path, "carol", create("ＴＥＡＭ alpha"), 409, "team_name_taken")
	f.fail("POST", path, "carol", create("Team Alpha\ufe0f"), 400, "invalid_team_name")
	// Joining takes the code alone, as people type it: in lower case, with a hyphen.
	joined = f.join(tc, "carol", joinCode(strings.ToLower(alpha.Code[:5])+"-"+alpha.Code[5:]))
	if !slices.Equal(joined.Team.Members, []string{"bob", "carol"}) || joined.Team.ID != alpha.ID {
		t.Fatalf("joined: %+v", joined.Team)
	}
	// Sending one's own code again, or nothing, changes nothing; another team is refused.
	f.join(tc, "carol", joinCode(alpha.Code))
	f.join(tc, "carol", nil)
	f.fail("POST", path, "carol", create("Beta"), 409, "team_already_joined")
	f.fail("POST", path, "carol", joinCode("ABCDEFGHJK"), 409, "team_already_joined")
	// A team is full at the contest's size. Staff stay out, and nothing is created for them.
	f.join(tc, "dan", joinCode(alpha.Code))
	f.fail("POST", path, "erin", joinCode(alpha.Code), 409, "team_full")
	f.fail("POST", path, "erin", joinCode("ABCDEFGHJK"), 400, "incorrect_team_code")
	for _, staff := range []string{"tess", "cos", "alice"} {
		f.fail("POST", path, staff, create("Staff "+staff), 409, "contest_participation_unavailable")
	}
	if n := f.count(`SELECT count(*) FROM contest_teams`); n != 1 {
		t.Fatalf("teams: %d", n)
	}
	// An individual contest takes no team, and an empty body still registers there as before.
	f.fail("POST", "/my/contests/"+ic+"/participation", "erin", create("Solo"), 409, "contest_team_unavailable")
	if solo := f.join(ic, "erin", nil); !solo.Participating || solo.Team != nil {
		t.Fatalf("individual: %+v", solo)
	}
	// A body other than JSON is refused; an empty one needs no content type.
	r := f.newRequest("POST", "/my/contests/"+ic+"/participation", "judy", create("Solo"))
	r.Header.Set("Content-Type", "text/plain")
	if w := f.do(r); w.Code != 415 {
		t.Fatalf("text body: %d %s", w.Code, w.Body.String())
	}
	r = f.newRequest("POST", "/my/contests/"+ic+"/participation", "judy", nil)
	r.Header.Del("Content-Type")
	if w := f.do(r); w.Code != 200 {
		t.Fatalf("empty body: %d %s", w.Code, w.Body.String())
	}

	// Only members see the code, and no other response carries it.
	f.fail("GET", "/my/contests/"+tc+"/team", "erin", nil, 404, "team_not_found")
	f.fail("GET", "/my/contests/"+tc+"/team", "alice", nil, 404, "team_not_found")
	f.fail("GET", "/my/contests/"+ic+"/team", "erin", nil, 404, "team_not_found")
	if member := f.contest(tc, "carol"); member.Team == nil || member.Team.ID != alpha.ID || len(member.Team.Members) != 3 {
		t.Fatalf("member view: %+v", member.Team)
	}
	for _, viewer := range []string{"", "carol", "alice", "tess"} {
		for _, p := range []string{"/contests/" + tc, "/contests", "/contests/" + tc + "/standings", "/my/contests/" + tc, "/my/contests", "/my/contests/" + tc + "/participants"} {
			if viewer == "" && strings.HasPrefix(p, "/my/") || viewer != "alice" && strings.HasSuffix(p, "/participants") {
				continue
			}
			if body := f.request("GET", p, viewer, nil, 200); strings.Contains(body, alpha.Code) {
				t.Fatalf("%s for %q shows the join code", p, viewer)
			}
		}
	}

	// Registrations fix the team mode, and the size cannot drop below the largest team.
	c = f.contest(tc, "alice")
	f.fail("PUT", "/my/contests/"+tc, "alice", update(c, map[string]any{"teamMode": false}), 409, "contest_team_mode_locked")
	f.fail("PUT", "/my/contests/"+tc, "alice", update(c, map[string]any{"teamMode": true, "maxTeamSize": 2}), 409, "contest_team_size_locked")
	f.decode(f.request("PUT", "/my/contests/"+tc, "alice", update(c, map[string]any{"teamMode": true, "maxTeamSize": nil}), 200), &c)
	if !c.TeamMode || c.MaxTeamSize != nil {
		t.Fatalf("unlimited: %+v", c)
	}
	// A client from before the settings keeps them.
	f.decode(f.request("PUT", "/my/contests/"+tc, "alice", update(c, map[string]any{"maxTeamSize": 3}), 200), &c)
	f.decode(f.request("PUT", "/my/contests/"+tc, "alice", update(c, nil), 200), &c)
	if !c.TeamMode || c.MaxTeamSize == nil || *c.MaxTeamSize != 3 {
		t.Fatalf("legacy save: %+v", c)
	}
	individual := f.contest(ic, "alice")
	f.fail("PUT", "/my/contests/"+ic, "alice", update(individual, map[string]any{"teamMode": true}), 409, "contest_team_mode_locked")

	// Members rename their team until the end; names stay unique.
	f.join(tc, "erin", create("Beta"))
	f.fail("PUT", "/my/contests/"+tc+"/team/name", "bob", map[string]any{"name": "ｂｅｔａ"}, 409, "team_name_taken")
	f.fail("PUT", "/my/contests/"+tc+"/team/name", "bob", map[string]any{"name": " "}, 400, "invalid_team_name")
	f.fail("PUT", "/my/contests/"+tc+"/team/name", "judy", map[string]any{"name": "Gamma"}, 404, "team_not_found")
	var renamed contests.Team
	f.decode(f.request("PUT", "/my/contests/"+tc+"/team/name", "carol", map[string]any{"name": "TEAM ALPHA"}, 200), &renamed)
	if renamed.Name != "TEAM ALPHA" || renamed.Code != alpha.Code {
		t.Fatalf("renamed: %+v", renamed)
	}
	// A new code shuts out the old one.
	var renewed contests.Team
	f.decode(f.request("PUT", "/my/contests/"+tc+"/team/code", "bob", nil, 200), &renewed)
	if renewed.Code == alpha.Code || len(renewed.Code) != 10 {
		t.Fatalf("renewed: %+v", renewed)
	}
	f.fail("POST", path, "judy", joinCode(alpha.Code), 400, "incorrect_team_code")

	// Before the start members leave or remove others. Either renews the code, so whoever went cannot use it.
	members := "/my/contests/" + tc + "/team/members/"
	f.fail("DELETE", members+"erin", "bob", nil, 404, "team_member_not_found")
	f.fail("DELETE", members+"Not-A-Handle", "bob", nil, 404, "team_member_not_found")
	f.fail("DELETE", members+"bob", "judy", nil, 404, "team_not_found")
	f.request("DELETE", members+"carol", "bob", nil, 204)
	if gone := f.contest(tc, "carol"); gone.Participating || gone.Team != nil {
		t.Fatalf("removed member: %+v", gone)
	}
	after := f.team(tc, "bob")
	if after.Code == renewed.Code || !slices.Equal(after.Members, []string{"bob", "dan"}) {
		t.Fatalf("after removal: %+v", after)
	}
	f.request("DELETE", members+"dan", "dan", nil, 204)
	if left := f.team(tc, "bob"); left.Code == after.Code || !slices.Equal(left.Members, []string{"bob"}) {
		t.Fatalf("after leaving: %+v", left)
	}
	f.fail("POST", path, "dan", joinCode(after.Code), 400, "incorrect_team_code")
	// The last member to leave takes the team with them; the name is free again.
	f.request("DELETE", members+"bob", "bob", nil, 204)
	if n := f.count(`SELECT count(*) FROM contest_teams WHERE id=$1`, alpha.ID); n != 0 {
		t.Fatalf("empty team kept")
	}
	f.join(tc, "bob", create("Team Alpha"))
	alpha = f.team(tc, "bob")
	f.join(tc, "carol", joinCode(alpha.Code))

	// Once the contest starts nobody leaves or is removed, but names, codes and late members still change.
	f.schedule(tc, "running")
	f.fail("DELETE", members+"carol", "bob", nil, 409, "team_locked")
	f.fail("DELETE", members+"carol", "carol", nil, 409, "team_locked")
	f.request("PUT", "/my/contests/"+tc+"/team/name", "carol", map[string]any{"name": "Alpha"}, 200)
	alpha = contests.Team{}
	f.decode(f.request("PUT", "/my/contests/"+tc+"/team/code", "carol", nil, 200), &alpha)
	f.join(tc, "dan", joinCode(alpha.Code))
	c = f.contest(tc, "alice")
	f.fail("PUT", "/my/contests/"+tc, "alice", update(c, map[string]any{"maxTeamSize": 4}), 409, "contest_conflict")

	// After the end members change nothing, and nobody new joins.
	f.schedule(tc, "ended")
	f.fail("PUT", "/my/contests/"+tc+"/team/name", "bob", map[string]any{"name": "Late"}, 409, "team_locked")
	f.fail("PUT", "/my/contests/"+tc+"/team/code", "bob", nil, 409, "team_locked")
	f.fail("POST", path, "judy", create("Late"), 409, "contest_participation_unavailable")
	f.join(tc, "bob", nil)
	f.join(tc, "bob", joinCode(alpha.Code))
	f.team(tc, "bob")

	// Setters list the teams and rename them until the results are final; testers and members cannot.
	// Oldest first: Alpha was formed again after Beta.
	list := f.entrants(tc)
	if len(list) != 2 || list[0].Team == nil || list[0].Team.Name != "Beta" || list[1].Team.Name != "Alpha" || list[1].Team.ID != alpha.ID ||
		!slices.Equal(list[1].Team.Members, []string{"bob", "carol", "dan"}) || list[1].Handle != "" || list[1].Onsite || list[1].OnsiteAt != nil || list[1].OnsiteVia != "" {
		t.Fatalf("entrants: %+v %+v", list[0].Team, list[1].Team)
	}
	f.request("GET", "/my/contests/"+tc+"/participants", "cos", nil, 200)
	for _, other := range []string{"tess", "bob"} {
		f.fail("GET", "/my/contests/"+tc+"/participants", other, nil, 404, "contest_not_found")
		f.fail("PUT", "/my/contests/"+tc+"/teams/"+alpha.ID+"/name", other, map[string]any{"name": "Hijack"}, 404, "contest_not_found")
	}
	// Without a freeze the results are final at the end.
	f.fail("PUT", "/my/contests/"+tc+"/teams/"+alpha.ID+"/name", "alice", map[string]any{"name": "Fixed"}, 409, "contest_conflict")
	// With one, setters fix names until the reveal.
	f.exec(`UPDATE contests SET freeze_minutes=10 WHERE id=$1`, tc)
	f.fail("PUT", "/my/contests/"+tc+"/teams/"+alpha.ID+"/name", "alice", map[string]any{"name": "beta"}, 409, "team_name_taken")
	f.fail("PUT", "/my/contests/"+tc+"/teams/"+pa+"/name", "alice", map[string]any{"name": "Nobody"}, 404, "team_not_found")
	f.fail("PUT", "/my/contests/"+tc+"/teams/"+alpha.ID+"/onsite", "alice", map[string]any{"onsite": true}, 409, "contest_onsite_unavailable")
	f.fail("PUT", "/my/contests/"+tc+"/participants/bob/onsite", "alice", map[string]any{"onsite": true}, 404, "participant_not_found")
	var fixed contests.Entrant
	f.decode(f.request("PUT", "/my/contests/"+tc+"/teams/"+alpha.ID+"/name", "cos", map[string]any{"name": " Fixed "}, 200), &fixed)
	if fixed.Team == nil || fixed.Team.Name != "Fixed" || fixed.Team.ID != alpha.ID {
		t.Fatalf("setter rename: %+v", fixed)
	}
	f.exec(`UPDATE contests SET revealed_at=now() WHERE id=$1`, tc)
	f.fail("PUT", "/my/contests/"+tc+"/teams/"+alpha.ID+"/name", "alice", map[string]any{"name": "Final"}, 409, "contest_conflict")
	if member := f.contest(tc, "dan"); member.Team == nil || member.Team.Name != "Fixed" {
		t.Fatalf("member after rename: %+v", member.Team)
	}
}

func TestContestTeamDraftsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	const (
		did = "d0000000-0000-4000-8000-000000000001"
		pa  = "a0000000-0000-4000-8000-000000000001"
	)
	f.problem(pa)
	start := time.Now().Add(time.Hour)
	body := func(version int64, extra map[string]any) map[string]any {
		d := map[string]any{"title": "Draft", "startsAt": start, "durationMinutes": 60, "penaltyMinutes": 5, "problems": []map[string]any{{"id": pa, "points": 100}}}
		for k, v := range extra {
			d[k] = v
		}
		return map[string]any{"version": version, "draft": d}
	}
	var saved contests.DraftResult
	f.decode(f.request("PUT", "/my/contests/"+did+"/draft", "alice", body(0, nil), 200), &saved)
	if saved.Draft.TeamMode == nil || *saved.Draft.TeamMode || saved.Draft.OnsiteDivision == nil || *saved.Draft.OnsiteDivision || len(saved.Issues) != 0 {
		t.Fatalf("legacy draft: %+v", saved)
	}
	// A draft keeps what it is given and reports it.
	f.decode(f.request("PUT", "/my/contests/"+did+"/draft", "alice", body(saved.Version, map[string]any{"teamMode": true, "maxTeamSize": 1, "onsiteDivision": true, "onsitePassphrase": "ab"}), 200), &saved)
	if !slices.Equal(saved.Issues, []string{contests.IssueTeamSize, contests.IssuePassphrase}) || *saved.Draft.MaxTeamSize.Value != 1 || *saved.Draft.OnsitePassphrase != "ab" {
		t.Fatalf("draft issues: %+v", saved)
	}
	f.fail("PUT", "/my/contests/"+did+"/publication", "alice", map[string]any{"version": saved.Version}, 409, "contest_not_ready")
	f.fail("PUT", "/my/contests/"+did+"/draft", "alice", body(saved.Version, map[string]any{"onsitePassphrase": strings.Repeat("x", 1001)}), 400, "invalid_contest")
	f.decode(f.request("PUT", "/my/contests/"+did+"/draft", "alice", body(saved.Version, map[string]any{"maxTeamSize": 4, "onsitePassphrase": " ｓｅｃｒｅｔ "}), 200), &saved)
	if len(saved.Issues) != 0 || !*saved.Draft.TeamMode || !*saved.Draft.OnsiteDivision {
		t.Fatalf("draft: %+v", saved)
	}
	// A client from before the settings keeps them in the draft.
	f.decode(f.request("PUT", "/my/contests/"+did+"/draft", "alice", body(saved.Version, nil), 200), &saved)
	if !*saved.Draft.TeamMode || *saved.Draft.MaxTeamSize.Value != 4 || *saved.Draft.OnsitePassphrase != " ｓｅｃｒｅｔ " {
		t.Fatalf("legacy save: %+v", saved.Draft)
	}
	f.fail("GET", "/my/contests/"+did+"/draft", "bob", nil, 404, "contest_not_found")
	var published contests.Contest
	f.decode(f.request("PUT", "/my/contests/"+did+"/publication", "alice", map[string]any{"version": saved.Version}, 200), &published)
	if !published.TeamMode || *published.MaxTeamSize != 4 || !published.OnsiteDivision {
		t.Fatalf("published: %+v", published)
	}
	var venue contests.Venue
	f.decode(f.request("GET", "/my/contests/"+did+"/onsite", "alice", nil, 200), &venue)
	if !venue.OnsiteDivision || venue.Passphrase == nil || *venue.Passphrase != "secret" {
		t.Fatalf("venue: %+v", venue)
	}
	// A draft that turns a setting off publishes without its size or passphrase.
	const other, pb = "d0000000-0000-4000-8000-000000000002", "a0000000-0000-4000-8000-000000000002"
	f.problem(pb)
	f.decode(f.request("PUT", "/my/contests/"+other+"/draft", "alice", map[string]any{"version": 0, "draft": map[string]any{"title": "Off", "startsAt": start, "durationMinutes": 60, "penaltyMinutes": 5,
		"problems": []map[string]any{{"id": pb, "points": 100}}, "teamMode": false, "maxTeamSize": 1, "onsiteDivision": false, "onsitePassphrase": "x"}}, 200), &saved)
	if len(saved.Issues) != 0 {
		t.Fatalf("issues: %v", saved.Issues)
	}
	f.decode(f.request("PUT", "/my/contests/"+other+"/publication", "alice", map[string]any{"version": saved.Version}, 200), &published)
	f.decode(f.request("GET", "/my/contests/"+other+"/onsite", "alice", nil, 200), &venue)
	if published.TeamMode || published.MaxTeamSize != nil || published.OnsiteDivision || venue.Passphrase != nil {
		t.Fatalf("published without settings: %+v %+v", published, venue)
	}
}

func TestContestOnsitePostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "cos", "tess", "bob", "carol", "dan", "erin", "frank", "gina", "hank", "ivan")
	const (
		tc = "c0000000-0000-4000-8000-000000000001"
		ic = "c0000000-0000-4000-8000-000000000002"
		pc = "c0000000-0000-4000-8000-000000000003"
		pa = "a0000000-0000-4000-8000-000000000001"
		pb = "a0000000-0000-4000-8000-000000000002"
		pp = "a0000000-0000-4000-8000-000000000003"
	)
	for _, id := range []string{pa, pb, pp} {
		f.problem(id)
	}
	// Turning the division on needs a passphrase of 4 to 64 characters.
	for _, bad := range []map[string]any{{"onsiteDivision": true}, {"onsiteDivision": true, "onsitePassphrase": " ａｂｃ "}, {"onsiteDivision": true, "onsitePassphrase": "a\u200bbcd"}} {
		bad["title"], bad["startsAt"], bad["endsAt"], bad["version"] = "Onsite", time.Now().Add(time.Hour), time.Now().Add(2*time.Hour), 0
		bad["problems"] = []map[string]any{{"id": pa, "points": 100}}
		f.fail("PUT", "/my/contests/"+tc, "alice", bad, 400, "invalid_contest")
	}
	const phrase = "Venue-2026"
	c := f.create(tc, pa, map[string]any{"teamMode": true, "onsiteDivision": true, "onsitePassphrase": "　Ｖｅｎｕｅ-2026 "})
	if !c.OnsiteDivision || !c.TeamMode || c.Onsite {
		t.Fatalf("onsite contest: %+v", c)
	}
	f.create(ic, pb, map[string]any{"onsiteDivision": true, "onsitePassphrase": "open sesame"})
	f.create(pc, pp, nil)
	f.exec(`INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,'cos'),($2,'cos')`, tc, ic)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tess'),($2,'tess')`, pa, pb)

	// Only setters read the passphrase back, normalized; testers and participants never see it.
	var venue contests.Venue
	f.decode(f.request("GET", "/my/contests/"+tc+"/onsite", "cos", nil, 200), &venue)
	if !venue.OnsiteDivision || venue.Passphrase == nil || *venue.Passphrase != phrase {
		t.Fatalf("venue: %+v", venue)
	}
	for _, other := range []string{"tess", "bob"} {
		f.fail("GET", "/my/contests/"+tc+"/onsite", other, nil, 404, "contest_not_found")
		f.fail("PUT", "/my/contests/"+tc+"/onsite", other, map[string]any{"passphrase": "stolen"}, 404, "contest_not_found")
	}
	secret := func() {
		t.Helper()
		for _, viewer := range []string{"", "alice", "tess", "bob"} {
			for _, p := range []string{"/contests/" + tc, "/contests", "/contests/" + tc + "/standings", "/my/contests/" + tc, "/my/contests"} {
				if viewer == "" && strings.HasPrefix(p, "/my/") || viewer != "" && !strings.HasPrefix(p, "/my/") {
					continue
				}
				if body := f.request("GET", p, viewer, nil, 200); strings.Contains(body, phrase) || strings.Contains(body, "Passphrase") {
					t.Fatalf("%s for %q shows the passphrase", p, viewer)
				}
			}
		}
	}
	secret()

	// A wrong passphrase creates nothing, not even an open registration.
	path := "/my/contests/" + tc + "/participation"
	withPhrase := func(body map[string]any, p string) map[string]any {
		body["onsitePassphrase"] = p
		return body
	}
	f.fail("POST", path, "erin", withPhrase(create("Gamma"), "venue-2026"), 400, "incorrect_venue_passphrase")
	if n := f.count(`SELECT count(*) FROM contest_teams`) + f.count(`SELECT count(*) FROM contest_participants`); n != 0 {
		t.Fatalf("wrong passphrase registered %d rows", n)
	}
	// Width and surrounding spaces do not matter; the whole team is onsite.
	joined := f.join(tc, "erin", withPhrase(create("Gamma"), " Ｖｅｎｕｅ-2026"))
	if !joined.Onsite || joined.Team == nil || !joined.Team.Onsite {
		t.Fatalf("onsite team: %+v %+v", joined, joined.Team)
	}
	gamma := f.team(tc, "erin")
	if !gamma.Onsite {
		t.Fatalf("team: %+v", gamma)
	}
	// Later members need no passphrase, and a wrong one keeps them out.
	f.fail("POST", path, "frank", withPhrase(joinCode(gamma.Code), "nope"), 400, "incorrect_venue_passphrase")
	if f.contest(tc, "frank").Participating {
		t.Fatal("wrong passphrase joined the team")
	}
	if member := f.join(tc, "frank", joinCode(gamma.Code)); !member.Onsite {
		t.Fatalf("member of an onsite team: %+v", member)
	}
	// A team that registered open becomes onsite when any member enters the passphrase.
	if open := f.join(tc, "bob", create("Open")); open.Onsite {
		t.Fatalf("open team: %+v", open)
	}
	f.join(tc, "carol", joinCode(f.team(tc, "bob").Code))
	f.fail("POST", path, "carol", map[string]any{"onsitePassphrase": "wrong"}, 400, "incorrect_venue_passphrase")
	if upgraded := f.join(tc, "carol", map[string]any{"onsitePassphrase": phrase}); !upgraded.Onsite || !f.contest(tc, "bob").Onsite {
		t.Fatalf("upgrade: %+v", upgraded)
	}
	// Someone without a team cannot become onsite alone, and a contest without the division refuses the passphrase.
	f.fail("POST", path, "dan", map[string]any{"onsitePassphrase": phrase}, 409, "contest_team_required")
	f.fail("POST", "/my/contests/"+pc+"/participation", "dan", map[string]any{"onsitePassphrase": phrase}, 409, "contest_onsite_unavailable")
	f.fail("PUT", "/my/contests/"+pc+"/onsite", "alice", map[string]any{"passphrase": "abcd"}, 409, "contest_onsite_unavailable")
	f.decode(f.request("GET", "/my/contests/"+pc+"/onsite", "alice", nil, 200), &venue)
	if venue.OnsiteDivision || venue.Passphrase != nil {
		t.Fatalf("no division: %+v", venue)
	}
	secret()

	// Setters see when and how each team became onsite, and switch it either way.
	list := f.entrants(tc)
	if len(list) != 2 || list[0].Team.Name != "Gamma" || !list[0].Onsite || list[0].OnsiteVia != "passphrase" || list[0].OnsiteAt == nil ||
		list[1].Team.Name != "Open" || !list[1].Onsite || list[1].OnsiteVia != "passphrase" || !list[1].OnsiteAt.After(*list[0].OnsiteAt) {
		t.Fatalf("entrants: %+v", list)
	}
	onsite := "/my/contests/" + tc + "/teams/" + gamma.ID + "/onsite"
	var e contests.Entrant
	f.decode(f.request("PUT", onsite, "alice", map[string]any{"onsite": false}, 200), &e)
	if e.Onsite || e.OnsiteAt != nil || e.OnsiteVia != "" || !e.KeptOpen || e.Team == nil || e.Team.Onsite || f.contest(tc, "frank").Onsite {
		t.Fatalf("switched open: %+v", e)
	}
	// The switch sticks: a member with the passphrase cannot undo it, and nobody joins the team with it either.
	failures := f.count(`SELECT count(*) FROM contest_attempt_failures`)
	f.fail("POST", path, "frank", map[string]any{"onsitePassphrase": phrase}, 409, "onsite_kept_open")
	f.fail("POST", path, "dan", withPhrase(joinCode(gamma.Code), phrase), 409, "onsite_kept_open")
	if f.contest(tc, "frank").Onsite || f.contest(tc, "dan").Participating || f.count(`SELECT count(*) FROM contest_attempt_failures`) != failures {
		t.Fatal("the passphrase undid the setter's switch")
	}
	f.decode(f.request("PUT", onsite, "cos", map[string]any{"onsite": true}, 200), &e)
	if !e.Onsite || e.OnsiteVia != "setter" || e.OnsiteAt == nil || e.KeptOpen {
		t.Fatalf("switched onsite: %+v", e)
	}
	// Switching again keeps the first time and route.
	var again contests.Entrant
	f.decode(f.request("PUT", onsite, "alice", map[string]any{"onsite": true}, 200), &again)
	if !again.OnsiteAt.Equal(*e.OnsiteAt) || again.OnsiteVia != "setter" {
		t.Fatalf("switched twice: %+v", again)
	}
	f.fail("PUT", onsite, "alice", map[string]any{}, 400, "invalid_request")
	f.fail("PUT", onsite, "tess", map[string]any{"onsite": false}, 404, "contest_not_found")
	f.fail("PUT", onsite, "erin", map[string]any{"onsite": false}, 404, "contest_not_found")

	// In an individual contest the division belongs to each participant.
	ipath := "/my/contests/" + ic + "/participation"
	if gina := f.join(ic, "gina", map[string]any{"onsitePassphrase": " open sesame "}); !gina.Onsite || gina.Team != nil {
		t.Fatalf("onsite participant: %+v", gina)
	}
	if hank := f.join(ic, "hank", nil); hank.Onsite || !hank.Participating {
		t.Fatalf("open participant: %+v", hank)
	}
	f.fail("POST", ipath, "hank", map[string]any{"onsitePassphrase": "Open Sesame"}, 400, "incorrect_venue_passphrase")
	if hank := f.join(ic, "hank", map[string]any{"onsitePassphrase": "ｏｐｅｎ sesame"}); !hank.Onsite {
		t.Fatalf("upgraded participant: %+v", hank)
	}
	f.fail("POST", ipath, "tess", map[string]any{"onsitePassphrase": "open sesame"}, 409, "contest_participation_unavailable")
	f.decode(f.request("PUT", "/my/contests/"+ic+"/participants/hank/onsite", "alice", map[string]any{"onsite": false}, 200), &e)
	if e.Handle != "hank" || e.Team != nil || e.Onsite || !e.KeptOpen || f.contest(ic, "hank").Onsite {
		t.Fatalf("participant switched open: %+v", e)
	}
	f.fail("POST", ipath, "hank", map[string]any{"onsitePassphrase": "open sesame"}, 409, "onsite_kept_open")
	f.decode(f.request("PUT", "/my/contests/"+ic+"/participants/hank/onsite", "alice", map[string]any{"onsite": true}, 200), &e)
	if !e.Onsite || e.OnsiteVia != "setter" || e.KeptOpen {
		t.Fatalf("participant switched onsite: %+v", e)
	}
	f.fail("PUT", "/my/contests/"+ic+"/participants/ivan/onsite", "alice", map[string]any{"onsite": true}, 404, "participant_not_found")
	f.fail("PUT", "/my/contests/"+ic+"/teams/"+gamma.ID+"/onsite", "alice", map[string]any{"onsite": true}, 404, "team_not_found")
	if list := f.entrants(ic); len(list) != 2 || list[0].Handle != "gina" || list[0].OnsiteVia != "passphrase" || list[1].Handle != "hank" || list[1].OnsiteVia != "setter" {
		t.Fatalf("individual entrants: %+v", list)
	}

	// Setters replace the passphrase; the old one stops working, and an editor holding it cannot save it back.
	before := f.contest(ic, "alice")
	f.fail("PUT", "/my/contests/"+ic+"/onsite", "alice", map[string]any{"passphrase": "abc"}, 400, "invalid_venue_passphrase")
	f.fail("PUT", "/my/contests/"+ic+"/onsite", "alice", map[string]any{"passphrase": ""}, 400, "invalid_venue_passphrase")
	f.decode(f.request("PUT", "/my/contests/"+ic+"/onsite", "cos", map[string]any{"passphrase": " New phrase "}, 200), &venue)
	if *venue.Passphrase != "New phrase" {
		t.Fatalf("rotated: %+v", venue)
	}
	f.fail("PUT", "/my/contests/"+ic, "alice", update(before, map[string]any{"onsiteDivision": true, "onsitePassphrase": "open sesame"}), 409, "contest_conflict")
	f.fail("POST", ipath, "ivan", map[string]any{"onsitePassphrase": "open sesame"}, 400, "incorrect_venue_passphrase")
	f.join(ic, "ivan", map[string]any{"onsitePassphrase": "New phrase"})
	f.request("PUT", "/my/contests/"+ic+"/participants/ivan/onsite", "alice", map[string]any{"onsite": false}, 200)

	// Dropping the division before the start makes everyone open and forgets the passphrase and the setters' switches.
	f.decode(f.request("PUT", "/my/contests/"+ic, "alice", update(f.contest(ic, "alice"), map[string]any{"onsiteDivision": false}), 200), &c)
	if c.OnsiteDivision || f.contest(ic, "gina").Onsite || f.count(`SELECT count(*) FROM contest_participants WHERE contest_id=$1 AND onsite_at IS NOT NULL`, ic) != 0 {
		t.Fatalf("division dropped: %+v", c)
	}
	f.decode(f.request("GET", "/my/contests/"+ic+"/onsite", "alice", nil, 200), &venue)
	if venue.OnsiteDivision || venue.Passphrase != nil {
		t.Fatalf("venue after drop: %+v", venue)
	}
	f.fail("POST", ipath, "gina", map[string]any{"onsitePassphrase": "New phrase"}, 409, "contest_onsite_unavailable")
	// Turning it on again starts everyone open.
	f.decode(f.request("PUT", "/my/contests/"+ic, "alice", update(c, map[string]any{"onsiteDivision": true, "onsitePassphrase": "second"}), 200), &c)
	if !c.OnsiteDivision || f.contest(ic, "gina").Onsite {
		t.Fatalf("division again: %+v", c)
	}
	if ivan := f.join(ic, "ivan", map[string]any{"onsitePassphrase": "second"}); !ivan.Onsite {
		t.Fatalf("kept open across the new division: %+v", ivan)
	}

	// While the contest runs the passphrase still changes and upgrades work; the division itself is fixed.
	f.schedule(tc, "running")
	f.decode(f.request("PUT", "/my/contests/"+tc+"/onsite", "alice", map[string]any{"passphrase": "running"}, 200), &venue)
	f.join(tc, "dan", withPhrase(create("Late"), "running"))
	f.fail("PUT", "/my/contests/"+tc, "alice", update(f.contest(tc, "alice"), map[string]any{"onsiteDivision": false}), 409, "contest_conflict")
	// After the end nobody upgrades or replaces the passphrase; setters switch until the results are final.
	f.schedule(tc, "ended")
	f.fail("POST", path, "bob", map[string]any{"onsitePassphrase": "running"}, 409, "contest_participation_unavailable")
	f.fail("PUT", "/my/contests/"+tc+"/onsite", "alice", map[string]any{"passphrase": "after"}, 409, "contest_conflict")
	f.fail("PUT", onsite, "alice", map[string]any{"onsite": false}, 409, "contest_conflict")
	f.exec(`UPDATE contests SET freeze_minutes=10 WHERE id=$1`, tc)
	f.request("PUT", onsite, "alice", map[string]any{"onsite": false}, 200)
	f.exec(`UPDATE contests SET revealed_at=now() WHERE id=$1`, tc)
	f.fail("PUT", onsite, "alice", map[string]any{"onsite": true}, 409, "contest_conflict")
	f.decode(f.request("GET", "/my/contests/"+tc+"/onsite", "alice", nil, 200), &venue)
	if *venue.Passphrase != "running" {
		t.Fatalf("venue after the end: %+v", venue)
	}
}

func TestContestJoinAttemptsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "ivan", "judy")
	const (
		tc = "c0000000-0000-4000-8000-000000000001"
		ic = "c0000000-0000-4000-8000-000000000002"
		pa = "a0000000-0000-4000-8000-000000000001"
		pb = "a0000000-0000-4000-8000-000000000002"
	)
	f.problem(pa)
	f.problem(pb)
	f.create(tc, pa, map[string]any{"teamMode": true, "onsiteDivision": true, "onsitePassphrase": "correct"})
	f.create(ic, pb, map[string]any{"onsiteDivision": true, "onsitePassphrase": "correct"})
	f.join(tc, "bob", create("Bob"))
	code := f.team(tc, "bob").Code
	path := "/my/contests/" + tc + "/participation"
	// Wrong codes and passphrases share one count per account and contest.
	for i := range 10 {
		body := joinCode("WRONGCODE" + strconv.Itoa(i))
		if i%2 == 1 {
			body = withOnsite(create("Ivan"), "wrong")
		}
		want := "incorrect_team_code"
		if i%2 == 1 {
			want = "incorrect_venue_passphrase"
		}
		f.fail("POST", path, "ivan", body, 400, want)
	}
	// The next guess is refused even when right, with the time until the oldest failure expires.
	for _, body := range []map[string]any{joinCode(code), withOnsite(create("Ivan"), "correct")} {
		w := f.do(f.newRequest("POST", path, "ivan", body))
		retry, err := strconv.Atoi(w.Header().Get("Retry-After"))
		if w.Code != 429 || !strings.Contains(w.Body.String(), "contest_attempts_limited") || err != nil || retry < 1 || retry > 600 {
			t.Fatalf("limited: %d %q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
		}
	}
	if f.contest(tc, "ivan").Participating {
		t.Fatal("a limited request registered")
	}
	// Requests without a secret are not limited, and other contests and accounts keep their own counts.
	f.join(tc, "ivan", create("Ivan"))
	f.join(ic, "ivan", map[string]any{"onsitePassphrase": "correct"})
	f.join(tc, "judy", joinCode(code))
	f.fail("POST", path, "ivan", map[string]any{"onsitePassphrase": "correct"}, 429, "contest_attempts_limited")
	// Failures count for ten minutes.
	f.exec(`UPDATE contest_attempt_failures SET failed_at=failed_at-interval '599 seconds'`)
	f.fail("POST", path, "ivan", map[string]any{"onsitePassphrase": "correct"}, 429, "contest_attempts_limited")
	f.exec(`UPDATE contest_attempt_failures SET failed_at=failed_at-interval '1 second'`)
	if onsite := f.join(tc, "ivan", map[string]any{"onsitePassphrase": "correct"}); !onsite.Onsite {
		t.Fatalf("after the window: %+v", onsite)
	}
	if n := f.count(`SELECT count(*) FROM contest_attempt_failures WHERE owner_id='ivan'`); n != 0 {
		t.Fatalf("expired failures kept: %d", n)
	}
}

func withOnsite(body map[string]any, passphrase string) map[string]any {
	body["onsitePassphrase"] = passphrase
	return body
}

// Parallel requests keep one team per person and the size limit.
func TestContestTeamRacesPostgres(t *testing.T) {
	users := []string{"alice", "bob", "carol", "dan", "erin", "frank"}
	f := newTeamTest(t, users...)
	const (
		tc = "c0000000-0000-4000-8000-000000000001"
		pa = "a0000000-0000-4000-8000-000000000001"
	)
	f.problem(pa)
	f.create(tc, pa, map[string]any{"teamMode": true, "maxTeamSize": 2})
	path := "/my/contests/" + tc + "/participation"
	parallel := func(requests []*http.Request) map[int]int {
		var wg sync.WaitGroup
		codes := make([]int, len(requests))
		for i, r := range requests {
			wg.Go(func() { codes[i] = f.do(r).Code })
		}
		wg.Wait()
		counts := map[int]int{}
		for _, code := range codes {
			counts[code]++
		}
		return counts
	}
	if got := parallel([]*http.Request{f.newRequest("POST", path, "bob", create("One")), f.newRequest("POST", path, "bob", create("Two"))}); got[200] != 1 || got[409] != 1 {
		t.Fatalf("double creation: %v", got)
	}
	if n := f.count(`SELECT count(*) FROM contest_teams`); n != 1 {
		t.Fatalf("teams: %d", n)
	}
	code := f.team(tc, "bob").Code
	requests := []*http.Request{}
	for _, user := range users[2:] {
		requests = append(requests, f.newRequest("POST", path, user, joinCode(code)))
	}
	if got := parallel(requests); got[200] != 1 || got[409] != 3 {
		t.Fatalf("joins: %v", got)
	}
	if n := f.count(`SELECT count(*) FROM contest_participants`); n != 2 {
		t.Fatalf("members: %d", n)
	}
}

// waits runs the statements in a transaction that keeps their locks while the request is served, checks that the
// request waits for that transaction, and returns the response it gives once the transaction commits.
func (f teamTest) waits(r *http.Request, statements ...string) *httptest.ResponseRecorder {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	tx, err := f.store.Pool().Begin(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement); err != nil {
			f.t.Fatal(err)
		}
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.do(r) }()
	select {
	case w := <-done:
		f.t.Fatalf("%s %s returned before the competing transaction ended: %d %s", r.Method, r.URL.Path, w.Code, w.Body.String())
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		f.t.Fatal(err)
	}
	select {
	case w := <-done:
		return w
	case <-ctx.Done():
		f.t.Fatal("request did not finish after the competing transaction ended")
	}
	return nil
}

func expect(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || !strings.Contains(w.Body.String(), `"error":"`+code+`"`) {
		t.Fatalf("got %d %s, want %d %s", w.Code, w.Body.String(), status, code)
	}
}

// Each lock a rule depends on, held by a competing transaction, makes the request wait and then see its result.
func TestContestTeamLocksPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan", "erin")
	const (
		tc   = "c0000000-0000-4000-8000-000000000001"
		pa   = "a0000000-0000-4000-8000-000000000001"
		team = "b0000000-0000-4000-8000-000000000001"
	)
	f.problem(pa)
	f.create(tc, pa, map[string]any{"teamMode": true, "maxTeamSize": 2, "onsiteDivision": true, "onsitePassphrase": "correct"})
	path := "/my/contests/" + tc + "/participation"
	// A second request of the same account waits for the first, so one person never ends up in two teams.
	w := f.waits(f.newRequest("POST", path, "bob", create("Second")),
		`SELECT pg_advisory_xact_lock(hashtextextended('bob '||'`+tc+`',709091024))`,
		`INSERT INTO contest_teams(id,contest_id,name,name_key,join_code) VALUES('`+team+`','`+tc+`','First','first','FIRSTCODE2')`,
		`INSERT INTO contest_participants(contest_id,owner_id,team_id) VALUES('`+tc+`','bob','`+team+`')`)
	expect(t, w, 409, "team_already_joined")
	// A join waits for another one into the same team, then sees it full.
	w = f.waits(f.newRequest("POST", path, "carol", joinCode("FIRSTCODE2")),
		`SELECT id FROM contest_teams WHERE id='`+team+`' FOR UPDATE`,
		`INSERT INTO contest_participants(contest_id,owner_id,team_id) VALUES('`+tc+`','dan','`+team+`')`)
	expect(t, w, 409, "team_full")
	// A member removed while entering the passphrase does not make the team onsite.
	w = f.waits(f.newRequest("POST", path, "dan", map[string]any{"onsitePassphrase": "correct"}),
		`SELECT id FROM contest_teams WHERE id='`+team+`' FOR UPDATE`,
		`DELETE FROM contest_participants WHERE owner_id='dan'`)
	expect(t, w, 409, "contest_conflict")
	if n := f.count(`SELECT count(*) FROM contest_teams WHERE onsite_at IS NOT NULL`); n != 0 {
		t.Fatal("removed member made the team onsite")
	}
	// A setter's switch to open, made while a member enters the passphrase, holds.
	w = f.waits(f.newRequest("POST", path, "bob", map[string]any{"onsitePassphrase": "correct"}),
		`UPDATE contest_teams SET kept_open=true WHERE id='`+team+`'`)
	expect(t, w, 409, "onsite_kept_open")
	// A save waits for a registration in progress, then keeps the team mode.
	c := f.contest(tc, "alice")
	f.exec(`DELETE FROM contest_participants`)
	f.exec(`DELETE FROM contest_teams`)
	w = f.waits(f.newRequest("PUT", "/my/contests/"+tc, "alice", update(c, map[string]any{"teamMode": false})),
		`SELECT id FROM contests WHERE id='`+tc+`' FOR SHARE`,
		`INSERT INTO contest_participants(contest_id,owner_id) VALUES('`+tc+`','erin')`)
	expect(t, w, 409, "contest_team_mode_locked")
	// In an individual contest the participant's row orders the passphrase against the setter's switch.
	const ic, pb = "c0000000-0000-4000-8000-000000000002", "a0000000-0000-4000-8000-000000000002"
	f.problem(pb)
	f.create(ic, pb, map[string]any{"onsiteDivision": true, "onsitePassphrase": "correct"})
	f.join(ic, "carol", nil)
	w = f.waits(f.newRequest("POST", "/my/contests/"+ic+"/participation", "carol", map[string]any{"onsitePassphrase": "correct"}),
		`UPDATE contest_participants SET kept_open=true WHERE contest_id='`+ic+`' AND owner_id='carol'`)
	expect(t, w, 409, "onsite_kept_open")
}

// ADR 0024: deleting an account before the start takes it out of the contest and its team; after the start it stays.
func TestContestTeamAccountDeletionPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob", "carol", "dan", "erin", "frank", "gina")
	const (
		tc = "c0000000-0000-4000-8000-000000000001"
		ic = "c0000000-0000-4000-8000-000000000002"
		rc = "c0000000-0000-4000-8000-000000000003"
		pa = "a0000000-0000-4000-8000-000000000001"
		pb = "a0000000-0000-4000-8000-000000000002"
		pr = "a0000000-0000-4000-8000-000000000003"
	)
	for _, id := range []string{pa, pb, pr} {
		f.problem(id)
	}
	f.create(tc, pa, map[string]any{"teamMode": true, "onsiteDivision": true, "onsitePassphrase": "correct"})
	f.create(ic, pb, nil)
	f.create(rc, pr, map[string]any{"teamMode": true})
	f.join(tc, "bob", create("Solo"))
	f.join(tc, "carol", create("Pair"))
	f.join(tc, "dan", joinCode(f.team(tc, "carol").Code))
	f.join(ic, "erin", nil)
	f.join(rc, "frank", create("Live"))
	f.join(rc, "gina", joinCode(f.team(rc, "frank").Code))
	pair, live := f.team(tc, "carol"), f.team(rc, "gina")
	f.fail("POST", "/my/contests/"+tc+"/participation", "bob", map[string]any{"onsitePassphrase": "wrong"}, 400, "incorrect_venue_passphrase")
	f.schedule(rc, "running")
	remove := func(owner string) {
		t.Helper()
		if err := (&accounts.Store{Pool: f.store.Pool()}).Delete(f.ctx, owner, owner, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	for _, owner := range []string{"bob", "dan", "erin", "frank"} {
		remove(owner)
	}
	var teams []string
	rows, err := f.store.Pool().Query(f.ctx, `SELECT t.name||':'||count(p.owner_id) FROM contest_teams t LEFT JOIN contest_participants p ON p.team_id=t.id GROUP BY t.id ORDER BY t.name`)
	if err == nil {
		teams, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(teams, []string{"Live:2", "Pair:1"}) {
		t.Fatalf("teams after deletion: %v", teams)
	}
	// The person behind a deleted account knew the code of the team it left, so the team gets a new one. A team of a
	// started contest keeps its member and its code.
	if f.team(tc, "carol").Code == pair.Code || f.team(rc, "gina").Code != live.Code {
		t.Fatal("join codes after deletion")
	}
	if n := f.count(`SELECT count(*) FROM contest_participants WHERE contest_id=$1`, ic); n != 0 {
		t.Fatalf("upcoming individual registration kept: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM contest_attempt_failures`); n != 0 {
		t.Fatalf("failures kept: %d", n)
	}
	// The started contest keeps the deleted member, under the anonymous handle.
	members := f.team(rc, "gina").Members
	if len(members) != 2 || !strings.HasPrefix(members[0], "deleted_") || members[1] != "gina" {
		t.Fatalf("members after deletion: %v", members)
	}
	f.fail("POST", "/my/contests/"+tc+"/participation", "gina", joinCode(pair.Code), 400, "incorrect_team_code")
}
