package problems

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrParticipantLocked keeps a contest participant off the staff until the results are final (ADR 0022).
var ErrParticipantLocked = errors.New("contest participant cannot join the staff yet")

type TesterInvitation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Joined bool   `json:"joined"`
}

// CreateTesterInvitation reuses the problem's link so repeated clicks are harmless. Only the author and invited
// testers see it: testers cannot be removed, so a setter's rights through a contest must not become tester rights.
func (s *Store) CreateTesterInvitation(ctx context.Context, actor, id string) (string, error) {
	var secret [20]byte
	_, _ = rand.Read(secret[:])
	var token string
	err := s.pool.QueryRow(ctx, `INSERT INTO problem_tester_invitations(problem_id,token)
 SELECT id,$3 FROM problem_drafts WHERE id=$1 AND can_manage_problem_directly(id,$2)
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
	// Lock the problem's contest before the problem, as contest saves do, so registration waits for the check below.
	lockContest := `SELECT c.id FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
 WHERE cp.problem_id=(SELECT problem_id FROM problem_tester_invitations WHERE token=$1) FOR NO KEY UPDATE OF c`
	if accept {
		if _, err = tx.Exec(ctx, lockContest, token); err != nil {
			return TesterInvitation{}, err
		}
	}
	// A setter who manages the problem through a contest becomes a real tester by accepting.
	var invitation TesterInvitation
	err = tx.QueryRow(ctx, `SELECT p.id,p.draft->>'title',u.handle,can_manage_problem_directly(p.id,$2)
 FROM problem_tester_invitations i JOIN problem_drafts p ON p.id=i.problem_id
 JOIN user_profiles u ON u.owner_id=p.owner_id WHERE i.token=$1 FOR SHARE OF i,p`, token, actor).Scan(&invitation.ID, &invitation.Title, &invitation.Author, &invitation.Joined)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return invitation, err
	}
	if accept && !invitation.Joined {
		// Lock again in case the problem entered a contest in between; with both locks held, its contest stays.
		if _, err = tx.Exec(ctx, lockContest, token); err != nil {
			return invitation, err
		}
		var participant bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id
 JOIN contest_participants p ON p.contest_id=c.id
 WHERE cp.problem_id=$1 AND p.owner_id=$2 AND NOT COALESCE(contest_results_final_at(c)<=clock_timestamp(),false))`, invitation.ID, actor).Scan(&participant)
		if err != nil {
			return invitation, err
		}
		if participant {
			return invitation, ErrParticipantLocked
		}
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
