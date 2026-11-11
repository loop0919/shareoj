package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

// A worker older than the error-judge design judges a error-judge job as a plain problem, where printing nothing can pass. The bridge
// keeps a error-judge result only with the worker's mark, as it keeps a partial-scoring result only with scores.
func TestErrorJudgeResultsNeedTheWorkerMark(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	const attempt = "22222222-2222-4222-8222-222222222222"
	ids := map[string]string{}
	for i, name := range []string{"unmarked", "marked", "unmarked judge error", "marked plain", "plain", "unmarked scored"} {
		job := submissions.Job{Image: digest, TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{{Name: "01", Input: "1", Output: ""}}}
		if !strings.HasSuffix(name, "plain") {
			job.ErrorJudge = &problems.ErrorJudge{Mode: "absolute", Epsilon: "1e-6"}
		}
		if name == "unmarked scored" {
			job.Scoring = &problems.Scoring{Mode: "groups", Groups: []problems.ScoringGroup{{Name: "g", Points: 100}}}
		}
		raw, _ := json.Marshal(job)
		ids[name] = fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		if _, err := db.Exec(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt)
 VALUES ($1,'alice',$1,1,'test','cpp17-isolate','source',$2,$3)`, ids[name], raw, attempt); err != nil {
			t.Fatal(err)
		}
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	// Paused, so a finished result does not dispatch the other queued submissions.
	b := bridge{db: db, runtime: digest, paused: true}
	one, memory := 1.0, int64(1024)
	accepted := func(marked bool) submissions.Result {
		return submissions.Result{Verdict: "AC", Passed: 1, Total: 1, ErrorJudge: marked,
			Cases: []submissions.CaseResult{{Name: "01", Verdict: "AC", CPUTimeMS: &one, WallTimeMS: &one, MemoryBytes: &memory}}}
	}
	final := func(name string, r submissions.Result) bool {
		t.Helper()
		body, _ := json.Marshal(envelope{ID: ids[name], Attempt: attempt, Result: &r})
		if strings.Contains(string(body), `"errorJudge":true`) != r.ErrorJudge {
			t.Fatal("the mark is not the worker's errorJudge key", string(body))
		}
		return len(b.results(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: name, Body: string(body)}}}).BatchItemFailures) == 0
	}
	stored := func(name string) (string, string, submissions.Result) {
		t.Helper()
		var status string
		var raw []byte
		if err := db.QueryRow(ctx, `SELECT status,COALESCE(result,'null'::jsonb) FROM submissions WHERE id=$1`, ids[name]).Scan(&status, &raw); err != nil {
			t.Fatal(err)
		}
		var r submissions.Result
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(string(raw))
		}
		return status, string(raw), r
	}
	if !final("unmarked", accepted(false)) {
		t.Fatal("unmarked result not handled")
	}
	if status, _, got := stored("unmarked"); status != "DONE" || got.Verdict != "JE" || got.Total != 0 || len(got.Cases) != 0 {
		t.Fatalf("an unmarked error-judge result kept its verdict: %s %+v", status, got)
	}
	if !strings.Contains(logs.String(), `"reason":"unmarked_error_judge_result"`) {
		t.Fatalf("the safety net left no trace: %s", logs.String())
	}
	if !final("marked", accepted(true)) {
		t.Fatal("marked result rejected")
	}
	if status, raw, got := stored("marked"); status != "DONE" || got.Verdict != "AC" || got.Passed != 1 || got.Interactive || strings.Contains(raw, "errorJudge") {
		t.Fatalf("marked result: %s %s", status, raw)
	}
	// A judge error may come without the mark, for example from a worker that failed before judging.
	if !final("unmarked judge error", submissions.Result{Verdict: "JE"}) {
		t.Fatal("judge error rejected")
	}
	if status, _, got := stored("unmarked judge error"); status != "DONE" || got.Verdict != "JE" {
		t.Fatalf("judge error: %s %+v", status, got)
	}
	if n := strings.Count(logs.String(), `"reason":"unmarked_error_judge_result"`); n != 1 {
		t.Fatalf("the safety net fired %d times: %s", n, logs.String())
	}
	// The mark on a plain job's result is invalid, like a score on a job without scoring.
	if final("marked plain", accepted(true)) {
		t.Fatal("a marked result for a plain job was stored")
	}
	if status, _, _ := stored("marked plain"); status == "DONE" {
		t.Fatal("rejected result finished the submission")
	}
	if !final("plain", accepted(false)) {
		t.Fatal("plain result rejected")
	}
	if status, raw, got := stored("plain"); status != "DONE" || got.Verdict != "AC" || got.Interactive || strings.Contains(raw, "errorJudge") {
		t.Fatalf("plain result: %s %s", status, raw)
	}
	// A job that is both error-judge and partial-scoring, unmarked and unscored, is recorded once with the error-judge reason.
	if !final("unmarked scored", accepted(false)) {
		t.Fatal("unmarked scored result not handled")
	}
	if status, _, got := stored("unmarked scored"); status != "DONE" || got.Verdict != "JE" {
		t.Fatalf("unmarked scored result: %s %+v", status, got)
	}
	if n := strings.Count(logs.String(), `"reason":"unmarked_error_judge_result"`); n != 2 || strings.Contains(logs.String(), "unscored_result") {
		t.Fatalf("the safety net fired %d times: %s", n, logs.String())
	}
}
