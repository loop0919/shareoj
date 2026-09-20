package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

func validProgress(p submissions.Progress) bool {
	if p.Verdict != "" {
		switch p.Verdict {
		case "WA", "TLE", "MLE", "OLE", "RE":
		default:
			return false
		}
		if p.Phase != "JUDGING" || p.Completed == 0 {
			return false
		}
	}
	return p.Total >= 1 && p.Total <= 100 && p.Completed >= 0 && p.Completed <= p.Total &&
		(p.Phase == "JUDGING" || (p.Phase == "PREPARING" && p.Completed == 0))
}

func validResult(r submissions.Result) bool {
	allowed := map[string]bool{"AC": true, "WA": true, "TLE": true, "MLE": true, "OLE": true, "RE": true, "CE": true, "JE": true}
	if !allowed[r.Verdict] || r.Total < 0 || r.Total > 100 || r.Passed < 0 || r.Passed > r.Total || len(r.CompileLog) > 196608 || len(r.CheckerLog) > 16384 {
		return false
	}
	if r.Verdict == "JE" || r.Verdict == "CE" {
		return r.Passed == 0 && len(r.Cases) == 0
	}
	if len(r.Cases) != r.Total || r.Total == 0 {
		return false
	}
	var generatedBytes int64
	passed := 0
	verdict := "AC"
	previewBytes := 0
	tleCount := 0
	skipped := false
	for _, c := range r.Cases {
		if len(c.Name) > 256 {
			return false
		}
		if c.Verdict == "SKIPPED" {
			if tleCount < 2 || c.CPUTimeMS != nil || c.WallTimeMS != nil || c.MemoryBytes != nil ||
				c.Output != nil || c.OutputFile != nil || c.SampleDetails != nil || c.CheckerLog != nil {
				return false
			}
			skipped = true
			continue
		}
		if skipped {
			return false
		}
		if c.CheckerLog != nil {
			if c.SampleDetails != nil || len(c.CheckerLog.Text) > 4096 || strings.ContainsRune(c.CheckerLog.Text, 0) {
				return false
			}
			previewBytes += len(c.CheckerLog.Text)
		}
		if c.SampleDetails != nil {
			for _, preview := range []submissions.TextPreview{c.SampleDetails.Input, c.SampleDetails.ExpectedOutput, c.SampleDetails.ActualOutput} {
				if len(preview.Text) > 4096 || strings.ContainsRune(preview.Text, 0) {
					return false
				}
				previewBytes += len(preview.Text)
			}
		}
		if previewBytes > submissions.SamplePreviewBudget {
			return false
		}
		if c.Output != nil && (c.Verdict != "AC" || *c.Output != "" || c.OutputFile != nil) {
			return false
		}
		if c.OutputFile != nil {
			if c.Verdict != "AC" || !testfiles.ValidGeneratedFile(c.OutputFile) {
				return false
			}
			generatedBytes += c.OutputFile.Size
			if generatedBytes > submissions.GenerationOutputLimit {
				return false
			}
		}
		if !allowed[c.Verdict] || c.Verdict == "CE" || c.Verdict == "JE" || c.CPUTimeMS == nil || c.WallTimeMS == nil || c.MemoryBytes == nil {
			return false
		}
		if *c.CPUTimeMS < 0 || *c.CPUTimeMS > 120000 || *c.WallTimeMS < 0 || *c.WallTimeMS > 120000 || *c.MemoryBytes < 0 || *c.MemoryBytes > 4<<30 {
			return false
		}
		if c.Verdict == "TLE" {
			tleCount++
		}
		if c.Verdict == "AC" {
			passed++
		} else if verdict == "AC" {
			verdict = c.Verdict
		}
	}
	return passed == r.Passed && verdict == r.Verdict
}

func (b bridge) results(ctx context.Context, event events.SQSEvent) events.SQSEventResponse {
	response := events.SQSEventResponse{}
	for _, record := range event.Records {
		started := time.Now()
		var e envelope
		if len(record.Body) > 256<<10 || json.Unmarshal([]byte(record.Body), &e) != nil ||
			(e.Result == nil) == (e.Progress == nil) ||
			(e.Result != nil && !validResult(*e.Result)) || (e.Progress != nil && !validProgress(*e.Progress)) {
			observe("failure", "platform", "invalid_result_envelope", "", "")
			response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
			continue
		}
		var err error
		if e.Progress != nil {
			p := e.Progress
			raw, _ := json.Marshal(p)
			// Standard SQS may reorder or duplicate events, including across worker retries.
			// Only the current attempt may advance; a final result always wins.
			_, err = b.db.Exec(ctx, `UPDATE submissions SET status='RUNNING',progress=$3,
    started_at=COALESCE(started_at,clock_timestamp())
    WHERE id::text=$1 AND judge_attempt::text=$2 AND runtime=ANY($4::text[]) AND status <> 'DONE'
    AND jsonb_array_length(job->'cases')=$5
    AND (progress IS NULL OR ($6='JUDGING' AND
      (progress->>'phase'='PREPARING' OR (progress->>'completed')::int < $7)))`,
				e.ID, e.Attempt, raw, submissions.IsolateRuntimeIDs(), p.Total, p.Phase, p.Completed)
		} else {
			err = (&submissions.Store{Pool: b.db}).FinishAttempt(ctx, e.ID, e.Attempt, *e.Result)
		}
		if err != nil {
			observe("failure", "platform", "result_apply_failed", e.ID, e.Attempt)
			response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
		}
		if err == nil && e.Result != nil {
			observe("result_processed", "", "", e.ID, e.Attempt, "verdict", e.Result.Verdict, "durationMs", time.Since(started).Milliseconds())
		}
	}
	return response
}
