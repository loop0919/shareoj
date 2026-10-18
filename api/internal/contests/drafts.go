package contests

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/database"
	"judge/api/internal/problems"
)

// ErrNotReady means the saved draft still has issues; the draft response lists them.
var ErrNotReady = errors.New("contest draft is not ready to publish")

// Draft is a contest being written. Any field may be missing or out of range until publication.
type Draft struct {
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	StartsAt        *time.Time     `json:"startsAt"`
	DurationMinutes *int           `json:"durationMinutes"`
	PenaltyMinutes  *int           `json:"penaltyMinutes"`
	FreezeMinutes   *int           `json:"freezeMinutes"`
	Problems        []DraftProblem `json:"problems"`
}
type DraftProblem struct {
	ID     string `json:"id"`
	Points *int   `json:"points"`
}
type DraftInput struct {
	Version int64 `json:"version"`
	Draft   Draft `json:"draft"`
}
type DraftResult struct {
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
	Draft     Draft     `json:"draft"`
	// Issues lists why the saved draft cannot be published yet; empty means it can.
	Issues []string `json:"issues"`
	// Problems describes, in draft order, the listed problems that a current setter authors or tests. Problem routes
	// show a setter only their own, so the editor learns the title and contest readiness of the others here.
	Problems []ListedProblem `json:"problems"`
	// Authors are the setters, owner first. IsOwner tells whether the viewer owns the draft.
	Authors []string `json:"authors"`
	IsOwner bool     `json:"owner"`
}
type ListedProblem struct {
	ID     string           `json:"id"`
	Title  string           `json:"title"`
	Author string           `json:"author"`
	Issues []problems.Issue `json:"issues"`
}
type DraftSummary struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Author    string     `json:"author"`
	StartsAt  *time.Time `json:"startsAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

const (
	IssueTitle       = "title_missing"
	IssueStart       = "start_missing"
	IssueStartPast   = "start_past"
	IssueDuration    = "duration_invalid"
	IssuePenalty     = "penalty_invalid"
	IssueFreeze      = "freeze_invalid"
	IssueProblems    = "problems_missing"
	IssuePoints      = "points_invalid"
	IssueUnavailable = "problem_unavailable"
	IssueIncomplete  = "problem_incomplete"
	IssueParticipant = "problem_participant"
)

// About 190 years; the bound keeps the end-time arithmetic from overflowing.
const maxDurationMinutes = 100_000_000

// ValidDraft rejects only what the editor never sends; everything else is saved and reported as an issue.
func ValidDraft(in DraftInput) bool {
	d := in.Draft
	text := d.Title + d.Description
	if in.Version < 0 || in.Version > 9007199254740990 || utf8.RuneCountInString(d.Title) > 120 || utf8.RuneCountInString(d.Description) > 100000 || strings.ContainsRune(text, 0) || !utf8.ValidString(text) || len(d.Problems) > 100 {
		return false
	}
	numbers := []*int{d.DurationMinutes, d.PenaltyMinutes, d.FreezeMinutes}
	seen := map[string]bool{}
	for _, p := range d.Problems {
		if !problems.ValidID(p.ID) || seen[p.ID] {
			return false
		}
		seen[p.ID] = true
		numbers = append(numbers, p.Points)
	}
	for _, n := range numbers {
		if n != nil && (*n < -1_000_000_000 || *n > 1_000_000_000) {
			return false
		}
	}
	return true
}

func (d Draft) endsAt() (time.Time, bool) {
	if d.StartsAt == nil || d.DurationMinutes == nil || *d.DurationMinutes < 1 || *d.DurationMinutes > maxDurationMinutes {
		return time.Time{}, false
	}
	end := d.StartsAt.Add(time.Duration(*d.DurationMinutes) * time.Minute)
	return end, end.Year() <= 9999
}

func (d Draft) contentIssues(now time.Time) []string {
	issues := []string{}
	if strings.TrimSpace(d.Title) == "" {
		issues = append(issues, IssueTitle)
	}
	if d.StartsAt == nil {
		issues = append(issues, IssueStart)
	} else if !d.StartsAt.After(now) {
		issues = append(issues, IssueStartPast)
	}
	if d.DurationMinutes == nil || *d.DurationMinutes < 1 || *d.DurationMinutes > maxDurationMinutes {
		issues = append(issues, IssueDuration)
	} else if _, ok := d.endsAt(); d.StartsAt != nil && !ok {
		issues = append(issues, IssueDuration)
	}
	if d.PenaltyMinutes == nil || *d.PenaltyMinutes < 0 || *d.PenaltyMinutes > 1440 {
		issues = append(issues, IssuePenalty)
	}
	// Null means no freeze; otherwise it must fit in the contest.
	if f := d.FreezeMinutes; f != nil && (*f < 1 || d.DurationMinutes != nil && *f > *d.DurationMinutes) {
		issues = append(issues, IssueFreeze)
	}
	if len(d.Problems) == 0 {
		issues = append(issues, IssueProblems)
	}
	for _, p := range d.Problems {
		if p.Points == nil || *p.Points < 1 || *p.Points > 1000000 {
			issues = append(issues, IssuePoints)
			break
		}
	}
	return issues
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// assess mirrors what publication checks, so the editor can show why it would fail. It judges the problems by
// whether a current setter authors or tests them, never by the viewer, so every setter sees the same result.
func assess(ctx context.Context, q querier, id string, d Draft, policy JudgePolicy) ([]string, []ListedProblem, error) {
	issues := d.contentIssues(time.Now())
	listed := []ListedProblem{}
	if len(d.Problems) == 0 {
		return issues, listed, nil
	}
	ids := make([]string, len(d.Problems))
	for i, p := range d.Problems {
		ids[i] = p.ID
	}
	rows, err := q.Query(ctx, `SELECT d.id,d.draft,u.handle,d.published_draft IS NOT NULL,d.scheduled_at IS NOT NULL,
 EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.problem_id=d.id AND cp.contest_id<>$2),
 COALESCE((SELECT preference FROM featured_applications a WHERE a.problem_id=d.id),'')
 FROM problem_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id
 WHERE d.id=ANY($1::uuid[]) AND (contest_setter($2,d.owner_id)
  OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=d.id AND contest_setter($2,t.owner_id)))`, ids, id)
	if err != nil {
		return nil, nil, err
	}
	found := map[string]ListedProblem{}
	unavailable, incomplete := len(d.Problems), false
	for rows.Next() {
		var p ListedProblem
		var raw []byte
		var state problems.ProblemState
		if err = rows.Scan(&p.ID, &raw, &p.Author, &state.Published, &state.Scheduled, &state.InContest, &state.FeaturedPreference); err != nil {
			rows.Close()
			return nil, nil, err
		}
		var draft problems.Draft
		valid := json.Unmarshal(raw, &draft) == nil
		p.Title = draft.Title
		p.Issues = problems.Assess(draft, state, policy.KnownRuntimes, policy.EnabledRuntimes).Contest
		found[p.ID] = p
		if !state.Published && !state.Scheduled && !state.InContest && state.FeaturedPreference == "" {
			unavailable--
			incomplete = incomplete || !valid || !policy.validProblem(draft)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	for _, pid := range ids {
		if p, ok := found[pid]; ok {
			listed = append(listed, p)
		}
	}
	if unavailable > 0 {
		issues = append(issues, IssueUnavailable)
	}
	if incomplete {
		issues = append(issues, IssueIncomplete)
	}
	// Mirrors save. A draft has no participants yet, so this shows only for a published contest.
	joined, err := participantProblem(ctx, q, id, ids)
	if err != nil {
		return nil, nil, err
	}
	if joined {
		issues = append(issues, IssueParticipant)
	}
	return issues, listed, nil
}

const draftColumns = `id,version,updated_at,draft`

func scanDraft(row pgx.Row) (DraftResult, error) {
	var r DraftResult
	var raw []byte
	if err := row.Scan(&r.ID, &r.Version, &r.UpdatedAt, &raw); err != nil {
		return r, err
	}
	err := json.Unmarshal(raw, &r.Draft)
	if r.Draft.Problems == nil {
		r.Draft.Problems = []DraftProblem{}
	}
	return r, err
}

// describe adds what the editor shows beside the saved draft: its issues, the setters' problems and the setters.
func (s *Store) describe(ctx context.Context, actor string, r *DraftResult, policy JudgePolicy) (err error) {
	r.Issues, r.Problems, err = assess(ctx, s.Pool, r.ID, r.Draft, policy)
	if err != nil {
		return err
	}
	var owner string
	err = s.Pool.QueryRow(ctx, `SELECT u.handle,`+coSetters+`,c.owner_id=$2 FROM contest_drafts c JOIN user_profiles u ON u.owner_id=c.owner_id
 WHERE c.id=$1`, r.ID, actor).Scan(&owner, &r.Authors, &r.IsOwner)
	r.Authors = append([]string{owner}, r.Authors...)
	return err
}

func (s *Store) Draft(ctx context.Context, actor, id string, policy JudgePolicy) (DraftResult, error) {
	r, err := scanDraft(s.Pool.QueryRow(ctx, `SELECT `+draftColumns+` FROM contest_drafts WHERE id=$1 AND contest_setter(id,$2)`, id, actor))
	if err != nil {
		return r, err
	}
	return r, s.describe(ctx, actor, &r, policy)
}

func (s *Store) SaveDraft(ctx context.Context, actor, id string, in DraftInput, policy JudgePolicy) (DraftResult, error) {
	if in.Draft.Problems == nil {
		in.Draft.Problems = []DraftProblem{}
	}
	data, err := json.Marshal(in.Draft)
	if err != nil {
		return DraftResult{}, err
	}
	var r DraftResult
	if in.Version == 0 {
		r, err = s.createDraft(ctx, actor, id, in.Draft, data)
	} else {
		r, err = s.updateDraft(ctx, actor, id, in, data)
	}
	if err = s.draftConflict(ctx, actor, id, err, false); err != nil {
		return r, err
	}
	return r, s.describe(ctx, actor, &r, policy)
}

// addable refuses a problem that actor adds but neither authors nor tests. Drafts reserve nothing and may name any
// problem, so without this a setter could add a problem that only another setter manages and gain rights to it once
// the contest is published. Problems that do not exist stay allowed and are reported as unavailable.
func addable(ctx context.Context, tx pgx.Tx, actor string, added []string) error {
	if len(added) == 0 {
		return nil
	}
	var refused bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM problem_drafts d WHERE d.id=ANY($1::uuid[]) AND NOT can_manage_problem_directly(d.id,$2))`, added, actor).Scan(&refused); err != nil {
		return err
	}
	if refused {
		return ErrConflict
	}
	return nil
}

