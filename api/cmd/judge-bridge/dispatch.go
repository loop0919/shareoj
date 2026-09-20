package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/submissions"
)

func (b bridge) dispatch(ctx context.Context) (err error) {
	started := time.Now()
	defer func() {
		if err != nil {
			observe("failure", "platform", "dispatch_failed", "", "")
		}
		observe("dispatch_finished", "", "", "", "", "durationMs", time.Since(started).Milliseconds())
	}()
	if err = b.pendingAge(ctx); err != nil {
		return err
	}
	// The existing minute schedule also releases contests without incoming web traffic.
	if err := (&contests.Store{Pool: b.db}).Release(ctx); err != nil {
		return err
	}
	// Bounded expiry covers queue retries too; it does not rejudge a finalized submission.
	expired, err := b.db.Exec(ctx, `UPDATE submissions SET status='DONE',finished_at=clock_timestamp(),result='{"verdict":"JE","passed":0,"total":0}'
  WHERE runtime=ANY($1::text[]) AND status <> 'DONE' AND created_at < clock_timestamp()-interval '6 hours'`, submissions.IsolateRuntimeIDs())
	if err != nil {
		return err
	}
	if expired.RowsAffected() > 0 {
		observe("failure", "platform", "submission_expired", "", "", "count", expired.RowsAffected())
	}
	for range 20 {
		tx, err := b.db.Begin(ctx)
		if err != nil {
			return err
		}
		err = b.dispatchOne(ctx, tx)
		if err != nil {
			_ = tx.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b bridge) dispatchOne(ctx context.Context, tx pgx.Tx) (err error) {
	var id, attempt, source, runtime string
	var raw []byte
	stage := "outbox_read"
	defer func() {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			observe("failure", "platform", stage+"_failed", id, attempt)
		}
	}()
	err = tx.QueryRow(ctx, `SELECT id::text,judge_attempt::text,source,job,runtime FROM submissions
  WHERE runtime=ANY($1::text[]) AND dispatched_at IS NULL AND status <> 'DONE'
  ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, submissions.IsolateRuntimeIDs()).Scan(&id, &attempt, &source, &raw, &runtime)
	if err != nil {
		return err
	}
	var job submissions.Job
	if err = json.Unmarshal(raw, &job); err != nil {
		return err
	}
	stage = "test_files_resolve"
	if err = (&submissions.Store{Pool: b.db}).ResolveTestFiles(ctx, &job); err != nil {
		return err
	}
	if job.Image != b.runtime || job.MemoryLimitMB != 512 {
		observe("failure", "platform", "runtime_mismatch", id, attempt)
		_, err = tx.Exec(ctx, `UPDATE submissions SET status='DONE',finished_at=clock_timestamp(),result='{"verdict":"JE","passed":0,"total":0}' WHERE id=$1`, id)
		return err
	}
	payload, err := json.Marshal(map[string]any{"submissionId": id, "attemptId": attempt, "runtime": runtime, "runtimeDigest": job.Image, "source": source, "checker": job.Checker, "interactor": job.Interactor, "easyTest": job.EasyTest, "generate": job.Generate, "validate": job.Validate, "generationBaseBytes": job.GenerationBaseBytes, "generationPrefix": job.GenerationPrefix, "cases": job.Cases, "timeLimitMs": job.TimeLimitMS, "memoryLimitMb": job.MemoryLimitMB})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	key := "jobs/" + id + "/" + attempt + ".json"
	stage = "job_upload"
	obj, err := b.objects.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(key), Body: bytes.NewReader(payload), ContentType: aws.String("application/json")})
	if err != nil {
		return err
	}
	if obj.VersionId == nil {
		return errors.New("versioned job bucket required")
	}
	pointer, _ := json.Marshal(map[string]string{"submissionId": id, "attemptId": attempt, "key": key, "versionId": *obj.VersionId, "sha256": hex.EncodeToString(sum[:])})
	stage = "request_send"
	_, err = b.queue.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(b.queueURL), MessageBody: aws.String(string(pointer))})
	if err != nil {
		return err
	}
	observe("request_sent", "", "", id, attempt)
	stage = "outbox_update"
	// If commit fails after SendMessage, the same stable attempt is sent again.
	_, err = tx.Exec(ctx, `UPDATE submissions SET dispatched_at=clock_timestamp() WHERE id=$1`, id)
	return err
}
