package submissions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"judge/api/internal/testfiles"
)

func (s *Store) Finish(ctx context.Context, id string, result Result) error {
	return s.FinishAttempt(ctx, id, "", result)
}

// FinishAttempt registers generated files and completes only the current attempt, atomically.
func (s *Store) FinishAttempt(ctx context.Context, id, attempt string, result Result) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner, problem string
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT owner_id,problem_id::text,job FROM submissions
 WHERE id::text=$1 AND status<>'DONE' AND
 (($2='' AND runtime='cpp17-local' AND status='RUNNING') OR (judge_attempt::text=$2 AND runtime=ANY($3::text[]))) FOR UPDATE`, id, attempt, IsolateRuntimeIDs()).Scan(&owner, &problem, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var job Job
	if err = json.Unmarshal(raw, &job); err != nil {
		return err
	}
	result.Interactive = job.Interactor != nil
	total := job.GenerationBaseBytes
	if total < 0 || total > GenerationOutputLimit {
		return testfiles.ErrInvalid
	}
	if job.Generate && result.Verdict == "AC" && (len(result.Cases) != len(job.Cases) || result.Total != len(job.Cases) || result.Passed != result.Total) {
		return testfiles.ErrInvalid
	}
	result.Cases = append([]CaseResult(nil), result.Cases...)
	for i := range result.Cases {
		c := &result.Cases[i]
		if c.SampleDetails != nil && (!job.EasyTest || job.Interactor != nil || job.Generate || job.Validate || i >= len(job.Cases) || c.Name != job.Cases[i].Name) {
			return testfiles.ErrInvalid
		}
		if c.CheckerLog != nil && (!job.EasyTest || job.Interactor == nil || i >= len(job.Cases) || c.SampleDetails != nil) {
			return testfiles.ErrInvalid
		}
		if !job.Generate {
			if c.Output != nil || c.OutputFile != nil {
				return testfiles.ErrInvalid
			}
			continue
		}
		if result.Verdict != "AC" {
			c.Output = nil
			c.OutputFile = nil
			continue
		}
		if c.Verdict != "AC" {
			return testfiles.ErrInvalid
		}
		if c.OutputFile == nil {
			if c.Output == nil || *c.Output != "" {
				return testfiles.ErrInvalid
			}
			continue
		}
		file := *c.OutputFile
		f := &file
		c.OutputFile = f
		if c.Output != nil || !testfiles.ValidGeneratedFile(f) || f.Key != testfiles.GenerationPrefix(owner, problem)+f.ID {
			return testfiles.ErrInvalid
		}
		total += f.Size
		if total > GenerationOutputLimit {
			return testfiles.ErrInvalid
		}
		// Each execution uploads to fresh IDs. A repeated result is ignored by the DONE guard.
		_, err = tx.Exec(ctx, `INSERT INTO test_files(id,owner_id,problem_id,object_key,sha256,size,upload_version_id)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, f.ID, owner, problem, f.Key, f.SHA256, f.Size, f.Version)
		if err != nil {
			return err
		}
		f.Key = ""
		f.Version = ""
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE submissions SET status='DONE',result=$2,progress=NULL,finished_at=clock_timestamp() WHERE id::text=$1`, id, data)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
