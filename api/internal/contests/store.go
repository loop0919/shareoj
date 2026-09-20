// Package contests manages scheduled problem sets.
package contests

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"judge/api/internal/problems"
)

var ErrConflict = errors.New("contest is locked or problems are unavailable")

type Problem struct {
	Solved bool   `json:"solved,omitempty"`
	ID     string `json:"id"`
	Points int    `json:"points"`
	Title  string `json:"title,omitempty"`
}
type Contest struct {
	ID                 string    `json:"id"`
	Owner              string    `json:"-"`
	Author             string    `json:"author"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	StartsAt           time.Time `json:"startsAt"`
	EndsAt             time.Time `json:"endsAt"`
	PenaltyMinutes     int       `json:"penaltyMinutes"`
	Version            int64     `json:"version"`
	Status             string    `json:"status"`
	CanEdit            bool      `json:"canEdit"`
	CanViewSubmissions bool      `json:"canViewSubmissions"`
	Official           bool      `json:"official"`
	Problems           []Problem `json:"problems"`
}
type Input struct {
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	PenaltyMinutes *int      `json:"penaltyMinutes"`
	Version        int64     `json:"version"`
	Problems       []Problem `json:"problems"`
}
type Store struct{ Pool *pgxpool.Pool }

const fields = `c.id,c.owner_id,u.handle,c.title,c.description,c.starts_at,c.ends_at,c.penalty_minutes,c.version,
 CASE WHEN statement_timestamp()<c.starts_at THEN 'scheduled' WHEN statement_timestamp()<c.ends_at THEN 'running' ELSE 'ended' END`

func scan(row pgx.Row) (Contest, error) {
	var c Contest
	err := row.Scan(&c.ID, &c.Owner, &c.Author, &c.Title, &c.Description, &c.StartsAt, &c.EndsAt, &c.PenaltyMinutes, &c.Version, &c.Status)
	c.Problems = []Problem{}
	return c, err
}

func (s *Store) List(ctx context.Context, owner string, offset int) ([]Contest, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+fields+` FROM contests c JOIN user_profiles u ON u.owner_id=c.owner_id
 WHERE ($1='' OR c.owner_id=$1) ORDER BY c.starts_at DESC,c.created_at DESC,c.id DESC LIMIT 51 OFFSET $2`, owner, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Contest, error) { return scan(row) })
}

func (s *Store) Get(ctx context.Context, id, viewer string) (Contest, error) {
	// A repeatable-read snapshot keeps status and problem visibility consistent at boundaries.
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Contest{}, err
	}
	defer tx.Rollback(ctx)
	c, err := scan(tx.QueryRow(ctx, `SELECT `+fields+` FROM contests c JOIN user_profiles u ON u.owner_id=c.owner_id WHERE c.id=$1`, id))
	if err != nil {
		return c, err
	}
	c.CanEdit = viewer == c.Owner && c.Status == "scheduled"
	err = tx.QueryRow(ctx, `SELECT $2<>'' AND $2<>c.owner_id AND NOT EXISTS (
 SELECT 1 FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id WHERE cp.contest_id=c.id AND t.owner_id=$2)
 FROM contests c WHERE c.id=$1`, id, viewer).Scan(&c.Official)
	if err != nil {
		return c, err
	}
	c.CanViewSubmissions = c.Status == "ended" || (viewer != "" && !c.Official)
	rows, err := tx.Query(ctx, `SELECT cp.problem_id,cp.points,cp.draft->>'title',
 EXISTS(SELECT 1 FROM submissions s WHERE s.contest_id=cp.contest_id AND s.problem_id=cp.problem_id
  AND s.owner_id=$3 AND s.status='DONE' AND s.result->>'verdict'='AC'
  AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
  AND NOT COALESCE((s.job->>'generate')::boolean,false)
  AND NOT COALESCE((s.job->>'validate')::boolean,false)
  AND s.created_at>=$4)
 FROM contest_problems cp
 WHERE cp.contest_id=$1 AND ($2 OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=cp.problem_id AND t.owner_id=$3)) ORDER BY cp.position`, id, c.Status != "scheduled" || viewer == c.Owner, viewer, c.StartsAt)
	if err != nil {
		return c, err
	}
	c.Problems, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Problem, error) {
		var p Problem
		e := row.Scan(&p.ID, &p.Points, &p.Title, &p.Solved)
		return p, e
	})
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

func (s *Store) Save(ctx context.Context, owner, id string, in Input, validate func(problems.Draft) bool) (err error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	defer func() {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23503") {
			err = ErrConflict
		}
	}()
	if in.Version == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,penalty_minutes)
 SELECT $1,$2,$3,$4,$5,$6,$7 WHERE $5>clock_timestamp()`, id, owner, in.Title, in.Description, in.StartsAt, in.EndsAt, *in.PenaltyMinutes)
	} else {
		// Serialize schedule edits and release against the contest row.
		var current int64
		err = tx.QueryRow(ctx, `SELECT version FROM contests WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&current)
		if err != nil {
			return err
		}
		if current != in.Version {
			return ErrConflict
		}
		tag, e := tx.Exec(ctx, `UPDATE contests SET title=$3,description=$4,starts_at=$5,ends_at=$6,penalty_minutes=$7,version=version+1
  WHERE id=$1 AND owner_id=$2 AND starts_at>clock_timestamp() AND $5>clock_timestamp()`, id, owner, in.Title, in.Description, in.StartsAt, in.EndsAt, *in.PenaltyMinutes)
		err = e
		if err == nil && tag.RowsAffected() != 1 {
			return ErrConflict
		}
	}
	if err != nil {
		return err
	}
	ids := make([]string, len(in.Problems))
	for i, p := range in.Problems {
		ids[i] = p.ID
	}
	// Lock in UUID order before checking publication; publishing uses the same row lock.
	rows, err := tx.Query(ctx, `SELECT id,draft,version FROM problem_drafts WHERE id=ANY($1::uuid[]) AND owner_id=$2 AND published_draft IS NULL ORDER BY id FOR UPDATE`, ids, owner)
	if err != nil {
		return err
	}
	type snapshot struct {
		data    []byte
		version int64
	}
	snapshots := map[string]snapshot{}
	for rows.Next() {
		var pid string
		var snap snapshot
		if err = rows.Scan(&pid, &snap.data, &snap.version); err != nil {
			rows.Close()
			return err
		}
		snapshots[pid] = snap
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(snapshots) != len(in.Problems) {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM contest_problems WHERE contest_id=$1`, id); err != nil {
		return err
	}
	for i, p := range in.Problems {
		snap := snapshots[p.ID]
		var d problems.Draft
		if json.Unmarshal(snap.data, &d) != nil || !validate(d) {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, `INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) VALUES($1,$2,$3,$4,$5,$6)`, id, p.ID, i, p.Points, snap.data, snap.version)
		if err != nil {
			return err
		}
	}
	// Reject a save that crossed the start boundary while waiting for locks.
	var future bool
	if err = tx.QueryRow(ctx, `SELECT starts_at>clock_timestamp() FROM contests WHERE id=$1`, id).Scan(&future); err != nil {
		return err
	}
	if !future {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

// Release materializes due publications from the periodic dispatcher and before API requests.
// PublishedAt is the scheduled end, even if there was no traffic at that instant.
// No resident timer is needed, so the same behavior works in Lambda and locally.
func (s *Store) Release(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id FROM contests WHERE NOT released AND ends_at<=statement_timestamp() ORDER BY id FOR UPDATE`)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE problem_drafts d SET published_draft=cp.draft,published_version=d.version+1,published_at=c.ends_at,version=d.version+1
 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE d.id=cp.problem_id AND c.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE contests SET released=true WHERE id=ANY($1::uuid[])`, ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Problem(ctx context.Context, id, pid, viewer string) (problems.PublicProblem, error) {
	var p problems.PublicProblem
	var raw []byte
	var editorial bool
	err := s.Pool.QueryRow(ctx, `SELECT cp.problem_id,cp.draft,u.handle,c.ends_at,
 (statement_timestamp()>=c.ends_at OR c.owner_id=$3 OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=cp.problem_id AND t.owner_id=$3))
 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id JOIN user_profiles u ON u.owner_id=c.owner_id
 WHERE c.id=$1 AND cp.problem_id=$2 AND (statement_timestamp()>=c.starts_at OR c.owner_id=$3 OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=cp.problem_id AND t.owner_id=$3))`, id, pid, viewer).Scan(&p.ID, &raw, &p.Author, &p.PublishedAt, &editorial)
	if err != nil {
		return p, err
	}
	var d problems.Draft
	if err = json.Unmarshal(raw, &d); err != nil {
		return p, err
	}
	p.Title = d.Title
	p.Markdown = d.Markdown
	p.Difficulty = d.Difficulty
	p.TimeLimitMS = d.TimeLimitMS
	p.MemoryLimitMB = d.MemoryLimitMB
	p.SpecialJudge = d.Checker != nil
	p.Interactive = d.Interactor != nil
	if editorial {
		p.Editorial = d.Editorial
	}
	p.Testers, err = problems.New(s.Pool).Testers(ctx, pid)
	return p, err
}
