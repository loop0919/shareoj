package httpapi

import (
	"net/http"
	"regexp"
)

var testerToken = regexp.MustCompile(`^[A-Z2-7]{32}$`)

func (p problemHandler) testerInvitation(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Store == nil {
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
		result, err = p.Store.TesterInvitation(r.Context(), actor, token, r.Method == http.MethodPost)
	} else {
		id := r.PathValue("id")
		if !problemID.MatchString(id) {
			authError(w, 404, "problem_not_found")
			return
		}
		var token string
		token, err = p.Store.CreateTesterInvitation(r.Context(), actor, id)
		result = map[string]string{"token": token}
	}
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
