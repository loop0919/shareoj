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
	// ErrActiveContest keeps an organizer without co-setters until contests that others may join or be competing in
	// have ended, and the author of an unpublished problem until the contest holding it has ended.
	ErrActiveContest = errors.New("account has a contest that has not ended")
	// ErrMismatch means the typed confirmation is not the current handle.
	ErrMismatch = errors.New("confirmation does not match the handle")
)

type Store struct{ Pool *pgxpool.Pool }

// Published problems, posts and ended contests stay, as do submissions, contest registrations, difficulty votes and tester credits,
// because other users' standings, submission lists and pages refer to them.
var cleanup = []string{
	// Unpublished problems take their testers, favorites, invitations and featured applications with them.
	// One that an ended contest holds stays, since contest problems are never deleted.
	`DELETE FROM problem_drafts WHERE owner_id=$1 AND published_draft IS NULL AND NOT EXISTS (SELECT 1 FROM contest_problems cp WHERE cp.problem_id=problem_drafts.id)`,
	// Published work keeps only its public version; unpublished edits are private drafts too.
	`UPDATE problem_drafts SET draft=published_draft WHERE owner_id=$1 AND published_draft IS NOT NULL AND draft<>published_draft`,
	`DELETE FROM problem_tester_invitations i USING problem_drafts d WHERE d.id=i.problem_id AND d.owner_id=$1`,
	`DELETE FROM blog_posts WHERE owner_id=$1 AND published_title IS NULL`,
	`UPDATE blog_posts SET title=published_title,markdown=published_markdown,problem_ids=published_problem_ids,tags=published_tags WHERE owner_id=$1 AND published_title IS NOT NULL`,
	// Drafts left here have no co-setters. They take what they share with a contest by the id with them.
	`DELETE FROM contest_setters s USING contest_drafts d WHERE d.id=s.contest_id AND d.owner_id=$1`,
	`DELETE FROM contest_setter_invitations i USING contest_drafts d WHERE d.id=i.contest_id AND d.owner_id=$1`,
	`DELETE FROM contest_exclusions x USING contest_drafts d WHERE d.id=x.contest_id AND d.owner_id=$1`,
	`DELETE FROM notifications n USING contest_drafts d WHERE d.id=n.contest_id AND d.owner_id=$1`,
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
	// A co-setter's deletion is leaving (ADR 0022). It runs first, so that an owner handing over at the same time
	// waits on these rows and passes over this account.
	if _, err = tx.Exec(ctx, `WITH gone AS (UPDATE contest_setters SET left_at=clock_timestamp() WHERE owner_id=$1 AND left_at IS NULL RETURNING contest_id)
 INSERT INTO contest_exclusions(contest_id,owner_id) SELECT contest_id,$1 FROM gone ON CONFLICT DO NOTHING`, owner); err != nil {
		return err
	}
	if err = handOver(ctx, tx, owner); err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contests WHERE owner_id=$1 AND ends_at>clock_timestamp())
 OR EXISTS (SELECT 1 FROM problem_drafts d JOIN contest_problems cp ON cp.problem_id=d.id JOIN contests c ON c.id=cp.contest_id
  WHERE d.owner_id=$1 AND d.published_draft IS NULL AND c.ends_at>clock_timestamp())`, owner).Scan(&active); err != nil {
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

// handOver gives the owner's drafts and contests whose results are not final to the co-setter who joined first.
// Until the results are final the contest takes co-setters, so it needs an owner who can remove one who joined through
// a leaked link. The former owner keeps a left row, so the images they put in the contest stay valid.
func handOver(ctx context.Context, tx pgx.Tx, owner string) error {
	rows, err := tx.Query(ctx, `SELECT id::text,'contests' FROM contests c WHERE owner_id=$1 AND NOT COALESCE(contest_results_final_at(c)<=clock_timestamp(),false)
 UNION ALL SELECT id::text,'contest_drafts' FROM contest_drafts WHERE owner_id=$1`, owner)
	if err != nil {
		return err
	}
	var ids, tables []string
	for rows.Next() {
		var id, table string
		if err = rows.Scan(&id, &table); err != nil {
			rows.Close()
			return err
		}
		ids, tables = append(ids, id), append(tables, table)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for i, id := range ids {
		// Lock the contest as joining and removal do, then every co-setter, so one who leaves meanwhile is passed over.
		var locked string
		err = tx.QueryRow(ctx, `SELECT id::text FROM `+tables[i]+` WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT owner_id FROM contest_setters WHERE contest_id=$1 AND left_at IS NULL ORDER BY created_at,owner_id FOR UPDATE`, id)
		if err != nil {
			return err
		}
		heirs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if len(heirs) == 0 {
			continue
		}
		for _, step := range []struct {
			sql  string
			args []any
		}{
			{`UPDATE ` + tables[i] + ` SET owner_id=$2 WHERE id=$1`, []any{id, heirs[0]}},
			{`DELETE FROM contest_setters WHERE contest_id=$1 AND owner_id=$2`, []any{id, heirs[0]}},
			{`INSERT INTO contest_setters(contest_id,owner_id,left_at) VALUES($1,$2,clock_timestamp()) ON CONFLICT(contest_id,owner_id) DO UPDATE SET left_at=EXCLUDED.left_at`, []any{id, owner}},
			{`INSERT INTO contest_exclusions(contest_id,owner_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, []any{id, owner}},
		} {
			if _, err = tx.Exec(ctx, step.sql, step.args...); err != nil {
				return err
			}
		}
	}
	return nil
}
