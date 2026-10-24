package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/problems"
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
	if err := problems.New(b.db).ReleaseScheduled(ctx); err != nil {
		return err
	}
	if err := problems.New(b.db).ReleaseFeatured(ctx, submissions.RuntimeIDs(), b.publishedRuntimes()); err != nil {
		return err
	}
	// Bounded expiry covers queue retries too; it does not rejudge a finalized submission.
	// A requested rejudge counts from its request, not from the original submission.
	expired, err := b.db.Exec(ctx, `UPDATE submissions SET status='DONE',finished_at=clock_timestamp(),result='{"verdict":"JE","passed":0,"total":0}'
  WHERE runtime=ANY($1::text[]) AND status <> 'DONE' AND COALESCE(rejudged_at,created_at) < clock_timestamp()-interval '6 hours'`, submissions.IsolateRuntimeIDs())
	if err != nil {
		return err
	}
	if expired.RowsAffected() > 0 {
		observe("failure", "platform", "submission_expired", "", "", "count", expired.RowsAffected())
	}
	return b.send(ctx)
}

// send moves queued submissions to SQS. The API invokes dispatch per submission, so pausing
// the schedule alone would not hold requests.
func (b bridge) send(ctx context.Context) error {
	if b.paused {
		observe("dispatch_paused", "", "", "", "")
		return nil
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

func (b bridge) publishedRuntimes() []string {
	return (submissions.Config{JudgeImage: b.runtime, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: b.enabled}).RuntimeIDs()
}

// spareInFlight limits judgements sent while authoring runs or rejudges wait. They use spare capacity only.
const spareInFlight = 2

func (b bridge) dispatchOne(ctx context.Context, tx pgx.Tx) (err error) {
	var id, attempt, source, runtime string
	var raw []byte
	stage := "outbox_read"
	defer func() {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			observe("failure", "platform", stage+"_failed", id, attempt)
		}
	}()
	// New submissions go first, then authoring runs, then rejudges (ADR 0032). The last two wait until
	// fewer than spareInFlight judgements are in SQS or running, so a new submission queues behind at most that many.
	err = tx.QueryRow(ctx, `SELECT id::text,judge_attempt::text,source,job,runtime FROM submissions
  WHERE runtime=ANY($1::text[]) AND dispatched_at IS NULL AND status <> 'DONE'
    AND ((rejudged_at IS NULL AND NOT `+submissions.AuthoringJob+`) OR (SELECT COUNT(*) FROM submissions f
      WHERE f.runtime=ANY($1::text[]) AND f.dispatched_at IS NOT NULL AND f.status <> 'DONE') < $2)
  ORDER BY rejudged_at IS NOT NULL,`+submissions.AuthoringJob+`,COALESCE(rejudged_at,created_at),id FOR UPDATE SKIP LOCKED LIMIT 1`, submissions.IsolateRuntimeIDs(), spareInFlight).Scan(&id, &attempt, &source, &raw, &runtime)
	if err != nil {
		return err
	}
	var job submissions.Job
	if json.Unmarshal(raw, &job) != nil {
		stage = "finish_je"
		return finishAsJE(ctx, tx, "job_invalid", id, attempt)
	}
	stage = "test_files_resolve"
	// Only a mismatch with test_files is permanent; a failed read is retried by the next run.
	err = (&submissions.Store{Pool: b.db}).ResolveTestFiles(ctx, &job)
	if errors.Is(err, problems.ErrTestFile) {
		stage = "finish_je"
		return finishAsJE(ctx, tx, "test_files_invalid", id, attempt)
	}
	if err != nil {
		return err
	}
	if job.Image != b.runtime && b.previous != "" && job.Image == b.previous &&
		slices.Contains(b.publishedRuntimes(), strings.TrimSuffix(runtime, "-isolate")) {
		// Never executed, so it can move to the environment that replaced its own.
		stage = "runtime_rebind"
		if _, err = tx.Exec(ctx, `UPDATE submissions SET job=jsonb_set(job,'{image}',to_jsonb($2::text)) WHERE id=$1`, id, b.runtime); err != nil {
			return err
		}
		job.Image = b.runtime
		observe("runtime_rebound", "", "", id, attempt)
	}
	if job.Image != b.runtime || job.MemoryLimitMB < 64 || job.MemoryLimitMB > 512 {
		stage = "finish_je"
		return finishAsJE(ctx, tx, "runtime_mismatch", id, attempt)
	}
	request := map[string]any{"submissionId": id, "attemptId": attempt, "runtime": runtime, "runtimeDigest": job.Image, "source": source, "checker": job.Checker, "interactor": job.Interactor, "easyTest": job.EasyTest, "generate": job.Generate, "validate": job.Validate, "generationBaseBytes": job.GenerationBaseBytes, "generationPrefix": job.GenerationPrefix, "cases": job.Cases, "timeLimitMs": job.TimeLimitMS, "memoryLimitMb": job.MemoryLimitMB}
	// Only a partial-scoring job carries the key, so other jobs reach the worker exactly as before ADR 0025.
	if job.Scoring != nil {
		request["scoring"] = job.Scoring
	}
	if job.AcceptPoints {
		request["acceptPoints"] = true
	}
	payload, err := json.Marshal(request)
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

// finishAsJE finalizes a submission that no retry can send, so it does not hold the ones behind it.
// It logs the reason only after the update succeeds; the caller's finish_je stage logs a failed update.
func finishAsJE(ctx context.Context, tx pgx.Tx, reason, id, attempt string) error {
	if _, err := tx.Exec(ctx, `UPDATE submissions SET status='DONE',finished_at=clock_timestamp(),result='{"verdict":"JE","passed":0,"total":0}' WHERE id=$1`, id); err != nil {
		return err
	}
	observe("failure", "platform", reason, id, attempt)
	return nil
}
