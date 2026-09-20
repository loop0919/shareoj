package problems

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type DifficultyVote struct {
	Distribution []int64  `json:"difficultyDistribution"`
	Difficulty   *int     `json:"difficulty"`
	Average      *float64 `json:"difficultyAverage"`
	Count        int64    `json:"difficultyVoteCount"`
}

// A nil value reads the vote; zero removes it. Lock the problem so voting
// cannot race with unpublishing, deletion, or another vote on this problem.
func (s *Store) DifficultyVote(ctx context.Context, owner, id string, value *int) (DifficultyVote, error) {
	var result DifficultyVote
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var found string
	err = tx.QueryRow(ctx, `SELECT id FROM problem_drafts WHERE id=$1 AND published_draft IS NOT NULL FOR UPDATE`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if value != nil {
		if *value == 0 {
			_, err = tx.Exec(ctx, `DELETE FROM problem_difficulty_votes WHERE problem_id=$1 AND owner_id=$2`, id, owner)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO problem_difficulty_votes(problem_id,owner_id,difficulty) VALUES($1,$2,$3) ON CONFLICT(problem_id,owner_id) DO UPDATE SET difficulty=EXCLUDED.difficulty`, id, owner, *value)
		}
		if err != nil {
			return result, err
		}
	}
	err = tx.QueryRow(ctx, `SELECT (SELECT difficulty FROM problem_difficulty_votes WHERE problem_id=$1 AND owner_id=$2),avg(difficulty)::float8,count(*),ARRAY(SELECT (SELECT count(*) FROM problem_difficulty_votes WHERE problem_id=$1 AND difficulty=level) FROM generate_series(1,10) level ORDER BY level) FROM problem_difficulty_votes WHERE problem_id=$1`, id, owner).Scan(&result.Difficulty, &result.Average, &result.Count, &result.Distribution)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
