package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
	"judge/api/internal/tags"
)

func contentJSON(w http.ResponseWriter, r *http.Request, target any) bool {
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

// catalogueFilter reads the author and tag that narrow a public list.
func catalogueFilter(w http.ResponseWriter, r *http.Request) (author, tag string, ok bool) {
	author = r.URL.Query().Get("author")
	if author != "" && !userHandle.MatchString(author) {
		authError(w, 400, "invalid_author")
		return "", "", false
	}
	tag = r.URL.Query().Get("tag")
	if tag != "" && !tags.Valid([]string{tag}) {
		authError(w, 400, "invalid_tag")
		return "", "", false
	}
	return author, tag, true
}

func nextContentCursor(id string, date time.Time) string {
	data, _ := json.Marshal(problems.Cursor{ID: id, UpdatedAt: date})
	return base64.RawURLEncoding.EncodeToString(data)
}

func (p problemHandler) publicProblems(w http.ResponseWriter, r *http.Request) {
	p.publishedProblems(w, r, "")
}

// publishedProblems also serves the author and testers, who read a featured editorial before it unlocks.
func (p problemHandler) publishedProblems(w http.ResponseWriter, r *http.Request, viewer string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := r.PathValue("id")
	author, tag, ok := catalogueFilter(w, r)
	if !ok {
		return
	}
	if id != "" && !problemID.MatchString(id) {
		authError(w, 404, "not_found")
		return
	}
	ctx := r.Context()

	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	if id != "" {
		value, err := p.Store.PublicGet(ctx, id, viewer)
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
	offset, ok := contestOffset(w, r)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	if cursor != nil && r.URL.Query().Has("offset") {
		authError(w, 400, "invalid_request")
		return
	}
	filter := problems.ListFilter{Author: author, Tag: tag}
	list, err := p.Store.PublicList(ctx, cursor, offset, limit, filter)
	if err != nil {
		problemError(w, err)
		return
	}
	next := ""
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1]
		next = nextContentCursor(last.ID, last.PublishedAt)
	}
	total, err := p.Store.PublicCount(ctx, filter)
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, countedCursorResponse[problems.PublicProblem]{Items: list, NextCursor: next, Total: total})
}

type publicationInput struct {
	ScheduledAt *time.Time `json:"scheduledAt,omitempty"`
	Version     int64      `json:"version"`
	Publish     *bool      `json:"publish"`
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
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	in, ok := publicationRequest(w, r)
	if !ok {
		return
	}
	if in.ScheduledAt != nil && (!*in.Publish || !in.ScheduledAt.After(time.Now())) {
		authError(w, 400, "invalid_schedule")
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

	var result problems.Problem
	if in.ScheduledAt != nil {
		result, err = p.Store.Schedule(r.Context(), owner, id, in.Version, *in.ScheduledAt)
	} else {
		result, err = p.Store.Publish(r.Context(), owner, id, in.Version, *in.Publish)
	}
	if err == nil {
		err = p.withReadiness(r.Context(), owner, &result)
	}
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}
