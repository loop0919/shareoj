package contests

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrResultsPending means a participant's submission made during the contest is still queued or being judged (or rejudged).
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
	// Rejudges hold the problem row while they queue submissions; waiting for them lets the check below see theirs.
	if _, err = tx.Exec(ctx, `SELECT 1 FROM problem_drafts d JOIN contest_problems cp ON cp.problem_id=d.id
 WHERE cp.contest_id=$1 ORDER BY d.id FOR SHARE OF d`, id); err != nil {
		return err
	}
	// Only the runs the standings count can still move them: those of competing participants during the contest.
	// Pending runs from before the freeze count too, since their verdicts would change the table after the reveal.
	var pending bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM submissions s JOIN contests c ON c.id=s.contest_id
 JOIN contest_participants p ON p.contest_id=c.id AND p.owner_id=s.owner_id
 WHERE c.id=$1 AND s.created_at>=c.starts_at AND s.created_at<c.ends_at AND s.status<>'DONE' AND contest_competitor(c,s.owner_id)
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
	// An operator without a profile cannot be the actor of a notification (the foreign key), so then only the bell stays quiet.
	if _, err = tx.Exec(ctx, `INSERT INTO notifications(owner_id,contest_id,actor_id,kind)
 SELECT p.owner_id,p.contest_id,$2,'contest_results' FROM contest_participants p
 WHERE p.contest_id=$1 AND p.owner_id<>$2 AND EXISTS(SELECT 1 FROM user_profiles WHERE owner_id=$2)`, id, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
