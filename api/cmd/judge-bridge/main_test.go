package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"judge/api/internal/database"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

func TestOperationalLogsDoNotAcceptUntrustedIdentities(t *testing.T) {
	var out bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	defer slog.SetDefault(old)
	observe("failure", "platform", "invalid_result_envelope", "secret source", "secret expected output")
	if strings.Contains(out.String(), "secret") {
		t.Fatal("untrusted identity leaked")
	}
	var event map[string]any
	if err := json.Unmarshal(out.Bytes(), &event); err != nil || event["category"] != "platform" {
		t.Fatal("invalid operational event")
	}
}

func TestInvalidDeploymentOperationCannotDispatch(t *testing.T) {
	for _, raw := range []string{`{"operation":"unknown"}`, `{"operation":"deployment-status","Records":[{}]}`, `{"operation":5}`} {
		if _, err := (bridge{}).handle(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatal("invalid administrative event was accepted")
		}
	}
}

func TestResultValidation(t *testing.T) {
	cpu, wall, memory := 0.0, 1.0, int64(1024)
	good := submissions.Result{Verdict: "AC", Total: 1, Passed: 1, Cases: []submissions.CaseResult{{Name: "one", Verdict: "AC", CPUTimeMS: &cpu, WallTimeMS: &wall, MemoryBytes: &memory}}}
	if !validResult(good) {
		t.Fatal("zero CPU measurement is valid")
	}
	good.Cases[0].SampleDetails = &submissions.SampleDetails{Input: submissions.TextPreview{Text: "1 2\n"}}
	if !validResult(good) {
		t.Fatal("sample preview rejected")
	}
	good.Cases[0].SampleDetails.Input.Text = strings.Repeat("x", 4097)
	if validResult(good) {
		t.Fatal("unbounded preview accepted")
	}
	good.Cases[0].SampleDetails.Input.Text = "\x00"
	if validResult(good) {
		t.Fatal("NUL preview accepted")
	}
	good.Cases[0].SampleDetails = nil
	good.Cases[0].CheckerLog = &submissions.TextPreview{Text: "対話判定: AC"}
	if !validResult(good) {
		t.Fatal("interactive diagnostic rejected")
	}
	good.Cases[0].CheckerLog.Text = strings.Repeat("x", 4097)
	if validResult(good) {
		t.Fatal("unbounded interactive diagnostic accepted")
	}
	good.Cases[0].CheckerLog = nil
	empty := ""
	good.Cases[0].Output = &empty
	if !validResult(good) {
		t.Fatal("empty generation output rejected")
	}
	inline := "x"
	good.Cases[0].Output = &inline
	if validResult(good) {
		t.Fatal("inline generation bytes must not enter queue")
	}
	good.Cases[0].Output = nil
	file := &problems.TestFile{ID: "33333333-3333-4333-8333-333333333333", Size: 16 << 20, SHA256: strings.Repeat("a", 64), Key: "test-files/owner/problem/generated/file", Version: "version"}
	good.Cases[0].OutputFile = file
	if !validResult(good) {
		t.Fatal("16 MiB file reference rejected")
	}
	encoded, _ := json.Marshal(envelope{Result: &good})
	if len(encoded) > 4096 {
		t.Fatal("result contains generated data")
	}
	file.Size++
	if validResult(good) {
		t.Fatal("oversized generated file accepted")
	}
	good.Cases[0].OutputFile = nil
	good.Cases[0].MemoryBytes = nil
	if validResult(good) {
		t.Fatal("missing measurement must not pass")
	}
	good.Cases[0].MemoryBytes = &memory
	good.Passed = 0
	if validResult(good) {
		t.Fatal("inconsistent aggregate must not pass")
	}
	if !validResult(submissions.Result{Verdict: "JE"}) {
		t.Fatal("infrastructure failure may have no measurements")
	}
	if !validResult(submissions.Result{Verdict: "JE", CheckerLog: strings.Repeat("a", 16384)}) || validResult(submissions.Result{Verdict: "JE", CheckerLog: strings.Repeat("a", 16385)}) {
		t.Fatal("checker diagnostic budget")
	}
}

