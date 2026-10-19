package problems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type PublicProblem struct {
	EditorialHidden        bool       `json:"editorialHidden"`
	EditorialRevealAt      *time.Time `json:"editorialRevealAt,omitempty"`
	DifficultyDistribution []int64    `json:"difficultyDistribution,omitempty"`
	DifficultyAverage      *float64   `json:"difficultyAverage"`
	DifficultyVoteCount    int64      `json:"difficultyVoteCount"`
	Testers                []string   `json:"testers,omitempty"`
	Tags                   []string   `json:"tags,omitempty"`
	Difficulty             *int       `json:"difficulty"`
	SolverCount            int64      `json:"solverCount"`
	FavoriteCount          int64      `json:"favoriteCount"`
	HasSamples             bool       `json:"hasSamples"`
	Interactive            bool       `json:"interactive,omitempty"`
	SpecialJudge           bool       `json:"specialJudge,omitempty"`
	// Scoring is set for a partially scored problem (ADR 0025) and left out of lists.
	Scoring       *ScoringSummary `json:"scoring,omitempty"`
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Markdown      string          `json:"markdown,omitempty"`
	Editorial     string          `json:"editorial,omitempty"`
	TimeLimitMS   string          `json:"timeLimitMs,omitempty"`
	MemoryLimitMB string          `json:"memoryLimitMb,omitempty"`
	Author        string          `json:"author"`
	PublishedAt   time.Time       `json:"publishedAt"`
}
type Publications interface {
	Publish(context.Context, string, string, int64, bool) (Problem, error)
	PublicGet(context.Context, string, string) (PublicProblem, error)
	PublicList(context.Context, *Cursor, ListFilter) ([]PublicProblem, error)
}

// ListFilter narrows the public problem list. Empty fields match every problem.
type ListFilter struct {
	Author string
	Tag    string
}

func (s *Store) Publish(ctx context.Context, owner, id string, version int64, publish bool) (Problem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Problem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var author, published bool
	if err = tx.QueryRow(ctx, `SELECT owner_id=$2,published_draft IS NOT NULL FROM problem_drafts WHERE id=$1 AND can_manage_problem(id,$2) FOR UPDATE`, id, owner).Scan(&author, &published); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return Problem{}, err
	}
	// Testers may take a published problem down; publishing and reservations stay with the author.
	if !author && (publish || !published) {
		return Problem{}, ErrNotFound
	}
	var reserved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=$1 AND NOT c.released)`, id).Scan(&reserved); err != nil {
		return Problem{}, err
	}
	if reserved {
		return Problem{}, ErrContestLocked
	}
	query := `UPDATE problem_drafts SET scheduled_at=NULL,scheduled_draft=NULL,scheduled_version=NULL,published_draft=NULL,published_version=0,published_at=NULL,version=version+1 WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3 RETURNING id,version,updated_at,draft,published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),''),scheduled_at`
	if publish {
		query = `UPDATE problem_drafts SET published_draft=draft,published_version=version+1,published_at=COALESCE(published_at,clock_timestamp()),version=version+1 WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3 RETURNING id,version,updated_at,draft,published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),''),scheduled_at`
	}
	p, err := scan(tx.QueryRow(ctx, query, owner, id, version))
	if errors.Is(err, ErrNotFound) {
		return p, ErrConflict
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return p, err
}

// A run solves its problem with the full score, which is an AC for an all-or-nothing problem (ADR 0025).
const acceptedRun = `s.status='DONE' AND submission_full_score(s.result)
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 AND NOT COALESCE((s.job->>'generate')::boolean,false)
 AND NOT COALESCE((s.job->>'validate')::boolean,false)`

// Accepted submissions visible on the public problem page.
const publicAcceptedSubmission = acceptedRun + `
 AND ((s.contest_id IS NULL AND NOT COALESCE((s.job->>'privateDraft')::boolean,true))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=s.contest_id AND contest_submission_public(c,s.created_at)))`

// The viewer's own solved marks ignore the freeze (ADR 0023), so contest runs count from the end.
const ownAcceptedSubmission = acceptedRun + `
 AND ((s.contest_id IS NULL AND NOT COALESCE((s.job->>'privateDraft')::boolean,true))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=s.contest_id AND s.created_at>=c.starts_at AND c.ends_at<=statement_timestamp()))`

const publicSolverCount = `(SELECT count(DISTINCT s.owner_id) FROM submissions s WHERE s.problem_id=d.id AND ` + publicAcceptedSubmission + `)`

// PublicGet keeps editorialHidden as the public embargo, but the author and testers still receive the editorial.
// Tags hint at the solution, so they are also hidden while a 定期便 round features the problem.
func (s *Store) PublicGet(ctx context.Context, id, viewer string) (PublicProblem, error) {
	var p PublicProblem
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT d.id,CASE WHEN can_manage_problem(d.id,$2) THEN d.published_draft WHEN statement_timestamp()<featured_reveal_at(d.id) THEN d.published_draft-ARRAY['editorial','tags'] WHEN problem_tags_hidden(d.id) THEN d.published_draft-'tags' ELSE d.published_draft END,u.handle,d.published_at,featured_reveal_at(d.id),COALESCE(statement_timestamp()<featured_reveal_at(d.id),false),(SELECT count(*) FROM problem_favorites f WHERE f.problem_id=d.id),`+publicSolverCount+`,(SELECT avg(difficulty)::float8 FROM problem_difficulty_votes WHERE problem_id=d.id),(SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=d.id),ARRAY(SELECT (SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=d.id AND difficulty=level) FROM generate_series(1,10) level ORDER BY level) FROM problem_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id WHERE d.id=$1 AND d.published_draft IS NOT NULL`, id, viewer).Scan(&p.ID, &data, &p.Author, &p.PublishedAt, &p.EditorialRevealAt, &p.EditorialHidden, &p.FavoriteCount, &p.SolverCount, &p.DifficultyAverage, &p.DifficultyVoteCount, &p.DifficultyDistribution)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, ErrNotFound
		}
		return p, err
	}
	var d Draft
	if err = json.Unmarshal(data, &d); err != nil {
		return p, err
	}
	p.SetStatement(d)
	p.Editorial = d.Editorial
	p.Tags = d.Tags
	p.Testers, err = s.Testers(ctx, id)
	return p, err
}

