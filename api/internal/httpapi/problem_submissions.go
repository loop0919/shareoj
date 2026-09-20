package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func submissionMine(w http.ResponseWriter, r *http.Request, owner string) (bool, bool) {
	raw := r.URL.Query().Get("mine")
	if raw != "" && raw != "0" && raw != "1" {
		authError(w, 400, "invalid_request")
		return false, false
	}
	if raw == "1" && owner == "" {
		authError(w, 401, "authentication_required")
		return false, false
	}
	return raw == "1", true
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
	if !problemID.MatchString(pid) || (sid != "" && !problemID.MatchString(sid)) {
		authError(w, 404, "submission_not_found")
		return
	}
	mine, ok := submissionMine(w, r, owner)
	if !ok {
		return
	}
	var result any
	var err error
	if sid != "" {
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
		result, err = p.Submissions.ProblemList(r.Context(), pid, "", owner, mine, offset)
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