func (s *Store) createDraft(ctx context.Context, owner, id string, d Draft, data []byte) (DraftResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DraftResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	added := make([]string, len(d.Problems))
	for i, p := range d.Problems {
		added[i] = p.ID
	}
	if err = addable(ctx, tx, owner, added); err != nil {
		return DraftResult{}, err
	}
	if err = database.ConsumeCreationQuota(ctx, tx, owner, "contest"); err != nil {
		return DraftResult{}, err
	}
	if err = lockID(ctx, tx, id); err != nil {
		return DraftResult{}, err
	}
	// The id must stay free in contests too, since publication keeps it.
	r, err := scanDraft(tx.QueryRow(ctx, `INSERT INTO contest_drafts(id,owner_id,draft) SELECT $1,$2,$3
 WHERE NOT EXISTS (SELECT 1 FROM contests WHERE id=$1) ON CONFLICT(id) DO NOTHING RETURNING `+draftColumns, id, owner, data))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrConflict
	}
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

// updateDraft locks the draft so that the problems it already lists, which need no new check, stay those of in.Version.
func (s *Store) updateDraft(ctx context.Context, actor, id string, in DraftInput, data []byte) (DraftResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DraftResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanDraft(tx.QueryRow(ctx, `SELECT `+draftColumns+` FROM contest_drafts WHERE id=$1 AND contest_setter(id,$2) FOR UPDATE`, id, actor))
	if err != nil {
		return current, err
	}
	if err = stillSetter(ctx, tx, id, actor); err != nil {
		return current, err
	}
	if current.Version != in.Version {
		return current, ErrConflict
	}
	listed := map[string]bool{}
	for _, p := range current.Draft.Problems {
		listed[p.ID] = true
	}
	added := []string{}
	for _, p := range in.Draft.Problems {
		if !listed[p.ID] {
			added = append(added, p.ID)
		}
	}
	if err = addable(ctx, tx, actor, added); err != nil {
		return current, err
	}
	r, err := scanDraft(tx.QueryRow(ctx, `UPDATE contest_drafts SET draft=$2,version=version+1,updated_at=clock_timestamp()
 WHERE id=$1 RETURNING `+draftColumns, id, data))
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

// stillSetter checks the role again after the row lock: the locking statement judged it before the wait, and a
// removal that committed meanwhile locks the draft without changing it, so PostgreSQL does not check again.
func stillSetter(ctx context.Context, tx pgx.Tx, id, actor string) error {
	var setter bool
	if err := tx.QueryRow(ctx, `SELECT contest_setter($1,$2)`, id, actor).Scan(&setter); err != nil {
		return err
	}
	if !setter {
		return pgx.ErrNoRows
	}
	return nil
}

// lockID serializes creating a draft, publishing it and creating a contest under one id. Each checks the other table
// for the id with its own snapshot, which a commit in between would slip past and leave a draft beside the contest.
func lockID(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text,709091022))`, id)
	return err
}

// draftConflict turns a missed write into 409 when the actor is still a setter of the draft or of its published
// contest, or, for owner-only actions, still owns it; otherwise the miss stays 404.
func (s *Store) draftConflict(ctx context.Context, actor, id string, err error, ownerOnly bool) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var exists bool
	if e := s.Pool.QueryRow(ctx, `SELECT CASE WHEN $3 THEN EXISTS (SELECT 1 FROM contest_drafts WHERE id=$1 AND owner_id=$2)
 OR EXISTS (SELECT 1 FROM contests WHERE id=$1 AND owner_id=$2) ELSE contest_setter($1,$2) END`, id, actor, ownerOnly).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return ErrConflict
	}
	return err
}

// Drafts lists the drafts the actor owns or co-sets.
func (s *Store) Drafts(ctx context.Context, actor string, offset int) ([]DraftSummary, error) {
	rows, err := s.Pool.Query(ctx, `SELECT d.id,d.draft->>'title',u.handle,(d.draft->>'startsAt')::timestamptz,d.updated_at
 FROM contest_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id
 WHERE contest_setter(d.id,$1)
 ORDER BY d.updated_at DESC,d.id DESC LIMIT 51 OFFSET $2`, actor, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DraftSummary, error) {
		var d DraftSummary
		err := row.Scan(&d.ID, &d.Title, &d.Author, &d.StartsAt, &d.UpdatedAt)
		return d, err
	})
}

// forgetContest removes what the id shares between a draft and its contest. A deleted draft takes its co-setters,
// link, exclusions and notifications with it, so a new draft that reuses the id starts clean. A contest with the id
// keeps them all, so that a stray draft beside it cannot clear who stays out of it.
func forgetContest(ctx context.Context, tx pgx.Tx, id string) error {
	var published bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id=$1)`, id).Scan(&published); err != nil || published {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM contest_setters WHERE contest_id=$1`,
		`DELETE FROM contest_setter_invitations WHERE contest_id=$1`,
		`DELETE FROM contest_exclusions WHERE contest_id=$1`,
		`DELETE FROM notifications WHERE contest_id=$1`,
	} {
		if _, err := tx.Exec(ctx, statement, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDraft is for the owner only; co-setters leave instead.
func (s *Store) DeleteDraft(ctx context.Context, owner, id string, version int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `DELETE FROM contest_drafts WHERE id=$1 AND owner_id=$2 AND version=$3`, id, owner, version)
	if err == nil && tag.RowsAffected() != 1 {
		err = pgx.ErrNoRows
	}
	if err == nil {
		err = forgetContest(ctx, tx, id)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return s.draftConflict(ctx, owner, id, err, true)
}

// Publish turns the saved draft into a scheduled contest under the same id, with the same checks as saving one.
// Any setter may publish; the draft's owner stays the owner, and the co-setters and link carry over by the id.
func (s *Store) Publish(ctx context.Context, actor, id string, version int64, policy JudgePolicy) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockID(ctx, tx, id); err != nil {
		return err
	}
	var owner string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM contest_drafts WHERE id=$1 AND contest_setter(id,$2) FOR UPDATE`, id, actor).Scan(&owner); err != nil {
		return err
	}
	if err = stillSetter(ctx, tx, id, actor); err != nil {
		return err
	}
	r, err := scanDraft(tx.QueryRow(ctx, `SELECT `+draftColumns+` FROM contest_drafts WHERE id=$1`, id))
	if err != nil {
		return err
	}
	if r.Version != version {
		return ErrConflict
	}
	issues, _, err := assess(ctx, tx, id, r.Draft, policy)
	if err != nil {
		return err
	}
	in := r.Draft.input()
	if len(issues) > 0 || !ValidInput(in) {
		return ErrNotReady
	}
	if err = save(ctx, tx, actor, id, in, policy, owner); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM contest_drafts WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d Draft) input() Input {
	in := Input{Title: d.Title, Description: d.Description, PenaltyMinutes: d.PenaltyMinutes, FreezeMinutes: d.FreezeMinutes, Problems: make([]Problem, len(d.Problems))}
	if end, ok := d.endsAt(); ok {
		in.StartsAt, in.EndsAt = *d.StartsAt, end
	}
	for i, p := range d.Problems {
		in.Problems[i].ID = p.ID
		if p.Points != nil {
			in.Problems[i].Points = *p.Points
		}
	}
	return in
}
