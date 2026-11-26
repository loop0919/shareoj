package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/submissions"
)

const requeueCodeRun = "99999999-2222-4222-8222-222222222222"

var requeueDigests = struct{ old, current string }{"sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("b", 64)}

// requeueFixture seeds one in-flight submission (index 0) and the cases that must not change.
func requeueFixture(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	db := migratedDB(t)
	old, current := requeueDigests.old, requeueDigests.current
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	for i, s := range []struct {
		image      string
		dispatched bool
		status     string
	}{
		{old, true, "RUNNING"},
		{current, true, "RUNNING"},
		{old, true, "DONE"},
		{old, false, "QUEUED"},
	} {
		id := fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		raw, _ := json.Marshal(submissions.Job{Image: s.image, TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{{Input: "1", Output: "1"}}})
		if _, err := db.Exec(ctx, `INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt,status,dispatched_at,started_at,progress,rejudged_at,created_at)
 VALUES ($1,'alice',$1,1,'test','python314-isolate','source',$2,$1,$3,CASE WHEN $4 THEN clock_timestamp() END,
  CASE WHEN $3='RUNNING' THEN clock_timestamp() END, CASE WHEN $3='RUNNING' THEN '{"phase":"RUNNING","total":1}'::jsonb END,
  clock_timestamp()-interval '1 hour',clock_timestamp()-interval '1 hour'+make_interval(secs=>$5))`, id, raw, s.status, s.dispatched, i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO code_runs(id,owner_id,runtime,image,source,stdin,judge_attempt,status,dispatched_at) VALUES ($1,'alice','python314-isolate',$2,'print(1)','',$1,'RUNNING',clock_timestamp())`, requeueCodeRun, old); err != nil {
		t.Fatal(err)
	}
	return db
}

func requeueSnapshot(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var s string
	err := db.QueryRow(context.Background(), `SELECT (SELECT COALESCE(string_agg(to_jsonb(x)::text, '|' ORDER BY id),'') FROM submissions x)
 || '#' || (SELECT COALESCE(string_agg(to_jsonb(x)::text, '|' ORDER BY id),'') FROM code_runs x)`).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func requeueCall(b bridge, digest string) (map[string]any, error) {
	body, _ := json.Marshal(map[string]string{"operation": "requeue-digest", "digest": digest})
	value, err := b.handle(context.Background(), body)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(value)
	var got map[string]any
	_ = json.Unmarshal(data, &got)
	return got, nil
}

func TestRequeueDigestReturnsOnlyThePreviousDigestInFlight(t *testing.T) {
	ctx := context.Background()
	db := requeueFixture(t)
	type row struct{ attempt, status, dispatched, created, rejudged string }
	read := func(table string) map[string]row {
		rows, err := db.Query(ctx, `SELECT id::text, judge_attempt::text, status, (dispatched_at IS NULL)::text, created_at::text, `+
			map[string]string{"submissions": "rejudged_at::text", "code_runs": "''"}[table]+` FROM `+table)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]row{}
		for rows.Next() {
			var id string
			var r row
			if err := rows.Scan(&id, &r.attempt, &r.status, &r.dispatched, &r.created, &r.rejudged); err != nil {
				t.Fatal(err)
			}
			out[id] = r
		}
		return out
	}
	beforeS, beforeC := read("submissions"), read("code_runs")
	got, err := requeueCall(bridge{db: db, runtime: requeueDigests.current, previous: requeueDigests.old}, requeueDigests.old)
	if err != nil || got["kind"] != "judge-requeue" || got["requeued"] != float64(2) {
		t.Fatalf("response: %v %v", got, err)
	}
	afterS, afterC := read("submissions"), read("code_runs")
	first := "00000000-1111-4111-8111-111111111111"
	if a := afterS[first]; a.dispatched != "true" || a.status != "QUEUED" || a.attempt == beforeS[first].attempt || a.created != beforeS[first].created || a.rejudged != beforeS[first].rejudged {
		t.Fatalf("submission: before=%v after=%v", beforeS[first], a)
	}
	var progress, started *string
	if err := db.QueryRow(ctx, `SELECT progress::text, started_at::text FROM submissions WHERE id=$1`, first).Scan(&progress, &started); err != nil || progress != nil || started != nil {
		t.Fatalf("progress/started_at: %v %v %v", progress, started, err)
	}
	if a := afterC[requeueCodeRun]; a.dispatched != "true" || a.status != "QUEUED" || a.attempt == beforeC[requeueCodeRun].attempt || a.created != beforeC[requeueCodeRun].created {
		t.Fatalf("code run: before=%v after=%v", beforeC[requeueCodeRun], a)
	}
	for id, b := range beforeS {
		if id != first && afterS[id] != b {
			t.Fatalf("submission %s changed: %v -> %v", id, b, afterS[id])
		}
	}
}

func TestRequeuedSubmissionIsRebound(t *testing.T) {
	ctx := context.Background()
	db := requeueFixture(t)
	fake := newFakeAWS(t)
	b := fake.bridge(db, requeueDigests.current)
	b.previous, b.enabled = requeueDigests.old, "python314"
	if _, err := requeueCall(b, requeueDigests.old); err != nil {
		t.Fatal(err)
	}
	// Free the slots held by the in-flight current-digest submission so everything can be sent.
	if _, err := db.Exec(ctx, `UPDATE submissions SET status='DONE' WHERE job->>'image'=$1 AND dispatched_at IS NOT NULL`, requeueDigests.current); err != nil {
		t.Fatal(err)
	}
	if err := b.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) == 0 {
		t.Fatal("nothing was sent after the requeue")
	}
	for _, job := range fake.puts {
		if !strings.Contains(string(job), `"runtimeDigest":"`+requeueDigests.current+`"`) || strings.Contains(string(job), requeueDigests.old) {
			t.Fatalf("sent with a stale digest: %s", job)
		}
	}
	var image string
	if err := db.QueryRow(ctx, `SELECT job->>'image' FROM submissions WHERE id='00000000-1111-4111-8111-111111111111'`).Scan(&image); err != nil || image != requeueDigests.current {
		t.Fatalf("image: %s %v", image, err)
	}
}

func TestRequeueDigestDropsStaleResults(t *testing.T) {
	ctx := context.Background()
	db := requeueFixture(t)
	const id = "00000000-1111-4111-8111-111111111111"
	if _, err := requeueCall(bridge{db: db, runtime: requeueDigests.current, previous: requeueDigests.old}, requeueDigests.old); err != nil {
		t.Fatal(err)
	}
	// The attempt before the requeue was the submission id itself.
	body, _ := json.Marshal(map[string]any{"submissionId": id, "attemptId": id, "result": map[string]any{"verdict": "JE", "passed": 0, "total": 0}})
	// A valid envelope for a stale attempt is dropped without asking for a retry. A paused bridge does not dispatch on a finished result.
	if response := (bridge{db: db, paused: true}).results(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "late", Body: string(body)}}}); len(response.BatchItemFailures) != 0 {
		t.Fatal(response)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM submissions WHERE id=$1`, id).Scan(&status); err != nil || status != "QUEUED" {
		t.Fatalf("stale result applied: %s %v", status, err)
	}
}

func TestRequeueDigestRejectsOtherDigests(t *testing.T) {
	db := requeueFixture(t)
	before := requeueSnapshot(t, db)
	for name, c := range map[string]struct {
		b      bridge
		digest string
	}{
		"current":        {bridge{db: db, runtime: requeueDigests.current, previous: requeueDigests.old}, requeueDigests.current},
		"empty":          {bridge{db: db, runtime: requeueDigests.current, previous: requeueDigests.old}, ""},
		"malformed":      {bridge{db: db, runtime: requeueDigests.current, previous: requeueDigests.old}, "sha256:zz"},
		"unknown":        {bridge{db: db, runtime: requeueDigests.current, previous: requeueDigests.old}, "sha256:" + strings.Repeat("c", 64)},
		"no previous":    {bridge{db: db, runtime: requeueDigests.current}, requeueDigests.old},
		"empty previous": {bridge{db: db, runtime: requeueDigests.current}, ""},
	} {
		if got, err := requeueCall(c.b, c.digest); err == nil || err.Error() != "invalid operation" {
			t.Fatalf("%s: %v %v", name, got, err)
		}
		if requeueSnapshot(t, db) != before {
			t.Fatalf("%s changed rows", name)
		}
	}
}
