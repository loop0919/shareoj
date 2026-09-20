package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"judge/api/internal/contests"
	"judge/api/internal/problems"
)

func (p PrivateProblems) publicContest(w http.ResponseWriter, r *http.Request) { p.contest(w, r, "") }

func (p PrivateProblems) contest(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if id != "" && !problemID.MatchString(id) {
		authError(w, 404, "contest_not_found")
		return
	}
	var result any
	var err error
	switch {
	case id == "":
		offset := 0
		if raw := r.URL.Query().Get("offset"); raw != "" {
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 || offset > 1000000 {
				authError(w, 400, "invalid_request")
				return
			}
		}
		list, e := p.Contests.List(r.Context(), owner, offset)
		err = e
		more := len(list) > 50
		if more {
			list = list[:50]
		}
		result = map[string]any{"items": list, "hasMore": more}
	case r.Method == http.MethodPut:
		var in contests.Input
		if !contentJSON(w, r, &in) {
			return
		}
		if in.PenaltyMinutes == nil {
			value := 5
			in.PenaltyMinutes = &value
		}
		if !validContest(in) {
			authError(w, 400, "invalid_contest")
			return
		}
		err = p.Contests.Save(r.Context(), owner, id, in, func(d problems.Draft) bool {
			if !validDraft(d) || strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Markdown) == "" || len(d.TestCases) == 0 {
				return false
			}
			if p.JudgeRuntime == "cpp17-isolate" && d.MemoryLimitMB != "512" {
				return false
			}
			for _, code := range []*problems.Generator{d.Checker, d.Interactor} {
				if code == nil {
					continue
				}
				available := false
				for _, rt := range p.availableRuntimes() {
					available = available || rt.ID == code.Runtime
				}
				if !available || strings.TrimSpace(code.Source) == "" {
					return false
				}
			}
			return true
		})
		if err == nil {
			result, err = p.Contests.Get(r.Context(), id, owner)
		}
	case r.PathValue("problem") != "" && !strings.HasSuffix(r.URL.Path, "/submissions"):
		pid := r.PathValue("problem")
		if !problemID.MatchString(pid) {
			authError(w, 404, "problem_not_found")
			return
		}
		result, err = p.Contests.Problem(r.Context(), id, pid, owner)
	case strings.HasSuffix(r.URL.Path, "/standings"):
		result, err = p.Contests.Standings(r.Context(), id)
	case strings.Contains(r.URL.Path, "/submissions"):
		if p.Submissions == nil {
			authError(w, 503, "judging_unavailable")
			return
		}
		sid := r.PathValue("submission")
		if sid != "" {
			if !problemID.MatchString(sid) {
				authError(w, 404, "submission_not_found")
				return
			}
			result, err = p.Submissions.ContestGet(r.Context(), id, sid, owner)
		} else {
			offset := 0
			if raw := r.URL.Query().Get("offset"); raw != "" {
				offset, err = strconv.Atoi(raw)
				if err != nil || offset < 0 || offset > 1000000 {
					authError(w, 400, "invalid_request")
					return
				}
			}
			if pid := r.PathValue("problem"); pid != "" {
				if !problemID.MatchString(pid) {
					authError(w, 404, "problem_not_found")
					return
				}
				mine, ok := submissionMine(w, r, owner)
				if !ok {
					return
				}
				result, err = p.Submissions.ProblemList(r.Context(), pid, id, owner, mine, offset)
			} else {
				result, err = p.Submissions.ContestList(r.Context(), id, owner, offset)
			}
		}
	default:
		result, err = p.Contests.Get(r.Context(), id, owner)
	}
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			authError(w, 404, "contest_not_found")
		case errors.Is(err, contests.ErrConflict):
			authError(w, 409, "contest_conflict")
		default:
			authError(w, 503, "database_unavailable")
		}
		return
	}
	writeAuthJSON(w, 200, result)
}

func validContest(in contests.Input) bool {
	if strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Description) > 100000 || strings.ContainsRune(in.Title+in.Description, 0) || !utf8.ValidString(in.Title+in.Description) || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) || in.EndsAt.Year() > 9999 || in.Version < 0 || in.Version > 9007199254740990 || in.PenaltyMinutes == nil || *in.PenaltyMinutes < 0 || *in.PenaltyMinutes > 1440 || len(in.Problems) == 0 || len(in.Problems) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, p := range in.Problems {
		if !problemID.MatchString(p.ID) || seen[p.ID] || p.Points < 1 || p.Points > 1000000 {
			return false
		}
		seen[p.ID] = true
	}
	return true
}
