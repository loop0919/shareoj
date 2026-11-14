package main

import (
	"context"
	"errors"
	"regexp"

	"judge/api/internal/submissions"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// requeueDigest returns dispatched but unfinished items of the previous digest to the undispatched state,
// so the next dispatch rebinds and re-sends them to the current environment. A new attempt makes results
// from the old environment stale. rejudged_at and created_at stay, which keeps the deadline and the send order.
// Invoked only through the IAM-protected Lambda API.
func (b bridge) requeueDigest(ctx context.Context, digest string) (any, error) {
	if !digestPattern.MatchString(digest) || b.previous == "" || digest != b.previous {
		return nil, errors.New("invalid operation")
	}
	tx, err := b.db.Begin(ctx)
	if err != nil {
		return nil, errors.New("requeue unavailable")
	}
	defer tx.Rollback(ctx)
	var requeued int64
	err = tx.QueryRow(ctx, `WITH s AS (UPDATE submissions SET dispatched_at=NULL,judge_attempt=gen_random_uuid(),status='QUEUED',progress=NULL,started_at=NULL
        WHERE runtime=ANY($1::text[]) AND dispatched_at IS NOT NULL AND status <> 'DONE' AND job->>'image' = $2 RETURNING 1),
      c AS (UPDATE code_runs SET dispatched_at=NULL,judge_attempt=gen_random_uuid(),status='QUEUED'
        WHERE runtime=ANY($1::text[]) AND dispatched_at IS NOT NULL AND status <> 'DONE' AND image = $2 RETURNING 1)
      SELECT (SELECT COUNT(*) FROM s) + (SELECT COUNT(*) FROM c)`, submissions.IsolateRuntimeIDs(), digest).Scan(&requeued)
	if err != nil || tx.Commit(ctx) != nil {
		return nil, errors.New("requeue unavailable")
	}
	observe("requeued", "", "", "", "", "count", requeued)
	return struct {
		Kind     string `json:"kind"`
		Requeued int64  `json:"requeued"`
	}{"judge-requeue", requeued}, nil
}