func TestKnockoutResultValidation(t *testing.T) {
	cpu, wall, memory := 1.0, 2.0, int64(1024)
	for _, tc := range []struct {
		verdicts []string
		valid    bool
	}{
		{[]string{"TLE", "AC", "TLE", "SKIPPED", "SKIPPED"}, true},
		{[]string{"WA", "TLE", "TLE", "SKIPPED"}, true},
		{[]string{"TLE", "TLE"}, true},
		{[]string{"TLE", "TLE", "TLE"}, true}, // Existing workers still execute all cases.
		{[]string{"TLE", "SKIPPED"}, false},
		{[]string{"WA", "WA", "SKIPPED"}, false},
		{[]string{"SKIPPED", "TLE", "TLE"}, false},
		{[]string{"TLE", "TLE", "SKIPPED", "AC"}, false},
	} {
		t.Run(strings.Join(tc.verdicts, "/"), func(t *testing.T) {
			r := submissions.Result{Verdict: tc.verdicts[0], Total: len(tc.verdicts)}
			for _, verdict := range tc.verdicts {
				c := submissions.CaseResult{Name: "case", Verdict: verdict}
				if verdict != "SKIPPED" {
					c.CPUTimeMS, c.WallTimeMS, c.MemoryBytes = &cpu, &wall, &memory
				}
				if verdict == "AC" {
					r.Passed++
				}
				r.Cases = append(r.Cases, c)
			}
			if validResult(r) != tc.valid {
				t.Fatalf("unexpected validation: %+v", r)
			}
			if tc.valid && r.Cases[len(r.Cases)-1].Verdict == "SKIPPED" {
				r.Cases[len(r.Cases)-1].CPUTimeMS = &cpu
				if validResult(r) {
					t.Fatal("skipped case must not carry measurements")
				}
			}
		})
	}
}

