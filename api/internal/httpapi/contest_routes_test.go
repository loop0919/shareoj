package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"judge/api/internal/contests"
	"judge/api/internal/profiles"
)

// contestRoute is a request to a contest route and the response it gets. Without contentType the request has no
// Content-Type, and without owner it is anonymous; without code only the status is checked.
type contestRoute struct {
	method, path, owner, body, contentType string
	status                                 int
	code                                   string
}

// serveContestRoutes sends each request and reports every response that differs, so one run shows all of them.
func serveContestRoutes(t *testing.T, h http.Handler, token func(owner string) string, routes []contestRoute) {
	t.Helper()
	for _, tc := range routes {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.contentType != "" {
			r.Header.Set("Content-Type", tc.contentType)
		}
		if tc.owner != "" {
			r.Header.Set("Authorization", "Bearer "+token(tc.owner))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || (tc.code != "" && !strings.Contains(w.Body.String(), `"error":"`+tc.code+`"`)) {
			t.Errorf("%s %s (%s): got %d %s, want %d %s", tc.method, tc.path, tc.owner, w.Code, strings.TrimSpace(w.Body.String()), tc.status, tc.code)
		}
	}
}

// Every contest route refuses without a database before it looks at its wildcards.
func TestContestRoutesWithoutDatabase(t *testing.T) {
	h := newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}})
	for _, route := range []string{
		"GET /contests",
		"GET /contests/bad",
		"GET /contests/bad/problems/bad",
		"GET /contests/bad/problems/bad/submissions",
		"GET /contests/bad/standings",
		"GET /contests/bad/submissions",
		"GET /contests/bad/submissions/bad",
		"GET /my/contests",
		"GET /my/contests/bad",
		"PUT /my/contests/bad",
		"POST /my/contests/bad/participation",
		"GET /my/contests/bad/clarifications",
		"POST /my/contests/bad/clarifications",
		"PUT /my/contests/bad/clarifications/bad",
		"POST /my/contests/bad/reveal",
		"GET /my/contests/bad/problems/bad",
		"GET /my/contests/bad/problems/bad/submissions",
		"GET /my/contests/bad/standings",
		"GET /my/contests/bad/submissions",
		"GET /my/contests/bad/submissions/bad",
		"GET /my/contests/drafts",
		"GET /my/contests/bad/draft",
		"PUT /my/contests/bad/draft",
		"DELETE /my/contests/bad/draft",
		"PUT /my/contests/bad/publication",
		"GET /my/contests/bad/team",
		"PUT /my/contests/bad/team/code",
		"PUT /my/contests/bad/team/name",
		"DELETE /my/contests/bad/team/members/bad",
		"GET /my/contests/bad/participants",
		"PUT /my/contests/bad/participants/bad/onsite",
		"PUT /my/contests/bad/teams/bad/onsite",
		"PUT /my/contests/bad/teams/bad/name",
	} {
		method, path, _ := strings.Cut(route, " ")
		r := httptest.NewRequest(method, path, nil)
		if strings.HasPrefix(path, "/my/") {
			r.Header.Set("Authorization", "Bearer x")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 503 || !strings.Contains(w.Body.String(), `"error":"database_unavailable"`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: got %d %q %s", route, w.Code, w.Header().Get("Cache-Control"), strings.TrimSpace(w.Body.String()))
		}
	}
}

