package httpapi

import (
	"crypto/rand"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/submissions"
)

func (p submissionHandler) submission(w http.ResponseWriter, r *http.Request, owner string) {
	if p.Submissions == nil {
		authError(w, 503, "judging_unavailable")
		return
	}
	id := r.PathValue("id")
	if r.Method == http.MethodGet {
		if id == "" {
			items, err := p.Submissions.List(r.Context(), owner)
			if err != nil {
				authError(w, 503, "database_unavailable")
				return
			}
			writeAuthJSON(w, 200, map[string]any{"items": items})
			return
		}
		if !problemID.MatchString(id) {
			authError(w, 404, "submission_not_found")
			return
		}
		item, err := p.Submissions.Get(r.Context(), owner, id)
		if errors.Is(err, pgx.ErrNoRows) {
			authError(w, 404, "submission_not_found")
			return
		}
		if err != nil {
			authError(w, 503, "database_unavailable")
			return
		}
		writeAuthJSON(w, 200, item)
		return
	}
	if err := p.Ready(); err != nil {
		submissionError(w, err)
		return
	}
	var input submissions.CreateInput
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		authError(w, 415, "json_required")
		return
	}
	if !readJSONBody(w, r, &input, 400<<10) {
		return
	}
	item, err := p.Create(r.Context(), owner, newSubmissionID(), input)
	if err != nil {
		submissionError(w, err)
		return
	}
	writeAuthJSON(w, 202, item)
}

func submissionError(w http.ResponseWriter, err error) {
	var limited *submissions.RateLimitError
	switch {
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(limited.RetryAfter))
		authError(w, 429, "submission_rate_limited")
	case errors.Is(err, submissions.ErrMaintenance):
		authError(w, 503, "judge_maintenance")
	case errors.Is(err, submissions.ErrUnavailable):
		authError(w, 503, "judging_unavailable")
	case errors.Is(err, submissions.ErrInvalid):
		authError(w, 400, "invalid_submission")
	case errors.Is(err, submissions.ErrNotReady):
		authError(w, 409, "tests_not_ready")
	default:
		problemError(w, err)
	}
}

func (p submissionHandler) runtimes(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeAuthJSON(w, 200, runtimesResponse{Items: p.AvailableRuntimes(), Maintenance: p.Maintenance()})
}

func newSubmissionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
