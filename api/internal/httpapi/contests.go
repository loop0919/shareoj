package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/submissions"
)

func (p contestHandler) publicContest(w http.ResponseWriter, r *http.Request) { p.contest(w, r, "") }

func (p contestHandler) contest(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if id != "" && !problemID.MatchString(id) {
		authError(w, 404, "contest_not_found")
		return
	}
	var result any
	var err error
	switch {
	case id == "":
		offset := 0
		if raw := r.URL.Query().Get("offset"); raw != "" {
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 || offset > 1000000 {
				authError(w, 400, "invalid_request")
				return
			}
		}
		list, e := p.Contests.List(r.Context(), owner, offset)
		err = e
		more := len(list) > 50
		if more {
			list = list[:50]
		}
		result = map[string]any{"items": list, "hasMore": more}
	case r.Method == http.MethodPut:
		var in contests.Input
		if !contentJSON(w, r, &in) {
			return
		}
		if in.PenaltyMinutes == nil {
			value := 5
			in.PenaltyMinutes = &value
		}
		if !contests.ValidInput(in) {
			authError(w, 400, "invalid_contest")
			return
		}
		err = p.Contests.Save(r.Context(), owner, id, in, contests.JudgePolicy{KnownRuntimes: submissions.RuntimeIDs(), EnabledRuntimes: p.Judging.RuntimeIDs(), Isolate: p.Judging.JudgeRuntime == "cpp17-isolate"})
		if err == nil {
			result, err = p.Contests.Get(r.Context(), id, owner)
		}
	case r.PathValue("problem") != "" && !strings.HasSuffix(r.URL.Path, "/submissions"):
		pid := r.PathValue("problem")
		if !problemID.MatchString(pid) {
			authError(w, 404, "problem_not_found")
			return
		}
		result, err = p.Contests.Problem(r.Context(), id, pid, owner)
	case strings.HasSuffix(r.URL.Path, "/standings"):
		result, err = p.Contests.Standings(r.Context(), id)
	case strings.Contains(r.URL.Path, "/submissions"):
		if p.Submissions == nil {
			authError(w, 503, "judging_unavailable")
			return
		}
		sid := r.PathValue("submission")
		if sid != "" {
			if !problemID.MatchString(sid) {
				authError(w, 404, "submission_not_found")
				return
			}
			result, err = p.Submissions.ContestGet(r.Context(), id, sid, owner)
		} else {
			offset := 0
			if raw := r.URL.Query().Get("offset"); raw != "" {
				offset, err = strconv.Atoi(raw)
				if err != nil || offset < 0 || offset > 1000000 {
					authError(w, 400, "invalid_request")
					return
				}
			}
			if pid := r.PathValue("problem"); pid != "" {
				if !problemID.MatchString(pid) {
					authError(w, 404, "problem_not_found")
					return
				}
				mine, ok := submissionMine(w, r, owner)
				if !ok {
					return
				}
				result, err = p.Submissions.ProblemList(r.Context(), pid, id, owner, mine, offset)
			} else {
				result, err = p.Submissions.ContestList(r.Context(), id, owner, offset)
			}
		}
	default:
		result, err = p.Contests.Get(r.Context(), id, owner)
	}
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			authError(w, 404, "contest_not_found")
		case errors.Is(err, contests.ErrConflict):
			authError(w, 409, "contest_conflict")
		default:
			authError(w, 503, "database_unavailable")
		}
		return
	}
	writeAuthJSON(w, 200, result)
}
