package problems

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"

	"github.com/jackc/pgx/v5"
)

type TesterInvitation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Joined bool   `json:"joined"`
}

// CreateTesterInvitation reuses the problem's link so repeated clicks are harmless.
func (s *Store) CreateTesterInvitation(ctx context.Context, actor, id string) (string, error) {
	var secret [20]byte
	_, _ = rand.Read(secret[:])
	var token string
	err := s.pool.QueryRow(ctx, `INSERT INTO problem_tester_invitations(problem_id,token)
 SELECT id,$3 FROM problem_drafts WHERE id=$1 AND can_manage_problem(id,$2)
 ON CONFLICT(problem_id) DO UPDATE SET token=problem_tester_invitations.token RETURNING token`, id, actor, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret[:])).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return token, err
}

func (s *Store) TesterInvitation(ctx context.Context, actor, token string, accept bool) (TesterInvitation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TesterInvitation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var invitation TesterInvitation
	err = tx.QueryRow(ctx, `SELECT p.id,p.draft->>'title',u.handle,can_manage_problem(p.id,$2)
 FROM problem_tester_invitations i JOIN problem_drafts p ON p.id=i.problem_id
 JOIN user_profiles u ON u.owner_id=p.owner_id WHERE i.token=$1 FOR SHARE OF i,p`, token, actor).Scan(&invitation.ID, &invitation.Title, &invitation.Author, &invitation.Joined)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return invitation, err
	}
	if accept && !invitation.Joined {
		_, err = tx.Exec(ctx, `INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, invitation.ID, actor)
		if err != nil {
			return invitation, err
		}
		invitation.Joined = true
	}
	return invitation, tx.Commit(ctx)
}

// Testers returns public handles after the caller has checked problem access.
func (s *Store) Testers(ctx context.Context, id string) ([]string, error) {
	handles := []string{}
	err := s.pool.QueryRow(ctx, `SELECT ARRAY(SELECT u.handle FROM problem_testers t JOIN user_profiles u ON u.owner_id=t.owner_id WHERE t.problem_id=$1 ORDER BY u.handle)`, id).Scan(&handles)
	return handles, err
}
