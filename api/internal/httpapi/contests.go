package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

func (p contestHandler) publicContest(w http.ResponseWriter, r *http.Request) { p.contest(w, r, "") }

func (p contestHandler) contest(w http.ResponseWriter, r *http.Request, owner string) {
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
		offset, ok := contestOffset(w, r)
		if !ok {
			return
		}
		list, e := p.Contests.List(r.Context(), owner, offset)
		err = e
		more := len(list) > 50
		if more {
			list = list[:50]
		}
		result = offsetResponse[contests.Contest]{Items: list, HasMore: more}
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reveal"):
		err = p.Contests.Reveal(r.Context(), id, owner, p.Operators[owner])
		if err == nil {
			result, err = p.get(r, id, owner)
		}
	case r.Method == http.MethodPost:
		var in contests.JoinInput
		if !optionalJSON(w, r, &in) {
			return
		}
		if err = contests.ValidJoin(in); err != nil {
			joinInputError(w, err)
			return
		}
		err = p.Contests.Join(r.Context(), id, owner, in)
		if err == nil {
			result, err = p.Contests.Get(r.Context(), id, owner)
		}
	case r.Method == http.MethodPut:
		var in contests.Input
		if !contentJSON(w, r, &in) {
			return
		}
		if in.PenaltyMinutes == nil {
			value := 5
			in.PenaltyMinutes = &value
		}
		if !contests.ValidInput(in) {
			authError(w, 400, "invalid_contest")
			return
		}
		err = p.Contests.Save(r.Context(), owner, id, in, contests.JudgePolicy{KnownRuntimes: submissions.RuntimeIDs(), EnabledRuntimes: p.Judging.RuntimeIDs()})
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
		// The public route always serves the frozen table; staff ask for the live one through /my (ADR 0023).
		result, err = p.Contests.Standings(r.Context(), id, owner, owner != "" && r.URL.Query().Get("live") == "1")
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
			offset, ok := contestOffset(w, r)
			if !ok {
				return
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
		result, err = p.get(r, id, owner)
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

// get adds what only the handler knows: operators may also reveal results.
func (p contestHandler) get(r *http.Request, id, viewer string) (contests.Contest, error) {
	c, err := p.Contests.Get(r.Context(), id, viewer)
	if err == nil && p.Operators[viewer] && c.Revealable() {
		c.CanRevealResults = true
	}
	return c, err
}

// contestDraft serves unpublished contests. They live in their own table, so contest routes never see them.
func (p contestHandler) contestDraft(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	policy := contests.JudgePolicy{KnownRuntimes: submissions.RuntimeIDs(), EnabledRuntimes: p.Judging.RuntimeIDs()}
	id := r.PathValue("id")
	if id == "" {
		offset, ok := contestOffset(w, r)
		if !ok {
			return
		}
		list, err := p.Contests.Drafts(r.Context(), owner, offset)
		if err != nil {
			contestError(w, err)
			return
		}
		more := len(list) > 50
		if more {
			list = list[:50]
		}
		writeAuthJSON(w, 200, offsetResponse[contests.DraftSummary]{Items: list, HasMore: more})
		return
	}
	if !problemID.MatchString(id) {
		authError(w, 404, "contest_not_found")
		return
	}
	var result any
	var err error
	switch {
	case strings.HasSuffix(r.URL.Path, "/publication"):
		var in struct {
			Version int64 `json:"version"`
		}
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Version <= 0 || in.Version > 9007199254740990 {
			authError(w, 400, "invalid_version")
			return
		}
		err = p.Contests.Publish(r.Context(), owner, id, in.Version, policy)
		if err == nil {
			result, err = p.Contests.Get(r.Context(), id, owner)
		}
	case r.Method == http.MethodPut:
		var in contests.DraftInput
		if !contentJSON(w, r, &in) {
			return
		}
		if !contests.ValidDraft(in) {
			authError(w, 400, "invalid_contest")
			return
		}
		result, err = p.Contests.SaveDraft(r.Context(), owner, id, in, policy)
	case r.Method == http.MethodDelete:
		version, e := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if e != nil || version <= 0 {
			authError(w, 400, "invalid_version")
			return
		}
		if err = p.Contests.DeleteDraft(r.Context(), owner, id, version); err == nil {
			w.WriteHeader(204)
			return
		}
	default:
		result, err = p.Contests.Draft(r.Context(), owner, id, policy)
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

// setterInvitation serves co-setter links. A setter's POST returns the contest's link, creating it on first use, and
// PUT replaces it. With a token, GET previews the invitation and POST joins it.
func (p contestHandler) setterInvitation(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	var result any
	var err error
	if token := r.PathValue("token"); token != "" {
		if !testerToken.MatchString(token) {
			authError(w, 404, "contest_not_found")
			return
		}
		result, err = p.Contests.SetterInvitation(r.Context(), actor, token, r.Method == http.MethodPost)
	} else {
		id := r.PathValue("id")
		if !problemID.MatchString(id) {
			authError(w, 404, "contest_not_found")
			return
		}
		var token string
		token, err = p.Contests.SetterInvitationLink(r.Context(), actor, id, r.Method == http.MethodPut)
		result = map[string]string{"token": token}
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

// setter removes a co-setter: the owner removes anyone, and a co-setter removes themselves to leave.
func (p contestHandler) setter(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id, handle := r.PathValue("id"), r.PathValue("handle")
	if !problemID.MatchString(id) || !userHandle.MatchString(handle) {
		authError(w, 404, "setter_not_found")
		return
	}
	if err := p.Contests.RemoveSetter(r.Context(), actor, id, handle); err != nil {
		contestError(w, err)
		return
	}
	w.WriteHeader(204)
}

// optionalJSON reads a JSON body that may be empty. Clients from before the participation body send none (ADR 0024).
func optionalJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	var large *http.MaxBytesError
	switch {
	case errors.As(err, &large):
		authError(w, 413, "request_too_large")
		return false
	case err != nil:
		authError(w, 400, "invalid_request")
		return false
	case len(bytes.TrimSpace(data)) == 0:
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return contentJSON(w, r, target)
}

func joinInputError(w http.ResponseWriter, err error) {
	if errors.Is(err, contests.ErrInvalidTeamName) {
		authError(w, 400, "invalid_team_name")
		return
	}
	authError(w, 400, "invalid_request")
}

// team serves the viewer's own team: its join code, a new code, a new name, and leaving or removing a member.
func (p contestHandler) team(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "team_not_found")
		return
	}
	var result any
	var err error
	switch {
	case r.Method == http.MethodDelete:
		handle := r.PathValue("handle")
		if !userHandle.MatchString(handle) {
			authError(w, 404, "team_member_not_found")
			return
		}
		if err = p.Contests.RemoveTeamMember(r.Context(), id, actor, handle); err == nil {
			w.WriteHeader(204)
			return
		}
	case strings.HasSuffix(r.URL.Path, "/code"):
		result, err = p.Contests.RenewTeamCode(r.Context(), id, actor)
	case strings.HasSuffix(r.URL.Path, "/name"):
		name, ok := teamName(w, r)
		if !ok {
			return
		}
		result, err = p.Contests.RenameTeam(r.Context(), id, actor, name)
	default:
		result, err = p.Contests.MyTeam(r.Context(), id, actor)
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

func teamName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var in struct {
		Name string `json:"name"`
	}
	if !contentJSON(w, r, &in) {
		return "", false
	}
	if !contests.ValidTeamName(in.Name) {
		authError(w, 400, "invalid_team_name")
		return "", false
	}
	return in.Name, true
}

// venue shows setters the onsite division and its passphrase, and replaces the passphrase until the end.
func (p contestHandler) venue(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "contest_not_found")
		return
	}
	var result contests.Venue
	var err error
	if r.Method == http.MethodPut {
		var in struct {
			Passphrase string `json:"passphrase"`
		}
		if !contentJSON(w, r, &in) {
			return
		}
		if !contests.ValidPassphrase(in.Passphrase) {
			authError(w, 400, "invalid_venue_passphrase")
			return
		}
		result, err = p.Contests.RotatePassphrase(r.Context(), id, actor, in.Passphrase)
	} else {
		result, err = p.Contests.Venue(r.Context(), id, actor)
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

// entrants serves the setters' participant list and their changes: the onsite division of a participant or a team,
// and a team's name.
func (p contestHandler) entrants(w http.ResponseWriter, r *http.Request, actor string) {
	if p.Contests == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	id, handle, team := r.PathValue("id"), r.PathValue("handle"), r.PathValue("team")
	switch {
	case !problemID.MatchString(id):
		authError(w, 404, "contest_not_found")
		return
	case handle != "" && !userHandle.MatchString(handle):
		authError(w, 404, "participant_not_found")
		return
	case team != "" && !problemID.MatchString(team):
		authError(w, 404, "team_not_found")
		return
	}
	var result any
	var err error
	switch {
	case r.Method == http.MethodGet:
		var list []contests.Entrant
		list, err = p.Contests.Entrants(r.Context(), id, actor)
		result = itemsResponse[contests.Entrant]{Items: list}
	case strings.HasSuffix(r.URL.Path, "/name"):
		name, ok := teamName(w, r)
		if !ok {
			return
		}
		result, err = p.Contests.RenameEntrantTeam(r.Context(), id, actor, team, name)
	default:
		var in struct {
			Onsite *bool `json:"onsite"`
		}
		if !contentJSON(w, r, &in) {
			return
		}
		if in.Onsite == nil {
			authError(w, 400, "invalid_request")
			return
		}
		result, err = p.Contests.SetOnsite(r.Context(), id, actor, handle, team, *in.Onsite)
	}
	if err != nil {
		contestError(w, err)
		return
	}
	writeAuthJSON(w, 200, result)
}

func contestOffset(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("offset")
	if raw == "" {
		return 0, true
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 || offset > 1000000 {
		authError(w, 400, "invalid_request")
		return 0, false
	}
	return offset, true
}

func contestError(w http.ResponseWriter, err error) {
	var attempts *contests.AttemptsError
	switch {
	case creationQuotaError(w, err):
	case errors.As(err, &attempts):
		w.Header().Set("Retry-After", strconv.Itoa(attempts.RetryAfter))
		authError(w, 429, "contest_attempts_limited")
	case errors.Is(err, pgx.ErrNoRows):
		authError(w, 404, "contest_not_found")
	case errors.Is(err, contests.ErrParticipationUnavailable):
		authError(w, 409, "contest_participation_unavailable")
	case errors.Is(err, contests.ErrSetterNotFound):
		authError(w, 404, "setter_not_found")
	case errors.Is(err, contests.ErrInvitationClosed):
		authError(w, 409, "contest_invitation_closed")
	case errors.Is(err, problems.ErrParticipantLocked):
		authError(w, 409, "contest_participant_locked")
	case errors.Is(err, contests.ErrParticipantProblem):
		authError(w, 409, "contest_participant_problem")
	case errors.Is(err, contests.ErrResultsPending):
		authError(w, 409, "contest_results_pending")
	case errors.Is(err, contests.ErrTeamRequired):
		authError(w, 409, "contest_team_required")
	case errors.Is(err, contests.ErrTeamUnavailable):
		authError(w, 409, "contest_team_unavailable")
	case errors.Is(err, contests.ErrOnsiteUnavailable):
		authError(w, 409, "contest_onsite_unavailable")
	case errors.Is(err, contests.ErrIncorrectTeamCode):
		authError(w, 400, "incorrect_team_code")
	case errors.Is(err, contests.ErrIncorrectPassphrase):
		authError(w, 400, "incorrect_venue_passphrase")
	case errors.Is(err, contests.ErrTeamFull):
		authError(w, 409, "team_full")
	case errors.Is(err, contests.ErrTeamNameTaken):
		authError(w, 409, "team_name_taken")
	case errors.Is(err, contests.ErrAlreadyInTeam):
		authError(w, 409, "team_already_joined")
	case errors.Is(err, contests.ErrTeamLocked):
		authError(w, 409, "team_locked")
	case errors.Is(err, contests.ErrTeamNotFound):
		authError(w, 404, "team_not_found")
	case errors.Is(err, contests.ErrTeamMemberNotFound):
		authError(w, 404, "team_member_not_found")
	case errors.Is(err, contests.ErrParticipantNotFound):
		authError(w, 404, "participant_not_found")
	case errors.Is(err, contests.ErrTeamModeLocked):
		authError(w, 409, "contest_team_mode_locked")
	case errors.Is(err, contests.ErrTeamSizeLocked):
		authError(w, 409, "contest_team_size_locked")
	case errors.Is(err, contests.ErrNotReady):
		authError(w, 409, "contest_not_ready")
	case errors.Is(err, contests.ErrConflict):
		authError(w, 409, "contest_conflict")
	default:
		authError(w, 503, "database_unavailable")
	}
}
