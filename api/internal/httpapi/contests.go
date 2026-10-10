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
		offset, ok := contestOffset(w, r)
		if !ok {
			return
		}
		list, e := p.Contests.List(r.Context(), owner, offset)
		err = e
		more := len(list) > 50
		if more {
			list = list[:50]
		}
		result = offsetResponse[contests.Contest]{Items: list, HasMore: more}
	case r.Method == http.MethodPost:
		err = p.Contests.Join(r.Context(), id, owner)
		if err == nil {
			result, err = p.Contests.Get(r.Context(), id, owner)
		}
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
		err = p.Contests.Save(r.Context(), owner, id, in, contests.JudgePolicy{KnownRuntimes: submissions.RuntimeIDs(), EnabledRuntimes: p.Judging.RuntimeIDs()})
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
			offset, ok := contestOffset(w, r)
			if !ok {
				return
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
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

// contestDraft serves unpublished contests. They live in their own table, so contest routes never see them.
func (p contestHandler) contestDraft(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	policy := contests.JudgePolicy{KnownRuntimes: submissions.RuntimeIDs(), EnabledRuntimes: p.Judging.RuntimeIDs()}
	id := r.PathValue("id")
	if id == "" {
		offset, ok := contestOffset(w, r)
		if !ok {
			return
		}
		list, err := p.Contests.Drafts(r.Context(), owner, offset)
		if err != nil {
			contestError(w, err)
			return
		}
		more := len(list) > 50
		if more {
			list = list[:50]
		}
		writeAuthJSON(w, 200, offsetResponse[contests.DraftSummary]{Items: list, HasMore: more})
		return
	}
	if !problemID.MatchString(id) {
		authError(w, 404, "contest_not_found")
		return
	}
	var result any
	var err error
	switch {
	case strings.HasSuffix(r.URL.Path, "/publication"):
		var in struct {
			Version int64 `json:"version"`
		}
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Version <= 0 || in.Version > 9007199254740990 {
			authError(w, 400, "invalid_version")
			return
		}
		err = p.Contests.Publish(r.Context(), owner, id, in.Version, policy)
		if err == nil {
			result, err = p.Contests.Get(r.Context(), id, owner)
		}
	case r.Method == http.MethodPut:
		var in contests.DraftInput
		if !contentJSON(w, r, &in) {
			return
		}
		if !contests.ValidDraft(in) {
			authError(w, 400, "invalid_contest")
			return
		}
		result, err = p.Contests.SaveDraft(r.Context(), owner, id, in, policy)
	case r.Method == http.MethodDelete:
		version, e := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if e != nil || version <= 0 {
			authError(w, 400, "invalid_version")
			return
		}
		if err = p.Contests.DeleteDraft(r.Context(), owner, id, version); err == nil {
			w.WriteHeader(204)
			return
		}
	default:
		result, err = p.Contests.Draft(r.Context(), owner, id, policy)
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

func contestOffset(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("offset")
	if raw == "" {
		return 0, true
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 || offset > 1000000 {
		authError(w, 400, "invalid_request")
		return 0, false
	}
	return offset, true
}

func contestError(w http.ResponseWriter, err error) {
	switch {
	case creationQuotaError(w, err):
	case errors.Is(err, pgx.ErrNoRows):
		authError(w, 404, "contest_not_found")
	case errors.Is(err, contests.ErrParticipationUnavailable):
		authError(w, 409, "contest_participation_unavailable")
	case errors.Is(err, contests.ErrNotReady):
		authError(w, 409, "contest_not_ready")
	case errors.Is(err, contests.ErrConflict):
		authError(w, 409, "contest_conflict")
	default:
		authError(w, 503, "database_unavailable")
	}
}
