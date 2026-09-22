package contests

import (
	"context"
	"errors"
)

var ErrParticipationUnavailable = errors.New("contest participation unavailable")

func (s *Store) Join(ctx context.Context, id, owner string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize against schedule and problem-list edits before checking eligibility.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR SHARE`, id).Scan(&locked); err != nil {
		return err
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT c.owner_id<>$2 AND NOT EXISTS (
 SELECT 1 FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id
 WHERE cp.contest_id=c.id AND t.owner_id=$2)
 AND (clock_timestamp()<c.ends_at OR EXISTS (
 SELECT 1 FROM contest_participants p WHERE p.contest_id=c.id AND p.owner_id=$2))
 FROM contests c WHERE c.id=$1`, id, owner).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return ErrParticipationUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO contest_participants(contest_id,owner_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
