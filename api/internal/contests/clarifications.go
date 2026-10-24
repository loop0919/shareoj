package contests

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	ErrClarificationUnavailable = errors.New("questions require participation in a running contest")
	ErrClarificationConflict    = errors.New("clarification has changed")
)

type Clarification struct {
	ID          string     `json:"id"`
	ProblemID   string     `json:"problemId"`
	Question    string     `json:"question"`
	Author      string     `json:"author"`
	Answer      string     `json:"answer"`
	AnsweredBy  string     `json:"answeredBy"`
	AnsweredAt  *time.Time `json:"answeredAt"`
	PublishedAt *time.Time `json:"publishedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	Version     int64      `json:"version"`
	CanManage   bool       `json:"canManage"`
}

func ValidClarificationText(text string) bool {
	return strings.TrimSpace(text) != "" && utf8.ValidString(text) && !strings.ContainsRune(text, 0) && utf8.RuneCountInString(text) <= 4000
}

func (s *Store) Clarifications(ctx context.Context, contest, viewer string, offset int) ([]Clarification, error) {
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contests WHERE id=$1)`, contest).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, pgx.ErrNoRows
	}
	rows, err := s.Pool.Query(ctx, `SELECT q.id,COALESCE(q.problem_id::text,''),q.question,COALESCE(u.handle,''),q.answer,COALESCE(a.handle,''),
 q.answered_at,q.published_at,q.created_at,q.version,
 CASE WHEN q.owner_id IS NULL THEN contest_staff(q.contest_id,$2) ELSE contest_clarification_manager(q.contest_id,q.problem_id,$2) END
 FROM contest_clarifications q LEFT JOIN user_profiles u ON u.owner_id=q.owner_id
 LEFT JOIN user_profiles a ON a.owner_id=q.answered_by
 WHERE q.contest_id=$1 AND $2<>'' AND (q.published_at IS NOT NULL OR q.owner_id=$2
 OR contest_clarification_manager(q.contest_id,q.problem_id,$2))
 ORDER BY q.created_at DESC,q.id DESC LIMIT 51 OFFSET $3`, contest, viewer, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Clarification, error) {
		var q Clarification
		err := row.Scan(&q.ID, &q.ProblemID, &q.Question, &q.Author, &q.Answer, &q.AnsweredBy, &q.AnsweredAt, &q.PublishedAt, &q.CreatedAt, &q.Version, &q.CanManage)
		return q, err
	})
}

func (s *Store) AskClarification(ctx context.Context, contest, problem, actor, question string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO contest_clarifications(contest_id,problem_id,owner_id,question)
 SELECT c.id,cp.problem_id,$3,$4 FROM contests c JOIN contest_problems cp ON cp.contest_id=c.id
 WHERE c.id=$1 AND cp.problem_id=$2 AND c.starts_at<=statement_timestamp() AND statement_timestamp()<c.ends_at
 AND contest_competitor(c,$3) AND EXISTS(SELECT 1 FROM contest_participants p WHERE p.contest_id=c.id AND p.owner_id=$3)
 RETURNING id`, contest, problem, actor, question).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClarificationUnavailable
	}
	if err != nil {
		return err
	}
	// The recipients are the people for whom contest_clarification_manager holds.
	if _, err = tx.Exec(ctx, `INSERT INTO notifications(owner_id,contest_id,actor_id,kind)
 SELECT m.owner_id,$1,$3,'clarification_question' FROM (
  SELECT owner_id FROM contests WHERE id=$1
  UNION SELECT owner_id FROM problem_drafts WHERE id=$2
  UNION SELECT owner_id FROM problem_testers WHERE problem_id=$2) m WHERE m.owner_id<>$3`, contest, problem, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AnnounceClarification(ctx context.Context, contest, problem, actor, answer string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO contest_clarifications(contest_id,problem_id,question,answer,answered_by,answered_at,published_at)
 SELECT c.id,NULLIF($2,'')::uuid,'',$4,$3,statement_timestamp(),statement_timestamp() FROM contests c
 WHERE c.id=$1 AND contest_staff(c.id,$3)
 AND ($2='' OR EXISTS(SELECT 1 FROM contest_problems WHERE contest_id=c.id AND problem_id=NULLIF($2,'')::uuid))
 RETURNING id`, contest, problem, actor, answer).Scan(&id)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notifications(owner_id,contest_id,actor_id,kind)
 SELECT owner_id,contest_id,$2,'clarification_published' FROM contest_participants WHERE contest_id=$1 AND owner_id<>$2`, contest, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// A version binds publication to the text the manager reviewed, and prevents lost answers.
func (s *Store) UpdateClarification(ctx context.Context, contest, id, actor string, version int64, answer *string, publish bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	var asker string
	var published bool
	err = tx.QueryRow(ctx, `SELECT version,COALESCE(owner_id,''),published_at IS NOT NULL FROM contest_clarifications
 WHERE contest_id=$1 AND id=$2 AND
 CASE WHEN owner_id IS NULL THEN contest_staff(contest_id,$3) ELSE contest_clarification_manager(contest_id,problem_id,$3) END
 FOR UPDATE`, contest, id, actor).Scan(&current, &asker, &published)
	if err != nil {
		return err
	}
	if current != version {
		return ErrClarificationConflict
	}
	_, err = tx.Exec(ctx, `UPDATE contest_clarifications SET answer=COALESCE($2,answer),
 answered_by=CASE WHEN $2::text IS NULL THEN answered_by ELSE $3 END,
 answered_at=CASE WHEN $2::text IS NULL THEN answered_at ELSE clock_timestamp() END,
 published_at=CASE WHEN $4 THEN COALESCE(published_at,clock_timestamp()) ELSE published_at END,
 version=version+1 WHERE id=$1`, id, answer, actor, publish)
	if err != nil {
		return err
	}
	if answer != nil && asker != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO notifications(owner_id,contest_id,actor_id,kind) VALUES($1,$2,$3,'clarification_answer')`, asker, contest, actor); err != nil {
			return err
		}
	}
	if publish && !published {
		if _, err = tx.Exec(ctx, `INSERT INTO notifications(owner_id,contest_id,actor_id,kind)
 SELECT owner_id,contest_id,$2,'clarification_published' FROM contest_participants WHERE contest_id=$1 AND owner_id<>$2`, contest, actor); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
