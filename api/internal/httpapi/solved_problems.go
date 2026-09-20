package httpapi

import (
	"context"
	"net/http"
)

func (p problemHandler) solvedProblems(w http.ResponseWriter, r *http.Request, owner string) {
	store, ok := p.Store.(interface {
		SolvedProblems(context.Context, string) ([]string, error)
	})
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	ids, err := store.SolvedProblems(r.Context(), owner)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, map[string]any{"items": ids})
}
