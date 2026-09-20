// Package problems persists problem drafts shared by authors and testers.
package problems

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"judge/api/internal/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrContestLocked = errors.New("problem is reserved for a contest")
	ErrNotFound      = errors.New("problem not found")
	ErrConflict      = errors.New("problem changed")
	ErrTestFile      = errors.New("invalid test file")
)

type Generator struct {
	Runtime  string `json:"runtime"`
	Source   string `json:"source"`
	Protocol string `json:"protocol,omitempty"`
}

func (g Generator) ValidJudgeProtocol() bool {
	return g.Protocol == "" || g.Protocol == "legacy" ||
		(g.Protocol == "testlib" && (g.Runtime == "cpp23-gcc" || g.Runtime == "cpp23-clang"))
}

type Generators struct {
	Validation Generator `json:"validation,omitzero"`
	Input      Generator `json:"input"`
	Output     Generator `json:"output"`
}

type Draft struct {
	Difficulty    *int        `json:"difficulty"`
	Interactor    *Generator  `json:"interactor,omitempty"`
	Checker       *Generator  `json:"checker,omitempty"`
	Generators    *Generators `json:"generators,omitempty"`
	Title         string      `json:"title"`
	Markdown      string      `json:"markdown"`
	Editorial     string      `json:"editorial,omitempty"`
	TimeLimitMS   string      `json:"timeLimitMs"`
	MemoryLimitMB string      `json:"memoryLimitMb"`
	TestCases     []TestCase  `json:"testCases,omitempty"`
}

type TestCase struct {
	IsSample   bool      `json:"isSample"`
	Name       string    `json:"name"`
	Input      string    `json:"input"`
	Output     string    `json:"output"`
	InputFile  *TestFile `json:"inputFile,omitempty"`
	OutputFile *TestFile `json:"outputFile,omitempty"`
}

type TestFile struct {
	ID      string `json:"id"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Key     string `json:"key,omitempty"`
	Version string `json:"versionId,omitempty"`
}

type Problem struct {
	Testers          []string  `json:"testers,omitempty"`
	ContestID        string    `json:"contestId,omitempty"`
	Author           string    `json:"author"`
	PublishedVersion int64     `json:"publishedVersion"`
	ID               string    `json:"id"`
	Version          int64     `json:"version"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Draft            Draft     `json:"draft"`
}

