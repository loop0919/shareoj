// Package profiles persists user-chosen identities separately from Cognito subjects.
package profiles

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound    = errors.New("profile not found")
	ErrHandleTaken = errors.New("handle already used")
	ErrConflict    = errors.New("profile changed")
	// ErrDeleted marks an account removed by its owner; its tokens must not act as it.
	ErrDeleted = errors.New("account deleted")
)

// DeletedHandle matches the placeholder that replaces a deleted account's handle; nobody can choose it.
var DeletedHandle = regexp.MustCompile(`^deleted_[0-9a-f]{11}$`)

type Accounts struct {
	X          string `json:"x"`
	AtCoder    string `json:"atcoder"`
	Codeforces string `json:"codeforces"`
	Yukicoder  string `json:"yukicoder"`
}

type Profile struct {
	Accounts  Accounts  `json:"accounts"`
	Handle    string    `json:"handle"`
	Avatar    string    `json:"avatar"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	// FolderView turns on the folder view beta of the owner's my page. SetFolderView saves it; Save keeps it.
	FolderView bool `json:"folderView"`
}
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// columns are what scan reads, in its order. Get reads one more column after them.
const columns = `handle,avatar,version,created_at,accounts,folder_view`

func scan(row pgx.Row) (Profile, error) {
	var p Profile
	err := row.Scan(&p.Handle, &p.Avatar, &p.Version, &p.CreatedAt, &p.Accounts, &p.FolderView)
	return p, err
}

func (s *Store) Get(ctx context.Context, owner string) (Profile, error) {
	var p Profile
	var deleted bool
	err := s.pool.QueryRow(ctx, `SELECT `+columns+`,deleted_at IS NOT NULL FROM user_profiles WHERE owner_id=$1`, owner).Scan(&p.Handle, &p.Avatar, &p.Version, &p.CreatedAt, &p.Accounts, &p.FolderView, &deleted)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = ErrNotFound
	case err == nil && deleted:
		return Profile{}, ErrDeleted
	}
	return p, err
}

func (s *Store) Save(ctx context.Context, owner, handle, avatar string, version int64, accounts Accounts) (Profile, error) {
	var p Profile
	var err error
	if version == 0 {
		p, err = scan(s.pool.QueryRow(ctx, `INSERT INTO user_profiles(owner_id,handle,avatar,accounts) VALUES ($1,$2,$3,$4) RETURNING `+columns, owner, handle, avatar, accounts))
	} else {
		p, err = scan(s.pool.QueryRow(ctx, `UPDATE user_profiles SET handle=$2,avatar=$3,accounts=$5,version=version+1 WHERE owner_id=$1 AND version=$4 AND deleted_at IS NULL RETURNING `+columns, owner, handle, avatar, version, accounts))
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "user_profiles_handle_key" {
			return p, ErrHandleTaken
		}
		return p, ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrConflict
	}
	return p, err
}

// SetFolderView turns the folder view beta on or off. The version stays, since the switch is no edit of the profile:
// a settings page left open must still save after it. A deleted account keeps its value.
func (s *Store) SetFolderView(ctx context.Context, owner string, enabled bool) error {
	var deleted bool
	err := s.pool.QueryRow(ctx, `UPDATE user_profiles SET folder_view=CASE WHEN deleted_at IS NULL THEN $2 ELSE folder_view END WHERE owner_id=$1 RETURNING deleted_at IS NOT NULL`, owner, enabled).Scan(&deleted)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err == nil && deleted:
		return ErrDeleted
	}
	return err
}

// Icon is what a page shows beside a name.
type Icon struct {
	Handle string `json:"handle"`
	Avatar string `json:"avatar"`
}

// Icons reads the icons of many handles in one query, in handle order. Handles nobody has are left out, and so are
// deleted accounts.
func (s *Store) Icons(ctx context.Context, handles []string) ([]Icon, error) {
	rows, err := s.pool.Query(ctx, `SELECT handle,avatar FROM user_profiles WHERE handle=ANY($1) AND deleted_at IS NULL ORDER BY handle`, handles)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Icon])
}

// GetByHandle exposes only the chosen public identity.
func (s *Store) GetByHandle(ctx context.Context, handle string) (Profile, error) {
	p, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM user_profiles WHERE handle=$1 AND deleted_at IS NULL`, handle))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return p, err
}