func TestScoringResultValidation(t *testing.T) {
	cpu, wall, memory := 1.0, 2.0, int64(1024)
	// result builds a scored result in hundredths from verdicts and two group scores of a 30 + 70 point problem.
	result := func(first, second int64, verdicts ...string) submissions.Result {
		r := submissions.Result{Verdict: "AC", Total: len(verdicts), Scoring: &submissions.SubmissionScore{Score: first + second, MaxScore: 10000,
			Groups: []submissions.GroupScore{{Score: first, MaxScore: 3000}, {Score: second, MaxScore: 7000}}}}
		for _, v := range verdicts {
			c := submissions.CaseResult{Name: "case", Verdict: v}
			if v != "SKIPPED" {
				c.CPUTimeMS, c.WallTimeMS, c.MemoryBytes = &cpu, &wall, &memory
				if v == "AC" {
					r.Passed++
				} else if r.Verdict == "AC" {
					r.Verdict = v
				}
			}
			r.Cases = append(r.Cases, c)
		}
		return r
	}
	score := func(v int64) *int64 { return &v }
	for _, tc := range []struct {
		name   string
		change func(*submissions.Result)
		valid  bool
	}{
		{"scored result", func(*submissions.Result) {}, true},
		{"old result without scores", func(r *submissions.Result) { r.Scoring = nil }, true},
		{"skipped in the middle", func(r *submissions.Result) { *r = result(0, 7000, "TLE", "SKIPPED", "AC") }, true},
		{"skipped in the middle without scores", func(r *submissions.Result) { *r = result(0, 7000, "TLE", "SKIPPED", "AC"); r.Scoring = nil }, false},
		{"accepted with skipped cases after the budget", func(r *submissions.Result) { *r = result(3000, 0, "AC", "SKIPPED", "SKIPPED") }, true},
		{"verdict from executed cases", func(r *submissions.Result) { *r = result(0, 0, "SKIPPED", "WA"); r.Verdict = "AC" }, false},
		{"skipped case with a measurement", func(r *submissions.Result) { *r = result(0, 0, "AC", "SKIPPED"); r.Cases[1].MemoryBytes = &memory }, false},
		{"skipped case with a score", func(r *submissions.Result) { *r = result(0, 0, "AC", "SKIPPED"); r.Cases[1].Score = score(1) }, false},
		{"case scores on AC and WA", func(r *submissions.Result) {
			r.Cases[0].Score, r.Cases[1].Score = score(0), score(submissions.MaxCaseScore)
		}, true},
		{"case score over 10^18", func(r *submissions.Result) { r.Cases[1].Score = score(submissions.MaxCaseScore + 1) }, false},
		{"negative case score", func(r *submissions.Result) { r.Cases[0].Score = score(-1) }, false},
		{"case score on TLE", func(r *submissions.Result) { r.Cases[2].Score = score(1) }, false},
		{"case score without scores", func(r *submissions.Result) { r.Scoring = nil; r.Cases[0].Score = score(1) }, false},
		{"group over its maximum", func(r *submissions.Result) { r.Scoring.Groups[0].Score, r.Scoring.Score = 3001, 3751 }, false},
		{"negative group", func(r *submissions.Result) { r.Scoring.Groups[1].Score, r.Scoring.Score = -1, 2999 }, false},
		{"total is not the sum", func(r *submissions.Result) { r.Scoring.Score = 10000 }, false},
		{"full score is not the sum", func(r *submissions.Result) { r.Scoring.MaxScore = 9900 }, false},
		{"group maximum not whole points", func(r *submissions.Result) { r.Scoring.Groups[0].MaxScore, r.Scoring.MaxScore = 3050, 10050 }, false},
		{"no group", func(r *submissions.Result) { r.Scoring = &submissions.SubmissionScore{} }, false},
		{"full score 0", func(r *submissions.Result) {
			r.Scoring = &submissions.SubmissionScore{Groups: []submissions.GroupScore{{}}}
		}, false},
		{"full score over 1,000,000 points", func(r *submissions.Result) {
			r.Scoring = &submissions.SubmissionScore{MaxScore: 100*1_000_000 + 100, Groups: []submissions.GroupScore{{MaxScore: 100 * 1_000_000}, {MaxScore: 100}}}
		}, false},
		{"101 groups", func(r *submissions.Result) {
			r.Scoring = &submissions.SubmissionScore{MaxScore: 10100, Groups: make([]submissions.GroupScore, 101)}
			for i := range r.Scoring.Groups {
				r.Scoring.Groups[i].MaxScore = 100
			}
		}, false},
		{"compile error scores 0", func(r *submissions.Result) {
			*r = submissions.Result{Verdict: "CE", Total: 3, Scoring: &submissions.SubmissionScore{MaxScore: 10000, Groups: []submissions.GroupScore{{MaxScore: 3000}, {MaxScore: 7000}}}}
		}, true},
		{"compile error with points", func(r *submissions.Result) {
			*r = submissions.Result{Verdict: "CE", Total: 3, Scoring: &submissions.SubmissionScore{Score: 3000, MaxScore: 10000, Groups: []submissions.GroupScore{{Score: 3000, MaxScore: 3000}, {MaxScore: 7000}}}}
		}, false},
		{"judge error with scores", func(r *submissions.Result) {
			*r = submissions.Result{Verdict: "JE", Total: 3, Scoring: &submissions.SubmissionScore{MaxScore: 10000, Groups: []submissions.GroupScore{{MaxScore: 3000}, {MaxScore: 7000}}}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := result(3000, 750, "AC", "WA", "TLE")
			tc.change(&r)
			if validResult(r) != tc.valid {
				t.Fatalf("unexpected validation: %+v", r)
			}
		})
	}
	// A case score that is not an int64 integer cannot even be decoded.
	for score, valid := range map[string]bool{"37": true, "1000000000000000000": true, "37.5": false, "1e18": false, "9223372036854775808": false, `"37"`: false} {
		body := `{"submissionId":"s","attemptId":"a","result":{"verdict":"AC","passed":1,"total":1,"cases":[{"name":"01","verdict":"AC","score":` + score + `,"cpuTimeMs":1,"wallTimeMs":1,"memoryBytes":1}],"scoring":{"score":100,"maxScore":100,"groups":[{"score":100,"maxScore":100}]}}}`
		var e envelope
		if (json.Unmarshal([]byte(body), &e) == nil && validResult(*e.Result)) != valid {
			t.Fatal("case score", score)
		}
	}
}

func TestOutboxAndResultIdempotency(t *testing.T) {
	for _, runtime := range []string{"cpp17-isolate", "rust2024-isolate"} {
		for _, memory := range []int{64, 315, 512} {
			t.Run(fmt.Sprintf("%s/%dMiB", runtime, memory), func(t *testing.T) { checkOutboxAndResultIdempotency(t, runtime, memory) })
		}
	}
}

// migratedDB returns a pool on a fresh schema that is dropped when the test ends.
func migratedDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_bridge_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

func checkOutboxAndResultIdempotency(t *testing.T, runtime string, memory int) {
	ctx := context.Background()
	db := migratedDB(t)
	var err error
	const id = "11111111-1111-4111-8111-111111111111"
	const attempt = "22222222-2222-4222-8222-222222222222"
	digest := "sha256:" + strings.Repeat("a", 64)
	_, err = db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`)
	if err != nil {
		t.Fatal(err)
	}
	fileID := "33333333-3333-4333-8333-333333333333"
	fileDigest := strings.Repeat("b", 64)
	_, err = db.Exec(ctx, `INSERT INTO test_files(id,owner_id,problem_id,object_key,version_id,sha256,size,ready) VALUES ($1,'alice',$2,'test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/33333333-3333-4333-8333-333333333333','test-version',$3,12,true)`, fileID, id, fileDigest)
	if err != nil {
		t.Fatal(err)
	}
	job := submissions.Job{Checker: &problems.Generator{Runtime: "python314", Source: "checker-secret"}, Image: digest, TimeLimitMS: 1000, MemoryLimitMB: memory, Cases: []submissions.Case{{Input: "input-secret", Output: "", OutputFile: &problems.TestFile{ID: fileID, Size: 12, SHA256: fileDigest}}}}
	if runtime == "rust2024-isolate" {
		job.Interactor, job.Checker = job.Checker, nil
	}
	raw, _ := json.Marshal(job)
	_, err = db.Exec(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt)
 VALUES ($1,'alice',$1,1,'test',$4,'source-secret',$2,$3)`, id, raw, attempt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	var sent []string
	var jobs [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			data, _ := io.ReadAll(r.Body)
			jobs = append(jobs, data)
			w.Header().Set("x-amz-version-id", "version-one")
			w.WriteHeader(200)
			return
		}
		data, _ := io.ReadAll(r.Body)
		var request struct{ MessageBody string }
		if err := json.Unmarshal(data, &request); err != nil {
			t.Error(err)
		}
		sent = append(sent, request.MessageBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if _, err := fmt.Fprint(w, `{"MessageId":"test"}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	cfg := aws.Config{Region: "ap-northeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	b := bridge{
		db: db, bucket: "test-bucket", queueURL: server.URL + "/queue", runtime: digest,
		objects: s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(server.URL); o.UsePathStyle = true }),
		queue:   sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(server.URL) }),
	}
	checkDeployment := func(pending, undispatched float64) {
		t.Helper()
		before := len(sent)
		value, err := b.handle(ctx, json.RawMessage(`{"operation":"deployment-status"}`))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(value)
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got["kind"] != "judge-deployment-status" || got["runtimeDigest"] != digest || got["pending"] != pending || got["undispatched"] != undispatched || len(sent) != before {
			t.Fatalf("invalid or mutating deployment status: %s", data)
		}
	}
	checkDeployment(1, 1)
	// A runtime removed from the list is neither dispatched nor expired, so it must not hold a rollout either.
	if _, err := db.Exec(ctx, `UPDATE submissions SET runtime='retired-language-isolate' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	checkDeployment(0, 0)
	if _, err := db.Exec(ctx, `UPDATE submissions SET runtime=$2 WHERE id=$1`, id, runtime); err != nil {
		t.Fatal(err)
	}
	// The DB query covers undispatched submissions, including an idle zero.
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	if err := b.pendingAge(ctx); err != nil {
		t.Fatal(err)
	}
	var observed map[string]any
	if err := json.Unmarshal(logs.Bytes(), &observed); err != nil || observed["oldestPendingSeconds"].(float64) < 0 {
		t.Fatal("pending age missing")
	}
	// A crash between successful send and DB commit must resend the same attempt.
	tx, _ := db.Begin(ctx)
	if err = b.dispatchOne(ctx, tx); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if err = b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	checkDeployment(1, 0)
	if len(sent) != 2 || sent[0] != sent[1] || strings.Contains(sent[0], "secret") {
		t.Fatalf("unsafe/unstable queue payloads: %v", sent)
	}
	if len(jobs) != 2 || !bytes.Contains(jobs[0], []byte(`"versionId":"test-version"`)) || !bytes.Contains(jobs[0], []byte(`"key":"test-files/`)) {
		t.Fatalf("test file locator missing from immutable job: %s", jobs[0])
	}
	if !bytes.Contains(jobs[0], []byte(`"runtime":"`+runtime+`"`)) {
		t.Fatal("submitted runtime was not preserved in dispatch")
	}
	var dispatched struct {
		MemoryLimitMB int `json:"memoryLimitMb"`
	}
	if err := json.Unmarshal(jobs[0], &dispatched); err != nil || dispatched.MemoryLimitMB != memory {
		t.Fatalf("memory limit was not preserved in dispatch: %+v %v", dispatched, err)
	}
	field := "checker"
	if job.Interactor != nil {
		field = "interactor"
	}
	if !bytes.Contains(jobs[0], []byte(`"`+field+`":{"runtime":"python314","source":"checker-secret"}`)) {
		t.Fatal("checker was not preserved in dispatch")
	}
	if _, _, err = (&submissions.Store{Pool: db}).Claim(ctx); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("local worker claimed a cloud job", err)
	}
	final := func(a string, r submissions.Result) {
		body, _ := json.Marshal(envelope{ID: id, Attempt: a, Result: &r})
		response := b.results(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "one", Body: string(body)}}})
		if len(response.BatchItemFailures) > 0 {
			t.Fatal(response)
		}
	}
	final("wrong-attempt", submissions.Result{Verdict: "JE"})
	var status string
	if err = db.QueryRow(ctx, `SELECT status FROM submissions WHERE id=$1`, id).Scan(&status); err != nil || status != "QUEUED" {
		t.Fatal(status, err)
	}
	progress := func(a, phase string, completed, total int, verdict ...string) {
		v := ""
		if len(verdict) > 0 {
			v = verdict[0]
		}
		body, _ := json.Marshal(envelope{ID: id, Attempt: a, Progress: &submissions.Progress{Phase: phase, Completed: completed, Total: total, Verdict: v}})
		if response := b.results(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "progress", Body: string(body)}}}); len(response.BatchItemFailures) != 0 {
			t.Fatal(response)
		}
	}
	check := func(wantPhase string, wantCompleted int) {
		s, err := (&submissions.Store{Pool: db}).Get(ctx, "alice", id)
		if err != nil || s.Status != "RUNNING" || s.Progress == nil || s.Progress.Phase != wantPhase || s.Progress.Completed != wantCompleted {
			t.Fatalf("unexpected progress: %+v %v", s, err)
		}
	}
	progress("wrong-attempt", "JUDGING", 1, 1)
	progress(attempt, "PREPARING", 0, 1)
	check("PREPARING", 0)
	progress(attempt, "JUDGING", 0, 1)
	check("JUDGING", 0)
	progress(attempt, "JUDGING", 1, 1, "TLE")
	progress(attempt, "JUDGING", 1, 1) // Duplicate cannot erase the verdict.
	progress(attempt, "JUDGING", 0, 1)
	progress(attempt, "PREPARING", 0, 1)
	progress(attempt, "JUDGING", 2, 2) // Valid envelope, but not this job's total.
	check("JUDGING", 1)
	current, err := (&submissions.Store{Pool: db}).Get(ctx, "alice", id)
	if err != nil || current.Progress == nil || current.Progress.Verdict != "TLE" {
		t.Fatalf("early verdict lost: %+v %v", current, err)
	}
	final(attempt, submissions.Result{Verdict: "CE", Total: 1, CompileLog: "compiler diagnostic"})
	progress(attempt, "JUDGING", 1, 1)
	final(attempt, submissions.Result{Verdict: "JE"})
	var verdict string
	if err = db.QueryRow(ctx, `SELECT result->>'verdict' FROM submissions WHERE id=$1`, id).Scan(&verdict); err != nil || verdict != "CE" {
		t.Fatal("final result overwritten", verdict, err)
	}
	checkDeployment(0, 0)
}

