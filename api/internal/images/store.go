package images

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInUse = errors.New("image in use")

type Store struct{ Pool *pgxpool.Pool }

type Info struct {
	ID   string `json:"id"`
	Size int    `json:"size"`
	Used bool   `json:"used"`
}

func (s *Store) Get(ctx context.Context, id, viewer string) ([]byte, string, error) {
	var data []byte
	var media string
	err := s.Pool.QueryRow(ctx, `SELECT data,media_type FROM content_images
 WHERE id=$1 AND (owner_id=$2 OR content_image_access(id,owner_id,$2,false))`, id, viewer).Scan(&data, &media)
	return data, media, err
}

func (s *Store) List(ctx context.Context, owner string, offset int) ([]Info, int64, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,size,content_image_access(id,owner_id,'',true) FROM content_images WHERE owner_id=$1 ORDER BY created_at DESC,id DESC LIMIT 51 OFFSET $2`, owner, offset)
	if err != nil {
		return nil, 0, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Info])
	if err != nil {
		return nil, 0, err
	}
	if items == nil {
		items = []Info{}
	}
	var used int64
	err = s.Pool.QueryRow(ctx, `SELECT COALESCE(sum(size),0) FROM content_images WHERE owner_id=$1`, owner).Scan(&used)
	return items, used, err
}

func (s *Store) Delete(ctx context.Context, owner, id string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Keep reference checks and deletion atomic with concurrent content saves.
	if _, err = tx.Exec(ctx, `LOCK TABLE problem_drafts,blog_posts,contests,contest_problems IN SHARE MODE`); err != nil {
		return err
	}
	var used bool
	err = tx.QueryRow(ctx, `SELECT content_image_access(id,owner_id,'',true) FROM content_images WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&used)
	if err != nil {
		return err
	}
	if used {
		return ErrInUse
	}
	if _, err = tx.Exec(ctx, `DELETE FROM content_images WHERE id=$1 AND owner_id=$2`, id, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Save deduplicates already sanitized image data for a single uploader.
func (s *Store) Save(ctx context.Context, owner, id, media string, data []byte) (string, bool, error) {
	digest := sha256.Sum256(data)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedOwner string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM user_profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&lockedOwner); err != nil {
		return "", false, err
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT id FROM content_images WHERE owner_id=$1 AND digest=$2`, owner, digest[:]).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO content_images(id,owner_id,digest,media_type,data,size) VALUES($1,$2,$3,$4,$5,$6)`, id, owner, digest[:], media, data, len(data))
	if err != nil {
		return "", false, err
	}
	return id, true, tx.Commit(ctx)
}
