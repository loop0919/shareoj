package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coreos/go-oidc/v3/oidc"
	"judge/api/internal/contests"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

type TokenVerifier interface {
	Verify(context.Context, string) (string, error)
}

type cognitoVerifier struct {
	verifier *oidc.IDTokenVerifier
	clientID string
}

func newCognitoVerifier(issuer, clientID string) *cognitoVerifier {
	ctx := oidc.ClientContext(context.Background(), &http.Client{Timeout: 5 * time.Second})
	keys := oidc.NewRemoteKeySet(ctx, issuer+"/.well-known/jwks.json")
	return &cognitoVerifier{oidc.NewVerifier(issuer, keys, &oidc.Config{
		SkipClientIDCheck:    true, // Cognito access tokens use client_id instead of aud.
		SupportedSigningAlgs: []string{"RS256"},
	}), clientID}
}

func (v *cognitoVerifier) Verify(ctx context.Context, raw string) (string, error) {
	token, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return "", err
	}
	var claims struct {
		Use      string `json:"token_use"`
		ClientID string `json:"client_id"`
	}
	if err := token.Claims(&claims); err != nil {
		return "", err
	}
	if claims.Use != "access" || claims.ClientID != v.clientID || token.Subject == "" {
		return "", errors.New("invalid access token")
	}
	return token.Subject, nil
}

type PrivateProblems struct {
	Contests             *contests.Store
	DispatchJudge        func(context.Context) error
	JudgeRuntime         string
	JudgeEnabledRuntimes string
	Submissions          *submissions.Store
	JudgeImage           string
	Posts                *posts.Store
	Operators            map[string]bool
	Store                problems.Repository
	Profiles             profiles.Repository
	Verifier             TokenVerifier
	Files                interface {
		Begin(context.Context, string, string, string, int64, string) (testfiles.Upload, error)
		Complete(context.Context, string, string, string) (problems.TestFile, error)
		Download(context.Context, string, string, string) (testfiles.Download, error)
	}
}

var (
	problemID      = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
	testFileDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func (p PrivateProblems) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /my/images", p.handle)
	mux.HandleFunc("POST /my/images", p.handle)
	mux.HandleFunc("GET /my/images/{id}", p.handle)
	mux.HandleFunc("DELETE /my/images/{id}", p.handle)
	mux.HandleFunc("GET /my/notifications", p.handle)
	mux.HandleFunc("POST /my/notifications/read", p.handle)
	mux.HandleFunc("POST /my/problems/{id}/tester-invitation", p.handle)
	mux.HandleFunc("GET /my/tester-invitations/{token}", p.handle)
	mux.HandleFunc("POST /my/tester-invitations/{token}", p.handle)
	mux.HandleFunc("GET /my/contests", p.handle)
	mux.HandleFunc("GET /my/contests/{id}", p.handle)
	mux.HandleFunc("PUT /my/contests/{id}", p.handle)
	mux.HandleFunc("GET /my/contests/{id}/problems/{problem}", p.handle)
	mux.HandleFunc("GET /my/contests/{id}/problems/{problem}/submissions", p.handle)
	mux.HandleFunc("GET /my/contests/{id}/submissions", p.handle)
	mux.HandleFunc("GET /my/contests/{id}/submissions/{submission}", p.handle)
	mux.HandleFunc("GET /my/solved-problems", p.handle)
	mux.HandleFunc("GET /my/difficulty-votes/{id}", p.handle)
	mux.HandleFunc("PUT /my/difficulty-votes/{id}", p.handle)
	mux.HandleFunc("DELETE /my/difficulty-votes/{id}", p.handle)
	mux.HandleFunc("GET /my/favorites/{id}", p.handle)
	mux.HandleFunc("PUT /my/favorites/{id}", p.handle)
	mux.HandleFunc("POST /my/submissions", p.handle)
	mux.HandleFunc("GET /my/submissions", p.handle)
	mux.HandleFunc("GET /my/submissions/{id}", p.handle)
	mux.HandleFunc("PUT /my/problems/{id}/publication", p.handle)
	mux.HandleFunc("GET /my/posts", p.handle)
	mux.HandleFunc("GET /my/posts/{id}", p.handle)
	mux.HandleFunc("PUT /my/posts/{id}", p.handle)
	mux.HandleFunc("DELETE /my/posts/{id}", p.handle)
	mux.HandleFunc("PUT /my/posts/{id}/publication", p.handle)
	mux.HandleFunc("GET /auth/me", p.handle)
	mux.HandleFunc("GET /my/profile", p.handle)
	mux.HandleFunc("PUT /my/profile", p.handle)
	mux.HandleFunc("GET /my/problems", p.handle)
	mux.HandleFunc("GET /my/problems/{id}", p.handle)
	mux.HandleFunc("GET /my/problems/{id}/submissions", p.handle)
	mux.HandleFunc("GET /my/problems/{id}/submissions/{submission}", p.handle)
	mux.HandleFunc("PUT /my/problems/{id}", p.handle)
	mux.HandleFunc("DELETE /my/problems/{id}", p.handle)
	mux.HandleFunc("POST /my/problems/{id}/test-files", p.handle)
	mux.HandleFunc("GET /my/problems/{id}/test-files/{file}", p.handle)
	mux.HandleFunc("POST /my/problems/{id}/test-files/{file}/complete", p.handle)
}

