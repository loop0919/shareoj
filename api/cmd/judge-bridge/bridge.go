package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

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
		Kind          string `json:"kind"`
		Pending       int64  `json:"pending"`
		Undispatched  int64  `json:"undispatched"`
		RuntimeDigest string `json:"runtimeDigest"`
	}{Kind: "judge-deployment-status", RuntimeDigest: b.runtime}
	err := b.db.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE dispatched_at IS NULL)
      FROM submissions WHERE runtime LIKE '%-isolate' AND status <> 'DONE'`).Scan(&status.Pending, &status.Undispatched)
	if err != nil {
		return nil, errors.New("deployment status unavailable")
	}
	return status, nil
}

func (b bridge) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var event struct {
		Operation string `json:"operation"`
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
	if err := b.dispatch(ctx); err != nil {
		return nil, errors.New("dispatch failed; see operational logs")
	}
	return nil, nil
}
