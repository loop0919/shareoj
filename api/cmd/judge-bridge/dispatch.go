package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	n, err := (&submissions.Store{Pool: b.db}).ExpireCodeRuns(ctx, submissions.IsolateRuntimeIDs())
	if err != nil {
		return err
	}
	if n > 0 {
		observe("code_runs_expired", "", "", "", "", "count", n)
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

// dispatchOne sends one queued item in this order (ADR 0031, ADR 0032). Each step takes its row FOR UPDATE SKIP LOCKED.
//  1. a new submission;
//  2. nothing else while spareInFlight or more judgements (submissions and code runs) are in SQS or running;
//  3. an authoring run that has waited 5 minutes, so code runs cannot starve it;
//  4. a code run;
//  5. an authoring run, then a rejudge.
//
// Steps 3 to 5 use spare capacity only, so a new submission queues behind at most spareInFlight judgements.
func (b bridge) dispatchOne(ctx context.Context, tx pgx.Tx) (err error) {
	isolate := submissions.IsolateRuntimeIDs()
	var id, attempt, runtime, source string
	var raw []byte
	// read selects one submission; false means none matched.
	read := func(query string) (found bool, err error) {
		err = tx.QueryRow(ctx, query, isolate).Scan(&id, &attempt, &source, &raw, &runtime)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			observe("failure", "platform", "outbox_read_failed", "", "")
		}
		return err == nil, err
	}
	const waiting = `SELECT id::text,judge_attempt::text,source,job,runtime FROM submissions
  WHERE runtime=ANY($1::text[]) AND dispatched_at IS NULL AND status <> 'DONE'`
	found, err := read(waiting + ` AND rejudged_at IS NULL AND NOT ` + submissions.AuthoringJob + ` ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`)
	if err != nil {
		return err
	}
	if found {
		return b.sendSubmission(ctx, tx, id, attempt, source, raw, runtime)
	}
	var inFlight int
	if err = tx.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM submissions WHERE runtime=ANY($1::text[]) AND dispatched_at IS NOT NULL AND status <> 'DONE')
  + (SELECT COUNT(*) FROM code_runs WHERE runtime=ANY($1::text[]) AND dispatched_at IS NOT NULL AND status <> 'DONE')`, isolate).Scan(&inFlight); err != nil {
		observe("failure", "platform", "outbox_read_failed", "", "")
		return err
	}
	if inFlight >= spareInFlight {
		return pgx.ErrNoRows
	}
	if found, err = read(waiting + ` AND ` + submissions.AuthoringJob + ` AND created_at <= clock_timestamp() - interval '5 minutes'
  ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`); err != nil {
		return err
	}
	if !found {
		var image, stdin string
		err = tx.QueryRow(ctx, `SELECT id::text,judge_attempt::text,runtime,image,source,stdin FROM code_runs
  WHERE runtime=ANY($1::text[]) AND dispatched_at IS NULL AND status <> 'DONE'
  ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, isolate).Scan(&id, &attempt, &runtime, &image, &source, &stdin)
		if err == nil {
			return b.sendCodeRun(ctx, tx, id, attempt, runtime, image, source, stdin)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			observe("failure", "platform", "outbox_read_failed", "", "")
			return err
		}
		if found, err = read(waiting + ` ORDER BY ` + submissions.AuthoringJob + ` DESC,COALESCE(rejudged_at,created_at),id FOR UPDATE SKIP LOCKED LIMIT 1`); err != nil {
			return err
		}
		if !found {
			return pgx.ErrNoRows
		}
	}
	return b.sendSubmission(ctx, tx, id, attempt, source, raw, runtime)
}

// sendSubmission sends the submission row the caller locked in tx.
func (b bridge) sendSubmission(ctx context.Context, tx pgx.Tx, id, attempt, source string, raw []byte, runtime string) (err error) {
	stage := "outbox_read"
	defer func() {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			observe("failure", "platform", stage+"_failed", id, attempt)
		}
	}()
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
	if err = b.upload(ctx, id, attempt, request, &stage); err != nil {
		return err
	}
	stage = "outbox_update"
	// If commit fails after SendMessage, the same stable attempt is sent again.
	_, err = tx.Exec(ctx, `UPDATE submissions SET dispatched_at=clock_timestamp() WHERE id=$1`, id)
	return err
}

// upload puts the job in S3 and sends its pointer to SQS. It sets stage so the caller logs which step failed.
func (b bridge) upload(ctx context.Context, id, attempt string, request map[string]any, stage *string) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	key := "jobs/" + id + "/" + attempt + ".json"
	*stage = "job_upload"
	obj, err := b.objects.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(key), Body: bytes.NewReader(payload), ContentType: aws.String("application/json")})
	if err != nil {
		return err
	}
	if obj.VersionId == nil {
		return errors.New("versioned job bucket required")
	}
	pointer, _ := json.Marshal(map[string]string{"submissionId": id, "attemptId": attempt, "key": key, "versionId": *obj.VersionId, "sha256": hex.EncodeToString(sum[:])})
	*stage = "request_send"
	if _, err = b.queue.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(b.queueURL), MessageBody: aws.String(string(pointer))}); err != nil {
		return err
	}
	observe("request_sent", "", "", id, attempt)
	return nil
}

// sendCodeRun sends the code run row the caller locked in tx, the same way as sendSubmission.
func (b bridge) sendCodeRun(ctx context.Context, tx pgx.Tx, id, attempt, runtime, image, source, stdin string) (err error) {
	stage := "outbox_read"
	defer func() {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			observe("failure", "platform", stage+"_failed", id, attempt)
		}
	}()
	if image != b.runtime && b.previous != "" && image == b.previous &&
		slices.Contains(b.publishedRuntimes(), strings.TrimSuffix(runtime, "-isolate")) {
		// Never executed, so it can move to the environment that replaced its own.
		stage = "runtime_rebind"
		if _, err = tx.Exec(ctx, `UPDATE code_runs SET image=$2 WHERE id=$1`, id, b.runtime); err != nil {
			return err
		}
		image = b.runtime
		observe("runtime_rebound", "", "", id, attempt)
	}
	if image != b.runtime {
		stage = "finish_je"
		if _, err = tx.Exec(ctx, `UPDATE code_runs SET status='DONE',finished_at=clock_timestamp(),result='{"verdict":"JE"}' WHERE id=$1`, id); err != nil {
			return err
		}
		observe("failure", "platform", "runtime_mismatch", id, attempt)
		return nil
	}
	request := map[string]any{"submissionId": id, "attemptId": attempt, "codeRun": true, "runtime": runtime, "runtimeDigest": image, "source": source,
		"stdin": base64.StdEncoding.EncodeToString([]byte(stdin)), "timeLimitMs": submissions.CodeRunTimeLimitMS, "memoryLimitMb": submissions.CodeRunMemoryLimitMB}
	if err = b.upload(ctx, id, attempt, request, &stage); err != nil {
		return err
	}
	stage = "outbox_update"
	_, err = tx.Exec(ctx, `UPDATE code_runs SET dispatched_at=clock_timestamp() WHERE id=$1`, id)
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
