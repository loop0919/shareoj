package httpapi

import "net/http"

func (p notificationHandler) notifications(w http.ResponseWriter, r *http.Request, owner string) {
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	items, err := p.Store.List(r.Context(), owner, r.Method == http.MethodPost, r.URL.Query().Get("history") == "1")
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, map[string]any{"notifications": items})
}
