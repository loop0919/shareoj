// Package submissions stores local-development C++ submissions and their immutable inputs.
package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"judge/api/internal/problems"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotReady = errors.New("problem is not ready for judging")

type Case = problems.TestCase

const GenerationOutputLimit = 512 << 20

type Job struct {
	EasyTest            bool                                                      `json:"easyTest,omitempty"`
	Interactor          *problems.Generator                                       `json:"interactor,omitempty"`
	Checker             *problems.Generator                                       `json:"checker,omitempty"`
	GenerationBaseBytes int64                                                     `json:"generationBaseBytes,omitempty"`
	GenerationPrefix    string                                                    `json:"generationPrefix,omitempty"`
	SaveOutput          func(context.Context, []byte) (*problems.TestFile, error) `json:"-"`
	LoadFile            func(context.Context, *problems.TestFile) (string, error) `json:"-"`
	Validate            bool                                                      `json:"validate,omitempty"`
	Generate            bool                                                      `json:"generate,omitempty"`
	Image               string                                                    `json:"image"`
	TimeLimitMS         int                                                       `json:"timeLimitMs"`
	MemoryLimitMB       int                                                       `json:"memoryLimitMb"`
	Cases               []Case                                                    `json:"cases"`
}

// Sample previews share a 24 KiB budget so escaped JSON fits in the result queue.
const SamplePreviewBudget = 24 << 10

type TextPreview struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

type SampleDetails struct {
	Input          TextPreview `json:"input"`
	ExpectedOutput TextPreview `json:"expectedOutput"`
	ActualOutput   TextPreview `json:"actualOutput"`
}

type CaseResult struct {
	CheckerLog    *TextPreview       `json:"checkerLog,omitempty"`
	SampleDetails *SampleDetails     `json:"sampleDetails,omitempty"`
	OutputFile    *problems.TestFile `json:"outputFile,omitempty"`
	Output        *string            `json:"output,omitempty"`
	Name          string             `json:"name"`
	Verdict       string             `json:"verdict"`
	CPUTimeMS     *float64           `json:"cpuTimeMs,omitempty"`
	WallTimeMS    *float64           `json:"wallTimeMs,omitempty"`
	MemoryBytes   *int64             `json:"memoryBytes,omitempty"`
}

type Result struct {
	CPUTimeMS   *float64     `json:"cpuTimeMs,omitempty"`
	MemoryBytes *int64       `json:"memoryBytes,omitempty"`
	Interactive bool         `json:"interactive,omitempty"`
	CheckerLog  string       `json:"checkerLog,omitempty"`
	Cases       []CaseResult `json:"cases,omitempty"`
	Verdict     string       `json:"verdict"`
	Passed      int          `json:"passed"`
	Total       int          `json:"total"`
	CompileLog  string       `json:"compileLog,omitempty"`
}

// Summaries use the maximum measured value across cases, not the sum.
func (r *Result) summarizeUsage() {
	r.CPUTimeMS, r.MemoryBytes = nil, nil
	for _, c := range r.Cases {
		if c.CPUTimeMS != nil && (r.CPUTimeMS == nil || *c.CPUTimeMS > *r.CPUTimeMS) {
			r.CPUTimeMS = c.CPUTimeMS
		}
		if c.MemoryBytes != nil && (r.MemoryBytes == nil || *c.MemoryBytes > *r.MemoryBytes) {
			r.MemoryBytes = c.MemoryBytes
		}
	}
}

