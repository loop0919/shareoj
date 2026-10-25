package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	puts [][]byte // job bodies written to S3
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
			body, _ := io.ReadAll(r.Body)
			f.puts = append(f.puts, body)
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

func TestAuthoringRunsWaitBehindNewSubmissionsAheadOfRejudges(t *testing.T) {
	ctx := context.Background()
	authoring := func(generate bool) []byte {
		raw, _ := json.Marshal(submissions.Job{Image: testDigest, TimeLimitMS: 5000, MemoryLimitMB: 512, Generate: generate, Validate: !generate, Cases: []submissions.Case{{Input: "1"}}})
		return raw
	}
	// Oldest first: generation, validation, a rejudge requested before both, then a fresh submission.
	f := newDispatchFixture(t, "", authoring(true), authoring(false), queuedJob(nil), queuedJob(nil))
	generation, validation, rejudge, fresh := f.ids[0], f.ids[1], f.ids[2], f.ids[3]
	if _, err := f.b.db.Exec(ctx, `UPDATE submissions SET created_at=clock_timestamp()-interval '7 hours',rejudged_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, rejudge); err != nil {
		t.Fatal(err)
	}
	if err := f.b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	// The fresh submission goes first; one authoring run fills the second in-flight slot.
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{fresh, generation}) {
		t.Fatalf("dispatch order: %v", f.sent)
	}
	// A freed slot goes to the next authoring run, not to the earlier rejudge.
	if _, err := f.b.db.Exec(ctx, `UPDATE submissions SET status='DONE' WHERE id=$1`, fresh); err != nil {
		t.Fatal(err)
	}
	if err := f.b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{fresh, generation, validation}) {
		t.Fatalf("authoring must go before rejudges: %v", f.sent)
	}
}

// codeRun queues a code run created age ago and returns its ID.
func (f *dispatchFixture) codeRun(id string, age time.Duration) string {
	f.t.Helper()
	if _, err := f.b.db.Exec(context.Background(), `INSERT INTO code_runs(id,owner_id,runtime,image,source,stdin,judge_attempt,created_at)
 VALUES ($1,'alice','python314-isolate',$2,'print(1)',$3,$1,clock_timestamp()-make_interval(secs=>$4))`, id, testDigest, "1 2\n", age.Seconds()); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// codeRunState reads a code run as status/result/dispatched.
func (f *dispatchFixture) codeRunState(id string) string {
	f.t.Helper()
	var s string
	if err := f.b.db.QueryRow(context.Background(), `SELECT status||'/'||COALESCE(result::text,'')||'/'||(dispatched_at IS NOT NULL) FROM code_runs WHERE id=$1`, id).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *dispatchFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.b.db.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func authoringJob() []byte {
	raw, _ := json.Marshal(submissions.Job{Image: testDigest, TimeLimitMS: 5000, MemoryLimitMB: 512, Generate: true, Cases: []submissions.Case{{Input: "1"}}})
	return raw
}

const (
	codeRunA = "aaaaaaaa-2222-4222-8222-222222222222"
	codeRunB = "bbbbbbbb-2222-4222-8222-222222222222"
)

func TestCodeRunsGoAfterNewSubmissionsAheadOfAuthoringRuns(t *testing.T) {
	ctx := context.Background()
	// Oldest first: authoring run, rejudge, fresh submission.
	f := newDispatchFixture(t, "", authoringJob(), queuedJob(nil), queuedJob(nil))
	authoring, rejudge, fresh := f.ids[0], f.ids[1], f.ids[2]
	f.exec(`UPDATE submissions SET created_at=clock_timestamp()-interval '2 minutes' WHERE id=$1`, authoring)
	f.exec(`UPDATE submissions SET created_at=clock_timestamp()-interval '7 hours',rejudged_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, rejudge)
	run := f.codeRun(codeRunA, time.Minute)
	if err := f.b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{fresh, run}) {
		t.Fatalf("dispatch order: %v", f.sent)
	}
	f.exec(`UPDATE submissions SET status='DONE' WHERE id=$1`, fresh)
	if err := f.b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{fresh, run, authoring}) {
		t.Fatalf("authoring run next: %v", f.sent)
	}
	f.exec(`UPDATE submissions SET status='DONE' WHERE id=$1`, authoring)
	if err := f.b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{fresh, run, authoring, rejudge}) {
		t.Fatalf("rejudge last: %v", f.sent)
	}
}

