package submissions

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type RateLimitError struct{ RetryAfter int }

func (e *RateLimitError) Error() string { return "submission rate limit exceeded" }

// Hold the account lock through insertion and commit. The separate query after
// the lock sees submissions committed by concurrent API instances.
func (s *Store) beginSubmission(ctx context.Context, owner string, easyTest bool) (pgx.Tx, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 709091002))`, owner); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	windowSeconds, limit := 60, 2
	if easyTest {
		windowSeconds, limit = 60, 3
	}
	var retryAfter int
	err = tx.QueryRow(ctx, `SELECT COALESCE(CEIL(EXTRACT(EPOCH FROM
		(created_at + $2::int * interval '1 second' - clock_timestamp())))::int, 0)
		FROM (SELECT (SELECT created_at FROM submissions
		WHERE owner_id=$1 AND COALESCE((job->>'easyTest')::boolean,false)=$3 AND created_at > clock_timestamp() - $2::int * interval '1 second'
		ORDER BY created_at DESC LIMIT 1 OFFSET $4) AS created_at) recent`, owner, windowSeconds, easyTest, limit-1).Scan(&retryAfter)
	if err == nil && retryAfter > 0 {
		err = &RateLimitError{RetryAfter: retryAfter}
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
