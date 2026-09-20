package httpapi

import (
	"context"
	"net/http"

	"judge/api/internal/problems"
)

func (p PrivateProblems) difficultyVote(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "problem_not_found")
		return
	}
	store, ok := p.Store.(interface {
		DifficultyVote(context.Context, string, string, *int) (problems.DifficultyVote, error)
	})
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	var in struct {
		Difficulty *int `json:"difficulty"`
	}
	if r.Method == http.MethodPut {
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Difficulty == nil || *in.Difficulty < 1 || *in.Difficulty > 10 {
			authError(w, 400, "invalid_difficulty")
			return
		}
	} else if r.Method == http.MethodDelete {
		in.Difficulty = new(int)
	}
	result, err := store.DifficultyVote(r.Context(), owner, id, in.Difficulty)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
