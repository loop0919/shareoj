package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"judge/api/internal/submissions"
)

func TestRejudgesWaitBehindNewSubmissions(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	digest := "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(submissions.Job{Image: digest, TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{{Input: "1", Output: "1"}}})
	ids := map[string]string{}
	// Submitted long ago, rejudged a minute ago; a fresh submission; a rejudge left for 7 hours.
	for i, s := range []struct{ name, created, rejudged string }{
		{"rejudge1", "-7 hours", "-1 minute"},
		{"rejudge2", "-7 hours", "-1 minute"},
		{"fresh", "0 seconds", ""},
		{"stale", "-8 hours", "-7 hours"},
	} {
		id := fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		ids[s.name] = id
		_, err := db.Exec(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt,created_at,rejudged_at)
 VALUES ($1,'alice',$1,1,'test','python314-isolate','source',$2,$1,clock_timestamp()+$3::interval,clock_timestamp()+NULLIF($4,'')::interval)`, id, raw, s.created, s.rejudged)
		if err != nil {
			t.Fatal(err)
		}
	}
	var sent []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		sent = append(sent, pointer.ID)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = fmt.Fprint(w, `{"MessageId":"test"}`)
	}))
	defer server.Close()
	cfg := aws.Config{Region: "ap-northeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	b := bridge{
		db: db, bucket: "bucket", queueURL: server.URL + "/queue", runtime: digest,
		objects: s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(server.URL); o.UsePathStyle = true }),
		queue:   sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(server.URL) }),
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	// The fresh submission goes first; one rejudge fills the second in-flight slot.
	if fmt.Sprint(sent) != fmt.Sprint([]string{ids["fresh"], ids["rejudge1"]}) {
		t.Fatalf("dispatch order: %v", sent)
	}
	verdict := func(name string) string {
		t.Helper()
		var v string
		if err := db.QueryRow(ctx, `SELECT status||'/'||COALESCE(result->>'verdict','') FROM submissions WHERE id=$1`, ids[name]).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	// Expiry counts from the rejudge request, so a recent rejudge of an old submission survives.
	if verdict("rejudge2") != "QUEUED/" || verdict("stale") != "DONE/JE" {
		t.Fatalf("expiry: rejudge2=%s stale=%s", verdict("rejudge2"), verdict("stale"))
	}
	// Measured before expiry: the stale rejudge is 7 hours old by its request, 8 by its submission.
	age := -1.0
	for line := range bytes.Lines(logs.Bytes()) {
		var event map[string]any
		if json.Unmarshal(line, &event) == nil && event["oldestPendingSeconds"] != nil {
			age = event["oldestPendingSeconds"].(float64)
		}
	}
	if age < 7*3600 || age > 7.5*3600 {
		t.Fatalf("pending age must count from the rejudge request: %v", age)
	}
	// Only the fresh submission counts toward starting extra hosts.
	d, err := loadDemand(ctx, db, 10)
	if err != nil || d.Queued != 1 {
		t.Fatalf("demand: %+v %v", d, err)
	}
	// A finished judgement frees a slot and sends the waiting rejudge without a schedule tick.
	one := 1.0
	memory := int64(1)
	result := submissions.Result{Verdict: "AC", Passed: 1, Total: 1, Cases: []submissions.CaseResult{{Name: "ケース1", Verdict: "AC", CPUTimeMS: &one, WallTimeMS: &one, MemoryBytes: &memory}}}
	body, _ := json.Marshal(envelope{ID: ids["fresh"], Attempt: ids["fresh"], Result: &result})
	if response := b.results(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "one", Body: string(body)}}}); len(response.BatchItemFailures) != 0 {
		t.Fatal(response)
	}
	if verdict("fresh") != "DONE/AC" || len(sent) != 3 || sent[2] != ids["rejudge2"] {
		t.Fatalf("waiting rejudge not sent after a result: fresh=%s sent=%v", verdict("fresh"), sent)
	}
}
