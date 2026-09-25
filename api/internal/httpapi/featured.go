package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

type featuredPageResponse = problems.FeaturedPage

func (p problemHandler) featured(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	store, ok := p.Store.(*problems.Store)
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 1000000 {
			authError(w, 400, "invalid_request")
			return
		}
	}
	page, err := store.FeaturedList(r.Context(), offset, submissions.RuntimeIDs(), p.Judging.RuntimeIDs())
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, page)
}

func (p problemHandler) featuredApplication(w http.ResponseWriter, r *http.Request, owner string) {
	store, ok := p.Store.(*problems.Store)
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	if r.Method == http.MethodGet {
		items, err := store.FeaturedApplications(r.Context(), owner)
		if err != nil {
			problemError(w, err)
			return
		}
		writeAuthJSON(w, 200, itemsResponse[problems.FeaturedApplication]{Items: items})
		return
	}
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "problem_not_found")
		return
	}
	var input struct {
		Version    int64   `json:"version"`
		Preference *string `json:"preference"`
	}
	if !contentJSON(w, r, &input) {
		return
	}
	if input.Version <= 0 || input.Preference == nil {
		authError(w, 400, "invalid_request")
		return
	}
	if err := store.ApplyFeatured(r.Context(), owner, id, input.Version, *input.Preference, submissions.RuntimeIDs(), p.Judging.RuntimeIDs()); err != nil {
		problemError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type featuredStandingsResponse = problems.FeaturedStandings

func featuredStandingsParameters() []*huma.Param {
	return append(offsetParameters(), &huma.Param{Name: "at", In: "query", Required: true, Description: "対象回のscheduledAt（RFC3339形式）。", Schema: &huma.Schema{Type: "string", Format: "date-time"}})
}

func (p problemHandler) featuredStandings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	at, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("at"))
	if err != nil || at.Year() < 1 {
		authError(w, 400, "invalid_request")
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 1000000 {
			authError(w, 400, "invalid_request")
			return
		}
	}
	store, ok := p.Store.(*problems.Store)
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	page, err := store.FeaturedStandings(r.Context(), at, offset)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, page)
}
