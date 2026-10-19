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

// ErrPointsRequired means an all-or-nothing problem came without points; a partially scored one takes its full score.
var ErrPointsRequired = errors.New("contest problem needs points")

type Problem struct {
	// Solved is set when the viewer, or their team, has the problem's full score, summing each group's best score
	// across their runs like the standings; for an all-or-nothing problem that is an AC (ADR 0025).
	Solved bool `json:"solved,omitempty"`
	// Partial marks a partially scored problem, whose Points are its full score.
	Partial bool `json:"partial,omitempty"`
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
// Staff means the viewer reviews every submission and never competes (ADR 0022). Excluded means a signed-in viewer
// who is not staff now but served as staff, so cannot compete; it is false for anyone signed out.
// Authors are the setters, owner first; Author is the owner alone. IsSetter and IsOwner describe the viewer.
type Contest struct {
	ID             string    `json:"id"`
	Owner          string    `json:"-"`
	Author         string    `json:"author"`
	Authors        []string  `json:"authors"`
	ProblemAuthors []string  `json:"problemAuthors"`
	Testers        []string  `json:"testers"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	PenaltyMinutes int       `json:"penaltyMinutes"`
	// FreezeMinutes freezes the public standings for the last minutes of the contest (ADR 0023); null means no freeze.
	// Submissions created from FreezeAt until the end stay hidden from others until the results are revealed.
	FreezeMinutes *int       `json:"freezeMinutes"`
	FreezeAt      *time.Time `json:"freezeAt"`
	Revealed      bool       `json:"revealed"`
	RevealedAt    *time.Time `json:"revealedAt"`
	// ResultsFinal is true after the end without a freeze, and after the reveal with one.
	ResultsFinal bool `json:"resultsFinal"`
	// CanViewLiveStandings lets staff request the unfrozen standings from GET /my/contests/{id}/standings?live=1.
	CanViewLiveStandings bool `json:"canViewLiveStandings"`
	// CanRevealResults lets a setter or an operator reveal an ended frozen contest (POST /my/contests/{id}/reveal).
	CanRevealResults bool `json:"canRevealResults"`
	// PartialScoring tells whether any problem gives partial scores (ADR 0025), even before the problems are shown.
	PartialScoring bool `json:"partialScoring"`
	// TeamMode lets participants form teams, each a row of the standings; those who enter alone keep rows of their own
	// (ADR 0024). MaxTeamSize is null in individual contests and in team contests without a limit of their own;
	// MaxTeamMembers still caps every team. OnsiteDivision tells whether the venue passphrase can make participants
	// onsite; the passphrase itself is only in setter responses.
	TeamMode           bool   `json:"teamMode"`
	MaxTeamSize        *int   `json:"maxTeamSize"`
	OnsiteDivision     bool   `json:"onsiteDivision"`
	Version            int64  `json:"version"`
	Status             string `json:"status"`
	CanEdit            bool   `json:"canEdit"`
	CanViewSubmissions bool   `json:"canViewSubmissions"`
	Participating      bool   `json:"participating"`
	Official           bool   `json:"official"`
	Staff              bool   `json:"staff"`
	Excluded           bool   `json:"excluded"`
	IsSetter           bool   `json:"setter"`
	IsOwner            bool   `json:"owner"`
	// Team is the viewer's team in a team contest, without its join code, and null for someone who entered alone.
	// Onsite tells whether the viewer, or their team, is in the onsite division.
	Team     *TeamSummary `json:"team"`
	Onsite   bool         `json:"onsite"`
	Problems []Problem    `json:"problems"`
}
type Input struct {
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	PenaltyMinutes *int      `json:"penaltyMinutes"`
	FreezeMinutes  Minutes   `json:"freezeMinutes"`
	// The participation settings (ADR 0024). A client from before them leaves them out, and its saves keep the stored
	// values. OnsitePassphrase is read only when OnsiteDivision is sent as true, and is then required.
	TeamMode         *bool         `json:"teamMode"`
	MaxTeamSize      Nullable[int] `json:"maxTeamSize"`
	OnsiteDivision   *bool         `json:"onsiteDivision"`
	OnsitePassphrase *string       `json:"onsitePassphrase"`
	Version          int64         `json:"version"`
	Problems         []Problem     `json:"problems"`
}
type Store struct{ Pool *pgxpool.Pool }

// Nullable is a nullable value that remembers whether the client sent it. A client from before the setting existed
// leaves it out, and its saves keep the stored value instead of clearing it; null clears it.
type Nullable[T any] struct {
	Value *T
	Sent  bool
}

// Minutes is a nullable number of minutes.
type Minutes = Nullable[int]

func (m *Nullable[T]) UnmarshalJSON(data []byte) error {
	m.Sent = true
	return json.Unmarshal(data, &m.Value)
}

func (m Nullable[T]) MarshalJSON() ([]byte, error) { return json.Marshal(m.Value) }

// Revealable reports whether the contest has ended with its results frozen and not yet revealed.
func (c Contest) Revealable() bool {
	return c.FreezeMinutes != nil && !c.Revealed && c.Status == "ended"
}

// coSetters lists the co-setters of contest c in the order they joined.
const coSetters = `ARRAY(SELECT su.handle FROM contest_setters s JOIN user_profiles su ON su.owner_id=s.owner_id
 WHERE s.contest_id=c.id AND s.left_at IS NULL ORDER BY s.created_at,s.owner_id)`

const fields = `c.id,c.owner_id,u.handle,` + coSetters + `,c.title,c.description,c.starts_at,c.ends_at,c.penalty_minutes,c.freeze_minutes,
 contest_freeze_at(c),c.revealed_at,contest_results_final(c),c.team_mode,c.max_team_size,c.onsite_division,c.version,
 CASE WHEN statement_timestamp()<c.starts_at THEN 'scheduled' WHEN statement_timestamp()<c.ends_at THEN 'running' ELSE 'ended' END,
 EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.contest_id=c.id AND jsonb_typeof(cp.draft->'scoring')='object')`

func scan(row pgx.Row) (Contest, error) {
	var c Contest
	var others []string
	err := row.Scan(&c.ID, &c.Owner, &c.Author, &others, &c.Title, &c.Description, &c.StartsAt, &c.EndsAt, &c.PenaltyMinutes, &c.FreezeMinutes, &c.FreezeAt, &c.RevealedAt, &c.ResultsFinal, &c.TeamMode, &c.MaxTeamSize, &c.OnsiteDivision, &c.Version, &c.Status, &c.PartialScoring)
	c.Revealed = c.RevealedAt != nil
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
	var team struct {
		ID      *string
		Name    string
		Members []string
	}
	err = tx.QueryRow(ctx, `SELECT contest_setter(c.id,$2),c.owner_id=$2,contest_staff(c.id,$2),$2<>'' AND contest_competitor(c,$2),
 $2<>'' AND NOT contest_staff(c.id,$2) AND NOT contest_competitor(c,$2),
 vp.owner_id IS NOT NULL,c.onsite_division AND COALESCE(CASE WHEN vt.id IS NULL THEN vp.onsite_at ELSE vt.onsite_at END IS NOT NULL,false),
 vt.id,COALESCE(vt.name,''),`+members("vt")+`,
 COALESCE((SELECT array_agg(DISTINCT u.handle ORDER BY u.handle)
  FROM contest_problems cp JOIN problem_drafts d ON d.id=cp.problem_id JOIN user_profiles u ON u.owner_id=d.owner_id
  WHERE cp.contest_id=c.id), ARRAY[]::text[]),
 COALESCE((SELECT array_agg(DISTINCT u.handle ORDER BY u.handle)
  FROM contest_problems cp JOIN problem_testers t ON t.problem_id=cp.problem_id JOIN user_profiles u ON u.owner_id=t.owner_id
  WHERE cp.contest_id=c.id), ARRAY[]::text[])
 FROM contests c LEFT JOIN contest_participants vp ON vp.contest_id=c.id AND vp.owner_id=$2
 LEFT JOIN contest_teams vt ON vt.id=vp.team_id AND c.team_mode
 WHERE c.id=$1`, id, viewer).Scan(&c.IsSetter, &c.IsOwner, &c.Staff, &c.Official, &c.Excluded, &c.Participating, &c.Onsite, &team.ID, &team.Name, &team.Members, &c.ProblemAuthors, &c.Testers)
	if err != nil {
		return c, err
	}
	if team.ID != nil {
		c.Team = &TeamSummary{ID: *team.ID, Name: team.Name, Members: team.Members, Onsite: c.Onsite}
	}
	c.CanEdit = c.IsSetter && c.Status == "scheduled"
	// Pre-freeze submissions become public at the end; frozen ones are filtered row by row. Members of a team see their
	// team's submissions during the contest (ADR 0024).
	c.CanViewSubmissions = c.Status == "ended" || c.Staff || (c.Team != nil && c.Status == "running")
	c.CanViewLiveStandings = c.Staff && c.FreezeMinutes != nil
	c.CanRevealResults = c.IsSetter && c.Revealable()
	// In a team contest a problem is solved when any member has solved it (ADR 0024).
	rows, err := tx.Query(ctx, `SELECT cp.problem_id,cp.points,cp.draft->>'title',cp.draft->>'timeLimitMs',cp.draft->>'memoryLimitMb',
 EXISTS(SELECT 1 FROM submissions s WHERE `+solvedRun+` AND submission_full_score(s.result))
  OR COALESCE((SELECT sum(best) FROM (SELECT max((x.g->>'score')::bigint) AS best FROM submissions s
   CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(s.result->'scoring'->'groups')='array' THEN s.result->'scoring'->'groups' ELSE '[]' END) WITH ORDINALITY x(g,i)
   WHERE `+solvedRun+` GROUP BY x.i) groups),0)>=cp.points::bigint*100,
 ($2 OR contest_problem_access(cp.contest_id,cp.problem_id,$3)),
 CASE WHEN ($5 OR contest_problem_access(cp.contest_id,cp.problem_id,$3))
  AND NOT COALESCE(statement_timestamp()<featured_reveal_at(d.id),false) THEN COALESCE(cp.draft->>'editorial','')<>'' END,
 $6 AND NOT (contest_setter(cp.contest_id,d.owner_id)
  OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=d.id AND contest_setter(cp.contest_id,t.owner_id))),
 COALESCE(jsonb_typeof(cp.draft->'scoring')='object',false)
 FROM contest_problems cp JOIN problem_drafts d ON d.id=cp.problem_id
 WHERE cp.contest_id=$1 ORDER BY cp.position`, id, c.Status != "scheduled", viewer, c.StartsAt, c.Status == "ended", c.CanEdit)
	if err != nil {
		return c, err
	}
	c.Problems, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Problem, error) {
		var p Problem
		var visible bool
		e := row.Scan(&p.ID, &p.Points, &p.Title, &p.TimeLimitMS, &p.MemoryLimitMB, &p.Solved, &visible, &p.HasEditorial, &p.Unmanaged, &p.Partial)
		if !visible {
			p.ID, p.Title, p.TimeLimitMS, p.MemoryLimitMB, p.Solved, p.HasEditorial, p.Partial = "", "", "", "", false, nil, false
		}
		return p, e
	})
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

// solvedRun picks the viewer's runs of a contest problem that can solve it, and their team's in a team contest: judged
// runs from the start on, practice after the end included.
const solvedRun = `s.contest_id=cp.contest_id AND s.problem_id=cp.problem_id
  AND (s.owner_id=$3 OR s.owner_id IN (SELECT unnest(contest_teammates($1::uuid,$3)))) AND s.status='DONE'
  AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
  AND NOT COALESCE((s.job->>'generate')::boolean,false)
  AND NOT COALESCE((s.job->>'validate')::boolean,false)
  AND s.created_at>=$4`

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
		teamMode, division := in.TeamMode != nil && *in.TeamMode, in.OnsiteDivision != nil && *in.OnsiteDivision
		var size *int
		if teamMode {
			size = in.MaxTeamSize.Value
		}
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at,penalty_minutes,freeze_minutes,
 team_mode,max_team_size,onsite_division,onsite_passphrase)
 SELECT $1,$2,$3,$4,$5,$6,$7,$9,$10,$11,$12,$13 WHERE $5>clock_timestamp() AND ($8 OR NOT EXISTS (SELECT 1 FROM contest_drafts WHERE id=$1))`, id, owner, in.Title, in.Description, in.StartsAt, in.EndsAt, *in.PenaltyMinutes, publishing, in.FreezeMinutes.Value,
			teamMode, size, division, in.passphrase())
		if err == nil && tag.RowsAffected() != 1 {
			return ErrConflict
		}
	} else {
		// Serialize schedule edits and release against the contest row.
		var current int64
		var teamMode bool
		err = tx.QueryRow(ctx, `SELECT version,team_mode FROM contests WHERE id=$1 AND contest_setter(id,$2) FOR UPDATE`, id, actor).Scan(&current, &teamMode)
		if err != nil {
			return err
		}
		if current != in.Version {
			return ErrConflict
		}
		// A save without freezeMinutes keeps the freeze, which must still fit the new schedule. Participation settings
		// left out are kept too; an individual contest has no team size, and a contest without the division no passphrase.
		tag, e := tx.Exec(ctx, `UPDATE contests SET title=$3,description=$4,starts_at=$5,ends_at=$6,penalty_minutes=$7,
  freeze_minutes=CASE WHEN $9 THEN $8 ELSE freeze_minutes END,
  team_mode=COALESCE($10,team_mode),
  max_team_size=CASE WHEN NOT COALESCE($10,team_mode) THEN NULL WHEN $12 THEN $11 ELSE max_team_size END,
  onsite_division=COALESCE($13,onsite_division),
  onsite_passphrase=CASE WHEN COALESCE($13,onsite_division) THEN COALESCE($14,onsite_passphrase) END,
  version=version+1
  WHERE id=$1 AND contest_setter(id,$2) AND starts_at>clock_timestamp() AND $5>clock_timestamp()
  AND ($9 OR freeze_minutes IS NULL OR make_interval(mins => freeze_minutes)<=$6-$5)`, id, actor, in.Title, in.Description, in.StartsAt, in.EndsAt, *in.PenaltyMinutes, in.FreezeMinutes.Value, in.FreezeMinutes.Sent,
			in.TeamMode, in.MaxTeamSize.Value, in.MaxTeamSize.Sent, in.OnsiteDivision, in.passphrase())
		err = e
		if err == nil && tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if err == nil {
			err = keepRegistrations(ctx, tx, id, teamMode, in)
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
		// A partially scored problem is worth its full score, which problem saves keep in step until the start (ADR 0025).
		points := p.Points
		if d.Scoring != nil {
			points = d.Scoring.FullScore()
		}
		if points < 1 {
			return ErrPointsRequired
		}
		_, err = tx.Exec(ctx, `INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) VALUES($1,$2,$3,$4,$5,$6)`, id, p.ID, i, points, snap.data, snap.version)
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
	p.Scoring = d.PublicScoring()
	if editorial && !p.EditorialHidden {
		p.Editorial = d.Editorial
	}
	p.Testers, err = problems.New(s.Pool).Testers(ctx, pid)
	return p, err
}
