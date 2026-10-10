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
}
type Repository interface {
	Get(context.Context, string) (Profile, error)
	Save(context.Context, string, string, string, int64, Accounts) (Profile, error)
}
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func scan(row pgx.Row) (Profile, error) {
	var p Profile
	err := row.Scan(&p.Handle, &p.Avatar, &p.Version, &p.CreatedAt, &p.Accounts)
	return p, err
}

func (s *Store) Get(ctx context.Context, owner string) (Profile, error) {
	var p Profile
	var deleted bool
	err := s.pool.QueryRow(ctx, `SELECT handle,avatar,version,created_at,accounts,deleted_at IS NOT NULL FROM user_profiles WHERE owner_id=$1`, owner).Scan(&p.Handle, &p.Avatar, &p.Version, &p.CreatedAt, &p.Accounts, &deleted)
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
		p, err = scan(s.pool.QueryRow(ctx, `INSERT INTO user_profiles(owner_id,handle,avatar,accounts) VALUES ($1,$2,$3,$4) RETURNING handle,avatar,version,created_at,accounts`, owner, handle, avatar, accounts))
	} else {
		p, err = scan(s.pool.QueryRow(ctx, `UPDATE user_profiles SET handle=$2,avatar=$3,accounts=$5,version=version+1 WHERE owner_id=$1 AND version=$4 AND deleted_at IS NULL RETURNING handle,avatar,version,created_at,accounts`, owner, handle, avatar, version, accounts))
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

// GetByHandle exposes only the chosen public identity.
func (s *Store) GetByHandle(ctx context.Context, handle string) (Profile, error) {
	p, err := scan(s.pool.QueryRow(ctx, `SELECT handle,avatar,version,created_at,accounts FROM user_profiles WHERE handle=$1 AND deleted_at IS NULL`, handle))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return p, err
}
