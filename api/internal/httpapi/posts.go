package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"judge/api/internal/posts"
)

func (p postHandler) publicPosts(w http.ResponseWriter, r *http.Request) {
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

	if p.Posts == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	if id != "" {
		post, err := p.Posts.PublicGet(ctx, id)
		if err != nil {
			problemError(w, err)
			return
		}
		post.Operator = p.Operators[post.Owner]
		writeAuthJSON(w, 200, post)
		return
	}
	cursor, ok := contentCursor(w, r)
	if !ok {
		return
	}
	var list []posts.Post
	var err error
	if author == "" {
		list, err = p.Posts.PublicList(ctx, cursor)
	} else {
		list, err = p.Posts.PublicListByHandle(ctx, cursor, author)
	}
	if err != nil {
		problemError(w, err)
		return
	}
	next := ""
	if len(list) > 50 {
		list = list[:50]
		last := list[49]
		next = nextContentCursor(last.ID, *last.PublishedAt)
	}
	for i := range list {
		list[i].Operator = p.Operators[list[i].Owner]
	}
	writeAuthJSON(w, 200, cursorResponse[posts.Post]{Items: list, NextCursor: next})
}

func (p postHandler) privatePost(w http.ResponseWriter, r *http.Request, owner string) {
	if p.Posts == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		cursor, ok := contentCursor(w, r)
		if !ok {
			return
		}
		list, err := p.Posts.List(r.Context(), owner, cursor)
		if err != nil {
			problemError(w, err)
			return
		}
		next := ""
		if len(list) > 50 {
			list = list[:50]
			last := list[49]
			next = nextContentCursor(last.ID, last.UpdatedAt)
		}
		writeAuthJSON(w, 200, map[string]any{"items": list, "nextCursor": next})
		return
	}
	if !problemID.MatchString(id) {
		authError(w, 404, "post_not_found")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/publication") {
		in, ok := publicationRequest(w, r)
		if !ok {
			return
		}
		current, err := p.Posts.Get(r.Context(), owner, id)
		if err != nil {
			problemError(w, err)
			return
		}
		if *in.Publish && (strings.TrimSpace(current.Title) == "" || strings.TrimSpace(current.Markdown) == "") {
			authError(w, 400, "incomplete_post")
			return
		}
		result, err := p.Posts.Publish(r.Context(), owner, id, in.Version, *in.Publish)
		if err != nil {
			problemError(w, err)
			return
		}
		writeAuthJSON(w, 200, result)
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := p.Posts.Get(r.Context(), owner, id)
		if err != nil {
			problemError(w, err)
			return
		}
		writeAuthJSON(w, 200, result)
	case http.MethodPut:
		var in struct {
			Version  int64  `json:"version"`
			Title    string `json:"title"`
			Markdown string `json:"markdown"`
		}
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Version < 0 || in.Version > 9007199254740990 || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Markdown) > 100000 || strings.ContainsRune(in.Title+in.Markdown, '\x00') {
			authError(w, 400, "invalid_post")
			return
		}
		result, err := p.Posts.Save(r.Context(), owner, id, in.Title, in.Markdown, in.Version)
		if err != nil {
			problemError(w, err)
			return
		}
		writeAuthJSON(w, 200, result)
	case http.MethodDelete:
		version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if err != nil || version <= 0 {
			authError(w, 400, "invalid_version")
			return
		}
		if err = p.Posts.Delete(r.Context(), owner, id, version); err != nil {
			problemError(w, err)
			return
		}
		w.WriteHeader(204)
	}
}
