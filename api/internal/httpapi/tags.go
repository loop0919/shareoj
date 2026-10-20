package httpapi

import (
	"net/http"

	"judge/api/internal/tags"
)

// list returns the tags in public use, which the editors suggest to keep spellings consistent.
func (p tagHandler) list(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	list, err := p.Store.List(r.Context())
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, itemsResponse[tags.Tag]{Items: list})
}
