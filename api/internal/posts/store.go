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

type Post struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Markdown         string     `json:"markdown,omitempty"`
	Version          int64      `json:"version,omitempty"`
	PublishedVersion int64      `json:"publishedVersion"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	PublishedAt      *time.Time `json:"publishedAt"`
	Author           string     `json:"author,omitempty"`
	Operator         bool       `json:"isOperator"`
	Owner            string     `json:"-"`
}
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool} }
func scan(row pgx.Row) (Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.Title, &p.Markdown, &p.Version, &p.UpdatedAt, &p.PublishedVersion, &p.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = problems.ErrNotFound
	}
	return p, err
}

const columns = `id,title,markdown,version,updated_at,published_version,published_at`

func (s *Store) Get(ctx context.Context, owner, id string) (Post, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM blog_posts WHERE owner_id=$1 AND id=$2`, owner, id))
}

func (s *Store) Save(ctx context.Context, owner, id, title, markdown string, version int64) (Post, error) {
	var p Post
	var err error
	if version == 0 {
		tx, e := s.pool.Begin(ctx)
		if e != nil {
			return p, e
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err = database.ConsumeCreationQuota(ctx, tx, owner, "post"); err != nil {
			return p, err
		}
		p, err = scan(tx.QueryRow(ctx, `INSERT INTO blog_posts(id,owner_id,title,markdown) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING RETURNING `+columns, id, owner, title, markdown))
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
	} else {
		p, err = scan(s.pool.QueryRow(ctx, `UPDATE blog_posts SET title=$4,markdown=$5,version=version+1,updated_at=clock_timestamp() WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING `+columns, owner, id, version, title, markdown))
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
	query := `UPDATE blog_posts SET published_title=NULL,published_markdown=NULL,published_version=0,published_at=NULL,version=version+1 WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING ` + columns
	if publish {
		query = `UPDATE blog_posts SET published_title=title,published_markdown=markdown,published_version=version+1,published_at=COALESCE(published_at,clock_timestamp()),version=version+1 WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING ` + columns
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
	err := s.pool.QueryRow(ctx, `SELECT b.id,b.published_title,b.published_markdown,b.published_at,u.handle,b.owner_id FROM blog_posts b JOIN user_profiles u ON b.owner_id=u.owner_id WHERE b.id=$1 AND b.published_title IS NOT NULL`, id).Scan(&p.ID, &p.Title, &p.Markdown, &p.PublishedAt, &p.Author, &p.Owner)
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
