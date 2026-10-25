package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"judge/api/internal/submissions"
)

// codeRuns accepts a run without a problem (POST) and returns the owner's newest one (GET latest).
func (p submissionHandler) codeRuns(w http.ResponseWriter, r *http.Request, owner string) {
	if p.Submissions == nil {
		authError(w, 503, "judging_unavailable")
		return
	}
	if r.Method == http.MethodGet {
		item, err := p.Submissions.LatestCodeRun(r.Context(), owner)
		switch {
		case errors.Is(err, submissions.ErrCodeRunNotFound):
			authError(w, 404, "code_run_not_found")
		case err != nil:
			authError(w, 503, "database_unavailable")
		default:
			writeAuthJSON(w, 200, item)
		}
		return
	}
	var input submissions.CodeRunInput
	if !readJSONBody(w, r, &input, 3<<20) {
		return
	}
	item, err := p.CreateCodeRun(r.Context(), owner, newSubmissionID(), input)
	var limited *submissions.RateLimitError
	switch {
	case err == nil:
		writeAuthJSON(w, 202, item)
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(limited.RetryAfter))
		authError(w, 429, "code_run_rate_limited")
	case errors.Is(err, submissions.ErrInvalid):
		authError(w, 400, "invalid_code_run")
	case errors.Is(err, submissions.ErrMaintenance):
		authError(w, 503, "judge_maintenance")
	case errors.Is(err, submissions.ErrUnavailable):
		authError(w, 503, "judging_unavailable")
	default:
		authError(w, 503, "database_unavailable")
	}
}
