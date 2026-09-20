package httpapi

import (
	"context"
	"net/http"

	"judge/api/internal/problems"
)

func (p problemHandler) favorite(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "not_found")
		return
	}
	store, ok := p.Store.(interface {
		Favorite(context.Context, string, string, *bool) (problems.Favorite, error)
	})
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	var in struct {
		Favorited *bool `json:"favorited"`
	}
	if r.Method == http.MethodPut {
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Favorited == nil {
			authError(w, 400, "invalid_request")
			return
		}
	}
	result, err := store.Favorite(r.Context(), owner, id, in.Favorited)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
