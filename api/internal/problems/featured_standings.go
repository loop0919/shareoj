package problems

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type FeaturedResult struct {
	Accepted bool   `json:"accepted"`
	TimeMS   *int64 `json:"timeMs"`
	Wrong    int64  `json:"wrong"`
}

type FeaturedStanding struct {
	Rank   int64          `json:"rank"`
	Handle string         `json:"handle"`
	TimeMS *int64         `json:"timeMs"`
	Easy   FeaturedResult `json:"easy"`
	Hard   FeaturedResult `json:"hard"`
}

type FeaturedStandings struct {
	Round    FeaturedRound      `json:"round"`
	ClosesAt time.Time          `json:"closesAt"`
	Closed   bool               `json:"closed"`
	Items    []FeaturedStanding `json:"items"`
	HasMore  bool               `json:"hasMore"`
}

func (s *Store) FeaturedStandings(ctx context.Context, at time.Time, offset int) (FeaturedStandings, error) {
	page := FeaturedStandings{Round: FeaturedRound{ScheduledAt: at, Slots: []FeaturedSlot{}}, ClosesAt: at.Add(23 * time.Hour), Items: []FeaturedStanding{}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = tx.QueryRow(ctx, `SELECT count(DISTINCT scheduled_at) FROM featured_slots WHERE scheduled_at<=$1`, at).Scan(&page.Round.Number); err != nil {
		return page, err
	}
	rows, err := tx.Query(ctx, `SELECT f.slot,f.kind,
 CASE WHEN d.published_draft IS NOT NULL THEN d.id::text ELSE '' END,
 COALESCE(d.published_draft->>'title',''),f.difficulty,f.reveal_at,
 f.kind='new' AND statement_timestamp()<f.reveal_at,COALESCE(u.handle,''),
 ARRAY(SELECT u.handle FROM problem_testers t JOIN user_profiles u ON u.owner_id=t.owner_id WHERE t.problem_id=d.id AND d.published_draft IS NOT NULL ORDER BY u.handle),
 statement_timestamp()>=f.scheduled_at+interval '23 hours'
 FROM featured_slots f LEFT JOIN problem_drafts d ON d.id=f.problem_id
 LEFT JOIN user_profiles u ON u.owner_id=d.owner_id AND d.published_draft IS NOT NULL
 WHERE f.scheduled_at=$1 AND f.scheduled_at<=statement_timestamp() ORDER BY f.slot`, at)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var slot FeaturedSlot
		if err = rows.Scan(&slot.Slot, &slot.Kind, &slot.ProblemID, &slot.Title, &slot.Difficulty, &slot.RevealAt, &slot.EditorialHidden, &slot.Writer, &slot.Testers, &page.Closed); err != nil {
			rows.Close()
			return page, err
		}
		page.Round.Slots = append(page.Round.Slots, slot)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(page.Round.Slots) == 0 {
		return page, ErrNotFound
	}
	// Participation needs an ordinary public submission in this edition's window.
	// Rank only by the solved set: both > Hard > Easy > neither. Completion time,
	// wrong answers and repeat ACs never change the rank. Within a tied rank,
	// show earlier completion first; zero-solvers use their first submission.
	rows, err = tx.Query(ctx, `WITH attempts AS (
 SELECT u.handle,f.slot,s.created_at,
 CASE WHEN s.status='DONE' THEN COALESCE(s.result->>'verdict','') ELSE '' END AS verdict
 FROM featured_slots f JOIN submissions s ON s.problem_id=f.problem_id
 JOIN user_profiles u ON u.owner_id=s.owner_id
 WHERE f.scheduled_at=$1 AND f.kind<>'missing'
 AND s.created_at>=$1 AND s.created_at<$1::timestamptz+interval '23 hours' AND s.created_at<=statement_timestamp()
 AND s.contest_id IS NULL AND NOT COALESCE((s.job->>'privateDraft')::boolean,true)
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 AND NOT COALESCE((s.job->>'generate')::boolean,false)
 AND NOT COALESCE((s.job->>'validate')::boolean,false)
 AND NOT EXISTS(SELECT 1 FROM featured_slots other WHERE other.scheduled_at=$1 AND can_manage_problem(other.problem_id,s.owner_id))
 ), solved AS (
 SELECT handle,slot,min(created_at) FILTER (WHERE verdict='AC') AS accepted_at,min(created_at) AS first_at
 FROM attempts GROUP BY handle,slot
 ), per_problem AS (
 SELECT s.handle,s.slot,s.accepted_at,s.first_at,
 count(*) FILTER (WHERE a.verdict IN ('WA','RE','TLE','MLE','OLE') AND (s.accepted_at IS NULL OR a.created_at<s.accepted_at)) AS wrong
 FROM solved s JOIN attempts a USING(handle,slot) GROUP BY s.handle,s.slot,s.accepted_at,s.first_at
 ), totals AS (
 SELECT handle,max(accepted_at) FILTER (WHERE slot='easy') AS easy_at,max(accepted_at) FILTER (WHERE slot='hard') AS hard_at,
 COALESCE(max(wrong) FILTER (WHERE slot='easy'),0) AS easy_wrong,COALESCE(max(wrong) FILTER (WHERE slot='hard'),0) AS hard_wrong,
 max(accepted_at) AS last_at,min(first_at) AS first_at FROM per_problem GROUP BY handle
 ), ranked AS (
 SELECT rank() OVER (ORDER BY (hard_at IS NOT NULL) DESC,(easy_at IS NOT NULL) DESC) AS rank,* FROM totals
 )
 SELECT rank,handle,(extract(epoch FROM last_at-$1::timestamptz)*1000)::bigint,
 easy_at IS NOT NULL,(extract(epoch FROM easy_at-$1::timestamptz)*1000)::bigint,easy_wrong,
 hard_at IS NOT NULL,(extract(epoch FROM hard_at-$1::timestamptz)*1000)::bigint,hard_wrong
 FROM ranked ORDER BY rank,COALESCE(last_at,first_at),handle LIMIT 51 OFFSET $2`, at, offset)
	if err != nil {
		return page, err
	}
	page.Items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (FeaturedStanding, error) {
		var item FeaturedStanding
		err := row.Scan(&item.Rank, &item.Handle, &item.TimeMS, &item.Easy.Accepted, &item.Easy.TimeMS, &item.Easy.Wrong, &item.Hard.Accepted, &item.Hard.TimeMS, &item.Hard.Wrong)
		return item, err
	})
	if err != nil {
		return page, err
	}
	page.HasMore = len(page.Items) > 50
	if page.HasMore {
		page.Items = page.Items[:50]
	}
	return page, tx.Commit(ctx)
}
