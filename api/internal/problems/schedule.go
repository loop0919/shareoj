package problems

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSchedule = errors.New("problem cannot be scheduled")

// Schedule captures the saved draft; editing it later does not change the reservation.
func (s *Store) Schedule(ctx context.Context, owner, id string, version int64, at time.Time) (Problem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Problem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	var published bool
	if err = tx.QueryRow(ctx, `SELECT version,published_draft IS NOT NULL FROM problem_drafts WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&current, &published); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return Problem{}, err
	}
	if current != version {
		return Problem{}, ErrConflict
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT $2::timestamptz>clock_timestamp()
 AND NOT EXISTS(SELECT 1 FROM contest_problems WHERE problem_id=$1)
 AND NOT EXISTS(SELECT 1 FROM featured_applications WHERE problem_id=$1)`, id, at).Scan(&eligible); err != nil {
		return Problem{}, err
	}
	if published || !eligible {
		return Problem{}, ErrSchedule
	}
	p, err := scan(tx.QueryRow(ctx, `UPDATE problem_drafts SET scheduled_at=$2,scheduled_draft=draft,scheduled_version=version+1,version=version+1
 WHERE id=$1 RETURNING id,version,updated_at,draft,published_version,
 COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),'',scheduled_at`, id, at))
	if err == nil {
		err = tx.Commit(ctx)
	}
	return p, err
}

// ReleaseScheduled completes due publications before reads; the minute tick is only a fallback.
// Do not limit or skip locked rows: readers must not miss due problems while another request publishes them.
func (s *Store) ReleaseScheduled(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE problem_drafts SET published_draft=scheduled_draft,
 published_version=scheduled_version,published_at=scheduled_at,version=version+1
 WHERE id IN (SELECT id FROM problem_drafts WHERE scheduled_at<=statement_timestamp()
 ORDER BY scheduled_at,id FOR UPDATE)`)
	return err
}
