package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RegistrationQuotaError struct{ RetryAfter int }

func (e *RegistrationQuotaError) Error() string { return "registration quota exceeded" }

// RegistrationQuota limits requests that make Cognito send a confirmation email.
// A network allows a burst of 20, recovering one every 3 minutes, so a classroom
// behind one NAT can still sign up. An address allows 3, recovering one every 20 minutes.
type RegistrationQuota struct{ Pool *pgxpool.Pool }

// Consume charges both subjects, or neither when either is exhausted.
func (q RegistrationQuota) Consume(ctx context.Context, network, email string) error {
	// An expired row means the same as no row; deleting it keeps the table bounded.
	// Delete outside the charge and skip locked rows so cleanup never joins a lock cycle.
	if _, err := q.Pool.Exec(ctx, `DELETE FROM registration_quotas WHERE subject IN
 (SELECT subject FROM registration_quotas WHERE full_at<=statement_timestamp() FOR UPDATE SKIP LOCKED)`); err != nil {
		return err
	}
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Charging the network before the email keeps the lock order fixed.
	var limited *RegistrationQuotaError
	for _, limit := range []struct {
		subject        string
		burst, seconds int
	}{{"network:" + network, 20, 180}, {"email:" + email, 3, 1200}} {
		retryAfter, err := consumeRegistrationSlot(ctx, tx, limit.subject, limit.burst, limit.seconds)
		if err != nil {
			return err
		}
		if retryAfter > 0 && (limited == nil || retryAfter > limited.RetryAfter) {
			limited = &RegistrationQuotaError{RetryAfter: retryAfter}
		}
	}
	if limited != nil {
		return limited
	}
	return tx.Commit(ctx)
}

// consumeRegistrationSlot uses the same full_at accounting as ConsumeCreationQuota.
func consumeRegistrationSlot(ctx context.Context, tx pgx.Tx, subject string, burst, seconds int) (int, error) {
	digest := sha256.Sum256([]byte(subject))
	key := hex.EncodeToString(digest[:])
	var accepted bool
	err := tx.QueryRow(ctx, `INSERT INTO registration_quotas(subject,full_at)
 VALUES($1,statement_timestamp()+$3::int*interval '1 second')
 ON CONFLICT(subject) DO UPDATE
 SET full_at=GREATEST(registration_quotas.full_at,statement_timestamp())+$3::int*interval '1 second'
 WHERE registration_quotas.full_at<=statement_timestamp()+($2::int-1)*$3::int*interval '1 second'
 RETURNING true`, key, burst, seconds).Scan(&accepted)
	if err == nil {
		return 0, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var retryAfter int
	err = tx.QueryRow(ctx, `SELECT GREATEST(1,CEIL(EXTRACT(EPOCH FROM
 (full_at-($2::int-1)*$3::int*interval '1 second'-clock_timestamp())))::int)
 FROM registration_quotas WHERE subject=$1`, key, burst, seconds).Scan(&retryAfter)
	return retryAfter, err
}
