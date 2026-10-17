package httpapi

import (
	"mime"
	"net/http"

	"judge/api/internal/problems"
)

// rejudge reports pending rejudges of a problem or requeues its finished submissions.
// Problem authors and testers may rejudge their problems; operators may rejudge any problem.
func (p submissionHandler) rejudge(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Submissions == nil {
		authError(w, 503, "judging_unavailable")
		return
	}
	id := r.PathValue("id")
	if !problems.ValidID(id) {
		authError(w, 404, "problem_not_found")
		return
	}
	operator := p.Operators[actor]
	if r.Method == http.MethodGet {
		pending, err := p.Submissions.PendingRejudges(r.Context(), actor, id, operator)
		if err != nil {
			problemError(w, err)
			return
		}
		writeAuthJSON(w, 200, map[string]int64{"pending": pending})
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		authError(w, 415, "json_required")
		return
	}
	var input struct {
		SubmissionIDs []string `json:"submissionIds"`
	}
	if !readJSONBody(w, r, &input, 16<<10) {
		return
	}
	queued, err := p.Rejudge(r.Context(), actor, id, input.SubmissionIDs, operator)
	if err != nil {
		submissionError(w, err)
		return
	}
	writeAuthJSON(w, 202, map[string]int64{"queued": queued})
}