// SetStatement sets, from the draft, the fields that the problem page and a contest problem page both show to whoever
// can open them. The editorial and the tags are left to the caller because the two pages hide them differently: the
// problem page hides them by viewer and during a 定期便 round, while a contest problem page shows the editorial only
// after the end or to the contest's setters and the problem's author and testers, hides it until the 定期便 reveal,
// and never shows the tags (ADR 0018).
func (p *PublicProblem) SetStatement(d Draft) {
	p.Title = d.Title
	p.Difficulty = d.Difficulty
	p.Markdown = d.Markdown
	p.TimeLimitMS = d.TimeLimitMS
	p.MemoryLimitMB = d.MemoryLimitMB
	p.SpecialJudge = d.Checker != nil
	p.Interactive = d.Interactor != nil
	p.HasSamples = d.HasSamples()
	p.Scoring = d.PublicScoring()
}

// PublicList returns at most 51 problems, newest first. A tag matches only while the problem's tags are public.
func (s *Store) PublicList(ctx context.Context, cursor *Cursor, filter ListFilter) ([]PublicProblem, error) {
	query := `SELECT d.id,d.published_draft->>'title',(d.published_draft->>'difficulty')::integer,d.published_draft->>'timeLimitMs',d.published_draft->>'memoryLimitMb',u.handle,d.published_at,(SELECT count(*) FROM problem_favorites f WHERE f.problem_id=d.id),` + publicSolverCount + `,(SELECT avg(difficulty)::float8 FROM problem_difficulty_votes WHERE problem_id=d.id),(SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=d.id),CASE WHEN problem_tags_hidden(d.id) THEN '[]' ELSE COALESCE(d.published_draft->'tags','[]') END FROM problem_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id WHERE d.published_draft IS NOT NULL AND ($1='' OR u.handle=$1)`
	args := []any{filter.Author}
	if filter.Tag != "" {
		args = append(args, filter.Tag)
		query += fmt.Sprintf(` AND d.published_draft->'tags' @> jsonb_build_array($%d::text) AND NOT problem_tags_hidden(d.id)`, len(args))
	}
	if cursor != nil {
		args = append(args, cursor.UpdatedAt, cursor.ID)
		query += fmt.Sprintf(` AND (d.published_at,d.id)<($%d,$%d::uuid)`, len(args)-1, len(args))
	}
	rows, err := s.pool.Query(ctx, query+` ORDER BY d.published_at DESC,d.id DESC LIMIT 51`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PublicProblem, error) {
		var p PublicProblem
		err := row.Scan(&p.ID, &p.Title, &p.Difficulty, &p.TimeLimitMS, &p.MemoryLimitMB, &p.Author, &p.PublishedAt, &p.FavoriteCount, &p.SolverCount, &p.DifficultyAverage, &p.DifficultyVoteCount, &p.Tags)
		return p, err
	})
}

// SolvedProblems returns all publicly solved problem IDs, beyond the recent submission history.
func (s *Store) SolvedProblems(ctx context.Context, owner string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT s.problem_id::text FROM submissions s
 JOIN problem_drafts d ON d.id=s.problem_id
 WHERE s.owner_id=$1 AND d.published_draft IS NOT NULL AND `+ownAcceptedSubmission, owner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// HasSamples reports whether sample validation has any cases to run.
func (d Draft) HasSamples() bool {
	for _, c := range d.TestCases {
		if c.IsSample {
			return true
		}
	}
	return false
}
