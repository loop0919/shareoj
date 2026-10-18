package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/submissions"
)

// submissionScope reads mine: 1 lists the viewer's own submissions, and team, on a contest's problem, those of the
// viewer's team as well (ADR 0024). Both need a viewer.
func submissionScope(w http.ResponseWriter, r *http.Request, owner string, team bool) (submissions.Scope, bool) {
	var scope submissions.Scope
	switch raw := r.URL.Query().Get("mine"); {
	case raw == "" || raw == "0":
		return submissions.ScopeAll, true
	case raw == "1":
		scope = submissions.ScopeMine
	case raw == "team" && team:
		scope = submissions.ScopeTeam
	default:
		authError(w, 400, "invalid_request")
		return "", false
	}
	if owner == "" {
		authError(w, 401, "authentication_required")
		return "", false
	}
	return scope, true
}

// onsiteScope reads scope: onsite keeps the onsite division's rows of the standings or of a contest's submission list
// (ADR 0024).
func onsiteScope(w http.ResponseWriter, r *http.Request) (bool, bool) {
	switch r.URL.Query().Get("scope") {
	case "":
		return false, true
	case "onsite":
		return true, true
	}
	authError(w, 400, "invalid_request")
	return false, false
}

func (p submissionHandler) publicProblemSubmissions(w http.ResponseWriter, r *http.Request) {
	p.problemSubmissions(w, r, "")
}

func (p submissionHandler) problemSubmissions(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Submissions == nil {
		authError(w, 503, "judging_unavailable")
		return
	}
	pid, sid := r.PathValue("id"), r.PathValue("submission")
	if (pid != "" && !problemID.MatchString(pid)) || (sid != "" && !problemID.MatchString(sid)) || (pid == "" && sid == "") {
		authError(w, 404, "submission_not_found")
		return
	}
	scope, ok := submissionScope(w, r, owner, false)
	if !ok {
		return
	}
	var result any
	var err error
	if pid == "" {
		result, err = p.Submissions.VisibleGet(r.Context(), sid, owner)
	} else if sid != "" {
		result, err = p.Submissions.ProblemGet(r.Context(), pid, sid, owner)
	} else {
		offset := 0
		if raw := r.URL.Query().Get("offset"); raw != "" {
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 || offset > 1000000 {
				authError(w, 400, "invalid_request")
				return
			}
		}
		result, err = p.Submissions.ProblemList(r.Context(), pid, "", owner, scope, false, offset)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		authError(w, 404, "submission_not_found")
		return
	}
	if err != nil {
		authError(w, 503, "database_unavailable")
		return
	}
	writeAuthJSON(w, 200, result)
}
