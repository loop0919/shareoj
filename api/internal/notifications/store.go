package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	Store        struct{ Pool *pgxpool.Pool }
	Notification struct {
		ID        string    `json:"id"`
		ProblemID string    `json:"problemId"`
		Title     string    `json:"title"`
		Actor     string    `json:"actor"`
		Kind      string    `json:"kind"`
		CreatedAt time.Time `json:"createdAt"`
	}
)

func (s *Store) List(ctx context.Context, owner string, read, history bool) ([]Notification, error) {
	// UPDATE ... RETURNING gives the bell exactly the notifications read by this
	// request. Events arriving afterwards stay unread.
	query := `SELECT n.id,n.problem_id,p.draft->>'title',u.handle,n.kind,n.created_at FROM notifications n JOIN problem_drafts p ON p.id=n.problem_id JOIN user_profiles u ON u.owner_id=n.actor_id WHERE n.owner_id=$1 AND n.read_at IS NULL ORDER BY n.created_at DESC,n.id DESC`
	if read {
		query = `WITH opened AS (UPDATE notifications SET read_at=clock_timestamp() WHERE owner_id=$1 AND read_at IS NULL RETURNING *) SELECT n.id,n.problem_id,p.draft->>'title',u.handle,n.kind,n.created_at FROM opened n JOIN problem_drafts p ON p.id=n.problem_id JOIN user_profiles u ON u.owner_id=n.actor_id ORDER BY n.created_at DESC,n.id DESC`
	} else if history {
		query = `SELECT n.id,n.problem_id,p.draft->>'title',u.handle,n.kind,n.created_at FROM notifications n JOIN problem_drafts p ON p.id=n.problem_id JOIN user_profiles u ON u.owner_id=n.actor_id WHERE n.owner_id=$1 ORDER BY n.created_at DESC,n.id DESC`
	}
	rows, err := s.Pool.Query(ctx, query, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Notification{}
	for rows.Next() {
		var item Notification
		if err = rows.Scan(&item.ID, &item.ProblemID, &item.Title, &item.Actor, &item.Kind, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
