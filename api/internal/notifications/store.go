package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	Store struct{ Pool *pgxpool.Pool }
	// Notification names a problem, or for kind contest_setter a contest or contest draft, with its title.
	Notification struct {
		ID        string    `json:"id"`
		ProblemID string    `json:"problemId,omitempty"`
		ContestID string    `json:"contestId,omitempty"`
		Title     string    `json:"title"`
		Actor     string    `json:"actor"`
		Kind      string    `json:"kind"`
		CreatedAt time.Time `json:"createdAt"`
	}
)

func (s *Store) List(ctx context.Context, owner string, read, history bool) ([]Notification, error) {
	// UPDATE ... RETURNING gives the bell exactly the notifications read by this
	// request. Events arriving afterwards stay unread.
	// A contest notification takes its title from the contest, or from the draft before publication.
	const columns = `SELECT n.id,COALESCE(n.problem_id::text,''),COALESCE(n.contest_id::text,''),COALESCE(p.draft->>'title',c.title,cd.draft->>'title',''),u.handle,n.kind,n.created_at`
	const joins = ` LEFT JOIN problem_drafts p ON p.id=n.problem_id LEFT JOIN contests c ON c.id=n.contest_id LEFT JOIN contest_drafts cd ON cd.id=n.contest_id
 JOIN user_profiles u ON u.owner_id=n.actor_id WHERE (p.id IS NOT NULL OR c.id IS NOT NULL OR cd.id IS NOT NULL)`
	query := columns + ` FROM notifications n` + joins + ` AND n.owner_id=$1 AND n.read_at IS NULL ORDER BY n.created_at DESC,n.id DESC`
	if read {
		query = `WITH opened AS (UPDATE notifications SET read_at=clock_timestamp() WHERE owner_id=$1 AND read_at IS NULL RETURNING *) ` + columns + ` FROM opened n` + joins + ` ORDER BY n.created_at DESC,n.id DESC`
	} else if history {
		query = columns + ` FROM notifications n` + joins + ` AND n.owner_id=$1 ORDER BY n.created_at DESC,n.id DESC`
	}
	rows, err := s.Pool.Query(ctx, query, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Notification{}
	for rows.Next() {
		var item Notification
		if err = rows.Scan(&item.ID, &item.ProblemID, &item.ContestID, &item.Title, &item.Actor, &item.Kind, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
