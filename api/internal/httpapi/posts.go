package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"judge/api/internal/posts"
	"judge/api/internal/tags"
)

func (p postHandler) publicPosts(w http.ResponseWriter, r *http.Request) {
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
	list, err := p.Posts.PublicList(ctx, cursor, author, tag)
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

// maxPostProblems covers a whole contest in one post.
const maxPostProblems = 30

func validPostProblems(ids *[]string) bool {
	if ids == nil {
		return true
	}
	if len(*ids) > maxPostProblems {
		return false
	}
	seen := map[string]bool{}
	for _, id := range *ids {
		if !problemID.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

// editorials lists the user editorials of a public problem or an ended contest.
func (p postHandler) editorials(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "not_found")
		return
	}
	if p.Posts == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	ctx := r.Context()
	var list []posts.Editorial
	var err error
	if strings.HasPrefix(r.URL.Path, "/contests/") {
		list, err = p.Posts.ContestEditorials(ctx, id)
	} else {
		list, err = p.Posts.Editorials(ctx, id)
	}
	if err != nil {
		problemError(w, err)
		return
	}
	for i := range list {
		list[i].Operator = p.Operators[list[i].Owner]
	}
	writeAuthJSON(w, 200, itemsResponse[posts.Editorial]{Items: list})
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
			Version    int64     `json:"version"`
			Title      string    `json:"title"`
			Markdown   string    `json:"markdown"`
			ProblemIDs *[]string `json:"problemIds"`
			Tags       *[]string `json:"tags"`
		}
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Version < 0 || in.Version > 9007199254740990 || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Markdown) > 100000 || strings.ContainsRune(in.Title+in.Markdown, '\x00') || !validPostProblems(in.ProblemIDs) || (in.Tags != nil && !tags.Valid(*in.Tags)) {
			authError(w, 400, "invalid_post")
			return
		}
		var problemIDs, tagList []string
		if in.ProblemIDs != nil {
			problemIDs = append([]string{}, *in.ProblemIDs...)
		}
		if in.Tags != nil {
			tagList = append([]string{}, *in.Tags...)
		}
		result, err := p.Posts.Save(r.Context(), owner, id, in.Title, in.Markdown, problemIDs, tagList, in.Version)
		if errors.Is(err, posts.ErrUnavailableProblem) {
			authError(w, 400, "editorial_problem_unavailable")
			return
		}
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
