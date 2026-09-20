package problems

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type PublicProblem struct {
	DifficultyDistribution []int64   `json:"difficultyDistribution,omitempty"`
	DifficultyAverage      *float64  `json:"difficultyAverage"`
	DifficultyVoteCount    int64     `json:"difficultyVoteCount"`
	Testers                []string  `json:"testers,omitempty"`
	Difficulty             *int      `json:"difficulty"`
	SolverCount            int64     `json:"solverCount"`
	FavoriteCount          int64     `json:"favoriteCount"`
	Interactive            bool      `json:"interactive,omitempty"`
	SpecialJudge           bool      `json:"specialJudge,omitempty"`
	ID                     string    `json:"id"`
	Title                  string    `json:"title"`
	Markdown               string    `json:"markdown,omitempty"`
	Editorial              string    `json:"editorial,omitempty"`
	TimeLimitMS            string    `json:"timeLimitMs,omitempty"`
	MemoryLimitMB          string    `json:"memoryLimitMb,omitempty"`
	Author                 string    `json:"author"`
	PublishedAt            time.Time `json:"publishedAt"`
}
type Publications interface {
	Publish(context.Context, string, string, int64, bool) (Problem, error)
	PublicGet(context.Context, string) (PublicProblem, error)
	PublicList(context.Context, *Cursor) ([]PublicProblem, error)
}

func (s *Store) Publish(ctx context.Context, owner, id string, version int64, publish bool) (Problem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Problem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM problem_drafts WHERE id=$1 AND can_manage_problem(id,$2) FOR UPDATE`, id, owner).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return Problem{}, err
	}
	var reserved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=$1 AND NOT c.released)`, id).Scan(&reserved); err != nil {
		return Problem{}, err
	}
	if reserved {
		return Problem{}, ErrContestLocked
	}
	query := `UPDATE problem_drafts SET published_draft=NULL,published_version=0,published_at=NULL,version=version+1 WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3 RETURNING id,version,updated_at,draft,published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),'')`
	if publish {
		query = `UPDATE problem_drafts SET published_draft=draft,published_version=version+1,published_at=COALESCE(published_at,clock_timestamp()),version=version+1 WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3 RETURNING id,version,updated_at,draft,published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),'')`
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

// Accepted submissions visible on the public problem page.
const publicAcceptedSubmission = `s.status='DONE' AND s.result->>'verdict'='AC'
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 AND NOT COALESCE((s.job->>'generate')::boolean,false)
 AND NOT COALESCE((s.job->>'validate')::boolean,false)
 AND ((s.contest_id IS NULL AND NOT COALESCE((s.job->>'privateDraft')::boolean,true))
  OR EXISTS(SELECT 1 FROM contests c WHERE c.id=s.contest_id
   AND statement_timestamp()>=c.ends_at AND s.created_at>=c.starts_at))`

const publicSolverCount = `(SELECT count(DISTINCT s.owner_id) FROM submissions s WHERE s.problem_id=d.id AND ` + publicAcceptedSubmission + `)`

func (s *Store) PublicGet(ctx context.Context, id string) (PublicProblem, error) {
	var p PublicProblem
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT d.id,d.published_draft,u.handle,d.published_at,(SELECT count(*) FROM problem_favorites f WHERE f.problem_id=d.id),`+publicSolverCount+`,(SELECT avg(difficulty)::float8 FROM problem_difficulty_votes WHERE problem_id=d.id),(SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=d.id),ARRAY(SELECT (SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=d.id AND difficulty=level) FROM generate_series(1,10) level ORDER BY level) FROM problem_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id WHERE d.id=$1 AND d.published_draft IS NOT NULL`, id).Scan(&p.ID, &data, &p.Author, &p.PublishedAt, &p.FavoriteCount, &p.SolverCount, &p.DifficultyAverage, &p.DifficultyVoteCount, &p.DifficultyDistribution)
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
	p.Title = d.Title
	p.Difficulty = d.Difficulty
	p.Markdown = d.Markdown
	p.Editorial = d.Editorial
	p.TimeLimitMS = d.TimeLimitMS
	p.MemoryLimitMB = d.MemoryLimitMB
	p.SpecialJudge = d.Checker != nil
	p.Interactive = d.Interactor != nil
	p.Testers, err = s.Testers(ctx, id)
	return p, err
}

func (s *Store) PublicList(ctx context.Context, cursor *Cursor) ([]PublicProblem, error) {
	return s.PublicListByHandle(ctx, cursor, "")
}

func (s *Store) PublicListByHandle(ctx context.Context, cursor *Cursor, handle string) ([]PublicProblem, error) {
	query := `SELECT d.id,d.published_draft->>'title',(d.published_draft->>'difficulty')::integer,d.published_draft->>'timeLimitMs',d.published_draft->>'memoryLimitMb',u.handle,d.published_at,(SELECT count(*) FROM problem_favorites f WHERE f.problem_id=d.id),` + publicSolverCount + `,(SELECT avg(difficulty)::float8 FROM problem_difficulty_votes WHERE problem_id=d.id),(SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=d.id) FROM problem_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id WHERE d.published_draft IS NOT NULL AND ($1='' OR u.handle=$1)`
	args := []any{handle}
	if cursor != nil {
		query += ` AND (d.published_at,d.id)<($2,$3::uuid)`
		args = append(args, cursor.UpdatedAt, cursor.ID)
	}
	rows, err := s.pool.Query(ctx, query+` ORDER BY d.published_at DESC,d.id DESC LIMIT 51`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PublicProblem, error) {
		var p PublicProblem
		err := row.Scan(&p.ID, &p.Title, &p.Difficulty, &p.TimeLimitMS, &p.MemoryLimitMB, &p.Author, &p.PublishedAt, &p.FavoriteCount, &p.SolverCount, &p.DifficultyAverage, &p.DifficultyVoteCount)
		return p, err
	})
}

// SolvedProblems returns all publicly solved problem IDs, beyond the recent submission history.
func (s *Store) SolvedProblems(ctx context.Context, owner string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT s.problem_id::text FROM submissions s
 JOIN problem_drafts d ON d.id=s.problem_id
 WHERE s.owner_id=$1 AND d.published_draft IS NOT NULL AND `+publicAcceptedSubmission, owner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
