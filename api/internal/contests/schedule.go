package contests

import (
	"context"
	"time"
)

// ScheduledContest is a contest that has not ended yet, as the schedules list it.
type ScheduledContest struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

// ParticipationSchedule returns up to 50 unfinished contests the owner has joined, earliest start first.
func (s *Store) ParticipationSchedule(ctx context.Context, owner string) ([]ScheduledContest, error) {
	return s.scheduledContests(ctx, `SELECT c.id,c.title,c.starts_at,c.ends_at FROM contests c
 JOIN contest_participants p ON p.contest_id=c.id AND p.owner_id=$1
 WHERE c.ends_at>clock_timestamp() ORDER BY c.starts_at,c.id LIMIT 50`, owner)
}

// UpcomingContests returns up to five contests that start within the next seven days, earliest start first.
func (s *Store) UpcomingContests(ctx context.Context) ([]ScheduledContest, error) {
	return s.scheduledContests(ctx, `SELECT c.id,c.title,c.starts_at,c.ends_at FROM contests c
 WHERE c.starts_at>statement_timestamp() AND c.starts_at<=statement_timestamp()+interval '7 days'
 ORDER BY c.starts_at,c.id LIMIT 5`)
}

func (s *Store) scheduledContests(ctx context.Context, query string, args ...any) ([]ScheduledContest, error) {
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ScheduledContest{}
	for rows.Next() {
		var c ScheduledContest
		if err = rows.Scan(&c.ID, &c.Title, &c.StartsAt, &c.EndsAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