func TestProgressValidation(t *testing.T) {
	for _, verdict := range []string{"WA", "TLE", "MLE", "OLE", "RE", "AC", "CE", "JE", "unknown"} {
		want := verdict == "WA" || verdict == "TLE" || verdict == "MLE" || verdict == "OLE" || verdict == "RE"
		if validProgress(submissions.Progress{Phase: "JUDGING", Completed: 1, Total: 4, Verdict: verdict}) != want {
			t.Fatal(verdict)
		}
		for _, phase := range []string{"PREPARING", "JUDGING"} {
			if validProgress(submissions.Progress{Phase: phase, Total: 4, Verdict: verdict}) {
				t.Fatal("verdict before any case completed", phase, verdict)
			}
		}
	}

	for _, p := range []submissions.Progress{{Phase: "PREPARING", Total: 4}, {Phase: "JUDGING", Completed: 2, Total: 4}} {
		if !validProgress(p) {
			t.Fatal(p)
		}
	}
	for _, p := range []submissions.Progress{{Phase: "DONE", Total: 4}, {Phase: "PREPARING", Completed: 1, Total: 4}, {Phase: "JUDGING", Completed: 5, Total: 4}, {Phase: "JUDGING", Total: 101}, {Phase: "JUDGING"}, {Phase: "JUDGING", Completed: -1, Total: 4}} {
		if validProgress(p) {
			t.Fatal(p)
		}
	}
	// Ambiguous or empty envelopes must not be applied as either a result or progress.
	for _, body := range []string{`{}`, `{"result":{"verdict":"JE"},"progress":{"phase":"PREPARING","total":4}}`, `{"progress":{"phase":"UNKNOWN","total":4}}`} {
		response := (bridge{}).results(context.Background(), events.SQSEvent{Records: []events.SQSMessage{{MessageId: "bad", Body: body}}})
		if len(response.BatchItemFailures) != 1 {
			t.Fatal(response)
		}
	}
}

func TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	old, current, other := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), "sha256:"+strings.Repeat("c", 64)
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	submissionsByName := map[string]string{}
	for i, s := range []struct{ name, runtime, image string }{
		{"rebound", "python314-isolate", old},
		{"unpublished", "java24-isolate", old},
		{"unknown", "python314-isolate", other},
		{"current", "python314-isolate", current},
	} {
		id := fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		submissionsByName[s.name] = id
		raw, _ := json.Marshal(submissions.Job{Image: s.image, TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{{Input: "1", Output: "1"}}})
		_, err := db.Exec(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt,created_at)
 VALUES ($1,'alice',$1,1,'test',$2,'source',$3,$1,clock_timestamp()+make_interval(secs=>$4))`, id, s.runtime, raw, i)
		if err != nil {
			t.Fatal(err)
		}
	}
	// A code run waiting on the previous digest is rebound like a submission.
	const codeRunID = "99999999-2222-4222-8222-222222222222"
	if _, err := db.Exec(ctx, `INSERT INTO code_runs(id,owner_id,runtime,image,source,stdin,judge_attempt) VALUES ($1,'alice','python314-isolate',$2,'print(1)','',$1)`, codeRunID, old); err != nil {
		t.Fatal(err)
	}
	// Only a contest with participants blocks a rollout; a finished one never does.
	contest := func(id string, starts, ends time.Duration, participant bool) {
		t.Helper()
		_, err := db.Exec(ctx, `INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at) VALUES ($1,'alice','c','d',clock_timestamp()+$2::interval,clock_timestamp()+$3::interval)`,
			id, starts.String(), ends.String())
		if err == nil && participant {
			_, err = db.Exec(ctx, `INSERT INTO contest_participants(contest_id,owner_id) VALUES ($1,'alice')`, id)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	contest("44444444-4444-4444-8444-444444444441", time.Hour, 2*time.Hour, false)
	contest("44444444-4444-4444-8444-444444444442", -3*time.Hour, -time.Hour, true)
	contest("44444444-4444-4444-8444-444444444443", 3*time.Hour, 5*time.Hour, true)
	var jobs [][]byte
	sent := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			data, _ := io.ReadAll(r.Body)
			jobs = append(jobs, data)
			w.Header().Set("x-amz-version-id", "version")
			return
		}
		sent++
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = fmt.Fprint(w, `{"MessageId":"test"}`)
	}))
	defer server.Close()
	cfg := aws.Config{Region: "ap-northeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	b := bridge{
		db: db, bucket: "bucket", queueURL: server.URL + "/queue", runtime: current, previous: old, enabled: "python314", paused: true,
		objects: s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(server.URL); o.UsePathStyle = true }),
		queue:   sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(server.URL) }),
	}
	status := func() map[string]any {
		t.Helper()
		value, err := b.handle(ctx, json.RawMessage(`{"operation":"deployment-status"}`))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(value)
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	got := status()
	if sent != 0 || got["undispatched"] != 5.0 || got["dispatchPaused"] != true || got["previousRuntimeDigest"] != old {
		t.Fatalf("paused dispatch sent requests: sent=%d status=%v", sent, got)
	}
	window, err := time.Parse(time.RFC3339Nano, got["nextContestWindowAt"].(string))
	if err != nil || time.Until(window) < 2*time.Hour || time.Until(window) > 3*time.Hour {
		t.Fatalf("contest window: %v %v", got["nextContestWindowAt"], err)
	}
	b.paused = false
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if sent != 2 || len(jobs) != 2 {
		t.Fatalf("expected the rebound and current submissions only: sent=%d", sent)
	}
	// Both slots are taken, so the code run waits until they are done.
	if _, err := db.Exec(ctx, `UPDATE submissions SET status='DONE' WHERE dispatched_at IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if sent != 3 || len(jobs) != 3 || !bytes.Contains(jobs[2], []byte(`"codeRun":true`)) {
		t.Fatalf("expected the code run next: sent=%d", sent)
	}
	var runImage string
	if err := db.QueryRow(ctx, `SELECT image FROM code_runs WHERE id=$1`, codeRunID).Scan(&runImage); err != nil || runImage != current {
		t.Fatalf("code run image: %s %v", runImage, err)
	}
	for _, job := range jobs {
		if !bytes.Contains(job, []byte(`"runtimeDigest":"`+current+`"`)) {
			t.Fatalf("dispatched with a stale digest: %s", job)
		}
	}
	for name, want := range map[string]string{"rebound": "DONE/" + current, "unpublished": "DONE/" + old, "unknown": "DONE/" + other, "current": "DONE/" + current} {
		var state, image string
		if err := db.QueryRow(ctx, `SELECT status, job->>'image' FROM submissions WHERE id=$1`, submissionsByName[name]).Scan(&state, &image); err != nil {
			t.Fatal(err)
		}
		if state+"/"+image != want {
			t.Fatalf("%s: got %s/%s, want %s", name, state, image, want)
		}
	}
	// Unlike the burst hold, the window has no 3-hour cap and lasts until 15 minutes after the end.
	contest("44444444-4444-4444-8444-444444444444", -4*time.Hour, -10*time.Minute, true)
	window, err = time.Parse(time.RFC3339Nano, status()["nextContestWindowAt"].(string))
	if err != nil || time.Since(window) < 4*time.Hour || time.Since(window) > 5*time.Hour {
		t.Fatalf("contest window without a cap: %v %v", window, err)
	}
}