func (p PrivateProblems) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Verifier == nil {
		authError(w, 503, "authentication_unavailable")
		return
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 16384 {
		authError(w, 401, "authentication_required")
		return
	}
	timeout := 10 * time.Second
	if strings.Contains(r.URL.Path, "/test-files") {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	owner, err := p.Verifier.Verify(ctx, parts[1])
	if err != nil || owner == "" {
		authError(w, 401, "invalid_token")
		return
	}
	if r.URL.Path == "/auth/me" {
		writeAuthJSON(w, 200, map[string]string{"id": owner})
		return
	}
	if r.URL.Path == "/my/profile" {
		p.profile(w, r.WithContext(ctx), owner)
		return
	}
	if p.Profiles != nil {
		if _, err := p.Profiles.Get(ctx, owner); err != nil {
			if errors.Is(err, profiles.ErrNotFound) {
				authError(w, 403, "profile_required")
			} else {
				authError(w, 503, "database_unavailable")
			}
			return
		}
	}
	if strings.HasPrefix(r.URL.Path, "/my/images") {
		p.contentImage(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/notifications") {
		p.notifications(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/tester-invitations/") || strings.HasSuffix(r.URL.Path, "/tester-invitation") {
		p.testerInvitation(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/contests") {
		p.contest(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/problems/") && strings.Contains(r.URL.Path, "/submissions") {
		p.problemSubmissions(w, r.WithContext(ctx), owner)
		return
	}
	if r.URL.Path == "/my/solved-problems" {
		p.solvedProblems(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/difficulty-votes/") {
		p.difficultyVote(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/favorites/") {
		p.favorite(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/submissions") {
		p.submission(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/my/posts") {
		p.privatePost(w, r.WithContext(ctx), owner)
		return
	}
	if strings.Contains(r.URL.Path, "/test-files") {
		p.testFile(w, r.WithContext(ctx), owner)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/publication") {
		p.publishProblem(w, r.WithContext(ctx), owner)
		return
	}
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		p.list(w, r.WithContext(ctx), owner)
		return
	}
	if !problemID.MatchString(id) {
		authError(w, 404, "problem_not_found")
		return
	}
	var result problems.Problem
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
		if input.Version < 0 || input.Version > 9007199254740990 || !validDraft(input.Draft) {
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
	if err != nil {
		problemError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

func (p PrivateProblems) testFile(w http.ResponseWriter, r *http.Request, owner string) {
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

func validDraft(d problems.Draft) bool {
	if d.Difficulty != nil && (*d.Difficulty < 1 || *d.Difficulty > 10) {
		return false
	}
	if d.Checker != nil && d.Interactor != nil {
		return false
	}
	for _, code := range []*problems.Generator{d.Checker, d.Interactor} {
		if code == nil {
			continue
		}
		known := false
		for _, r := range submissions.Runtimes {
			known = known || r.ID == code.Runtime
		}
		if !known || !code.ValidJudgeProtocol() || len(code.Source) > 65536 || !utf8.ValidString(code.Source) || strings.ContainsRune(code.Source, 0) {
			return false
		}
	}
	if d.Generators != nil {
		for _, g := range []problems.Generator{d.Generators.Input, d.Generators.Output, d.Generators.Validation} {
			if g.Protocol != "" || len(g.Runtime) > 64 || len(g.Source) > 65536 || !utf8.ValidString(g.Source) || strings.ContainsRune(g.Source+g.Runtime, 0) {
				return false
			}
		}
	}

	timeMS, e1 := strconv.Atoi(d.TimeLimitMS)
	if e1 != nil || timeMS < 100 || timeMS > 5000 || timeMS%100 != 0 || len(d.TestCases) > 100 {
		return false
	}
	var total, inline int64
	names := make(map[string]bool)
	for _, c := range d.TestCases {
		name := strings.TrimSpace(c.Name)
		if utf8.RuneCountInString(c.Name) > 64 || strings.IndexFunc(c.Name, unicode.IsControl) >= 0 || (c.Name != "" && name == "") || (name != "" && names[name]) {
			return false
		}
		if name != "" {
			names[name] = true
		}
		for _, value := range []struct {
			text string
			file *problems.TestFile
		}{{c.Input, c.InputFile}, {c.Output, c.OutputFile}} {
			if value.file == nil {
				if len(value.text) > 64<<10 || !utf8.ValidString(value.text) || strings.ContainsRune(value.text, 0) {
					return false
				}
				total += int64(len(value.text))
				inline += int64(len(value.text))
				continue
			}
			if value.text != "" || !problemID.MatchString(value.file.ID) || value.file.Size <= 0 || value.file.Size > 16<<20 ||
				!testFileDigest.MatchString(value.file.SHA256) || value.file.Key != "" || value.file.Version != "" {
				return false
			}
			total += value.file.Size
		}
	}
	if total > 512<<20 || inline > 256<<10 {
		return false
	}
	memory, e2 := strconv.Atoi(d.MemoryLimitMB)
	return utf8.RuneCountInString(d.Title) <= 120 && utf8.RuneCountInString(d.Markdown) <= 100000 && utf8.RuneCountInString(d.Editorial) <= 100000 &&
		!strings.ContainsRune(d.Title+d.Markdown+d.Editorial, '\x00') && len(d.TimeLimitMS) <= 10 && len(d.MemoryLimitMB) <= 10 &&
		e1 == nil && e2 == nil && timeMS >= 100 && timeMS <= 5000 && timeMS%100 == 0 && memory >= 64 && memory <= 512
}

func (p PrivateProblems) list(w http.ResponseWriter, r *http.Request, owner string) {
	cursor, ok := contentCursor(w, r)
	if !ok {
		return
	}
	var items []problems.Summary
	var err error
	switch r.URL.Query().Get("role") {
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
	writeAuthJSON(w, 200, struct {
		Items      []problems.Summary `json:"items"`
		NextCursor string             `json:"nextCursor"`
	}{items, next})
}

func problemError(w http.ResponseWriter, err error) {
	switch {
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
