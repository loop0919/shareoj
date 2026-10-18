package contests

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// Frozen counts the submissions made after the freeze, whatever their verdict (ADR 0023); it stays 0 on a
// problem accepted before the freeze, and without a freeze or after the results are final.
type Score struct {
	Points     int        `json:"points"`
	Wrong      int        `json:"wrong"`
	Pending    int        `json:"pending"`
	Frozen     int        `json:"frozen"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
}
type Standing struct {
	Rank     int               `json:"rank"`
	Handle   string            `json:"handle"`
	Points   int               `json:"points"`
	TimeMS   int64             `json:"timeMs"`
	Problems map[string]*Score `json:"problems"`
}
type Attempt struct {
	Handle, ProblemID, Verdict string
	Points                     int
	CreatedAt                  time.Time
}

// Attempts must be ordered by acceptance time, never judge completion time.
func Rank(participants []string, attempts []Attempt, start time.Time, penaltyMinutes int) []Standing {
	return RankFrozen(participants, attempts, start, penaltyMinutes, time.Time{})
}

// RankFrozen ranks only the attempts created before freezeAt and counts the later ones in Score.Frozen.
// A zero freezeAt ranks every attempt, like Rank.
func RankFrozen(participants []string, attempts []Attempt, start time.Time, penaltyMinutes int, freezeAt time.Time) []Standing {
	users := map[string]*Standing{}
	for _, handle := range participants {
		users[handle] = &Standing{Handle: handle, Problems: map[string]*Score{}}
	}
	last := map[string]time.Time{}
	for _, a := range attempts {
		u := users[a.Handle]
		if u == nil {
			u = &Standing{Handle: a.Handle, Problems: map[string]*Score{}}
			users[a.Handle] = u
		}
		p := u.Problems[a.ProblemID]
		if p == nil {
			p = &Score{}
			u.Problems[a.ProblemID] = p
		}
		if p.AcceptedAt != nil {
			continue
		}
		if !freezeAt.IsZero() && !a.CreatedAt.Before(freezeAt) {
			p.Frozen++
			continue
		}
		switch a.Verdict {
		case "AC":
			at := a.CreatedAt
			p.AcceptedAt = &at
			p.Points = a.Points
			u.Points += a.Points
			u.TimeMS += int64(p.Wrong) * int64(penaltyMinutes) * 60000
			if at.After(last[a.Handle]) {
				last[a.Handle] = at
			}
		case "WA", "RE", "TLE", "MLE", "OLE":
			p.Wrong++
		case "":
			p.Pending++
		}
	}
	result := make([]Standing, 0, len(users))
	for handle, u := range users {
		if at, ok := last[handle]; ok {
			u.TimeMS += at.Sub(start).Milliseconds()
		}
		result = append(result, *u)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if a.TimeMS != b.TimeMS {
			return a.TimeMS < b.TimeMS
		}
		return a.Handle < b.Handle
	})
	for i := range result {
		result[i].Rank = i + 1
		if i > 0 && result[i].Points == result[i-1].Points && result[i].TimeMS == result[i-1].TimeMS {
			result[i].Rank = result[i-1].Rank
		}
	}
	return result
}

// Standings returns the frozen table until the results are final. Only staff may ask for the live one (ADR 0023).
func (s *Store) Standings(ctx context.Context, id, viewer string, live bool) ([]Standing, error) {
	c, err := s.Get(ctx, id, viewer)
	if err != nil {
		return nil, err
	}
	var freezeAt time.Time
	if c.FreezeAt != nil && !c.ResultsFinal && !(live && c.Staff) {
		freezeAt = *c.FreezeAt
	}
	// Staff and former staff are left out. A participant who became a tester after the results were final keeps the final rank.
	rows, err := s.Pool.Query(ctx, `SELECT p.owner_id,u.handle FROM contest_participants p
 JOIN contests c ON c.id=p.contest_id JOIN user_profiles u ON u.owner_id=p.owner_id
 WHERE c.id=$1 AND contest_competitor(c,p.owner_id)`, id)
	if err != nil {
		return nil, err
	}
	var owners, participants []string
	var owner, handle string
	_, err = pgx.ForEachRow(rows, []any{&owner, &handle}, func() error {
		owners, participants = append(owners, owner), append(participants, handle)
		return nil
	})
	if err != nil {
		return nil, err
	}
	rows, err = s.Pool.Query(ctx, `SELECT u.handle,s.problem_id,COALESCE(s.result->>'verdict',''),cp.points,s.created_at
 FROM submissions s JOIN contests c ON c.id=s.contest_id JOIN contest_problems cp ON cp.contest_id=c.id AND cp.problem_id=s.problem_id
 JOIN user_profiles u ON u.owner_id=s.owner_id
 WHERE c.id=$1 AND s.owner_id=ANY($2::text[]) AND s.created_at>=c.starts_at AND s.created_at<c.ends_at
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 ORDER BY s.created_at,s.id`, id, owners)
	if err != nil {
		return nil, err
	}
	attempts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Attempt, error) {
		var a Attempt
		e := row.Scan(&a.Handle, &a.ProblemID, &a.Verdict, &a.Points, &a.CreatedAt)
		return a, e
	})
	if err != nil {
		return nil, err
	}
	return RankFrozen(participants, attempts, c.StartsAt, c.PenaltyMinutes, freezeAt), nil
}
