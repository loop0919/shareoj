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

// Standing is a row of the standings: a participant in an individual contest, a team in a team contest (ADR 0024).
// Handle names the participant and is empty in a team row, whose Team holds the name and the members. Onsite marks the
// onsite division. OverallRank is the rank among all rows; it differs from Rank only in the onsite division's table.
type Standing struct {
	Rank        int               `json:"rank"`
	OverallRank int               `json:"overallRank"`
	Handle      string            `json:"handle"`
	Team        *TeamSummary      `json:"team"`
	Onsite      bool              `json:"onsite"`
	Points      int               `json:"points"`
	TimeMS      int64             `json:"timeMs"`
	Problems    map[string]*Score `json:"problems"`
}

// Attempt is a submission to rank. Team is the submitter's team in a team contest; the attempt counts toward its row.
type Attempt struct {
	Handle, ProblemID, Verdict string
	Team                       string
	Points                     int
	CreatedAt                  time.Time
}

func (a Attempt) key() string {
	if a.Team != "" {
		return a.Team
	}
	return a.Handle
}

func (s Standing) key() string {
	if s.Team != nil {
		return s.Team.ID
	}
	return s.Handle
}

// name orders tied rows: participants by handle, teams by name.
func (s Standing) name() string {
	if s.Team != nil {
		return s.Team.Name + "\x00" + s.Team.ID
	}
	return s.Handle
}

// Attempts must be ordered by acceptance time, never judge completion time.
func Rank(participants []string, attempts []Attempt, start time.Time, penaltyMinutes int) []Standing {
	return RankFrozen(participants, attempts, start, penaltyMinutes, time.Time{})
}

// RankFrozen ranks only the attempts created before freezeAt and counts the later ones in Score.Frozen.
// A zero freezeAt ranks every attempt, like Rank.
func RankFrozen(participants []string, attempts []Attempt, start time.Time, penaltyMinutes int, freezeAt time.Time) []Standing {
	rows := make([]Standing, len(participants))
	for i, handle := range participants {
		rows[i] = Standing{Handle: handle}
	}
	return RankRows(rows, attempts, start, penaltyMinutes, freezeAt)
}

// RankRows ranks rows, participants keyed by Handle or teams keyed by Team.ID, with the rules of RankFrozen. An attempt
// counts toward its Team's row, or its Handle's outside teams; one for a key not in rows adds a row.
func RankRows(rows []Standing, attempts []Attempt, start time.Time, penaltyMinutes int, freezeAt time.Time) []Standing {
	users := map[string]*Standing{}
	for _, row := range rows {
		row.Problems = map[string]*Score{}
		users[row.key()] = &row
	}
	last := map[string]time.Time{}
	for _, a := range attempts {
		key := a.key()
		u := users[key]
		if u == nil {
			u = &Standing{Handle: a.Handle, Problems: map[string]*Score{}}
			if a.Team != "" {
				u = &Standing{Team: &TeamSummary{ID: a.Team, Members: []string{}}, Problems: map[string]*Score{}}
			}
			users[key] = u
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
			if at.After(last[key]) {
				last[key] = at
			}
		case "WA", "RE", "TLE", "MLE", "OLE":
			p.Wrong++
		case "":
			p.Pending++
		}
	}
	result := make([]Standing, 0, len(users))
	for key, u := range users {
		if at, ok := last[key]; ok {
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
		return a.name() < b.name()
	})
	rank(result)
	for i := range result {
		result[i].OverallRank = result[i].Rank
	}
	return result
}

// rank numbers sorted rows from 1; rows with the same points and time share the rank of the first of them.
func rank(rows []Standing) {
	for i := range rows {
		rows[i].Rank = i + 1
		if i > 0 && rows[i].Points == rows[i-1].Points && rows[i].TimeMS == rows[i-1].TimeMS {
			rows[i].Rank = rows[i-1].Rank
		}
	}
}

// OnsiteRows keeps the onsite rows of a ranked table and ranks them again among themselves; OverallRank keeps each
// row's rank among everyone (ADR 0024).
func OnsiteRows(rows []Standing) []Standing {
	kept := make([]Standing, 0, len(rows))
	for _, row := range rows {
		if row.Onsite {
			kept = append(kept, row)
		}
	}
	rank(kept)
	return kept
}

// Standings returns the frozen table until the results are final. Only staff may ask for the live one (ADR 0023).
// A team contest has a row per team, ranked on all its members' submissions (ADR 0024). onsite keeps the onsite
// division's rows, ranked again among themselves.
func (s *Store) Standings(ctx context.Context, id, viewer string, live, onsite bool) ([]Standing, error) {
	c, err := s.Get(ctx, id, viewer)
	if err != nil {
		return nil, err
	}
	var freezeAt time.Time
	if c.FreezeAt != nil && !c.ResultsFinal && !(live && c.Staff) {
		freezeAt = *c.FreezeAt
	}
	// Staff and former staff are left out. A participant who became a tester after the results were final keeps the final rank.
	// In a team contest each competing member counts toward their team, listed in the order they joined; a registration
	// without a team, left by the API from before teams, is in no row until it joins one.
	rows, err := s.Pool.Query(ctx, `SELECT p.owner_id,u.handle,COALESCE(t.id::text,''),COALESCE(t.name,''),
 c.onsite_division AND CASE WHEN c.team_mode THEN t.onsite_at ELSE p.onsite_at END IS NOT NULL
 FROM contest_participants p JOIN contests c ON c.id=p.contest_id JOIN user_profiles u ON u.owner_id=p.owner_id
 LEFT JOIN contest_teams t ON t.id=p.team_id AND c.team_mode
 WHERE c.id=$1 AND contest_competitor(c,p.owner_id) AND (NOT c.team_mode OR t.id IS NOT NULL)
 ORDER BY t.created_at,t.id,p.created_at,p.owner_id`, id)
	if err != nil {
		return nil, err
	}
	var owners []string
	var entries []Standing
	teams, teamOf := map[string]int{}, map[string]string{}
	var owner, handle, team, name string
	var on bool
	_, err = pgx.ForEachRow(rows, []any{&owner, &handle, &team, &name, &on}, func() error {
		owners = append(owners, owner)
		if team == "" {
			entries = append(entries, Standing{Handle: handle, Onsite: on})
			return nil
		}
		i, ok := teams[team]
		if !ok {
			i, teams[team] = len(entries), len(entries)
			entries = append(entries, Standing{Team: &TeamSummary{ID: team, Name: name, Members: []string{}, Onsite: on}, Onsite: on})
		}
		entries[i].Team.Members = append(entries[i].Team.Members, handle)
		teamOf[handle] = team
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
		a.Team = teamOf[a.Handle]
		return a, e
	})
	if err != nil {
		return nil, err
	}
	table := RankRows(entries, attempts, c.StartsAt, c.PenaltyMinutes, freezeAt)
	if onsite {
		table = OnsiteRows(table)
	}
	return table, nil
}
