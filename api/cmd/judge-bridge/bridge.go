package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/submissions"
)

type bridge struct {
	db                        *pgxpool.Pool
	objects                   *s3.Client
	queue                     *sqs.Client
	bucket, queueURL, runtime string
	// A rolling OS update rebinds undispatched submissions from the digest it replaced (ADR 0014).
	previous, enabled string
	paused            bool      // rollout holds requests while the hosts switch digests
	capacity          *capacity // nil until the EC2 pool is enabled
}

type envelope struct {
	ID       string                `json:"submissionId"`
	Attempt  string                `json:"attemptId"`
	Result   *submissions.Result   `json:"result,omitempty"`
	Progress *submissions.Progress `json:"progress,omitempty"`
}

var identity = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// Invoked only through the IAM-protected Lambda API, never through the public API.
func (b bridge) deploymentStatus(ctx context.Context) (any, error) {
	status := struct {
		Kind                  string `json:"kind"`
		Pending               int64  `json:"pending"`
		Undispatched          int64  `json:"undispatched"`
		RuntimeDigest         string `json:"runtimeDigest"`
		PreviousRuntimeDigest string `json:"previousRuntimeDigest"`
		DispatchPaused        bool   `json:"dispatchPaused"`
		// A rolling update must not change the environment within a contest (ADR 0014).
		// The window matches the burst hold: 30 minutes before the start to 15 minutes after the end.
		NextContestWindowAt *time.Time `json:"nextContestWindowAt"`
	}{Kind: "judge-deployment-status", RuntimeDigest: b.runtime, PreviousRuntimeDigest: b.previous, DispatchPaused: b.paused}
	err := b.db.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE dispatched_at IS NULL),
      (SELECT MIN(c.starts_at-interval '30 minutes') FROM contests c
        WHERE c.ends_at+interval '15 minutes' > statement_timestamp()
          AND EXISTS (SELECT 1 FROM contest_participants p WHERE p.contest_id=c.id))
      FROM submissions WHERE runtime LIKE '%-isolate' AND status <> 'DONE'`).Scan(&status.Pending, &status.Undispatched, &status.NextContestWindowAt)
	if err != nil {
		return nil, errors.New("deployment status unavailable")
	}
	return status, nil
}

func (b bridge) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var event struct {
		Operation  string `json:"operation"`
		Source     string `json:"source"`
		DetailType string `json:"detail-type"`
		events.SQSEvent
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("invalid event")
	}
	if event.Operation != "" {
		if event.Operation != "deployment-status" || len(event.Records) != 0 {
			return nil, errors.New("invalid operation")
		}
		return b.deploymentStatus(ctx)
	}
	if len(event.Records) > 0 {
		return b.results(ctx, event.SQSEvent), nil
	}
	err := b.dispatch(ctx)
	// Only the minute schedule sizes the pool; per-submission wake-ups never call EC2.
	if b.capacity != nil && event.Source == "aws.events" && event.DetailType == "Scheduled Event" {
		b.capacity.run(ctx, b.db, b.runtime)
	}
	if err != nil {
		return nil, errors.New("dispatch failed; see operational logs")
	}
	return nil, nil
}
