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
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

// The draft is saved through the store, as the API saves it (ADR 0025).
func TestPartialScoringJobsAndResults(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob'),('carol','carol'),('dave','dave')`); err != nil {
		t.Fatal(err)
	}
	const problem = "11111111-1111-4111-8111-111111111111"
	difficulty := 1
	groups := []problems.ScoringGroup{{Name: "小課題1", Points: 30}, {Name: "小課題2", Points: 70}}
	draft := problems.Draft{Title: "部分点", Markdown: "m", Difficulty: &difficulty, TimeLimitMS: "1000", MemoryLimitMB: "256",
		Scoring: &problems.Scoring{Mode: "groups", Groups: groups},
		TestCases: []problems.TestCase{
			{IsSample: true, Name: "sample", Input: "1", Output: "1"},
			{Name: "01", Input: "2", Output: "2", Groups: []string{"小課題1", "小課題2"}},
			{Name: "02", Input: "3", Output: "3", Groups: []string{"小課題2"}},
		}}
	store := problems.New(db)
	publish := func(d problems.Draft) {
		t.Helper()
		current, err := store.Get(ctx, "alice", problem)
		if errors.Is(err, problems.ErrNotFound) {
			current.Version = 0
		} else if err != nil {
			t.Fatal(err)
		}
		saved, err := store.Save(ctx, "alice", problem, current.Version, d)
		if err == nil {
			_, err = store.Publish(ctx, "alice", problem, saved.Version, true)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	publish(draft)
	queue := &submissions.Store{Pool: db}
	checkers := []string{"cpp17", "python314"}
	ids := map[string]string{}
	create := func(name, owner, runtime string, easyTest bool) error {
		t.Helper()
		ids[name] = fmt.Sprintf("%08d-3333-4333-8333-333333333333", len(ids))
		_, err := queue.CreateRun(ctx, submissions.RunInput{Owner: owner, ID: ids[name], ProblemID: problem, Source: "source", Image: digest, Runtime: runtime, EasyTest: easyTest, CheckerRuntimes: checkers})
		return err
	}
	for _, s := range []struct {
		name, owner string
		easyTest    bool
	}{{"valid", "bob", false}, {"sample", "bob", true}, {"unscored", "carol", false}, {"wrong", "carol", false}} {
		if err := create(s.name, s.owner, "cpp17-isolate", s.easyTest); err != nil {
			t.Fatal(s.name, err)
		}
	}
	// The local Docker judge does not score; a sample check never scores, so it still runs there.
	if err := create("local", "dave", "cpp17-local", false); !errors.Is(err, submissions.ErrNotReady) {
		t.Fatal("local judge accepted a partial-scoring job", err)
	}
	if err := create("local sample", "dave", "cpp17-local", true); err != nil {
		t.Fatal(err)
	}
	job := func(name string) (submissions.Job, map[string]json.RawMessage) {
		t.Helper()
		var raw []byte
		if err := db.QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, ids[name]).Scan(&raw); err != nil {
			t.Fatal(name, err)
		}
		var j submissions.Job
		var keys map[string]json.RawMessage
		if json.Unmarshal(raw, &j) != nil || json.Unmarshal(raw, &keys) != nil {
			t.Fatal(name, string(raw))
		}
		return j, keys
	}
	scored, _ := job("valid")
	if scored.Scoring == nil || scored.Scoring.Mode != "groups" || !slices.Equal(scored.Scoring.Groups, groups) || scored.Scoring.Aggregator != nil ||
		len(scored.Cases) != 3 || scored.Cases[0].Groups != nil || !slices.Equal(scored.Cases[1].Groups, []string{"小課題1", "小課題2"}) {
		t.Fatalf("scoring not pinned: %+v %+v", scored.Scoring, scored.Cases)
	}
	for _, name := range []string{"sample", "local sample"} {
		if _, keys := job(name); keys["scoring"] != nil || keys["acceptPoints"] != nil {
			t.Fatal("a sample check carries scoring", name)
		}
	}

	var payloads []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			data, _ := io.ReadAll(r.Body)
			var payload map[string]json.RawMessage
			if json.Unmarshal(data, &payload) != nil {
				t.Error("invalid job payload")
			}
			payloads = append(payloads, payload)
			w.Header().Set("x-amz-version-id", "version")
			return
		}
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
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	sent := map[string]map[string]json.RawMessage{}
	for _, p := range payloads {
		var id string
		_ = json.Unmarshal(p["submissionId"], &id)
		sent[id] = p
	}
	if len(sent) != 4 || !bytes.Equal(sent[ids["valid"]]["scoring"], []byte(`{"mode":"groups","groups":[{"name":"小課題1","points":30},{"name":"小課題2","points":70}]}`)) ||
		!bytes.Contains(sent[ids["valid"]]["cases"], []byte(`"groups":["小課題1","小課題2"]`)) {
		t.Fatalf("scoring lost in dispatch: %s %s", sent[ids["valid"]]["scoring"], sent[ids["valid"]]["cases"])
	}
	for _, key := range []string{"scoring", "acceptPoints"} {
		if _, ok := sent[ids["sample"]][key]; ok {
			t.Fatal("a job without scoring gained the key", key)
		}
	}

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	one, memory := 1.0, int64(1024)
	cases := func(verdicts ...string) []submissions.CaseResult {
		var out []submissions.CaseResult
		for i, v := range verdicts {
			out = append(out, submissions.CaseResult{Name: scored.Cases[i].Name, Verdict: v, CPUTimeMS: &one, WallTimeMS: &one, MemoryBytes: &memory})
		}
		return out
	}
	final := func(name string, r submissions.Result) bool {
		t.Helper()
		var attempt string
		if err := db.QueryRow(ctx, `SELECT judge_attempt::text FROM submissions WHERE id=$1`, ids[name]).Scan(&attempt); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(envelope{ID: ids[name], Attempt: attempt, Result: &r})
		return len(b.results(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: name, Body: string(body)}}}).BatchItemFailures) == 0
	}
	stored := func(name string) (string, *submissions.Result) {
		t.Helper()
		var status string
		var raw []byte
		if err := db.QueryRow(ctx, `SELECT status,result FROM submissions WHERE id=$1`, ids[name]).Scan(&status, &raw); err != nil {
			t.Fatal(err)
		}
		var r *submissions.Result
		if raw != nil && json.Unmarshal(raw, &r) != nil {
			t.Fatal(string(raw))
		}
		return status, r
	}
	// The sample is WA and in no group; 01 is AC; 02 is WA, so only 小課題1 scores.
	result := submissions.Result{Verdict: "WA", Passed: 1, Total: 3, Cases: cases("WA", "AC", "WA"),
		Scoring: &submissions.SubmissionScore{Score: 3000, MaxScore: 10000, Groups: []submissions.GroupScore{{Score: 3000, MaxScore: 3000}, {MaxScore: 7000}}}}
	wrong := result
	wrong.Scoring = &submissions.SubmissionScore{Score: 10000, MaxScore: 10000, Groups: []submissions.GroupScore{{Score: 3000, MaxScore: 3000}, {Score: 7000, MaxScore: 7000}}}
	if final("wrong", wrong) {
		t.Fatal("a groups-mode score that the verdicts do not give was stored")
	}
	if status, _ := stored("wrong"); status == "DONE" {
		t.Fatal("rejected result finished the submission")
	}
	if !final("valid", result) {
		t.Fatal("valid scored result rejected")
	}
	status, got := stored("valid")
	if status != "DONE" || got.Scoring == nil || got.Scoring.Score != 3000 || got.Scoring.Groups[0].Name != "小課題1" ||
		!slices.Equal(got.Scoring.Groups[0].Cases, []int{1}) || !slices.Equal(got.Scoring.Groups[1].Cases, []int{1, 2}) {
		t.Fatalf("scored result: %s %+v", status, got)
	}
	// An old worker ignores scoring and sends its own all-or-nothing shape, here the cutoff after 2 TLEs.
	old := cases("TLE", "TLE", "AC")
	old[2] = submissions.CaseResult{Name: "02", Verdict: "SKIPPED"}
	if !final("unscored", submissions.Result{Verdict: "TLE", Total: 3, Cases: old}) {
		t.Fatal("unscored result not handled")
	}
	if status, got = stored("unscored"); status != "DONE" || got.Verdict != "JE" || got.Scoring != nil || got.Total != 0 {
		t.Fatalf("unscored result kept its verdict: %s %+v", status, got)
	}
	if !strings.Contains(logs.String(), `"reason":"unscored_result"`) || !strings.Contains(logs.String(), `"verdict":"JE"`) {
		t.Fatalf("the safety net left no trace: %s", logs.String())
	}
	// A sample check keeps the old shape.
	sample := submissions.Result{Verdict: "AC", Passed: 1, Total: 1, Cases: []submissions.CaseResult{{Name: "sample", Verdict: "AC", CPUTimeMS: &one, WallTimeMS: &one, MemoryBytes: &memory}}}
	if !final("sample", sample) {
		t.Fatal("sample check rejected")
	}
	sample.Scoring = result.Scoring
	if final("wrong", sample) {
		t.Fatal("a one-case result finished a three-case scored job")
	}

	// A rejudge pins the same scoring block as the submission did.
	queued, err := queue.Rejudge(ctx, submissions.RejudgeInput{Actor: "alice", ProblemID: problem, SubmissionIDs: []string{ids["valid"]}, Image: digest, Runtimes: []string{"cpp17-isolate"}, CheckerRuntimes: checkers})
	if err != nil || queued != 1 {
		t.Fatal(queued, err)
	}
	rejudged, _ := job("valid")
	before, _ := json.Marshal(scored.Scoring)
	after, _ := json.Marshal(rejudged.Scoring)
	if !bytes.Equal(before, after) || !slices.Equal(rejudged.Cases[2].Groups, []string{"小課題2"}) {
		t.Fatalf("rejudge changed scoring: %s %s", before, after)
	}

	// An aggregator must be in a judge language and have source, as a checker must.
	aggregate := draft
	aggregate.Scoring = &problems.Scoring{Mode: "aggregate", Groups: groups, Aggregator: &problems.Generator{Runtime: "cpp23-gcc", Source: "int main(){}"}}
	publish(aggregate)
	if err := create("unknown aggregator", "dave", "cpp17-isolate", false); !errors.Is(err, submissions.ErrNotReady) {
		t.Fatal("aggregator outside the judge languages", err)
	}
	if _, err = queue.Rejudge(ctx, submissions.RejudgeInput{Actor: "alice", ProblemID: problem, SubmissionIDs: []string{ids["unscored"]}, Image: digest, Runtimes: []string{"cpp17-isolate"}, CheckerRuntimes: checkers}); !errors.Is(err, submissions.ErrNotReady) {
		t.Fatal("rejudge with an aggregator outside the judge languages", err)
	}
	aggregate.Scoring.Aggregator.Source = " \n"
	publish(aggregate)
	checkers = append(checkers, "cpp23-gcc")
	if err := create("blank aggregator", "dave", "cpp17-isolate", false); !errors.Is(err, submissions.ErrNotReady) {
		t.Fatal("blank aggregator", err)
	}
	aggregate.Scoring.Aggregator.Source = "int main(){}"
	publish(aggregate)
	if err := create("aggregate", "dave", "cpp17-isolate", false); err != nil {
		t.Fatal(err)
	}
	if j, _ := job("aggregate"); j.Scoring == nil || j.Scoring.Aggregator == nil || j.Scoring.Aggregator.Source != "int main(){}" || j.AcceptPoints {
		t.Fatalf("aggregator not pinned: %+v", j.Scoring)
	}
	// A sample check of an aggregate-mode problem has no scoring, but the worker must still accept quitp there.
	if err := create("aggregate sample", "dave", "cpp17-isolate", true); err != nil {
		t.Fatal(err)
	}
	if j, keys := job("aggregate sample"); keys["scoring"] != nil || !j.AcceptPoints || len(j.Cases) != 1 {
		t.Fatalf("aggregate sample check: %+v", keys)
	}
	payloads = nil
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	sent = map[string]map[string]json.RawMessage{}
	for _, p := range payloads {
		var id string
		_ = json.Unmarshal(p["submissionId"], &id)
		sent[id] = p
	}
	if p := sent[ids["aggregate sample"]]; p == nil || string(p["acceptPoints"]) != "true" || p["scoring"] != nil {
		t.Fatalf("acceptPoints lost in dispatch: %v", p)
	}
	if p := sent[ids["aggregate"]]; p == nil || p["acceptPoints"] != nil || p["scoring"] == nil {
		t.Fatalf("scored aggregate job: %v", p)
	}
	// Without its aggregator, or without groups, scoring waits too: the judge would turn every run into JE.
	aggregate.Scoring.Aggregator = nil
	ungrouped := draft
	ungrouped.Scoring = &problems.Scoring{Mode: "groups"}
	ungrouped.TestCases = []problems.TestCase{{IsSample: true, Name: "sample", Input: "1", Output: "1"}, {Name: "01", Input: "2", Output: "2"}}
	for name, d := range map[string]problems.Draft{"no aggregator": aggregate, "no groups": ungrouped} {
		publish(d)
		if err := create(name, "dave", "cpp17-isolate", false); !errors.Is(err, submissions.ErrNotReady) {
			t.Fatal(name, err)
		}
		if _, err = queue.Rejudge(ctx, submissions.RejudgeInput{Actor: "alice", ProblemID: problem, SubmissionIDs: []string{ids["unscored"]}, Image: digest, Runtimes: []string{"cpp17-isolate"}, CheckerRuntimes: checkers}); !errors.Is(err, submissions.ErrNotReady) {
			t.Fatal("rejudge", name, err)
		}
	}
	// A sample check never scores, so it runs whatever the scoring lacks.
	publish(ungrouped)
	if err := create("sample without groups", "dave", "cpp17-isolate", true); err != nil {
		t.Fatal(err)
	}
}