func TestAuthoringRunsWaitingFiveMinutesGoBeforeCodeRuns(t *testing.T) {
	f := newDispatchFixture(t, "", authoringJob(), queuedJob(nil))
	authoring, busy := f.ids[0], f.ids[1]
	f.exec(`UPDATE submissions SET created_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, authoring)
	f.exec(`UPDATE submissions SET dispatched_at=clock_timestamp() WHERE id=$1`, busy)
	f.codeRun(codeRunA, time.Minute)
	if err := f.b.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{authoring}) {
		t.Fatalf("sent: %v", f.sent)
	}
}

func TestCodeRunsCountTowardSpareCapacity(t *testing.T) {
	f := newDispatchFixture(t, "", queuedJob(nil), queuedJob(nil))
	rejudge, fresh := f.ids[0], f.ids[1]
	f.exec(`UPDATE submissions SET created_at=clock_timestamp()-interval '7 hours',rejudged_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, rejudge)
	for _, id := range []string{codeRunA, codeRunB} {
		f.codeRun(id, time.Minute)
		f.exec(`UPDATE code_runs SET dispatched_at=clock_timestamp() WHERE id=$1`, id)
	}
	if err := f.b.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.sent) != fmt.Sprint([]string{fresh}) {
		t.Fatalf("sent: %v", f.sent)
	}
}

func TestCodeRunJobCarriesStdinAndFixedLimits(t *testing.T) {
	f := newDispatchFixture(t, "")
	run := f.codeRun(codeRunA, time.Minute)
	if err := f.b.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.puts) != 1 || fmt.Sprint(f.sent) != fmt.Sprint([]string{run}) {
		t.Fatalf("puts=%d sent=%v", len(f.puts), f.sent)
	}
	var job map[string]any
	if err := json.Unmarshal(f.puts[0], &job); err != nil {
		t.Fatal(err)
	}
	if _, has := job["cases"]; has || job["codeRun"] != true || job["stdin"] != base64.StdEncoding.EncodeToString([]byte("1 2\n")) ||
		job["timeLimitMs"] != 5000.0 || job["memoryLimitMb"] != 512.0 || job["runtimeDigest"] != testDigest ||
		job["submissionId"] != run || job["attemptId"] != run || job["runtime"] != "python314-isolate" || job["source"] != "print(1)" {
		t.Fatalf("job: %s", f.puts[0])
	}
	if got := f.codeRunState(run); !strings.HasSuffix(got, "/true") {
		t.Fatalf("state: %s", got)
	}
}

func TestCodeRunsExpireAndAreDeleted(t *testing.T) {
	f := newDispatchFixture(t, "")
	unsent := f.codeRun(codeRunA, 11*time.Minute)
	fresh := f.codeRun(codeRunB, 9*time.Minute)
	stuck := f.codeRun("cccccccc-2222-4222-8222-222222222222", 62*time.Minute)
	f.exec(`UPDATE code_runs SET status='RUNNING',dispatched_at=clock_timestamp()-interval '61 minutes' WHERE id=$1`, stuck)
	old := f.codeRun("dddddddd-2222-4222-8222-222222222222", 25*time.Hour)
	if err := f.b.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.codeRunState(unsent); got != `DONE/{"expired": true, "verdict": "JE"}/false` {
		t.Fatalf("unsent: %s", got)
	}
	// Nine minutes old is still sendable, so it is sent and stays QUEUED.
	if got := f.codeRunState(fresh); got != "QUEUED//true" {
		t.Fatalf("fresh: %s", got)
	}
	if got := f.codeRunState(stuck); !strings.HasPrefix(got, `DONE/{"verdict": "JE"}/`) {
		t.Fatalf("stuck: %s", got)
	}
	var n int
	if err := f.b.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM code_runs WHERE id=$1`, old).Scan(&n); err != nil || n != 0 {
		t.Fatalf("old row: %d %v", n, err)
	}
	for _, id := range f.sent {
		if id == unsent {
			t.Fatal("an expired code run was sent")
		}
	}
}

func TestDeploymentStatusCountsCodeRuns(t *testing.T) {
	f := newDispatchFixture(t, "")
	ctx := context.Background()
	f.b.paused = true
	f.codeRun(codeRunA, time.Minute)
	sent := f.codeRun(codeRunB, time.Minute)
	f.exec(`UPDATE code_runs SET dispatched_at=clock_timestamp() WHERE id=$1`, sent)
	counts := func() (float64, float64) {
		value, err := f.b.handle(ctx, json.RawMessage(`{"operation":"deployment-status"}`))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(value)
		var got map[string]any
		_ = json.Unmarshal(data, &got)
		return got["pending"].(float64), got["undispatched"].(float64)
	}
	if p, u := counts(); p != 2 || u != 1 {
		t.Fatalf("pending=%v undispatched=%v", p, u)
	}
	f.exec(`UPDATE code_runs SET status='DONE'`)
	if p, u := counts(); p != 0 || u != 0 {
		t.Fatalf("pending=%v undispatched=%v", p, u)
	}
}