// Each contest route checks its own wildcards and query in the order it always has; a %2F inside a wildcard value is
// that value, not another route.
func TestContestRoutesPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	const (
		p1      = "11111111-1111-4111-8111-111111111111"
		p2      = "22222222-2222-4222-8222-222222222222"
		cid     = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		cid2    = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		missing = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		team    = "ffffffff-ffff-4fff-8fff-ffffffffffff"
		J       = "application/json"
	)
	f.problem(p1)
	f.problem(p2)
	f.create(cid, p1, nil)
	token := func(owner string) string { return f.sign.token(t, owner, nil) }
	c := "/contests/" + cid
	my := "/my/contests/" + cid
	serveContestRoutes(t, f.handler, token, []contestRoute{
		{"GET", "/contests/bad", "", "", "", 404, "contest_not_found"},
		{"GET", "/contests/x%2Fy", "", "", "", 404, "contest_not_found"},
		{"GET", "/my/contests/bad/standings?scope=bad", "alice", "", "", 404, "contest_not_found"},
		{"GET", "/contests?offset=abc", "", "", "", 400, "invalid_request"},
		{"GET", "/contests?offset=1000001", "", "", "", 400, "invalid_request"},
		{"GET", "/contests?offset=1000000", "", "", "", 200, ""},
		{"GET", "/my/contests?offset=-1", "alice", "", "", 400, "invalid_request"},
		{"GET", c + "?offset=abc&scope=bad", "", "", "", 200, ""},
		{"GET", c + "/problems/bad", "", "", "", 404, "problem_not_found"},
		{"GET", c + "/problems/x%2Fstandings", "", "", "", 404, "problem_not_found"},
		{"GET", my + "/problems/" + p1 + "?offset=abc&scope=bad", "alice", "", "", 200, ""},
		{"GET", c + "/standings?offset=abc", "", "", "", 200, ""},
		{"GET", c + "/standings?scope=bad", "", "", "", 400, "invalid_request"},
		{"GET", c + "/submissions?offset=abc", "", "", "", 400, "invalid_request"},
		{"GET", c + "/submissions?scope=bad", "", "", "", 400, "invalid_request"},
		{"GET", c + "/submissions/bad", "", "", "", 404, "submission_not_found"},
		{"GET", my + "/submissions/" + missing + "?offset=abc", "alice", "", "", 404, "contest_not_found"},
		{"GET", c + "/problems/bad/submissions?offset=abc", "", "", "", 400, "invalid_request"},
		{"GET", c + "/problems/bad/submissions?mine=bad", "", "", "", 404, "problem_not_found"},
		{"GET", c + "/problems/" + p1 + "/submissions?mine=1&scope=bad", "", "", "", 401, "authentication_required"},
		{"GET", c + "/problems/" + p1 + "/submissions?mine=team", "", "", "", 401, "authentication_required"},
		{"PUT", my, "alice", `{}`, "", 415, "json_required"},
		{"PUT", my, "alice", `{`, J, 400, "invalid_request"},
		{"PUT", my, "alice", `{}`, J, 400, "invalid_contest"},
		{"POST", my + "/participation", "bob", `{"team":{"action":"create","name":""}}`, J, 400, "invalid_team_name"},
		{"POST", my + "/participation", "bob", `{"team":{"action":"x"}}`, J, 400, "invalid_request"},
		{"POST", my + "/participation", "bob", `{"x":"` + strings.Repeat("a", 9216) + `"}`, J, 413, "request_too_large"},
		{"POST", my + "/participation", "bob", `{"x":1}`, "", 415, "json_required"},
		{"POST", my + "/participation", "bob", "", "", 200, ""},
		{"POST", my + "/reveal", "alice", "", "", 409, "contest_conflict"},
		{"POST", my + "/reveal", "bob", "", "", 404, "contest_not_found"},
		{"GET", "/my/contests/drafts?offset=abc", "alice", "", "", 400, "invalid_request"},
		{"GET", "/my/contests/bad/draft", "alice", "", "", 404, "contest_not_found"},
		{"PUT", my + "/draft", "alice", `{}`, "", 415, "json_required"},
		{"PUT", my + "/publication", "alice", `{"version":0}`, J, 400, "invalid_version"},
		{"PUT", my + "/publication", "alice", `{"version":9007199254740991}`, J, 400, "invalid_version"},
		{"PUT", my + "/publication", "alice", `{"version":1}`, "", 415, "json_required"},
		{"DELETE", my + "/draft?version=abc", "alice", "", "", 400, "invalid_version"},
		{"DELETE", my + "/draft?version=0", "alice", "", "", 400, "invalid_version"},
		{"GET", "/my/contests/bad/team", "alice", "", "", 404, "team_not_found"},
		{"DELETE", "/my/contests/bad/team/members/Bad-Handle", "alice", "", "", 404, "team_not_found"},
		{"DELETE", my + "/team/members/Bad-Handle", "alice", "", "", 404, "team_member_not_found"},
		{"DELETE", my + "/team/members/x%2Fy", "alice", "", "", 404, "team_member_not_found"},
		{"PUT", my + "/team/name", "alice", `{"name":"x"}`, "", 415, "json_required"},
		{"PUT", my + "/team/name", "alice", `{"name":" "}`, J, 400, "invalid_team_name"},
		{"PUT", "/my/contests/bad/participants/Bad-Handle/onsite", "alice", `{"onsite":true}`, J, 404, "contest_not_found"},
		{"PUT", my + "/participants/Bad-Handle/onsite", "alice", `{"onsite":true}`, J, 404, "participant_not_found"},
		{"PUT", my + "/participants/x%2Fy/onsite", "alice", `{"onsite":true}`, J, 404, "participant_not_found"},
		{"PUT", my + "/participants/bob/onsite", "alice", `{}`, J, 400, "invalid_request"},
		{"PUT", my + "/teams/bad/onsite", "alice", `{"onsite":true}`, J, 404, "team_not_found"},
		{"PUT", my + "/teams/bad/name", "alice", `{"name":"x"}`, J, 404, "team_not_found"},
		{"PUT", my + "/teams/" + team + "/name", "alice", `{"name":" "}`, J, 400, "invalid_team_name"},
		// New: a %2F inside a wildcard value no longer reroutes the request to another route.
		{"GET", c + "/submissions/x%2Fstandings", "", "", "", 404, "submission_not_found"},
		{"GET", my + "/submissions/x%2Fstandings?live=1", "alice", "", "", 404, "submission_not_found"},
		{"GET", c + "/problems/x%2Fsubmissions?offset=abc", "", "", "", 404, "problem_not_found"},
	})

	// Saving without a penalty uses five minutes.
	start := time.Now().Add(time.Hour)
	body, err := json.Marshal(map[string]any{"title": "Teams", "description": "", "startsAt": start, "endsAt": start.Add(time.Hour), "version": 0, "problems": []map[string]any{{"id": p2, "points": 100}}})
	if err != nil {
		t.Fatal(err)
	}
	var saved contests.Contest
	f.decode(f.request("PUT", "/my/contests/"+cid2, "alice", json.RawMessage(body), 200), &saved)
	if saved.PenaltyMinutes != 5 {
		t.Errorf("default penalty: %d", saved.PenaltyMinutes)
	}

	// Without the submission store, only the submission routes refuse, and still after the contest id.
	pool := f.store.Pool()
	judgeless := newHandler(AuthConfig{}, handlerDependencies{Store: f.store, Contests: &contests.Store{Pool: pool}, Profiles: profiles.New(pool), Verifier: newCognitoVerifier(f.sign.server.URL, "client")})
	serveContestRoutes(t, judgeless, token, []contestRoute{
		{"GET", "/contests/bad/submissions", "", "", "", 404, "contest_not_found"},
		{"GET", c + "/submissions/bad", "", "", "", 503, "judging_unavailable"},
		{"GET", c + "/submissions?offset=abc", "", "", "", 503, "judging_unavailable"},
		{"GET", c + "/problems/bad/submissions?offset=abc", "", "", "", 503, "judging_unavailable"},
		{"GET", c + "/standings", "", "", "", 200, ""},
		{"GET", my + "/problems/" + p1, "alice", "", "", 200, ""},
		// New: the %2F stays inside the wildcard value.
		{"GET", c + "/submissions/x%2Fstandings", "", "", "", 503, "judging_unavailable"},
		{"GET", c + "/problems/x%2Fsubmissions", "", "", "", 404, "problem_not_found"},
	})
}
