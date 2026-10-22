package httpapi

import (
	"net/http"

	"judge/api/internal/contests"
)

func (p contestHandler) clarifications(w http.ResponseWriter, r *http.Request, actor string) {
	id, ok := p.contestID(w, r, "contest_not_found")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		offset, ok := contestOffset(w, r)
		if !ok {
			return
		}
		items, err := p.Contests.Clarifications(r.Context(), id, actor, offset)
		if err != nil {
			contestError(w, err)
			return
		}
		more := len(items) > 50
		if more {
			items = items[:50]
		}
		writeAuthJSON(w, 200, offsetResponse[contests.Clarification]{Items: items, HasMore: more})
		return
	}
	var in struct {
		ProblemID string `json:"problemId"`
		Question  string `json:"question"`
	}
	if !contentJSON(w, r, &in) {
		return
	}
	if !problemID.MatchString(in.ProblemID) || !contests.ValidClarificationText(in.Question) {
		authError(w, 400, "invalid_clarification")
		return
	}
	if err := p.Contests.AskClarification(r.Context(), id, in.ProblemID, actor, in.Question); err != nil {
		contestError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p contestHandler) updateClarification(w http.ResponseWriter, r *http.Request, actor string) {
	id, ok := p.contestID(w, r, "contest_not_found")
	if !ok {
		return
	}
	qid := r.PathValue("clarification")
	if !problemID.MatchString(qid) {
		authError(w, 404, "contest_not_found")
		return
	}
	var in struct {
		Answer  *string `json:"answer"`
		Publish bool    `json:"publish"`
		Version int64   `json:"version"`
	}
	if !contentJSON(w, r, &in) {
		return
	}
	if in.Version <= 0 || in.Version > 9007199254740990 || (in.Answer == nil && !in.Publish) || (in.Answer != nil && !contests.ValidClarificationText(*in.Answer)) {
		authError(w, 400, "invalid_clarification")
		return
	}
	if err := p.Contests.UpdateClarification(r.Context(), id, qid, actor, in.Version, in.Answer, in.Publish); err != nil {
		contestError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
