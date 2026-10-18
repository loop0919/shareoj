package contests

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrResultsPending means a submission made during the contest is still queued or being judged (or rejudged).
var ErrResultsPending = errors.New("contest submissions are still being judged")

// Reveal makes the results of an ended frozen contest final (ADR 0023). Setters and operators may reveal; it cannot be
// undone, and revealing again is a no-op. A contest without a freeze, or one that has not ended, cannot be revealed.
func (s *Store) Reveal(ctx context.Context, id, actor string, operator bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The row lock orders the reveal after submissions that are being created (they take FOR SHARE).
	var allowed, frozen, ended, revealed bool
	err = tx.QueryRow(ctx, `SELECT $3::boolean OR contest_setter(c.id,$2),c.freeze_minutes IS NOT NULL,c.ends_at<=clock_timestamp(),c.revealed_at IS NOT NULL
 FROM contests c WHERE c.id=$1 FOR NO KEY UPDATE`, id, actor, operator).Scan(&allowed, &frozen, &ended, &revealed)
	if err != nil {
		return err
	}
	if !allowed {
		return pgx.ErrNoRows
	}
	if revealed {
		return nil
	}
	if !frozen || !ended {
		return ErrConflict
	}
	var pending bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM submissions s JOIN contests c ON c.id=s.contest_id
 WHERE c.id=$1 AND s.created_at>=c.starts_at AND s.created_at<c.ends_at AND s.status<>'DONE'
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 AND NOT COALESCE((s.job->>'generate')::boolean,false)
 AND NOT COALESCE((s.job->>'validate')::boolean,false))`, id).Scan(&pending)
	if err != nil {
		return err
	}
	if pending {
		return ErrResultsPending
	}
	if _, err = tx.Exec(ctx, `UPDATE contests SET revealed_at=clock_timestamp() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
