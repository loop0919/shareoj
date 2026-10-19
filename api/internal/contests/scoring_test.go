package contests

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// legacyRankRows is RankRows as it was before partial scores (ADR 0025), kept to show that all-or-nothing problems
// rank exactly as they did.
func legacyRankRows(rows []Standing, attempts []Attempt, start time.Time, penaltyMinutes int, freezeAt time.Time) []Standing {
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
	for i := range result {
		result[i].Rank = i + 1
		if i > 0 && result[i].Points == result[i-1].Points && result[i].TimeMS == result[i-1].TimeMS {
			result[i].Rank = result[i-1].Rank
		}
	}
	return result
}

// summary keeps the fields that existed before partial scores.
func summary(rows []Standing) []string {
	lines := []string{}
	for _, r := range rows {
		problems := []string{}
		for id, p := range r.Problems {
			accepted := "-"
			if p.AcceptedAt != nil {
				accepted = p.AcceptedAt.Format(time.RFC3339)
			}
			problems = append(problems, fmt.Sprintf("%s:%d/%d/%d/%d/%s", id, p.Points, p.Wrong, p.Pending, p.Frozen, accepted))
		}
		sort.Strings(problems)
		lines = append(lines, fmt.Sprintf("%d %q %d %d %v", r.Rank, r.name(), r.Points, r.TimeMS, problems))
	}
	return lines
}

// For all-or-nothing problems the generalized rules are the old ones: the only improvement is the first AC, and the
// wrong runs are the judged non-AC runs before it (ADR 0025).
func TestRankKeepsAllOrNothingContestsUnchanged(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	verdicts := []string{"AC", "WA", "RE", "TLE", "MLE", "OLE", "CE", "JE", ""}
	points := map[string]int{"A": 100, "B": 250, "C": 1}
	random := rand.New(rand.NewSource(25))
	for round := 0; round < 2000; round++ {
		rows := []Standing{{Handle: "idle"}, {Handle: "solo", Onsite: true}, {Team: &TeamSummary{ID: "t1", Name: "Alpha", Members: []string{"a", "b"}}}}
		var attempts []Attempt
		minute := 0
		for i := random.Intn(40); i > 0; i-- {
			minute += random.Intn(3)
			a := Attempt{ProblemID: string(rune('A' + random.Intn(3))), Verdict: verdicts[random.Intn(len(verdicts))], CreatedAt: start.Add(time.Duration(minute) * time.Minute)}
			a.Points = points[a.ProblemID]
			switch random.Intn(4) {
			case 0:
				a.Handle = "solo"
			case 1:
				a.Handle, a.Team = "a", "t1"
			case 2:
				a.Handle, a.Team = "b", "t1"
			default:
				a.Handle = fmt.Sprintf("late%d", random.Intn(2))
			}
			attempts = append(attempts, a)
		}
		var freezeAt time.Time
		if random.Intn(2) == 0 {
			freezeAt = start.Add(time.Duration(random.Intn(minute+2)) * time.Minute)
		}
		penalty := random.Intn(21)
		got := RankRows(rows, attempts, start, penalty, freezeAt)
		want := legacyRankRows(rows, attempts, start, penalty, freezeAt)
		if fmt.Sprint(summary(got)) != fmt.Sprint(summary(want)) {
			t.Fatalf("round %d:\n got %v\nwant %v", round, summary(got), summary(want))
		}
		for _, r := range got {
			if r.Score != int64(r.Points)*100 || r.ScoreText != fmt.Sprint(r.Points) || r.OverallRank != r.Rank {
				t.Fatalf("round %d row %+v", round, r)
			}
			for _, p := range r.Problems {
				if p.Score != int64(p.Points)*100 || (p.ScoredAt == nil) != (p.AcceptedAt == nil) || (p.ScoredAt != nil && !p.ScoredAt.Equal(*p.AcceptedAt)) {
					t.Fatalf("round %d cell %+v", round, p)
				}
			}
		}
	}
}