type Progress struct {
	Verdict   string `json:"verdict,omitempty"`
	Phase     string `json:"phase"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

type Submission struct {
	Author         string    `json:"author"`
	ContestID      string    `json:"contestId,omitempty"`
	EasyTest       bool      `json:"easyTest"`
	ID             string    `json:"id"`
	ProblemID      string    `json:"problemId"`
	ProblemVersion int64     `json:"problemVersion"`
	ProblemTitle   string    `json:"problemTitle"`
	Runtime        string    `json:"runtime"`
	Source         string    `json:"source,omitempty"`
	Status         string    `json:"status"`
	Result         *Result   `json:"result"`
	Progress       *Progress `json:"progress"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Store struct{ Pool *pgxpool.Pool }

const columns = `id,problem_id,problem_version,problem_title,runtime,source,status,result,created_at,progress,COALESCE((job->>'easyTest')::boolean,false),COALESCE(contest_id::text,''),(SELECT handle FROM user_profiles WHERE user_profiles.owner_id=submissions.owner_id)`

func scan(row pgx.Row) (Submission, error) {
	var s Submission
	var result, progress []byte
	err := row.Scan(&s.ID, &s.ProblemID, &s.ProblemVersion, &s.ProblemTitle, &s.Runtime, &s.Source, &s.Status, &result, &s.CreatedAt, &progress, &s.EasyTest, &s.ContestID, &s.Author)
	if err == nil && result != nil {
		err = json.Unmarshal(result, &s.Result)
		if err == nil && s.Result != nil {
			s.Result.summarizeUsage()
		}
	}
	if err == nil && progress != nil {
		err = json.Unmarshal(progress, &s.Progress)
	}
	return s, err
}

// Create pins the published version, or the owner's unpublished draft, at insertion.
func (s *Store) Create(ctx context.Context, owner, id, problemID, source, image string) (Submission, error) {
	return s.CreateRuntime(ctx, owner, id, problemID, source, image, "cpp17-local")
}

func (s *Store) CreateRuntime(ctx context.Context, owner, id, problemID, source, image, runtime string, checkerRuntimes ...string) (Submission, error) {
	return s.CreateTestRun(ctx, owner, id, problemID, source, image, runtime, false, checkerRuntimes...)
}

// CreateTestRun selects sample cases inside the same statement that pins the problem version.
func (s *Store) CreateTestRun(ctx context.Context, owner, id, problemID, source, image, runtime string, easyTest bool, checkerRuntimes ...string) (Submission, error) {
	return s.createTestRun(ctx, owner, id, problemID, source, image, runtime, easyTest, "", checkerRuntimes...)
}

func (s *Store) CreateContestRun(ctx context.Context, owner, id, problemID, source, image, runtime string, easyTest bool, contestID string, checkerRuntimes ...string) (Submission, error) {
	return s.createTestRun(ctx, owner, id, problemID, source, image, runtime, easyTest, contestID, checkerRuntimes...)
}

func (s *Store) createTestRun(ctx context.Context, owner, id, problemID, source, image, runtime string, easyTest bool, contestID string, checkerRuntimes ...string) (Submission, error) {
	if len(checkerRuntimes) == 0 {
		checkerRuntimes = []string{"cpp17"}
	}
	tx, err := s.beginSubmission(ctx, owner, easyTest)
	if err != nil {
		return Submission{}, err
	}
	defer tx.Rollback(ctx)
	query := tx.QueryRow
	if contestID != "" {
		var locked string
		err = tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR SHARE`, contestID).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return Submission{}, ErrNotReady
		}
		if err != nil {
			return Submission{}, err
		}
	}
	result, err := scan(query(ctx, `WITH moment AS MATERIALIZED (SELECT clock_timestamp() AS now)
 INSERT INTO submissions
  (id,owner_id,problem_id,problem_version,problem_title,runtime,source,contest_id,created_at,job)
  SELECT $1,$2,id,selected_version,selected_draft->>'title',$6,$4,NULLIF($9,'')::uuid,moment.now,
  jsonb_build_object('privateDraft',($9='' AND NOT EXISTS(SELECT 1 FROM problem_drafts p WHERE p.id=$3 AND p.published_draft IS NOT NULL)),'image',$5::text,'easyTest',$8::boolean,'cases',selected_cases,'checker',selected_draft->'checker','interactor',selected_draft->'interactor',
  'timeLimitMs',(selected_draft->>'timeLimitMs')::int,
  'memoryLimitMb',(selected_draft->>'memoryLimitMb')::int)
  FROM (
    SELECT id, COALESCE(published_draft,draft) AS selected_draft,
      CASE WHEN published_draft IS NULL THEN version ELSE published_version END AS selected_version
    FROM problem_drafts WHERE $9='' AND id=$3 AND (published_draft IS NOT NULL OR can_manage_problem(id,$2))
    UNION ALL
    SELECT cp.problem_id,cp.draft,cp.problem_version FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id CROSS JOIN moment
    WHERE $9<>'' AND c.id=NULLIF($9,'')::uuid AND cp.problem_id=$3 AND (c.published OR c.owner_id=$2) AND
    (moment.now>=c.starts_at OR c.owner_id=$2 OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=cp.problem_id AND t.owner_id=$2))
  ) problem CROSS JOIN moment
  CROSS JOIN LATERAL (
    SELECT COALESCE(jsonb_agg(c ORDER BY ordinal), '[]'::jsonb) AS selected_cases
    FROM jsonb_array_elements(COALESCE(selected_draft->'testCases','[]'::jsonb)) WITH ORDINALITY AS cases(c, ordinal)
    WHERE NOT $8::boolean OR c->'isSample' = 'true'::jsonb
  ) tests
  WHERE jsonb_array_length(selected_cases) > 0 AND jsonb_array_length(COALESCE(selected_draft->'testCases','[]'::jsonb)) > 0
  AND jsonb_array_length(COALESCE(selected_draft->'testCases','[]'::jsonb)) <= 100
  AND ($6 = 'cpp17-local' OR (selected_draft->>'memoryLimitMb')::int = 512)
  AND (COALESCE(selected_draft->'checker','null'::jsonb) = 'null'::jsonb OR
    (selected_draft->'checker'->>'runtime'=ANY($7::text[]) AND
     length(btrim(selected_draft->'checker'->>'source', E' \t\r\n'))>0 AND
     ($6 <> 'cpp17-local' OR selected_draft->'checker'->>'runtime'='cpp17')))
  AND (COALESCE(selected_draft->'interactor','null'::jsonb) = 'null'::jsonb OR
    ($6 <> 'cpp17-local' AND COALESCE(selected_draft->'checker','null'::jsonb) = 'null'::jsonb AND
     selected_draft->'interactor'->>'runtime'=ANY($7::text[]) AND
     length(btrim(selected_draft->'interactor'->>'source', E' \t\r\n'))>0))
  RETURNING `+columns, id, owner, problemID, source, image, runtime, checkerRuntimes, easyTest, contestID))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotReady
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return result, err
}

func (s *Store) Get(ctx context.Context, owner, id string) (Submission, error) {
	// Checker diagnostics can contain private tests and checker source.
	privateColumns := strings.Replace(columns, "result,", `CASE WHEN (COALESCE((job->>'easyTest')::boolean,false) AND job->'interactor' IS NOT NULL AND job->'interactor' <> 'null'::jsonb) OR EXISTS(SELECT 1 FROM problem_drafts p WHERE p.id=submissions.problem_id AND can_manage_problem(p.id,$2)) THEN result ELSE result-'checkerLog' END,`, 1)
	return scan(s.Pool.QueryRow(ctx, `SELECT `+privateColumns+` FROM submissions WHERE id=$1 AND owner_id=$2`, id, owner))
}

func (s *Store) List(ctx context.Context, owner string) ([]Submission, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+columns+` FROM submissions WHERE owner_id=$1 AND NOT COALESCE((job->>'easyTest')::boolean,false) AND NOT COALESCE((job->>'generate')::boolean,false) AND NOT COALESCE((job->>'validate')::boolean,false) ORDER BY created_at DESC,id DESC LIMIT 50`, owner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Submission, error) {
		item, err := scan(row)
		item.Source = ""
		if item.Result != nil {
			item.Result.CheckerLog = ""
		}
		return item, err
	})
}

func (s *Store) Claim(ctx context.Context) (Submission, Job, error) {
	// ponytail: one attempt; interrupted jobs become JE after the judge deadline. Add retry attempts for production.
	_, err := s.Pool.Exec(ctx, `UPDATE submissions SET status='DONE',finished_at=clock_timestamp(),result='{"verdict":"JE","passed":0,"total":0}' WHERE runtime='cpp17-local' AND status='RUNNING' AND started_at < clock_timestamp()-$1::int * interval '1 second'`, int((JudgeTimeout+time.Minute)/time.Second))
	if err != nil {
		return Submission{}, Job{}, err
	}
	var id string
	var raw []byte
	err = s.Pool.QueryRow(ctx, `UPDATE submissions SET status='RUNNING',started_at=clock_timestamp()
		WHERE id=(SELECT id FROM submissions WHERE runtime='cpp17-local' AND status='QUEUED' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id,job`).Scan(&id, &raw)
	if err != nil {
		return Submission{}, Job{}, err
	}
	item, err := scan(s.Pool.QueryRow(ctx, `SELECT `+columns+` FROM submissions WHERE id=$1`, id))
	var job Job
	if err == nil {
		err = json.Unmarshal(raw, &job)
	}
	return item, job, err
}

// ResolveTestFiles replaces owner-visible file IDs with the immutable S3 locations used by the worker.
func (s *Store) ResolveTestFiles(ctx context.Context, job *Job) error {
	files := make(map[string][]*problems.TestFile)
	for i := range job.Cases {
		for _, file := range []*problems.TestFile{job.Cases[i].InputFile, job.Cases[i].OutputFile} {
			if file != nil {
				files[file.ID] = append(files[file.ID], file)
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	ids := make([]string, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,object_key,version_id,sha256,size FROM test_files WHERE ready AND id::text=ANY($1::text[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key, version, digest string
		var size int64
		if err = rows.Scan(&id, &key, &version, &digest, &size); err != nil {
			return err
		}
		for _, file := range files[id] {
			if file.Size != size || file.SHA256 != digest {
				return problems.ErrTestFile
			}
			file.Key, file.Version = key, version
		}
		delete(files, id)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(files) != 0 {
		return problems.ErrTestFile
	}
	return nil
}

// CreateGeneration accepts only cases constructed by the owner-authorized HTTP handler.
func (s *Store) CreateGeneration(ctx context.Context, owner, id, problemID, source, runtime string, job Job) (Submission, error) {
	raw, err := json.Marshal(job)
	if err != nil {
		return Submission{}, err
	}
	tx, err := s.beginSubmission(ctx, owner, false)
	if err != nil {
		return Submission{}, err
	}
	defer tx.Rollback(ctx)
	result, err := scan(tx.QueryRow(ctx, `INSERT INTO submissions
 (id,owner_id,problem_id,problem_version,problem_title,runtime,source,job)
 SELECT $1,$2,id,version,draft->>'title',$5,$4,$6 FROM problem_drafts
 WHERE id=$3 AND can_manage_problem(id,$2) RETURNING `+columns, id, owner, problemID, source, runtime, raw))
	if err == nil {
		err = tx.Commit(ctx)
	}
	return result, err
}
