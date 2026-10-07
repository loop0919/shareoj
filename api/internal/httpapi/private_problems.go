package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"judge/api/internal/database"
	"judge/api/internal/problems"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

var problemID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

// problemReadiness is implemented by the database store; test fakes may omit it.
type problemReadiness interface {
	ApplicationCount(context.Context, string) (int, error)
	State(context.Context, string, string) (problems.ProblemState, error)
}

// withReadiness tells the author where the saved problem can go; testers receive none.
func (p problemHandler) withReadiness(ctx context.Context, owner string, result *problems.Problem) error {
	store, ok := p.Store.(problemReadiness)
	if !ok {
		return nil
	}
	state, err := store.State(ctx, owner, result.ID)
	if errors.Is(err, problems.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	readiness := problems.Assess(result.Draft, state, submissions.RuntimeIDs(), p.Judging.RuntimeIDs())
	result.Readiness = &readiness
	result.FeaturedPreference, result.ContestScheduled, result.EverPublished = state.FeaturedPreference, state.ContestScheduled, state.EverPublished
	return nil
}

func (p problemHandler) problem(w http.ResponseWriter, r *http.Request, owner string) {
	ctx := r.Context()
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		p.list(w, r, owner)
		return
	}
	if !problemID.MatchString(id) {
		authError(w, 404, "problem_not_found")
		return
	}
	var result problems.Problem
	var err error
	switch r.Method {
	case http.MethodGet:
		result, err = p.Store.Get(ctx, owner, id)
	case http.MethodPut:
		var input struct {
			Version int64          `json:"version"`
			Draft   problems.Draft `json:"draft"`
		}
		media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || media != "application/json" {
			authError(w, 415, "json_required")
			return
		}
		if !readJSONBody(w, r, &input, 3<<20) {
			return
		}
		if input.Version < 0 || input.Version > 9007199254740990 || !problems.ValidDraft(input.Draft, submissions.RuntimeIDs()) {
			authError(w, 400, "invalid_draft")
			return
		}
		result, err = p.Store.Save(ctx, owner, id, input.Version, input.Draft)
	case http.MethodDelete:
		version, parseErr := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if parseErr != nil || version <= 0 {
			authError(w, 400, "invalid_version")
			return
		}
		err = p.Store.Delete(ctx, owner, id, version)
		if err == nil {
			w.WriteHeader(204)
			return
		}
	}
	if err == nil {
		err = p.withReadiness(ctx, owner, &result)
	}
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

func (p problemHandler) testFile(w http.ResponseWriter, r *http.Request, owner string) {
	id, fileID := r.PathValue("id"), r.PathValue("file")
	if !problemID.MatchString(id) || (fileID != "" && !problemID.MatchString(fileID)) {
		authError(w, 404, "test_file_not_found")
		return
	}
	if p.Files == nil {
		authError(w, 503, "test_file_storage_unavailable")
		return
	}
	var result any
	var err error
	switch {
	case r.Method == http.MethodPost && fileID == "":
		var input struct {
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		}
		if !readJSONBody(w, r, &input, 2<<10) {
			return
		}
		if input.Size <= 0 || input.Size > testfiles.MaxSize || !testfiles.IsValidDigest(input.SHA256) {
			authError(w, 400, "invalid_test_file")
			return
		}
		result, err = p.Files.Begin(r.Context(), owner, id, newSubmissionID(), input.Size, input.SHA256)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete"):
		result, err = p.Files.Complete(r.Context(), owner, id, fileID)
	case r.Method == http.MethodGet:
		result, err = p.Files.Download(r.Context(), owner, id, fileID)
	default:
		authError(w, 405, "method_not_allowed")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, testfiles.ErrNotFound):
			authError(w, 404, "test_file_not_found")
		case errors.Is(err, testfiles.ErrInvalid):
			authError(w, 400, "invalid_test_file")
		default:
			authError(w, 503, "test_file_storage_unavailable")
		}
		return
	}
	writeAuthJSON(w, 200, result)
}

func (p problemHandler) list(w http.ResponseWriter, r *http.Request, owner string) {
	cursor, ok := contentCursor(w, r)
	if !ok {
		return
	}
	var items []problems.Summary
	var err error
	role := r.URL.Query().Get("role")
	switch role {
	case "", "author":
		items, err = p.Store.List(r.Context(), owner, cursor)
	case "tester":
		store, ok := p.Store.(*problems.Store)
		if !ok {
			authError(w, 503, "database_unavailable")
			return
		}
		items, err = store.ListTesting(r.Context(), owner, cursor)
	default:
		authError(w, 400, "invalid_request")
		return
	}
	if err != nil {
		problemError(w, err)
		return
	}
	next := ""
	if len(items) > 50 {
		items = items[:50]
		last := items[49]
		next = nextContentCursor(last.ID, last.UpdatedAt)
	}
	if store, ok := p.Store.(problemReadiness); ok && role != "tester" {
		applications, err := store.ApplicationCount(r.Context(), owner)
		if err != nil {
			problemError(w, err)
			return
		}
		known, enabled := submissions.RuntimeIDs(), p.Judging.RuntimeIDs()
		for i := range items {
			readiness := problems.Assess(items[i].Draft, problems.SummaryState(items[i], applications), known, enabled)
			items[i].Readiness = &readiness
		}
	}
	writeAuthJSON(w, 200, struct {
		Items      []problems.Summary `json:"items"`
		NextCursor string             `json:"nextCursor"`
	}{items, next})
}

func problemError(w http.ResponseWriter, err error) {
	switch {
	case creationQuotaError(w, err):
	case errors.Is(err, problems.ErrFeaturedIneligible):
		authError(w, 400, "featured_ineligible")
	case errors.Is(err, problems.ErrFeaturedLimit):
		authError(w, 409, "featured_limit")
	case errors.Is(err, problems.ErrContestLocked):
		authError(w, 409, "contest_problem_locked")
	case errors.Is(err, problems.ErrNotFound):
		authError(w, 404, "problem_not_found")
	case errors.Is(err, problems.ErrConflict):
		authError(w, 409, "version_conflict")
	case errors.Is(err, problems.ErrTestFile):
		authError(w, 400, "invalid_test_file")
	default:
		authError(w, 503, "database_unavailable")
	}
}

func creationQuotaError(w http.ResponseWriter, err error) bool {
	var limited *database.CreationQuotaError
	if !errors.As(err, &limited) {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(limited.RetryAfter))
	authError(w, http.StatusTooManyRequests, "creation_quota_exceeded")
	return true
}
