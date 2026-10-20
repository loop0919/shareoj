package httpapi

import "net/http"

func (p problemHandler) solvedProblems(w http.ResponseWriter, r *http.Request, owner string) {
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	ids, err := p.Store.SolvedProblems(r.Context(), owner)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, map[string]any{"items": ids})
}
