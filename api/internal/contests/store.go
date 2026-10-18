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

	"judge/api/internal/database"
	"judge/api/internal/problems"
)

var ErrConflict = errors.New("contest is locked or problems are unavailable")

type Problem struct {
	Solved bool `json:"solved,omitempty"`
	// HasEditorial is set only when the viewer may read the editorial now. False means none is registered.
	HasEditorial  *bool  `json:"hasEditorial,omitempty"`
	ID            string `json:"id"`
	Points        int    `json:"points"`
	Title         string `json:"title,omitempty"`
	TimeLimitMS   string `json:"timeLimitMs,omitempty"`
	MemoryLimitMB string `json:"memoryLimitMb,omitempty"`
	// Unmanaged tells setters that no current setter authors or tests the problem, so saving fails until it is removed.
	Unmanaged bool `json:"unmanaged,omitempty"`
}

// Official means the viewer would count in the standings.
// Staff means the viewer reviews every submission and never competes (ADR 0022).
// Authors are the setters, owner first; Author is the owner alone. IsSetter and IsOwner describe the viewer.
type Contest struct {
	ID                 string    `json:"id"`
	Owner              string    `json:"-"`
	Author             string    `json:"author"`
	Authors            []string  `json:"authors"`
	ProblemAuthors     []string  `json:"problemAuthors"`
	Testers            []string  `json:"testers"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	StartsAt           time.Time `json:"startsAt"`
	EndsAt             time.Time `json:"endsAt"`
	PenaltyMinutes     int       `json:"penaltyMinutes"`
	Version            int64     `json:"version"`
	Status             string    `json:"status"`
	CanEdit            bool      `json:"canEdit"`
	CanViewSubmissions bool      `json:"canViewSubmissions"`
	Participating      bool      `json:"participating"`
	Official           bool      `json:"official"`
	Staff              bool      `json:"staff"`
	IsSetter           bool      `json:"setter"`
	IsOwner            bool      `json:"owner"`
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

// coSetters lists the co-setters of contest c in the order they joined.
const coSetters = `ARRAY(SELECT su.handle FROM contest_setters s JOIN user_profiles su ON su.owner_id=s.owner_id
 WHERE s.contest_id=c.id AND s.left_at IS NULL ORDER BY s.created_at,s.owner_id)`

const fields = `c.id,c.owner_id,u.handle,` + coSetters + `,c.title,c.description,c.starts_at,c.ends_at,c.penalty_minutes,c.version,
 CASE WHEN statement_timestamp()<c.starts_at THEN 'scheduled' WHEN statement_timestamp()<c.ends_at THEN 'running' ELSE 'ended' END`

func scan(row pgx.Row) (Contest, error) {
	var c Contest
	var others []string
	err := row.Scan(&c.ID, &c.Owner, &c.Author, &others, &c.Title, &c.Description, &c.StartsAt, &c.EndsAt, &c.PenaltyMinutes, &c.Version, &c.Status)
	c.Authors = append([]string{c.Author}, others...)
	c.Problems = []Problem{}
	c.ProblemAuthors = []string{}
	c.Testers = []string{}
	return c, err
}

func (s *Store) List(ctx context.Context, owner string, offset int) ([]Contest, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+fields+` FROM contests c JOIN user_profiles u ON u.owner_id=c.owner_id
 WHERE ($1='' OR contest_setter(c.id,$1)) ORDER BY c.starts_at DESC,c.created_at DESC,c.id DESC LIMIT 51 OFFSET $2`, owner, offset)
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
	defer func() { _ = tx.Rollback(ctx) }()
	c, err := scan(tx.QueryRow(ctx, `SELECT `+fields+` FROM contests c JOIN user_profiles u ON u.owner_id=c.owner_id WHERE c.id=$1`, id))
	if err != nil {
		return c, err
	}
	var final bool
	err = tx.QueryRow(ctx, `SELECT contest_setter(c.id,$2),c.owner_id=$2,contest_staff(c.id,$2),$2<>'' AND contest_competitor(c,$2),contest_results_final(c),
 EXISTS(SELECT 1 FROM contest_participants p WHERE p.contest_id=c.id AND p.owner_id=$2),
 COALESCE((SELECT array_agg(DISTINCT u.handle ORDER BY u.handle)
  FROM contest_problems cp JOIN problem_drafts d ON d.id=cp.problem_id JOIN user_profiles u ON u.owner_id=d.owner_id
  WHERE cp.contest_id=c.id), ARRAY[]::text[]),
 COALESCE((SELECT array_agg(DISTINCT u.handle ORDER BY u.handle)
  FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id JOIN user_profiles u ON u.owner_id=t.owner_id
  WHERE cp.contest_id=c.id), ARRAY[]::text[])
 FROM contests c WHERE c.id=$1`, id, viewer).Scan(&c.IsSetter, &c.IsOwner, &c.Staff, &c.Official, &final, &c.Participating, &c.ProblemAuthors, &c.Testers)
	if err != nil {
		return c, err
	}
	c.CanEdit = c.IsSetter && c.Status == "scheduled"
	c.CanViewSubmissions = final || c.Staff
	rows, err := tx.Query(ctx, `SELECT cp.problem_id,cp.points,cp.draft->>'title',cp.draft->>'timeLimitMs',cp.draft->>'memoryLimitMb',
 EXISTS(SELECT 1 FROM submissions s WHERE s.contest_id=cp.contest_id AND s.problem_id=cp.problem_id
  AND s.owner_id=$3 AND s.status='DONE' AND s.result->>'verdict'='AC'
  AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
  AND NOT COALESCE((s.job->>'generate')::boolean,false)
  AND NOT COALESCE((s.job->>'validate')::boolean,false)
  AND s.created_at>=$4),
 ($2 OR contest_problem_access(cp.contest_id,cp.problem_id,$3)),
 CASE WHEN ($5 OR contest_problem_access(cp.contest_id,cp.problem_id,$3))
  AND NOT COALESCE(statement_timestamp()<featured_reveal_at(d.id),false) THEN COALESCE(cp.draft->>'editorial','')<>'' END,
 $6 AND NOT (contest_setter(cp.contest_id,d.owner_id)
  OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=d.id AND contest_setter(cp.contest_id,t.owner_id)))
 FROM contest_problems cp JOIN problem_drafts d ON d.id=cp.problem_id
 WHERE cp.contest_id=$1 ORDER BY cp.position`, id, c.Status != "scheduled", viewer, c.StartsAt, c.Status == "ended", c.CanEdit)
	if err != nil {
		return c, err
	}
	c.Problems, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Problem, error) {
		var p Problem
		var visible bool
		e := row.Scan(&p.ID, &p.Points, &p.Title, &p.TimeLimitMS, &p.MemoryLimitMB, &p.Solved, &visible, &p.HasEditorial, &p.Unmanaged)
		if !visible {
			p.ID, p.Title, p.TimeLimitMS, p.MemoryLimitMB, p.Solved, p.HasEditorial = "", "", "", "", false, nil
		}
		return p, e
	})
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

func (s *Store) Save(ctx context.Context, owner, id string, in Input, policy JudgePolicy) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = save(ctx, tx, owner, id, in, policy, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// save writes a contest inside tx for actor, a setter. Publishing a draft passes its owner, who keeps owning the
// contest; it uses no quota, since the draft already used one, and the draft's problems were checked as they were added.
func save(ctx context.Context, tx pgx.Tx, actor, id string, in Input, policy JudgePolicy, draftOwner string) (err error) {
	defer func() {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23503") {
			err = ErrConflict
		}
	}()
	publishing := draftOwner != ""
	if in.Version == 0 {
		owner := draftOwner
		if !publishing {
			owner = actor
			if err = database.ConsumeCreationQuota(ctx, tx, owner, "contest"); err != nil {
				return err
			}
		}
		// A draft's id belongs to its setters: a contest created over it would make them setters of someone else's
		// contest, and its owner a setter of their draft.
		if err = lockID(ctx, tx, id); err != nil {
			return err
		}
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,penalty_minutes)
 SELECT $1,$2,$3,$4,$5,$6,$7 WHERE $5>clock_timestamp() AND ($8 OR NOT EXISTS (SELECT 1 FROM contest_drafts WHERE id=$1))`, id, owner, in.Title, in.Description, in.StartsAt, in.EndsAt, *in.PenaltyMinutes, publishing)
		if err == nil && tag.RowsAffected() != 1 {
			return ErrConflict
		}
	} else {
		// Serialize schedule edits and release against the contest row.
		var current int64
		err = tx.QueryRow(ctx, `SELECT version FROM contests WHERE id=$1 AND contest_setter(id,$2) FOR UPDATE`, id, actor).Scan(&current)
		if err != nil {
			return err
		}
		if current != in.Version {
			return ErrConflict
		}
		tag, e := tx.Exec(ctx, `UPDATE contests SET title=$3,description=$4,starts_at=$5,ends_at=$6,penalty_minutes=$7,version=version+1
  WHERE id=$1 AND contest_setter(id,$2) AND starts_at>clock_timestamp() AND $5>clock_timestamp()`, id, actor, in.Title, in.Description, in.StartsAt, in.EndsAt, *in.PenaltyMinutes)
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
	// Every problem needs a current setter as its author or invited tester, and a setter adds only problems they
	// manage that way themselves; rights a setter has through the contest never count (ADR 0022).
	rows, err := tx.Query(ctx, `SELECT d.id,d.draft,d.version FROM problem_drafts d
 WHERE d.id=ANY($1::uuid[]) AND d.published_draft IS NULL AND d.scheduled_at IS NULL
 AND (contest_setter($3,d.owner_id) OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=d.id AND contest_setter($3,t.owner_id)))
 AND ($4 OR can_manage_problem_directly(d.id,$2) OR EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.contest_id=$3 AND cp.problem_id=d.id))
 ORDER BY d.id FOR UPDATE OF d`, ids, actor, id, publishing)
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
	// A 定期便 application came first; its author withdraws it before the problem can enter a contest.
	var applied bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM featured_applications WHERE problem_id=ANY($1::uuid[]))`, ids).Scan(&applied); err != nil {
		return err
	}
	if applied {
		return ErrConflict
	}
	joined, err := participantProblem(ctx, tx, id, ids)
	if err != nil {
		return err
	}
	if joined {
		return ErrParticipantProblem
	}
	// The author and testers of a removed problem have seen it, so they stay out of the contest.
	if _, err = tx.Exec(ctx, `INSERT INTO contest_exclusions(contest_id,owner_id)
 SELECT cp.contest_id,x.owner_id FROM contest_problems cp JOIN problem_drafts d ON d.id=cp.problem_id
 CROSS JOIN LATERAL (SELECT d.owner_id UNION SELECT t.owner_id FROM problem_testers t WHERE t.problem_id=cp.problem_id) x
 WHERE cp.contest_id=$1 AND cp.problem_id<>ALL($2::uuid[]) ON CONFLICT DO NOTHING`, id, ids); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM contest_problems WHERE contest_id=$1`, id); err != nil {
		return err
	}
	for i, p := range in.Problems {
		snap := snapshots[p.ID]
		var d problems.Draft
		if json.Unmarshal(snap.data, &d) != nil || !policy.validProblem(d) {
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
	return nil
}

// Release materializes due publications from the periodic dispatcher and before API requests.
// PublishedAt is the scheduled end, even if there was no traffic at that instant.
// No resident timer is needed, so the same behavior works in Lambda and locally.
func (s *Store) Release(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
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

// ContestProblem is a problem as its contest shows it. Author is the contest owner and Authors all of its setters,
// owner first. CanReadEditorial tells whether the editorial, if one is registered, is returned to this viewer now.
type ContestProblem struct {
	problems.PublicProblem
	Authors          []string `json:"authors"`
	CanReadEditorial bool     `json:"canReadEditorial"`
}

func (s *Store) Problem(ctx context.Context, id, pid, viewer string) (ContestProblem, error) {
	var p ContestProblem
	var raw []byte
	var editorial bool
	var others []string
	err := s.Pool.QueryRow(ctx, `SELECT cp.problem_id,cp.draft,u.handle,`+coSetters+`,c.ends_at,featured_reveal_at(d.id),COALESCE(statement_timestamp()<featured_reveal_at(d.id),false),
 (statement_timestamp()>=c.ends_at OR contest_problem_access(c.id,cp.problem_id,$3))
 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id JOIN problem_drafts d ON d.id=cp.problem_id JOIN user_profiles u ON u.owner_id=c.owner_id
 WHERE c.id=$1 AND cp.problem_id=$2 AND (statement_timestamp()>=c.starts_at OR contest_problem_access(c.id,cp.problem_id,$3))`, id, pid, viewer).Scan(&p.ID, &raw, &p.Author, &others, &p.PublishedAt, &p.EditorialRevealAt, &p.EditorialHidden, &editorial)
	if err != nil {
		return p, err
	}
	p.Authors = append([]string{p.Author}, others...)
	p.CanReadEditorial = editorial && !p.EditorialHidden
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
	p.HasSamples = d.HasSamples()
	if editorial && !p.EditorialHidden {
		p.Editorial = d.Editorial
	}
	p.Testers, err = problems.New(s.Pool).Testers(ctx, pid)
	return p, err
}
