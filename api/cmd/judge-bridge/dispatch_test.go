package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

// A test file that a job names but test_files does not hold.
var missingTestFile = &problems.TestFile{ID: "33333333-3333-4333-8333-333333333333", Size: 12, SHA256: strings.Repeat("b", 64)}

// The runtime digest shared by the bridge and every queued job.
var testDigest = "sha256:" + strings.Repeat("a", 64)

// queuedJob returns a sendable job. A file makes its only case read the expected output from that test file.
func queuedJob(file *problems.TestFile) []byte {
	c := submissions.Case{Input: "1", Output: "1"}
	if file != nil {
		c = submissions.Case{Input: "1", OutputFile: file}
	}
	raw, _ := json.Marshal(submissions.Job{Image: testDigest, TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{c}})
	return raw
}

type dispatchFixture struct {
	t    *testing.T
	b    bridge
	ids  []string // queued submissions, oldest first
	sent []string // submission IDs that reached SQS
	logs *bytes.Buffer
}

// newDispatchFixture queues one submission per job on a fresh database. Its S3 and SQS are a local
// server that answers failMethod (PUT for S3, POST for SQS) with 500.
func newDispatchFixture(t *testing.T, failMethod string, jobs ...[]byte) *dispatchFixture {
	t.Helper()
	ctx := context.Background()
	db := migratedDB(t)
	f := &dispatchFixture{t: t}
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	for i, job := range jobs {
		id := fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		f.ids = append(f.ids, id)
		_, err := db.Exec(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt,created_at)
 VALUES ($1,'alice',$1,1,'test','python314-isolate','source',$2,$1,clock_timestamp()+make_interval(secs=>$3))`, id, job, i)
		if err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == failMethod {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.Method == "PUT" {
			w.Header().Set("x-amz-version-id", "version")
			return
		}
		data, _ := io.ReadAll(r.Body)
		var request struct{ MessageBody string }
		var pointer struct {
			ID string `json:"submissionId"`
		}
		if json.Unmarshal(data, &request) != nil || json.Unmarshal([]byte(request.MessageBody), &pointer) != nil {
			t.Error("invalid queue request")
		}
		f.sent = append(f.sent, pointer.ID)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = fmt.Fprint(w, `{"MessageId":"test"}`)
	}))
	t.Cleanup(server.Close)
	cfg := aws.Config{Region: "ap-northeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	f.b = bridge{
		db: db, bucket: "bucket", queueURL: server.URL + "/queue", runtime: testDigest,
		objects: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(server.URL)
			o.UsePathStyle = true
			o.RetryMaxAttempts = 1
		}),
		queue: sqs.NewFromConfig(cfg, func(o *sqs.Options) {
			o.BaseEndpoint = aws.String(server.URL)
			o.RetryMaxAttempts = 1
		}),
	}
	f.logs = captureLogs(t)
	return f
}

// state reads a submission as status/verdict/finished/dispatched.
func (f *dispatchFixture) state(id string) string {
	f.t.Helper()
	var s string
	if err := f.b.db.QueryRow(context.Background(), `SELECT status||'/'||COALESCE(result->>'verdict','')||'/'||(finished_at IS NOT NULL)||'/'||(dispatched_at IS NOT NULL)
  FROM submissions WHERE id=$1`, id).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

// failures lists the failure logs in order as category/reason/submissionId.
func (f *dispatchFixture) failures() []string {
	var got []string
	for _, event := range logEvents(f.t, f.logs) {
		if event["event"] == "failure" {
			category, _ := event["category"].(string)
			reason, _ := event["reason"].(string)
			id, _ := event["submissionId"].(string)
			got = append(got, category+"/"+reason+"/"+id)
		}
	}
	return got
}

func TestUnsendableSubmissionBecomesJEAndDispatchContinues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		job    []byte
		reason string
	}{
		{"broken job", []byte(`{"cases":"broken"}`), "job_invalid"},
		{"missing test file", queuedJob(missingTestFile), "test_files_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDispatchFixture(t, "", tc.job, queuedJob(nil))
			head, next := f.ids[0], f.ids[1]
			if err := f.b.dispatch(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := f.state(head); got != "DONE/JE/true/false" {
				t.Fatalf("unsendable submission: %s", got)
			}
			if got := f.state(next); got != "QUEUED//false/true" {
				t.Fatalf("submission behind it: %s", got)
			}
			if fmt.Sprint(f.sent) != fmt.Sprint([]string{next}) {
				t.Fatalf("sent: %v", f.sent)
			}
			// One platform failure for the submission, with no stage failure or failed run on top.
			if got := f.failures(); fmt.Sprint(got) != fmt.Sprint([]string{"platform/" + tc.reason + "/" + head}) {
				t.Fatalf("failure logs: %v", got)
			}
		})
	}
}

func TestDispatchLeavesInfrastructureFailuresForRetry(t *testing.T) {
	// A submission cannot become DONE, so the update that makes it JE fails.
	const refuseDone = `CREATE FUNCTION refuse_done() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'refused'; END$$;
CREATE TRIGGER refuse_done BEFORE UPDATE ON submissions FOR EACH ROW WHEN (NEW.status = 'DONE') EXECUTE FUNCTION refuse_done()`
	mismatched, _ := json.Marshal(submissions.Job{Image: "sha256:" + strings.Repeat("c", 64), TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{{Input: "1", Output: "1"}}})
	for _, tc := range []struct {
		name, failMethod, setup string
		job                     []byte
		reason                  string
	}{
		{"job upload", "PUT", "", queuedJob(nil), "job_upload_failed"},
		{"request send", "POST", "", queuedJob(nil), "request_send_failed"},
		// A failed read of test_files is not a mismatch, even for a file that is missing.
		{"test file lookup", "", `ALTER TABLE test_files RENAME TO test_files_unavailable`, queuedJob(missingTestFile), "test_files_resolve_failed"},
		// The reason is logged only once the submission is JE, so only the failed update shows.
		{"JE update for broken job", "", refuseDone, []byte(`{"cases":"broken"}`), "finish_je_failed"},
		{"JE update for missing test file", "", refuseDone, queuedJob(missingTestFile), "finish_je_failed"},
		{"JE update for runtime mismatch", "", refuseDone, mismatched, "finish_je_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDispatchFixture(t, tc.failMethod, tc.job)
			id := f.ids[0]
			if tc.setup != "" {
				if _, err := f.b.db.Exec(context.Background(), tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.b.dispatch(context.Background()); err == nil {
				t.Fatal("an infrastructure failure must fail the run")
			}
			if got := f.state(id); got != "QUEUED//false/false" {
				t.Fatalf("an infrastructure failure must leave the submission for retry: %s", got)
			}
			if len(f.sent) != 0 {
				t.Fatalf("sent: %v", f.sent)
			}
			if got := f.failures(); fmt.Sprint(got) != fmt.Sprint([]string{"platform/" + tc.reason + "/" + id, "platform/dispatch_failed/"}) {
				t.Fatalf("failure logs: %v", got)
			}
		})
	}
}
