package main

import (
	"context"
	"encoding/json"
	"errors"
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

// validResult checks a result's own shape. Rules that need the job, such as which cases a partial-scoring job may
// skip and the score of a groups-mode problem, are checked by FinishAttempt (ADR 0025).
func validResult(r submissions.Result) bool {
	allowed := map[string]bool{"AC": true, "WA": true, "TLE": true, "MLE": true, "OLE": true, "RE": true, "CE": true, "JE": true}
	if !allowed[r.Verdict] || r.Total < 0 || r.Total > 100 || r.Passed < 0 || r.Passed > r.Total || len(r.CompileLog) > 196608 || len(r.CheckerLog) > 16384 {
		return false
	}
	if r.Scoring != nil && !validScore(*r.Scoring, r.Verdict) {
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
			// All-or-nothing judging skips only the cases after the second TLE. Partial scoring also skips by group and
			// when its time budget runs out, anywhere in the list.
			if (r.Scoring == nil && tleCount < 2) || c.CPUTimeMS != nil || c.WallTimeMS != nil || c.MemoryBytes != nil ||
				c.Output != nil || c.OutputFile != nil || c.SampleDetails != nil || c.CheckerLog != nil || c.Score != nil {
				return false
			}
			skipped = true
			continue
		}
		if skipped && r.Scoring == nil {
			return false
		}
		// Only an accepted or wrong output has a case score, and only in a partial-scoring result.
		if c.Score != nil && (r.Scoring == nil || (c.Verdict != "AC" && c.Verdict != "WA") || *c.Score < 0 || *c.Score > submissions.MaxCaseScore) {
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

// validScore checks the sums and bounds of a score in hundredths. A judge error has no score, and a compile error
// scores 0 in every group.
func validScore(s submissions.SubmissionScore, verdict string) bool {
	if verdict == "JE" || len(s.Groups) == 0 || len(s.Groups) > 100 || s.MaxScore < 100 || s.MaxScore > 100*1_000_000 {
		return false
	}
	var score, maxScore int64
	for _, g := range s.Groups {
		if g.MaxScore < 0 || g.MaxScore > 100*1_000_000 || g.MaxScore%100 != 0 || g.Score < 0 || g.Score > g.MaxScore || (verdict == "CE" && g.Score != 0) {
			return false
		}
		score += g.Score
		maxScore += g.MaxScore
	}
	return score == s.Score && maxScore == s.MaxScore
}

// validCodeRunResult checks the fields each verdict may carry. A code run is not compared with an expected output, so
// a finished run has its measurements and both outputs, a compile error only its log, and a judge error nothing.
// Expiry is set by the bridge itself, never by a worker.
func validCodeRunResult(r submissions.CodeRunResult) bool {
	if r.Expired || len(r.CompileLog) > 196608 {
		return false
	}
	switch r.Verdict {
	case "JE":
		return r.ExitCode == nil && r.CPUTimeMS == nil && r.WallTimeMS == nil && r.MemoryBytes == nil && r.Stdout == nil && r.Stderr == nil && r.CompileLog == ""
	case "CE":
		return r.ExitCode == nil && r.CPUTimeMS == nil && r.WallTimeMS == nil && r.MemoryBytes == nil && r.Stdout == nil && r.Stderr == nil
	case "AC", "RE", "TLE", "MLE", "OLE":
	default:
		return false
	}
	if r.CompileLog != "" || r.ExitCode == nil || r.CPUTimeMS == nil || r.WallTimeMS == nil || r.MemoryBytes == nil || r.Stdout == nil || r.Stderr == nil {
		return false
	}
	if *r.ExitCode < 0 || *r.ExitCode > 255 || (r.Verdict == "AC" && *r.ExitCode != 0) {
		return false
	}
	if *r.CPUTimeMS < 0 || *r.CPUTimeMS > 120000 || *r.WallTimeMS < 0 || *r.WallTimeMS > 120000 || *r.MemoryBytes < 0 || *r.MemoryBytes > 4<<30 {
		return false
	}
	for _, preview := range []*submissions.TextPreview{r.Stdout, r.Stderr} {
		if len(preview.Text) > 65536 || strings.ContainsRune(preview.Text, 0) {
			return false
		}
	}
	return true
}

// codeRunEnvelope is a worker message about a code run. It never carries progress.
type codeRunEnvelope struct {
	ID       string                     `json:"submissionId"`
	Attempt  string                     `json:"attemptId"`
	Result   *submissions.CodeRunResult `json:"result"`
	Progress json.RawMessage            `json:"progress"`
}

func (b bridge) results(ctx context.Context, event events.SQSEvent) events.SQSEventResponse {
	response := events.SQSEventResponse{}
	finished := false
	for _, record := range event.Records {
		started := time.Now()
		// A code run has its own result shape and table, so it is read and stored apart from submissions.
		var kind struct {
			CodeRun bool `json:"codeRun"`
		}
		if len(record.Body) <= 256<<10 && json.Unmarshal([]byte(record.Body), &kind) == nil && kind.CodeRun {
			var c codeRunEnvelope
			// An empty attempt is refused because FinishCodeRun would treat it as a local run.
			if json.Unmarshal([]byte(record.Body), &c) != nil || c.Attempt == "" || c.Result == nil || c.Progress != nil || !validCodeRunResult(*c.Result) {
				observe("failure", "platform", "invalid_result_envelope", "", "")
				response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
				continue
			}
			if err := (&submissions.Store{Pool: b.db}).FinishCodeRun(ctx, c.ID, c.Attempt, *c.Result); err != nil {
				observe("failure", "platform", "result_apply_failed", c.ID, c.Attempt)
				response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
				continue
			}
			finished = true
			observe("result_processed", "", "", c.ID, c.Attempt, "verdict", c.Result.Verdict, "durationMs", time.Since(started).Milliseconds())
			continue
		}
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
			store := &submissions.Store{Pool: b.db}
			err = store.FinishAttempt(ctx, e.ID, e.Attempt, *e.Result)
			if errors.Is(err, submissions.ErrUnscored) {
				// A judge older than ADR 0025 judged a partial-scoring job as all-or-nothing. Its verdict says nothing
				// about the score, so the submission becomes JE rather than silently all-or-nothing.
				observe("failure", "platform", "unscored_result", e.ID, e.Attempt)
				e.Result = &submissions.Result{Verdict: "JE"}
				err = store.FinishAttempt(ctx, e.ID, e.Attempt, *e.Result)
			}
		}
		if err == nil && e.Result != nil && e.Result.Verdict == "JE" {
			// A worker older than ADR 0031 answers a code-run job with a submission JE. This matches only a
			// pending isolate code run with this ID and attempt, so it does nothing for a real submission.
			err = (&submissions.Store{Pool: b.db}).FinishCodeRun(ctx, e.ID, e.Attempt, submissions.CodeRunResult{Verdict: "JE"})
		}
		if err != nil {
			observe("failure", "platform", "result_apply_failed", e.ID, e.Attempt)
			response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
		}
		if err == nil && e.Result != nil {
			finished = true
			observe("result_processed", "", "", e.ID, e.Attempt, "verdict", e.Result.Verdict, "durationMs", time.Since(started).Milliseconds())
		}
	}
	// A finished judgement frees capacity. Send waiting rejudges now instead of on the next tick.
	if finished && !b.paused {
		if err := b.send(ctx); err != nil {
			observe("failure", "platform", "dispatch_failed", "", "")
		}
	}
	return response
}
