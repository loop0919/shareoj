// Package posts stores private blog drafts and explicit publication snapshots.
package posts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"judge/api/internal/database"
	"judge/api/internal/problems"
)

// ErrUnavailableProblem means a post names a problem whose editorial is not public yet.
var ErrUnavailableProblem = errors.New("problem editorial is not public")

// ProblemRef is a problem a post explains. Title is empty once the problem stops being public.
type ProblemRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Post struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	Markdown         string       `json:"markdown,omitempty"`
	Version          int64        `json:"version,omitempty"`
	PublishedVersion int64        `json:"publishedVersion"`
	UpdatedAt        time.Time    `json:"updatedAt"`
	PublishedAt      *time.Time   `json:"publishedAt"`
	Author           string       `json:"author,omitempty"`
	Operator         bool         `json:"isOperator"`
	Owner            string       `json:"-"`
	Problems         []ProblemRef `json:"problems,omitempty"`
}

// Editorial is a published post that explains a problem.
type Editorial struct {
	ProblemID   string    `json:"problemId"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	PublishedAt time.Time `json:"publishedAt"`
	Operator    bool      `json:"isOperator"`
	Owner       string    `json:"-"`
}
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool} }
func scan(row pgx.Row) (Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.Title, &p.Markdown, &p.Version, &p.UpdatedAt, &p.PublishedVersion, &p.PublishedAt, &p.Problems)
	if errors.Is(err, pgx.ErrNoRows) {
		err = problems.ErrNotFound
	}
	return p, err
}

// editorialPublic matches problems d whose official editorial everyone can read.
const editorialPublic = `d.published_draft IS NOT NULL AND NOT COALESCE(statement_timestamp()<featured_reveal_at(d.id),false)`

const columns = `id,title,markdown,version,updated_at,published_version,published_at,
 COALESCE((SELECT json_agg(json_build_object('id',x.id,'title',CASE WHEN ` + editorialPublic + ` THEN d.published_draft->>'title' ELSE '' END) ORDER BY x.n)
  FROM unnest(problem_ids) WITH ORDINALITY x(id,n) LEFT JOIN problem_drafts d ON d.id=x.id),'[]')`

func (s *Store) Get(ctx context.Context, owner, id string) (Post, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM blog_posts WHERE owner_id=$1 AND id=$2`, owner, id))
}