type Summary struct {
	ContestID        string    `json:"contestId,omitempty"`
	PublishedVersion int64     `json:"publishedVersion"`
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type Cursor struct {
	UpdatedAt time.Time `json:"updatedAt"`
	ID        string    `json:"id"`
}

type Repository interface {
	Get(context.Context, string, string) (Problem, error)
	List(context.Context, string, *Cursor) ([]Summary, error)
	Save(context.Context, string, string, int64, Draft) (Problem, error)
	Delete(context.Context, string, string, int64) error
}

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := database.Open(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Pool() *pgxpool.Pool               { return s.pool }
func (s *Store) Close()                            { s.pool.Close() }
func (s *Store) Migrate(ctx context.Context) error { return database.Migrate(ctx, s.pool) }

func scan(row pgx.Row) (Problem, error) {
	var p Problem
	var data []byte
	err := row.Scan(&p.ID, &p.Version, &p.UpdatedAt, &data, &p.PublishedVersion, &p.Author, &p.ContestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(data, &p.Draft)
	return p, err
}

func (s *Store) Get(ctx context.Context, owner, id string) (Problem, error) {
	p, err := scan(s.pool.QueryRow(ctx, `SELECT id, version, updated_at, draft, published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),'') FROM problem_drafts WHERE can_manage_problem(id,$1) AND id=$2`, owner, id))
	if err != nil {
		return p, err
	}
	p.Testers, err = s.Testers(ctx, id)
	return p, err
}

// List returns at most 51 rows; the HTTP layer exposes 50 and a next cursor.
func (s *Store) List(ctx context.Context, owner string, cursor *Cursor) ([]Summary, error) {
	return s.list(ctx, owner, cursor, false)
}

func (s *Store) ListTesting(ctx context.Context, owner string, cursor *Cursor) ([]Summary, error) {
	return s.list(ctx, owner, cursor, true)
}

func (s *Store) list(ctx context.Context, owner string, cursor *Cursor, testing bool) ([]Summary, error) {
	query := `SELECT id, draft->>'title', updated_at, published_version,COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),'') FROM problem_drafts WHERE owner_id=$1`
	if testing {
		query = strings.Replace(query, "WHERE owner_id=$1", "WHERE owner_id<>$1 AND EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=problem_drafts.id AND t.owner_id=$1)", 1)
	}
	args := []any{owner}
	if cursor != nil {
		query += ` AND (updated_at, id) < ($2, $3::uuid)`
		args = append(args, cursor.UpdatedAt, cursor.ID)
	}
	rows, err := s.pool.Query(ctx, query+` ORDER BY updated_at DESC, id DESC LIMIT 51`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Summary, error) {
		var p Summary
		err := row.Scan(&p.ID, &p.Title, &p.UpdatedAt, &p.PublishedVersion, &p.ContestID)
		return p, err
	})
}

func (s *Store) Save(ctx context.Context, owner, id string, version int64, draft Draft) (Problem, error) {
	if err := s.validateTestFiles(ctx, owner, id, draft.TestCases); err != nil {
		return Problem{}, err
	}
	data, err := json.Marshal(draft)
	if err != nil {
		return Problem{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Problem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var p Problem
	if version == 0 {
		p, err = scan(tx.QueryRow(ctx, `INSERT INTO problem_drafts (id, owner_id, draft) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING RETURNING id, version, updated_at, draft, published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),'')`, id, owner, data))
	} else {
		p, err = scan(tx.QueryRow(ctx, `UPDATE problem_drafts SET draft=$4, version=version+1, updated_at=clock_timestamp() WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3 RETURNING id, version, updated_at, draft, published_version,COALESCE((SELECT handle FROM user_profiles WHERE owner_id=problem_drafts.owner_id),''),COALESCE((SELECT contest_id::text FROM contest_problems WHERE problem_id=problem_drafts.id),'')`, owner, id, version, data))
	}
	if err == nil {
		// Contest saves also lock the source problem before changing contest_problems.
		// Keep statement, judging settings and version atomic for new submissions.
		p.ContestID = ""
		err = tx.QueryRow(ctx, `UPDATE contest_problems SET draft=$2,problem_version=$3 WHERE problem_id=$1 RETURNING contest_id::text`, id, data, p.Version).Scan(&p.ContestID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Problem{}, err
		}
		err = tx.Commit(ctx)
	}

	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	_ = tx.Rollback(ctx)
	if _, getErr := s.Get(ctx, owner, id); getErr != nil {
		return Problem{}, getErr
	}
	return Problem{}, ErrConflict
}

func (s *Store) validateTestFiles(ctx context.Context, owner, problemID string, cases []TestCase) error {
	want := make(map[string]TestFile)
	for _, c := range cases {
		for _, file := range []*TestFile{c.InputFile, c.OutputFile} {
			if file == nil {
				continue
			}
			if previous, ok := want[file.ID]; ok && previous != *file {
				return ErrTestFile
			}
			want[file.ID] = *file
		}
	}
	if len(want) == 0 {
		return nil
	}
	ids := make([]string, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,size,sha256 FROM test_files WHERE (owner_id=$1 OR can_manage_problem(problem_id,$1)) AND problem_id=$2 AND ready AND id::text=ANY($3::text[])`, owner, problemID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, sha string
		var size int64
		if err = rows.Scan(&id, &size, &sha); err != nil {
			return err
		}
		file := want[id]
		if file.Size != size || file.SHA256 != sha || file.Key != "" || file.Version != "" {
			return ErrTestFile
		}
		delete(want, id)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(want) != 0 {
		return ErrTestFile
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, owner, id string, version int64) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM problem_drafts WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3`, owner, id, version)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			return ErrContestLocked
		}
		return err
	}
	if result.RowsAffected() == 1 {
		return nil
	}
	if _, err := s.Get(ctx, owner, id); err != nil {
		return err
	}
	return ErrConflict
}
