package main

import (
	"context"
	"log/slog"

	"judge/api/internal/submissions"
)

func observe(event, category, reason, id, attempt string, fields ...any) {
	attrs := []any{"service", "judge-bridge", "event", event}
	if category != "" {
		attrs = append(attrs, "category", category, "reason", reason)
	}
	if identity.MatchString(id) {
		attrs = append(attrs, "submissionId", id)
	}
	if identity.MatchString(attempt) {
		attrs = append(attrs, "attemptId", attempt)
	}
	slog.Info("judge", append(attrs, fields...)...)
}

// Emitted only after a successful DB query, so missing data detects dispatcher failure.
func (b bridge) pendingAge(ctx context.Context) error {
	var age float64
	err := b.db.QueryRow(ctx, `SELECT COALESCE(GREATEST(EXTRACT(EPOCH FROM clock_timestamp()-MIN(created_at)),0),0)::float8
      FROM submissions WHERE runtime=ANY($1::text[]) AND status <> 'DONE'`, submissions.IsolateRuntimeIDs()).Scan(&age)
	if err == nil {
		observe("pending_age", "", "", "", "", "oldestPendingSeconds", age)
	}
	return err
}
