package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type CreationQuotaError struct{ RetryAfter int }

func (e *CreationQuotaError) Error() string { return "creation quota exceeded" }

// ConsumeCreationQuota allows a burst of 20 creations, recovering one every two
// hours. full_at tracks when all 20 slots will be available again. Consume in the
// creation transaction so failures roll back the charge; deleting content does not.
func ConsumeCreationQuota(ctx context.Context, tx pgx.Tx, owner, kind string) error {
	var accepted bool
	err := tx.QueryRow(ctx, `INSERT INTO creation_quotas(owner_id,kind,full_at)
 VALUES($1,$2,statement_timestamp()+interval '2 hours')
 ON CONFLICT(owner_id,kind) DO UPDATE
 SET full_at=GREATEST(creation_quotas.full_at,statement_timestamp())+interval '2 hours'
 WHERE creation_quotas.full_at<=statement_timestamp()+(20-1)*interval '2 hours'
 RETURNING true`, owner, kind).Scan(&accepted)
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var retryAfter int
	err = tx.QueryRow(ctx, `SELECT GREATEST(1,CEIL(EXTRACT(EPOCH FROM
 (full_at-(20-1)*interval '2 hours'-clock_timestamp())))::int)
 FROM creation_quotas WHERE owner_id=$1 AND kind=$2`, owner, kind).Scan(&retryAfter)
	if err != nil {
		return err
	}
	return &CreationQuotaError{RetryAfter: retryAfter}
}