// Save writes the draft. A nil problemIDs keeps the problems already listed.
func (s *Store) Save(ctx context.Context, owner, id, title, markdown string, problemIDs []string, version int64) (Post, error) {
	var p Post
	var err error
	if problemIDs != nil {
		var public int
		if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM problem_drafts d WHERE d.id=ANY($1::uuid[]) AND `+editorialPublic, problemIDs).Scan(&public); err != nil {
			return p, err
		}
		if public != len(problemIDs) {
			return p, ErrUnavailableProblem
		}
	}
	if version == 0 {
		tx, e := s.pool.Begin(ctx)
		if e != nil {
			return p, e
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err = database.ConsumeCreationQuota(ctx, tx, owner, "post"); err != nil {
			return p, err
		}
		p, err = scan(tx.QueryRow(ctx, `INSERT INTO blog_posts(id,owner_id,title,markdown,problem_ids) VALUES($1,$2,$3,$4,COALESCE($5::uuid[],'{}')) ON CONFLICT(id) DO NOTHING RETURNING `+columns, id, owner, title, markdown, problemIDs))
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
	} else {
		p, err = scan(s.pool.QueryRow(ctx, `UPDATE blog_posts SET title=$4,markdown=$5,problem_ids=COALESCE($6::uuid[],problem_ids),version=version+1,updated_at=clock_timestamp() WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING `+columns, owner, id, version, title, markdown, problemIDs))
	}
	return s.conflict(ctx, owner, id, p, err)
}

func (s *Store) conflict(ctx context.Context, owner, id string, p Post, err error) (Post, error) {
	if errors.Is(err, problems.ErrNotFound) {
		if _, e := s.Get(ctx, owner, id); e != nil {
			return p, e
		}
		return p, problems.ErrConflict
	}
	return p, err
}

func (s *Store) Publish(ctx context.Context, owner, id string, version int64, publish bool) (Post, error) {
	query := `UPDATE blog_posts SET published_title=NULL,published_markdown=NULL,published_problem_ids='{}',published_version=0,published_at=NULL,version=version+1 WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING ` + columns
	if publish {
		query = `UPDATE blog_posts SET published_title=title,published_markdown=markdown,published_problem_ids=problem_ids,published_version=version+1,published_at=COALESCE(published_at,clock_timestamp()),version=version+1 WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING ` + columns
	}
	p, err := scan(s.pool.QueryRow(ctx, query, owner, id, version))
	return s.conflict(ctx, owner, id, p, err)
}

func (s *Store) Delete(ctx context.Context, owner, id string, version int64) error {
	r, err := s.pool.Exec(ctx, `DELETE FROM blog_posts WHERE owner_id=$1 AND id=$2 AND version=$3`, owner, id, version)
	if err != nil {
		return err
	}
	if r.RowsAffected() == 1 {
		return nil
	}
	if _, e := s.Get(ctx, owner, id); e != nil {
		return e
	}
	return problems.ErrConflict
}

func (s *Store) List(ctx context.Context, owner string, cursor *problems.Cursor) ([]Post, error) {
	args := []any{owner}
	query := `SELECT id,title,version,updated_at,published_version,published_at FROM blog_posts WHERE owner_id=$1`
	if cursor != nil {
		query += ` AND (updated_at,id)<($2,$3::uuid)`
		args = append(args, cursor.UpdatedAt, cursor.ID)
	}
	rows, err := s.pool.Query(ctx, query+` ORDER BY updated_at DESC,id DESC LIMIT 51`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Post, error) {
		var p Post
		err := row.Scan(&p.ID, &p.Title, &p.Version, &p.UpdatedAt, &p.PublishedVersion, &p.PublishedAt)
		return p, err
	})
}

func (s *Store) PublicGet(ctx context.Context, id string) (Post, error) {
	var p Post
	err := s.pool.QueryRow(ctx, `SELECT b.id,b.published_title,b.published_markdown,b.published_at,u.handle,b.owner_id,
 COALESCE((SELECT json_agg(json_build_object('id',d.id,'title',d.published_draft->>'title') ORDER BY x.n)
  FROM unnest(b.published_problem_ids) WITH ORDINALITY x(id,n) JOIN problem_drafts d ON d.id=x.id WHERE `+editorialPublic+`),'[]')
 FROM blog_posts b JOIN user_profiles u ON b.owner_id=u.owner_id WHERE b.id=$1 AND b.published_title IS NOT NULL`, id).Scan(&p.ID, &p.Title, &p.Markdown, &p.PublishedAt, &p.Author, &p.Owner, &p.Problems)
	if errors.Is(err, pgx.ErrNoRows) {
		err = problems.ErrNotFound
	}
	return p, err
}

func (s *Store) PublicList(ctx context.Context, cursor *problems.Cursor) ([]Post, error) {
	return s.PublicListByHandle(ctx, cursor, "")
}

func (s *Store) PublicListByHandle(ctx context.Context, cursor *problems.Cursor, handle string) ([]Post, error) {
	query := `SELECT b.id,b.published_title,b.published_at,u.handle,b.owner_id FROM blog_posts b JOIN user_profiles u ON b.owner_id=u.owner_id WHERE b.published_title IS NOT NULL AND ($1='' OR u.handle=$1)`
	args := []any{handle}
	if cursor != nil {
		query += ` AND (b.published_at,b.id)<($2,$3::uuid)`
		args = append(args, cursor.UpdatedAt, cursor.ID)
	}
	rows, err := s.pool.Query(ctx, query+` ORDER BY b.published_at DESC,b.id DESC LIMIT 51`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Post, error) {
		var p Post
		err := row.Scan(&p.ID, &p.Title, &p.PublishedAt, &p.Author, &p.Owner)
		return p, err
	})
}

// Editorials lists the published posts that explain the problem, oldest first.
func (s *Store) Editorials(ctx context.Context, problemID string) ([]Editorial, error) {
	return s.editorials(ctx, `$1::uuid`, problemID)
}

// ContestEditorials lists the published posts for the problems of an ended contest.
func (s *Store) ContestEditorials(ctx context.Context, contestID string) ([]Editorial, error) {
	return s.editorials(ctx, `SELECT cp.problem_id FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE c.id=$1 AND statement_timestamp()>=c.ends_at`, contestID)
}

func (s *Store) editorials(ctx context.Context, problems, arg string) ([]Editorial, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.id,b.id,b.published_title,u.handle,b.published_at,b.owner_id FROM problem_drafts d
 JOIN blog_posts b ON b.published_problem_ids @> ARRAY[d.id] AND b.published_title IS NOT NULL JOIN user_profiles u ON u.owner_id=b.owner_id
 WHERE d.id IN (`+problems+`) AND `+editorialPublic+` ORDER BY b.published_at,b.id LIMIT 1000`, arg)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Editorial, error) {
		var e Editorial
		err := row.Scan(&e.ProblemID, &e.ID, &e.Title, &e.Author, &e.PublishedAt, &e.Owner)
		return e, err
	})
}
