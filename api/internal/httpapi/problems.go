package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

func contentJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		authError(w, 415, "json_required")
		return false
	}
	return readJSONBody(w, r, target, 700<<10)
}

func contentCursor(w http.ResponseWriter, r *http.Request) (*problems.Cursor, bool) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return nil, true
	}
	if len(raw) > 512 {
		authError(w, 400, "invalid_cursor")
		return nil, false
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	var c problems.Cursor
	if err != nil || json.Unmarshal(data, &c) != nil || !problemID.MatchString(c.ID) || c.UpdatedAt.IsZero() {
		authError(w, 400, "invalid_cursor")
		return nil, false
	}
	return &c, true
}

func nextContentCursor(id string, date time.Time) string {
	data, _ := json.Marshal(problems.Cursor{ID: id, UpdatedAt: date})
	return base64.RawURLEncoding.EncodeToString(data)
}

func (p problemHandler) publicProblems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := r.PathValue("id")
	author := r.URL.Query().Get("author")
	if author != "" && !userHandle.MatchString(author) {
		authError(w, 400, "invalid_author")
		return
	}
	if id != "" && !problemID.MatchString(id) {
		authError(w, 404, "not_found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	store, ok := p.Store.(problems.Publications)
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	if id != "" {
		value, err := store.PublicGet(ctx, id)
		if err != nil {
			problemError(w, err)
			return
		}
		writeAuthJSON(w, 200, value)
		return
	}
	cursor, ok := contentCursor(w, r)
	if !ok {
		return
	}
	var list []problems.PublicProblem
	var err error
	if author == "" {
		list, err = store.PublicList(ctx, cursor)
	} else {
		filtered, ok := store.(interface {
			PublicListByHandle(context.Context, *problems.Cursor, string) ([]problems.PublicProblem, error)
		})
		if !ok {
			authError(w, 503, "database_unavailable")
			return
		}
		list, err = filtered.PublicListByHandle(ctx, cursor, author)
	}
	if err != nil {
		problemError(w, err)
		return
	}
	next := ""
	if len(list) > 50 {
		list = list[:50]
		last := list[49]
		next = nextContentCursor(last.ID, last.PublishedAt)
	}
	writeAuthJSON(w, 200, cursorResponse[problems.PublicProblem]{Items: list, NextCursor: next})
}

type publicationInput struct {
	Version int64 `json:"version"`
	Publish *bool `json:"publish"`
}

func publicationRequest(w http.ResponseWriter, r *http.Request) (publicationInput, bool) {
	var in publicationInput
	if !contentJSON(w, r, &in) {
		return in, false
	}
	if in.Version <= 0 || in.Publish == nil {
		authError(w, 400, "invalid_request")
		return in, false
	}
	return in, true
}

func (p problemHandler) publishProblem(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "problem_not_found")
		return
	}
	store, ok := p.Store.(problems.Publications)
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	in, ok := publicationRequest(w, r)
	if !ok {
		return
	}
	current, err := p.Store.Get(r.Context(), owner, id)
	if err != nil {
		problemError(w, err)
		return
	}
	if *in.Publish && !problems.Publishable(current.Draft, submissions.RuntimeIDs(), p.Judging.RuntimeIDs()) {
		authError(w, 400, "incomplete_problem")
		return
	}

	result, err := store.Publish(r.Context(), owner, id, in.Version, *in.Publish)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
