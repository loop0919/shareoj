package problems

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Favorite struct {
	Favorited bool  `json:"favorited"`
	Count     int64 `json:"favoriteCount"`
}

// Favorite reads or sets this user's favorite. Lock the public problem until
// commit so unpublishing/deletion cannot race with a new favorite.
// A contest problem is not published until the end, so whoever may open it in the contest may favorite it (ADR 0040).
func (s *Store) Favorite(ctx context.Context, owner, id string, value *bool) (Favorite, error) {
	var result Favorite
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var found string
	err = tx.QueryRow(ctx, `SELECT d.id FROM problem_drafts d WHERE d.id=$1 AND (d.published_draft IS NOT NULL OR EXISTS(
 SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
 WHERE cp.problem_id=d.id AND (statement_timestamp()>=c.starts_at OR contest_problem_access(c.id,cp.problem_id,$2)))) FOR SHARE OF d`, id, owner).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if value != nil {
		if *value {
			_, err = tx.Exec(ctx, `INSERT INTO problem_favorites (problem_id,owner_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, owner)
		} else {
			_, err = tx.Exec(ctx, `DELETE FROM problem_favorites WHERE problem_id=$1 AND owner_id=$2`, id, owner)
		}
		if err != nil {
			return result, err
		}
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM problem_favorites WHERE problem_id=$1 AND owner_id=$2),(SELECT count(*) FROM problem_favorites WHERE problem_id=$1)`, id, owner).Scan(&result.Favorited, &result.Count)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
