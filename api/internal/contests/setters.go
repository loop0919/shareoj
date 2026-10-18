package contests

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/problems"
)

var (
	// ErrInvitationClosed refuses new co-setters once the results are final; nothing is left for them to do (ADR 0022).
	ErrInvitationClosed = errors.New("contest results are final")
	// ErrSetterNotFound means the handle names no co-setter whom the actor may remove.
	ErrSetterNotFound = errors.New("co-setter not found")
)

// SetterInvitation previews a co-setter link. Published is false while the contest is still a draft.
type SetterInvitation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Published bool   `json:"published"`
	Joined    bool   `json:"joined"`
}

// Tokens look like problem tester tokens: 160 random bits in unpadded base32.
func newToken() string {
	var secret [20]byte
	_, _ = rand.Read(secret[:])
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret[:])
}

// SetterInvitationLink returns the contest's link for a setter, creating it on first use, so repeated clicks are
// harmless. Renewing replaces the token, and the old link stops working.
func (s *Store) SetterInvitationLink(ctx context.Context, actor, id string, renew bool) (string, error) {
	var token string
	err := s.Pool.QueryRow(ctx, `INSERT INTO contest_setter_invitations(contest_id,token) SELECT $1::uuid,$3 WHERE contest_setter($1::uuid,$2)
 ON CONFLICT(contest_id) DO UPDATE SET token=CASE WHEN $4 THEN EXCLUDED.token ELSE contest_setter_invitations.token END,
  created_at=CASE WHEN $4 THEN clock_timestamp() ELSE contest_setter_invitations.created_at END RETURNING token`, id, actor, newToken(), renew).Scan(&token)
	return token, err
}

type setterTarget struct {
	owner, title     string
	published, final bool
}

// lockSetters reads the contest or, before publication, its draft. With lock it locks the row, so that joining,
// leaving, removal, registration, saving and publication take turns. A draft may turn into its contest while the
// lock waits, so the contest is read once more.
func lockSetters(ctx context.Context, tx pgx.Tx, id string, lock bool) (setterTarget, error) {
	contestLock, draftLock := "", ""
	if lock {
		contestLock, draftLock = " FOR NO KEY UPDATE", " FOR UPDATE"
	}
	var t setterTarget
	var err error
	for range 2 {
		err = tx.QueryRow(ctx, `SELECT owner_id,title,true,COALESCE(contest_results_final_at(c)<=clock_timestamp(),false)
 FROM contests c WHERE id=$1`+contestLock, id).Scan(&t.owner, &t.title, &t.published, &t.final)
		if !errors.Is(err, pgx.ErrNoRows) {
			return t, err
		}
		err = tx.QueryRow(ctx, `SELECT owner_id,COALESCE(draft->>'title',''),false,false FROM contest_drafts WHERE id=$1`+draftLock, id).Scan(&t.owner, &t.title, &t.published, &t.final)
		if !errors.Is(err, pgx.ErrNoRows) {
			return t, err
		}
	}
	return t, err
}

// SetterInvitation previews a co-setter link or, with accept, joins the actor as a co-setter. Joining again, or as
// the owner, changes nothing. A registered participant cannot join until the results are final, and nobody can after.
func (s *Store) SetterInvitation(ctx context.Context, actor, token string, accept bool) (SetterInvitation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SetterInvitation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var invitation SetterInvitation
	if err = tx.QueryRow(ctx, `SELECT contest_id FROM contest_setter_invitations WHERE token=$1`, token).Scan(&invitation.ID); err != nil {
		return invitation, err
	}
	target, err := lockSetters(ctx, tx, invitation.ID, accept)
	if err != nil {
		return invitation, err
	}
	// The link may have been renewed while the lock waited.
	err = tx.QueryRow(ctx, `SELECT u.handle,contest_setter(i.contest_id,$3) FROM contest_setter_invitations i JOIN user_profiles u ON u.owner_id=$4
 WHERE i.token=$1 AND i.contest_id=$2 FOR SHARE OF i`, token, invitation.ID, actor, target.owner).Scan(&invitation.Author, &invitation.Joined)
	if err != nil {
		return invitation, err
	}
	invitation.Title, invitation.Published = target.title, target.published
	if !accept || invitation.Joined {
		return invitation, tx.Commit(ctx)
	}
	if target.final {
		return invitation, ErrInvitationClosed
	}
	var participant bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id=$1 AND owner_id=$2)`, invitation.ID, actor).Scan(&participant); err != nil {
		return invitation, err
	}
	if participant {
		return invitation, problems.ErrParticipantLocked
	}
	// Someone who left may come back; the join time then starts again.
	if _, err = tx.Exec(ctx, `INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,$2)
 ON CONFLICT(contest_id,owner_id) DO UPDATE SET left_at=NULL,created_at=clock_timestamp()`, invitation.ID, actor); err != nil {
		return invitation, err
	}
	// Tell the owner, who can remove someone who joined through a leaked link.
	if _, err = tx.Exec(ctx, `INSERT INTO notifications(owner_id,contest_id,actor_id,kind) VALUES($1,$2,$3,'contest_setter')`, target.owner, invitation.ID, actor); err != nil {
		return invitation, err
	}
	invitation.Joined = true
	return invitation, tx.Commit(ctx)
}

// RemoveSetter takes a co-setter off the contest: the owner removes anyone, and a co-setter removes only themselves
// to leave. Either way they stay out of the competition, having seen the problems. A removal also renews the link,
// so the removed setter cannot come back through the one they hold.
func (s *Store) RemoveSetter(ctx context.Context, actor, id, handle string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	target, err := lockSetters(ctx, tx, id, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSetterNotFound
	}
	if err != nil {
		return err
	}
	var setter string
	err = tx.QueryRow(ctx, `SELECT s.owner_id FROM contest_setters s JOIN user_profiles u ON u.owner_id=s.owner_id
 WHERE s.contest_id=$1 AND u.handle=$2 AND s.left_at IS NULL`, id, handle).Scan(&setter)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && setter != actor && target.owner != actor) {
		return ErrSetterNotFound
	}
	if err != nil {
		return err
	}
	if err = leave(ctx, tx, id, setter); err != nil {
		return err
	}
	if setter != actor {
		if _, err = tx.Exec(ctx, `UPDATE contest_setter_invitations SET token=$2,created_at=clock_timestamp() WHERE contest_id=$1`, id, newToken()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func leave(ctx context.Context, tx pgx.Tx, id, setter string) error {
	if _, err := tx.Exec(ctx, `UPDATE contest_setters SET left_at=clock_timestamp() WHERE contest_id=$1 AND owner_id=$2 AND left_at IS NULL`, id, setter); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO contest_exclusions(contest_id,owner_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, setter)
	return err
}
