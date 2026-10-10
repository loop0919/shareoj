// Package accounts deletes an account: private data goes, and what others rely on stays under an anonymous identity.
package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/profiles"
)

var (
	// ErrActiveContest keeps an organizer until contests that others may join or be competing in have ended.
	ErrActiveContest = errors.New("account has a contest that has not ended")
	// ErrMismatch means the typed confirmation is not the current handle.
	ErrMismatch = errors.New("confirmation does not match the handle")
)

type Store struct{ Pool *pgxpool.Pool }

// Published problems, posts and ended contests stay, as do submissions, contest registrations, difficulty votes and tester credits,
// because other users' standings, submission lists and pages refer to them.
var cleanup = []string{
	// Unpublished problems take their testers, favorites, invitations and featured applications with them.
	`DELETE FROM problem_drafts WHERE owner_id=$1 AND published_draft IS NULL`,
	// Published work keeps only its public version; unpublished edits are private drafts too.
	`UPDATE problem_drafts SET draft=published_draft WHERE owner_id=$1 AND published_draft IS NOT NULL AND draft<>published_draft`,
	`DELETE FROM problem_tester_invitations i USING problem_drafts d WHERE d.id=i.problem_id AND d.owner_id=$1`,
	`DELETE FROM blog_posts WHERE owner_id=$1 AND published_title IS NULL`,
	`UPDATE blog_posts SET title=published_title,markdown=published_markdown WHERE owner_id=$1 AND published_title IS NOT NULL`,
	`DELETE FROM contest_drafts WHERE owner_id=$1`,
	`DELETE FROM problem_favorites WHERE owner_id=$1`,
	`DELETE FROM notifications WHERE owner_id=$1`,
	`DELETE FROM creation_quotas WHERE owner_id=$1`,
	// Images still shown in what stays remain; the rest were only visible to the owner.
	`DELETE FROM content_images WHERE owner_id=$1 AND NOT content_image_access(id,owner_id,'',true)`,
}

// Delete runs removeIdentity before committing, so a failure to delete the sign-in leaves the account untouched.
func (s *Store) Delete(ctx context.Context, owner, handle string, removeIdentity func(context.Context) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current string
	var deleted bool
	err = tx.QueryRow(ctx, `SELECT handle,deleted_at IS NOT NULL FROM user_profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&current, &deleted)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return profiles.ErrNotFound
	case err != nil:
		return err
	case deleted:
		return profiles.ErrDeleted
	case current != handle:
		return ErrMismatch
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contests WHERE owner_id=$1 AND ends_at>clock_timestamp())`, owner).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrActiveContest
	}
	for _, statement := range cleanup {
		if _, err = tx.Exec(ctx, statement, owner); err != nil {
			return err
		}
	}
	suffix := make([]byte, 6)
	if _, err = rand.Read(suffix); err != nil {
		return err
	}
	// The placeholder fits the handle rule, so every query that joins profiles keeps working.
	placeholder := "deleted_" + hex.EncodeToString(suffix)[:11]
	if _, err = tx.Exec(ctx, `UPDATE user_profiles SET handle=$2,avatar='',accounts='{}',deleted_at=clock_timestamp(),version=version+1 WHERE owner_id=$1`, owner, placeholder); err != nil {
		return err
	}
	if err = removeIdentity(ctx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
