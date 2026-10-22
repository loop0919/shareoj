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
	rows, err := s.Pool.Query(ctx, `SELECT q.id,q.problem_id,q.question,u.handle,q.answer,COALESCE(a.handle,''),
 q.answered_at,q.published_at,q.created_at,q.version,contest_clarification_manager(q.contest_id,q.problem_id,$2)
 FROM contest_clarifications q JOIN user_profiles u ON u.owner_id=q.owner_id
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
	tag, err := s.Pool.Exec(ctx, `INSERT INTO contest_clarifications(contest_id,problem_id,owner_id,question)
 SELECT c.id,cp.problem_id,$3,$4 FROM contests c JOIN contest_problems cp ON cp.contest_id=c.id
 WHERE c.id=$1 AND cp.problem_id=$2 AND c.starts_at<=statement_timestamp() AND statement_timestamp()<c.ends_at
 AND contest_competitor(c,$3) AND EXISTS(SELECT 1 FROM contest_participants p WHERE p.contest_id=c.id AND p.owner_id=$3)`, contest, problem, actor, question)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrClarificationUnavailable
	}
	return err
}

// A version binds publication to the text the manager reviewed, and prevents lost answers.
func (s *Store) UpdateClarification(ctx context.Context, contest, id, actor string, version int64, answer *string, publish bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	err = tx.QueryRow(ctx, `SELECT version FROM contest_clarifications
 WHERE contest_id=$1 AND id=$2 AND contest_clarification_manager(contest_id,problem_id,$3) FOR UPDATE`, contest, id, actor).Scan(&current)
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
	return tx.Commit(ctx)
}
