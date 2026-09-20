package httpapi

import (
	"net/http"
	"regexp"

	"judge/api/internal/problems"
)

var testerToken = regexp.MustCompile(`^[A-Z2-7]{32}$`)

func (p problemHandler) testerInvitation(w http.ResponseWriter, r *http.Request, actor string) {
	store, ok := p.Store.(*problems.Store)
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	var result any
	var err error
	if token := r.PathValue("token"); token != "" {
		if !testerToken.MatchString(token) {
			authError(w, 404, "problem_not_found")
			return
		}
		result, err = store.TesterInvitation(r.Context(), actor, token, r.Method == http.MethodPost)
	} else {
		id := r.PathValue("id")
		if !problemID.MatchString(id) {
			authError(w, 404, "problem_not_found")
			return
		}
		var token string
		token, err = store.CreateTesterInvitation(r.Context(), actor, id)
		result = map[string]string{"token": token}
	}
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
