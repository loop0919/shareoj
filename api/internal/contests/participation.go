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
	// Serialize against schedule and problem-list edits and tester acceptance before checking eligibility.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR SHARE`, id).Scan(&locked); err != nil {
		return err
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT contest_competitor(c,$2)
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

// participantProblem reports whether a registered participant authors or tests a problem that a save would add.
// Adding it would make them staff while they compete (ADR 0022).
func participantProblem(ctx context.Context, q querier, id string, ids []string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM problem_drafts d JOIN contest_participants p ON p.contest_id=$1
 WHERE d.id=ANY($2::uuid[]) AND NOT EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.contest_id=$1 AND cp.problem_id=d.id)
 AND (p.owner_id=d.owner_id OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=d.id AND t.owner_id=p.owner_id)))`, id, ids).Scan(&found)
	return found, err
}
