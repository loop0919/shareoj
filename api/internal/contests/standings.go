package contests

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Score is a problem's cell. Points is the problem's score in whole points, Score the same in hundredths, exact for a
// partially scored problem (ADR 0025), and ScoreText the way to show it: a whole number, or two decimals ("37.50").
// AcceptedAt is when the full score was reached and ScoredAt when the score last rose; for an all-or-nothing problem
// both are the first AC. Wrong counts every wrong run of a problem without a score, and otherwise the wrong runs before
// ScoredAt, which the penalty counts. Pending counts the runs waiting for a verdict before the full score.
// Frozen counts the submissions made after the freeze, whatever their verdict (ADR 0023); it stays 0 on a
// problem with the full score before the freeze, and without a freeze or after the results are final.
type Score struct {
	Points     int        `json:"points"`
	Score      int64      `json:"score"`
	ScoreText  string     `json:"scoreText"`
	Wrong      int        `json:"wrong"`
	Pending    int        `json:"pending"`
	Frozen     int        `json:"frozen"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
	ScoredAt   *time.Time `json:"scoredAt,omitempty"`
}

// Standing is a row of the standings: a team, or a participant who is in no team (ADR 0024). Handle names the
// participant and is empty in a team row, whose Team holds the name and the members. Onsite marks the onsite division.
// OverallRank is the rank among all rows; it differs from Rank only in the onsite division's table. Points, Score and
// ScoreText total the cells like a cell's own.
type Standing struct {
	Rank        int               `json:"rank"`
	OverallRank int               `json:"overallRank"`
	Handle      string            `json:"handle"`
	Team        *TeamSummary      `json:"team"`
	Onsite      bool              `json:"onsite"`
	Points      int               `json:"points"`
	Score       int64             `json:"score"`
	ScoreText   string            `json:"scoreText"`
	TimeMS      int64             `json:"timeMs"`
	Problems    map[string]*Score `json:"problems"`
}

// Attempt is a submission to rank. Team is the submitter's team, if any; the attempt counts toward its row. Points is
// the problem's points, its full score. Groups holds a partially scored result's group scores in hundredths; nil
// means an all-or-nothing result, which scores the full points with an AC.
type Attempt struct {
	Handle, ProblemID, Verdict string
	Team                       string
	Points                     int
	Groups                     []int64
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

// ScoreText writes a score in hundredths as a whole number, or with two decimals when it has a fraction (ADR 0025).
func ScoreText(hundredths int64) string {
	if hundredths%100 == 0 {
		return strconv.FormatInt(hundredths/100, 10)
	}
	return fmt.Sprintf("%d.%02d", hundredths/100, hundredths%100)
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

// progress is what ranking a problem of a row needs beyond its cell.
type progress struct {
	// best holds each group's best score and whole the best all-or-nothing score, both in hundredths.
	best  []int64
	whole int64
	// wrong counts every wrong run, and before those before the last run that raised the score.
	wrong, before int
}

// score adds the attempt's scores and returns the problem's score: each group's best summed, as at JOI and IOI
// (ADR 0025), and never more than the full score.
func (p *progress) score(a Attempt) int64 {
	full := int64(a.Points) * 100
	if a.Groups == nil {
		if a.Verdict == "AC" {
			p.whole = full
		}
	}
	for i, g := range a.Groups {
		if i >= len(p.best) {
			p.best = append(p.best, make([]int64, i+1-len(p.best))...)
		}
		p.best[i] = max(p.best[i], g)
	}
	total := int64(0)
	for _, g := range p.best {
		total += g
	}
	return min(max(total, p.whole), full)
}

// RankRows ranks rows, participants keyed by Handle or teams keyed by Team.ID, with the rules of RankFrozen. An attempt
// counts toward its Team's row, or its Handle's outside teams; one for a key not in rows adds a row.
// A run that raises the problem's score improves it; a judged run that does not, CE and JE aside, is wrong. The
// problem's time is the last improvement, and the penalty counts the wrong runs before it. Rows rank by total score,
// then by the latest problem time plus the penalty. For all-or-nothing problems the only improvement is the first AC,
// so these are the rules of a contest without partial scores (ADR 0025).
func RankRows(rows []Standing, attempts []Attempt, start time.Time, penaltyMinutes int, freezeAt time.Time) []Standing {
	users := map[string]*Standing{}
	for _, row := range rows {
		row.Problems = map[string]*Score{}
		users[row.key()] = &row
	}
	states := map[*Score]*progress{}
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
			states[p] = &progress{}
		}
		if p.AcceptedAt != nil {
			continue
		}
		if !freezeAt.IsZero() && !a.CreatedAt.Before(freezeAt) {
			p.Frozen++
			continue
		}
		switch a.Verdict {
		case "AC", "WA", "RE", "TLE", "MLE", "OLE":
		case "":
			p.Pending++
			continue
		default:
			continue
		}
		state := states[p]
		score := state.score(a)
		if score <= p.Score {
			state.wrong++
			continue
		}
		at := a.CreatedAt
		p.Score, p.ScoredAt, state.before = score, &at, state.wrong
		if score >= int64(a.Points)*100 {
			p.AcceptedAt = &at
		}
	}
	result := make([]Standing, 0, len(users))
	for _, u := range users {
		var last time.Time
		for _, p := range u.Problems {
			state := states[p]
			p.Wrong = state.wrong
			if p.ScoredAt != nil {
				p.Wrong = state.before
				u.TimeMS += int64(state.before) * int64(penaltyMinutes) * 60000
				if p.ScoredAt.After(last) {
					last = *p.ScoredAt
				}
			}
			p.Points, p.ScoreText = int(p.Score/100), ScoreText(p.Score)
			u.Score += p.Score
		}
		if !last.IsZero() {
			u.TimeMS += last.Sub(start).Milliseconds()
		}
		u.Points, u.ScoreText = int(u.Score/100), ScoreText(u.Score)
		result = append(result, *u)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Score != b.Score {
			return a.Score > b.Score
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

// rank numbers sorted rows from 1; rows with the same score and time share the rank of the first of them.
func rank(rows []Standing) {
	for i := range rows {
		rows[i].Rank = i + 1
		if i > 0 && rows[i].Score == rows[i-1].Score && rows[i].TimeMS == rows[i-1].TimeMS {
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
// A team contest has a row per team, ranked on all its members' submissions, and a row per participant who entered
// alone, under the same rules (ADR 0024). onsite keeps the onsite division's rows, ranked again among themselves.
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
	// In a team contest each competing member counts toward their team, listed in the order they joined. Someone in no
	// team has a row of their own and their own division; they enter a team only before the start, so no submission of
	// theirs ever counts for a team.
	rows, err := s.Pool.Query(ctx, `SELECT p.owner_id,u.handle,COALESCE(t.id::text,''),COALESCE(t.name,''),
 c.onsite_division AND CASE WHEN t.id IS NULL THEN p.onsite_at ELSE t.onsite_at END IS NOT NULL
 FROM contest_participants p JOIN contests c ON c.id=p.contest_id JOIN user_profiles u ON u.owner_id=p.owner_id
 LEFT JOIN contest_teams t ON t.id=p.team_id
 WHERE c.id=$1 AND contest_competitor(c,p.owner_id)
 ORDER BY t.created_at,t.id,p.created_at,p.owner_id`, id)
	if err != nil {
		return nil, err
	}
	var owners []string
	var entries []Standing
	// Attempts are tied to rows by owner, as read here: a handle changed between the two queries would otherwise add a
	// row of its own.
	teams, handleOf, teamOf := map[string]int{}, map[string]string{}, map[string]string{}
	var owner, handle, team, name string
	var on bool
	_, err = pgx.ForEachRow(rows, []any{&owner, &handle, &team, &name, &on}, func() error {
		owners = append(owners, owner)
		handleOf[owner], teamOf[owner] = handle, team
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	// A partially scored result brings its group scores; the others score by their verdict.
	rows, err = s.Pool.Query(ctx, `SELECT s.owner_id,s.problem_id,COALESCE(s.result->>'verdict',''),cp.points,
 CASE WHEN jsonb_typeof(s.result->'scoring'->'groups')='array' THEN ARRAY(SELECT (g->>'score')::bigint
  FROM jsonb_array_elements(s.result->'scoring'->'groups') WITH ORDINALITY x(g,i) ORDER BY i) END,s.created_at
 FROM submissions s JOIN contests c ON c.id=s.contest_id JOIN contest_problems cp ON cp.contest_id=c.id AND cp.problem_id=s.problem_id
 WHERE c.id=$1 AND s.owner_id=ANY($2::text[]) AND s.created_at>=c.starts_at AND s.created_at<c.ends_at
 AND NOT COALESCE((s.job->>'easyTest')::boolean,false)
 ORDER BY s.created_at,s.id`, id, owners)
	if err != nil {
		return nil, err
	}
	attempts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Attempt, error) {
		var a Attempt
		var by string
		e := row.Scan(&by, &a.ProblemID, &a.Verdict, &a.Points, &a.Groups, &a.CreatedAt)
		a.Handle, a.Team = handleOf[by], teamOf[by]
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
