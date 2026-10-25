package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/problems"
)

// Code runs execute a source on a typed input without a problem (ADR 0031). The limits are fixed.
const (
	CodeRunInputLimit    = 1 << 20
	CodeRunTimeLimitMS   = 5000
	CodeRunMemoryLimitMB = 512
	CodeRunPreviewLimit  = 64 << 10
	codeRunWindowSeconds = 60
	codeRunLimit         = 3
)

var ErrCodeRunNotFound = errors.New("code run not found")

type CodeRunResult struct {
	Verdict     string       `json:"verdict"`
	Expired     bool         `json:"expired,omitempty"`
	ExitCode    *int         `json:"exitCode,omitempty"`
	CPUTimeMS   *int64       `json:"cpuTimeMs,omitempty"`
	WallTimeMS  *int64       `json:"wallTimeMs,omitempty"`
	MemoryBytes *int64       `json:"memoryBytes,omitempty"`
	Stdout      *TextPreview `json:"stdout,omitempty"`
	Stderr      *TextPreview `json:"stderr,omitempty"`
	CompileLog  string       `json:"compileLog,omitempty"`
}

type CodeRun struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Result    *CodeRunResult `json:"result"`
	CreatedAt time.Time      `json:"createdAt"`
}

type CodeRunInput struct {
	Runtime string `json:"runtime"`
	Source  string `json:"source"`
	Stdin   string `json:"stdin"`
}

type CodeRunJob struct{ ID, Attempt, Runtime, Image, Source, Stdin string }

// CreateCodeRun counts only code runs against the limit, under the same account lock as submissions.
func (s *Store) CreateCodeRun(ctx context.Context, owner, id, runtime, image, source, stdin string) (CodeRun, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return CodeRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 709091002))`, owner); err != nil {
		return CodeRun{}, err
	}
	var retryAfter int
	err = tx.QueryRow(ctx, `SELECT COALESCE(CEIL(EXTRACT(EPOCH FROM
		(created_at + $2::int * interval '1 second' - clock_timestamp())))::int, 0)
		FROM (SELECT (SELECT created_at FROM code_runs
		WHERE owner_id=$1 AND created_at > clock_timestamp() - $2::int * interval '1 second'
		ORDER BY created_at DESC LIMIT 1 OFFSET $3) AS created_at) recent`, owner, codeRunWindowSeconds, codeRunLimit-1).Scan(&retryAfter)
	if err != nil {
		return CodeRun{}, err
	}
	if retryAfter > 0 {
		return CodeRun{}, &RateLimitError{RetryAfter: retryAfter}
	}
	item := CodeRun{ID: id, Status: "QUEUED"}
	if err = tx.QueryRow(ctx, `INSERT INTO code_runs(id,owner_id,runtime,image,source,stdin) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`,
		id, owner, runtime, image, source, stdin).Scan(&item.CreatedAt); err != nil {
		return CodeRun{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Store) LatestCodeRun(ctx context.Context, owner string) (CodeRun, error) {
	var item CodeRun
	var raw []byte
	err := s.Pool.QueryRow(ctx, `SELECT id::text,status,result,created_at FROM code_runs WHERE owner_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, owner).
		Scan(&item.ID, &item.Status, &raw, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CodeRun{}, ErrCodeRunNotFound
	}
	if err != nil {
		return CodeRun{}, err
	}
	if raw != nil {
		item.Result = &CodeRunResult{}
		err = json.Unmarshal(raw, item.Result)
	}
	return item, err
}

// ClaimCodeRun takes the oldest queued run of a local runtime. The local worker finishes it with an empty attempt.
func (s *Store) ClaimCodeRun(ctx context.Context) (CodeRunJob, error) {
	var job CodeRunJob
	err := s.Pool.QueryRow(ctx, `UPDATE code_runs SET status='RUNNING',dispatched_at=clock_timestamp()
		WHERE id=(SELECT id FROM code_runs WHERE runtime LIKE '%-local' AND status='QUEUED' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id::text,runtime,image,source,stdin`).Scan(&job.ID, &job.Runtime, &job.Image, &job.Source, &job.Stdin)
	return job, err
}

// FinishCodeRun completes only the current attempt, once. A stale or repeated result changes nothing.
func (s *Store) FinishCodeRun(ctx context.Context, id, attempt string, result CodeRunResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE code_runs SET status='DONE',result=$3,finished_at=clock_timestamp(),source='',stdin=''
		WHERE id::text=$1 AND status<>'DONE' AND
		(($2='' AND runtime LIKE '%-local' AND status='RUNNING') OR (judge_attempt::text=$2 AND runtime LIKE '%-isolate'))`, id, attempt, raw)
	return err
}

// CreateCodeRun validates the run like a submission without a problem, then stores it and wakes the dispatcher.
func (s Service) CreateCodeRun(ctx context.Context, owner, id string, input CodeRunInput) (CodeRun, error) {
	if s.Submissions == nil {
		return CodeRun{}, ErrUnavailable
	}
	if err := s.Ready(); err != nil {
		return CodeRun{}, err
	}
	selected := s.storedRuntime(input.Runtime)
	if selected == "" || !problems.HasSource(selected, input.Source) || len(input.Source) > 128<<10 || !utf8.ValidString(input.Source) || strings.ContainsRune(input.Source, 0) ||
		len(input.Stdin) > CodeRunInputLimit || !utf8.ValidString(input.Stdin) || strings.ContainsRune(input.Stdin, 0) {
		return CodeRun{}, ErrInvalid
	}
	stdin := strings.ReplaceAll(strings.ReplaceAll(input.Stdin, "\r\n", "\n"), "\r", "\n")
	item, err := s.Submissions.CreateCodeRun(ctx, owner, id, selected, s.JudgeImage, input.Source, stdin)
	if err != nil {
		return CodeRun{}, err
	}
	if !s.local() && s.DispatchJudge != nil {
		// The committed row and periodic dispatcher recover a failed wake-up.
		wakeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := s.DispatchJudge(wakeCtx)
		cancel()
		if err != nil {
			slog.Warn("immediate judge dispatch unavailable; periodic recovery retained", "codeRunId", item.ID)
		}
	}
	return item, nil
}

// ExpireCodeRuns ends runs the judge never took within 10 minutes (the user may run again) and runs it never
// finished within an hour, then deletes rows a day after they were accepted. Only the first two are counted.
func (s *Store) ExpireCodeRuns(ctx context.Context, runtimes []string) (int64, error) {
	unsent, err := s.Pool.Exec(ctx, `UPDATE code_runs SET status='DONE',finished_at=clock_timestamp(),source='',stdin='',result='{"verdict":"JE","expired":true}'
		WHERE runtime=ANY($1::text[]) AND dispatched_at IS NULL AND status <> 'DONE' AND created_at < clock_timestamp()-interval '10 minutes'`, runtimes)
	if err != nil {
		return 0, err
	}
	stuck, err := s.Pool.Exec(ctx, `UPDATE code_runs SET status='DONE',finished_at=clock_timestamp(),source='',stdin='',result='{"verdict":"JE"}'
		WHERE runtime=ANY($1::text[]) AND dispatched_at < clock_timestamp()-interval '1 hour' AND status <> 'DONE'`, runtimes)
	if err != nil {
		return 0, err
	}
	if _, err = s.Pool.Exec(ctx, `DELETE FROM code_runs WHERE created_at < clock_timestamp()-interval '1 day'`); err != nil {
		return 0, err
	}
	return unsent.RowsAffected() + stuck.RowsAffected(), nil
}