func TestRankSumsEachGroupsBestScore(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return start.Add(time.Duration(minutes) * time.Minute) }
	scored := func(handle, verdict string, minute int, groups ...int64) Attempt {
		return Attempt{Handle: handle, ProblemID: "P", Verdict: verdict, Points: 100, Groups: groups, CreatedAt: at(minute)}
	}
	t.Run("groups from different runs add up to the full score", func(t *testing.T) {
		rows := Rank(nil, []Attempt{scored("a", "WA", 5, 3000, 0), scored("a", "TLE", 10, 0, 7000), scored("a", "WA", 12, 3000, 7000)}, start, 5)
		p := rows[0].Problems["P"]
		if p.Score != 10000 || p.Points != 100 || p.AcceptedAt == nil || !p.AcceptedAt.Equal(at(10)) || !p.ScoredAt.Equal(at(10)) || p.Wrong != 0 || rows[0].TimeMS != 10*60000 {
			t.Fatalf("%+v %+v", rows[0], p)
		}
	})
	t.Run("a judged run that does not raise the score is wrong, whatever its verdict", func(t *testing.T) {
		// 50, then an AC with 40 and a tie at 50, then 60: two wrong runs before the last improvement.
		rows := Rank(nil, []Attempt{scored("a", "AC", 1, 5000), scored("a", "AC", 2, 4000), scored("a", "WA", 3, 5000), scored("a", "AC", 4, 6000), scored("a", "AC", 5, 6000)}, start, 10)
		p := rows[0].Problems["P"]
		if p.Score != 6000 || p.AcceptedAt != nil || !p.ScoredAt.Equal(at(4)) || p.Wrong != 2 || rows[0].TimeMS != (4+2*10)*60000 {
			t.Fatalf("%+v %+v", rows[0], p)
		}
	})
	t.Run("a problem without a score shows every wrong run and no time", func(t *testing.T) {
		rows := Rank(nil, []Attempt{scored("a", "WA", 1, 0), scored("a", "CE", 2, 0), scored("a", "JE", 3), scored("a", "AC", 4, 0), scored("a", "", 5)}, start, 5)
		p := rows[0].Problems["P"]
		if p.Score != 0 || p.ScoreText != "0" || p.ScoredAt != nil || p.Wrong != 2 || p.Pending != 1 || rows[0].TimeMS != 0 {
			t.Fatalf("%+v %+v", rows[0], p)
		}
	})
	t.Run("pending runs count until the full score", func(t *testing.T) {
		rows := Rank(nil, []Attempt{scored("a", "", 1), scored("a", "WA", 2, 5000), scored("a", "", 3), scored("a", "AC", 4, 10000), scored("a", "", 5), scored("a", "WA", 6, 0)}, start, 5)
		p := rows[0].Problems["P"]
		if p.Pending != 2 || p.Wrong != 0 || !p.AcceptedAt.Equal(at(4)) {
			t.Fatalf("%+v", p)
		}
	})
	t.Run("the freeze hides later improvements", func(t *testing.T) {
		attempts := []Attempt{scored("a", "WA", 10, 2550), scored("a", "WA", 20, 2550), scored("a", "AC", 50, 10000), scored("a", "WA", 55, 0)}
		frozen := RankFrozen([]string{"a"}, attempts, start, 5, at(40))[0]
		p := frozen.Problems["P"]
		if frozen.Score != 2550 || frozen.ScoreText != "25.50" || frozen.Points != 25 || p.Frozen != 2 || p.Wrong != 0 || p.AcceptedAt != nil || !p.ScoredAt.Equal(at(10)) || frozen.TimeMS != 10*60000 {
			t.Fatalf("frozen %+v %+v", frozen, p)
		}
		live := RankFrozen([]string{"a"}, attempts, start, 5, time.Time{})[0]
		if p := live.Problems["P"]; live.Score != 10000 || p.Frozen != 0 || p.Wrong != 1 || !p.AcceptedAt.Equal(at(50)) || live.TimeMS != (50+5)*60000 {
			t.Fatalf("live %+v %+v", live, p)
		}
	})
	t.Run("hundredths rank before time, and all-or-nothing problems add whole points", func(t *testing.T) {
		attempts := []Attempt{
			scored("a", "WA", 1, 3700, 0),
			scored("b", "WA", 50, 3750, 0),
			{Handle: "c", ProblemID: "Q", Verdict: "AC", Points: 37, CreatedAt: at(1)},
			scored("c", "WA", 2, 0, 50),
		}
		rows := Rank(nil, attempts, start, 5)
		// c ties b at 37.50 and is faster; a has 37 and the least time but ranks last.
		if got := summary(rows); rows[0].Handle != "c" || rows[1].Handle != "b" || rows[2].Handle != "a" || rows[0].Score != 3750 || rows[0].ScoreText != "37.50" ||
			rows[0].Points != 37 || rows[0].TimeMS != 2*60000 || rows[1].Rank != 2 || rows[2].ScoreText != "37" {
			t.Fatalf("%v %+v", got, rows)
		}
	})
	t.Run("team members' runs combine, and the score never passes the full score", func(t *testing.T) {
		rows := RankRows([]Standing{{Team: &TeamSummary{ID: "t", Name: "T", Members: []string{"a", "b"}}}}, []Attempt{
			{Handle: "a", Team: "t", ProblemID: "P", Verdict: "WA", Points: 100, Groups: []int64{6000, 0}, CreatedAt: at(3)},
			{Handle: "b", Team: "t", ProblemID: "P", Verdict: "WA", Points: 100, Groups: []int64{0, 6000}, CreatedAt: at(7)},
		}, start, 5, time.Time{})
		if p := rows[0].Problems["P"]; p.Score != 10000 || !p.AcceptedAt.Equal(at(7)) {
			t.Fatalf("%+v", p)
		}
	})
}

func TestScoreText(t *testing.T) {
	for hundredths, want := range map[int64]string{0: "0", 5: "0.05", 100: "1", 3750: "37.50", 3705: "37.05", 100000000: "1000000", 9999: "99.99"} {
		if got := ScoreText(hundredths); got != want {
			t.Fatalf("%d: got %q want %q", hundredths, got, want)
		}
	}
}
