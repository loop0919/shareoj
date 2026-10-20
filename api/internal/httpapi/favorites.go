package httpapi

import "net/http"

func (p problemHandler) favorite(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "not_found")
		return
	}
	if p.Store == nil {
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
	result, err := p.Store.Favorite(r.Context(), owner, id, in.Favorited)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
