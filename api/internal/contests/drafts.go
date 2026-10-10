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
}
type DraftSummary struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	StartsAt  *time.Time `json:"startsAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

const (
	IssueTitle       = "title_missing"
	IssueStart       = "start_missing"
	IssueStartPast   = "start_past"
	IssueDuration    = "duration_invalid"
	IssuePenalty     = "penalty_invalid"
	IssueProblems    = "problems_missing"
	IssuePoints      = "points_invalid"
	IssueUnavailable = "problem_unavailable"
	IssueIncomplete  = "problem_incomplete"
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
	numbers := []*int{d.DurationMinutes, d.PenaltyMinutes}
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
}

// assess mirrors what publication checks, so the editor can show why it would fail.
func assess(ctx context.Context, q querier, owner, id string, d Draft, policy JudgePolicy) ([]string, error) {
	issues := d.contentIssues(time.Now())
	if len(d.Problems) == 0 {
		return issues, nil
	}
	ids := make([]string, len(d.Problems))
	for i, p := range d.Problems {
		ids[i] = p.ID
	}
	rows, err := q.Query(ctx, `SELECT d.draft FROM problem_drafts d WHERE d.id=ANY($1::uuid[]) AND d.owner_id=$2 AND d.published_draft IS NULL
 AND NOT EXISTS (SELECT 1 FROM contest_problems cp WHERE cp.problem_id=d.id AND cp.contest_id<>$3)`, ids, owner, id)
	if err != nil {
		return nil, err
	}
	drafts, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	if err != nil {
		return nil, err
	}
	if len(drafts) != len(ids) {
		issues = append(issues, IssueUnavailable)
	}
	for _, raw := range drafts {
		var p problems.Draft
		if json.Unmarshal(raw, &p) != nil || !policy.validProblem(p) {
			issues = append(issues, IssueIncomplete)
			break
		}
	}
	return issues, nil
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

func (s *Store) Draft(ctx context.Context, owner, id string, policy JudgePolicy) (DraftResult, error) {
	r, err := scanDraft(s.Pool.QueryRow(ctx, `SELECT `+draftColumns+` FROM contest_drafts WHERE id=$1 AND owner_id=$2`, id, owner))
	if err != nil {
		return r, err
	}
	r.Issues, err = assess(ctx, s.Pool, owner, id, r.Draft, policy)
	return r, err
}

func (s *Store) SaveDraft(ctx context.Context, owner, id string, in DraftInput, policy JudgePolicy) (DraftResult, error) {
	if in.Draft.Problems == nil {
		in.Draft.Problems = []DraftProblem{}
	}
	data, err := json.Marshal(in.Draft)
	if err != nil {
		return DraftResult{}, err
	}
	var r DraftResult
	if in.Version == 0 {
		r, err = s.createDraft(ctx, owner, id, data)
	} else {
		r, err = scanDraft(s.Pool.QueryRow(ctx, `UPDATE contest_drafts SET draft=$4,version=version+1,updated_at=clock_timestamp()
 WHERE id=$1 AND owner_id=$2 AND version=$3 RETURNING `+draftColumns, id, owner, in.Version, data))
	}
	if err = s.draftConflict(ctx, owner, id, err); err != nil {
		return r, err
	}
	r.Issues, err = assess(ctx, s.Pool, owner, id, r.Draft, policy)
	return r, err
}

func (s *Store) createDraft(ctx context.Context, owner, id string, data []byte) (DraftResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DraftResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = database.ConsumeCreationQuota(ctx, tx, owner, "contest"); err != nil {
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

// draftConflict turns a missed write into 409 when the owner still has the draft or already published it.
func (s *Store) draftConflict(ctx context.Context, owner, id string, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var exists bool
	if e := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contest_drafts WHERE id=$1 AND owner_id=$2)
 OR EXISTS (SELECT 1 FROM contests WHERE id=$1 AND owner_id=$2)`, id, owner).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return ErrConflict
	}
	return err
}

func (s *Store) Drafts(ctx context.Context, owner string, offset int) ([]DraftSummary, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,draft->>'title',(draft->>'startsAt')::timestamptz,updated_at FROM contest_drafts
 WHERE owner_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 51 OFFSET $2`, owner, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DraftSummary, error) {
		var d DraftSummary
		err := row.Scan(&d.ID, &d.Title, &d.StartsAt, &d.UpdatedAt)
		return d, err
	})
}

func (s *Store) DeleteDraft(ctx context.Context, owner, id string, version int64) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM contest_drafts WHERE id=$1 AND owner_id=$2 AND version=$3`, id, owner, version)
	if err == nil && tag.RowsAffected() != 1 {
		err = pgx.ErrNoRows
	}
	return s.draftConflict(ctx, owner, id, err)
}

// Publish turns the saved draft into a scheduled contest under the same id, with the same checks as saving one.
func (s *Store) Publish(ctx context.Context, owner, id string, version int64, policy JudgePolicy) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := scanDraft(tx.QueryRow(ctx, `SELECT `+draftColumns+` FROM contest_drafts WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner))
	if err != nil {
		return err
	}
	if r.Version != version {
		return ErrConflict
	}
	issues, err := assess(ctx, tx, owner, id, r.Draft, policy)
	if err != nil {
		return err
	}
	in := r.Draft.input()
	if len(issues) > 0 || !ValidInput(in) {
		return ErrNotReady
	}
	if err = save(ctx, tx, owner, id, in, policy, false); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM contest_drafts WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d Draft) input() Input {
	in := Input{Title: d.Title, Description: d.Description, PenaltyMinutes: d.PenaltyMinutes, Problems: make([]Problem, len(d.Problems))}
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
