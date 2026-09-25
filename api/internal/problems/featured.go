package problems

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrFeaturedIneligible = errors.New("problem is not eligible for featured delivery")
	ErrFeaturedLimit      = errors.New("featured application limit reached")
)

type FeaturedApplication struct {
	ProblemID  string    `json:"problemId"`
	Title      string    `json:"title"`
	Preference string    `json:"preference"`
	EnteredAt  time.Time `json:"enteredAt"`
}

type FeaturedSlot struct {
	Slot            string    `json:"slot"`
	Kind            string    `json:"kind"`
	ProblemID       string    `json:"problemId"`
	Title           string    `json:"title"`
	Writer          string    `json:"writer"`
	Testers         []string  `json:"testers"`
	Difficulty      *int      `json:"difficulty"`
	RevealAt        time.Time `json:"revealAt"`
	EditorialHidden bool      `json:"editorialHidden"`
}

type FeaturedRound struct {
	ScheduledAt time.Time      `json:"scheduledAt"`
	Slots       []FeaturedSlot `json:"slots"`
}

// FeaturedPreview intentionally has no problem ID or title: unpublished content
// must never enter a public response, including authenticated requests and SSR.
type FeaturedPreview struct {
	Slot       string   `json:"slot"`
	Kind       string   `json:"kind"`
	Difficulty *int     `json:"difficulty"`
	Writer     string   `json:"writer"`
	Testers    []string `json:"testers"`
}

type FeaturedWaiting struct {
	Easy int `json:"easy"`
	Hard int `json:"hard"`
}

type FeaturedPage struct {
	Items     []FeaturedRound   `json:"items"`
	HasMore   bool              `json:"hasMore"`
	NextAt    time.Time         `json:"nextAt"`
	Waiting   FeaturedWaiting   `json:"waiting"`
	NextSlots []FeaturedPreview `json:"nextSlots"`
	Current   *FeaturedRound    `json:"current"`
}

func featuredEligible(d Draft, known, enabled []string) bool {
	return d.Difficulty != nil && strings.TrimSpace(d.Editorial) != "" && Publishable(d, known, enabled)
}

// NextFeatured returns the first Monday/Thursday 23:00 JST strictly after now.
func NextFeatured(now time.Time) time.Time {
	jst := time.FixedZone("JST", 9*60*60)
	local := now.In(jst)
	day := time.Date(local.Year(), local.Month(), local.Day(), 23, 0, 0, 0, jst)
	for !day.After(now) || (day.Weekday() != time.Monday && day.Weekday() != time.Thursday) {
		day = day.AddDate(0, 0, 1)
	}
	return day
}

func (s *Store) ApplyFeatured(ctx context.Context, owner, id string, version int64, preference string, known, enabled []string) error {
	if preference != "" && preference != "soon" && preference != "later" {
		return ErrFeaturedIneligible
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize an author's applications so concurrent requests cannot exceed the cap.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,709091021))`, owner); err != nil {
		return err
	}
	var raw []byte
	var current int64
	var published bool
	err = tx.QueryRow(ctx, `SELECT draft,version,ever_published FROM problem_drafts WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&raw, &current, &published)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != version {
		return ErrConflict
	}
	if preference == "" {
		_, err = tx.Exec(ctx, `DELETE FROM featured_applications WHERE problem_id=$1`, id)
	} else {
		var reserved bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_problems WHERE problem_id=$1)`, id).Scan(&reserved); err != nil {
			return err
		}
		var draft Draft
		if published || reserved || json.Unmarshal(raw, &draft) != nil || !featuredEligible(draft, known, enabled) {
			return ErrFeaturedIneligible
		}
		if err = validateTestFiles(ctx, tx.Query, owner, id, draft.TestCases); err != nil {
			return err
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM featured_applications a JOIN problem_drafts d ON d.id=a.problem_id WHERE d.owner_id=$1 AND d.id<>$2`, owner, id).Scan(&count); err != nil {
			return err
		}
		if count >= 3 {
			return ErrFeaturedLimit
		}
		_, err = tx.Exec(ctx, `INSERT INTO featured_applications(problem_id,preference) VALUES($1,$2) ON CONFLICT(problem_id) DO UPDATE SET preference=excluded.preference`, id, preference)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FeaturedApplications(ctx context.Context, owner string) ([]FeaturedApplication, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.id,d.draft->>'title',a.preference,a.entered_at FROM featured_applications a JOIN problem_drafts d ON d.id=a.problem_id WHERE d.owner_id=$1 ORDER BY a.entered_at,d.id`, owner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[FeaturedApplication])
}

// ReleaseFeatured shares the dispatcher's minute tick and runs before relevant reads.
// The cursor and both slots commit together; retries cannot change an edition.
func (s *Store) ReleaseFeatured(ctx context.Context, known, enabled []string) error {
	var due bool
	if err := s.pool.QueryRow(ctx, `SELECT next_at<=clock_timestamp() FROM featured_schedule`).Scan(&due); err != nil || !due {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var at, now time.Time
	if err = tx.QueryRow(ctx, `SELECT next_at FROM featured_schedule FOR UPDATE`).Scan(&at); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	// Bound recovery work after a long outage; subsequent ticks continue the cursor.
	for n := 0; n < 32 && !at.After(now); n++ {
		for _, slot := range []string{"easy", "hard"} {
			pick := FeaturedSlot{Slot: slot, Kind: "missing", RevealAt: at.Add(23 * time.Hour)}
			if now.Before(pick.RevealAt) {
				pick, _, err = s.selectFeatured(ctx, tx, at, now, slot, known, enabled)
				if err != nil {
					return err
				}
			}
			if pick.Kind == "new" {
				if _, err = tx.Exec(ctx, `UPDATE problem_drafts SET published_draft=draft,published_version=version+1,version=version+1,published_at=$2 WHERE id=$1`, pick.ProblemID, now); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, `INSERT INTO featured_slots(scheduled_at,slot,kind,problem_id,difficulty,reveal_at) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6)`, at, slot, pick.Kind, pick.ProblemID, pick.Difficulty, pick.RevealAt); err != nil {
				return err
			}
		}
		at = NextFeatured(at)
	}
	if _, err = tx.Exec(ctx, `UPDATE featured_schedule SET next_at=$1`, at); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM featured_previews WHERE scheduled_at<$1`, at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) selectFeatured(ctx context.Context, tx pgx.Tx, at, now time.Time, slot string, known, enabled []string) (FeaturedSlot, int, error) {
	pick := FeaturedSlot{Slot: slot, Kind: "missing", RevealAt: at.Add(23 * time.Hour)}
	waiting := 0
	var reserved string
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT problem_id::text FROM featured_previews WHERE scheduled_at=$1 AND slot=$2),'')`, at, slot).Scan(&reserved); err != nil {
		return pick, waiting, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM featured_slots WHERE slot=$1 AND kind='new'`, slot).Scan(&count); err != nil {
		return pick, waiting, err
	}
	preference := "soon"
	if count%3 == 2 {
		preference = "later"
	}
	// Lock in the same UUID order as contest saves, then rank the queue in memory.
	rows, err := tx.Query(ctx, `SELECT d.id,d.owner_id,d.draft,a.preference,a.entered_at FROM problem_drafts d JOIN featured_applications a ON a.problem_id=d.id
 WHERE NOT d.ever_published AND a.entered_at<=$1 AND NOT EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.problem_id=d.id) ORDER BY d.id FOR UPDATE OF d`, at)
	if err != nil {
		return pick, waiting, err
	}
	type candidate struct {
		id, owner  string
		raw        []byte
		preference string
		entered    time.Time
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (candidate, error) {
		var c candidate
		e := row.Scan(&c.id, &c.owner, &c.raw, &c.preference, &c.entered)
		return c, e
	})
	if err != nil {
		return pick, waiting, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.id == reserved) != (b.id == reserved) {
			return a.id == reserved
		}
		if (a.preference == preference) != (b.preference == preference) {
			return a.preference == preference
		}
		return a.entered.Before(b.entered)
	})
	for _, c := range candidates {
		var eligible bool
		if err = tx.QueryRow(ctx, `SELECT NOT ever_published AND NOT EXISTS(SELECT 1 FROM contest_problems WHERE problem_id=$1) AND EXISTS(SELECT 1 FROM featured_applications WHERE problem_id=$1) FROM problem_drafts WHERE id=$1`, c.id).Scan(&eligible); err != nil {
			return pick, waiting, err
		}
		if !eligible {
			continue
		}
		var draft Draft
		if json.Unmarshal(c.raw, &draft) != nil || !featuredEligible(draft, known, enabled) {
			continue
		}
		if (slot == "easy") != (*draft.Difficulty <= 4) {
			continue
		}
		if err = validateTestFiles(ctx, tx.Query, c.owner, c.id, draft.TestCases); errors.Is(err, ErrTestFile) {
			continue
		} else if err != nil {
			return pick, waiting, err
		}
		waiting++
		if pick.Kind == "missing" {
			pick.Kind, pick.ProblemID, pick.Difficulty = "new", c.id, draft.Difficulty
		}
	}
	if pick.Kind == "new" {
		return pick, waiting, nil
	}
	// Future revivals must be available at the scheduled publication time.
	if at.After(now) {
		now = at
	}
	// Prefer candidates not featured in 30 days. If exhausted, relax oldest first.
	err = tx.QueryRow(ctx, `SELECT d.id,(d.published_draft->>'difficulty')::int FROM problem_drafts d
 LEFT JOIN LATERAL (SELECT max(scheduled_at) AS last_at FROM featured_slots WHERE problem_id=d.id) f ON true
 WHERE d.published_draft IS NOT NULL AND (d.published_draft->>'difficulty')::int BETWEEN $1 AND $2
 AND COALESCE(featured_reveal_at(d.id),'-infinity')<=$3
 AND NOT EXISTS(SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=d.id AND c.starts_at<=$3 AND c.ends_at>$3)
 ORDER BY (d.id::text=$4) DESC, (f.last_at IS NULL OR f.last_at<$3-interval '30 days') DESC,
 CASE WHEN f.last_at>=$3-interval '30 days' THEN f.last_at END ASC,random() LIMIT 1 FOR UPDATE OF d`, map[string]int{"easy": 1, "hard": 5}[slot], map[string]int{"easy": 4, "hard": 10}[slot], now, reserved).Scan(&pick.ProblemID, &pick.Difficulty)
	if errors.Is(err, pgx.ErrNoRows) {
		return pick, waiting, nil
	}
	if err == nil {
		pick.Kind = "revival"
	}
	return pick, waiting, err
}

func (s *Store) FeaturedList(ctx context.Context, offset int, known, enabled []string) (FeaturedPage, error) {
	page := FeaturedPage{Items: []FeaturedRound{}, NextSlots: []FeaturedPreview{}}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return page, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize previews with release so counts, selected authors and the edition agree.
	if err = tx.QueryRow(ctx, `SELECT next_at FROM featured_schedule FOR UPDATE`).Scan(&page.NextAt); err != nil {
		return page, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return page, err
	}
	for _, name := range []string{"easy", "hard"} {
		pick, waiting, e := s.selectFeatured(ctx, tx, page.NextAt, now, name, known, enabled)
		if e != nil {
			return page, e
		}
		if name == "easy" {
			page.Waiting.Easy = waiting
		} else {
			page.Waiting.Hard = waiting
		}
		preview := FeaturedPreview{Slot: name, Kind: pick.Kind, Difficulty: pick.Difficulty, Testers: []string{}}
		if pick.ProblemID != "" {
			if _, err = tx.Exec(ctx, `INSERT INTO featured_previews(scheduled_at,slot,problem_id) VALUES($1,$2,$3) ON CONFLICT(scheduled_at,slot) DO UPDATE SET problem_id=excluded.problem_id`, page.NextAt, name, pick.ProblemID); err != nil {
				return page, err
			}
			if err = tx.QueryRow(ctx, `SELECT u.handle, ARRAY(SELECT u.handle FROM problem_testers t JOIN user_profiles u ON u.owner_id=t.owner_id WHERE t.problem_id=d.id ORDER BY u.handle) FROM problem_drafts d JOIN user_profiles u ON u.owner_id=d.owner_id WHERE d.id=$1`, pick.ProblemID).Scan(&preview.Writer, &preview.Testers); err != nil {
				return page, err
			}
		} else if _, err = tx.Exec(ctx, `DELETE FROM featured_previews WHERE scheduled_at=$1 AND slot=$2`, page.NextAt, name); err != nil {
			return page, err
		}
		page.NextSlots = append(page.NextSlots, preview)
	}
	rows, err := tx.Query(ctx, `SELECT f.scheduled_at,f.slot,f.kind,
 CASE WHEN d.published_draft IS NOT NULL THEN d.id::text ELSE '' END,COALESCE(d.published_draft->>'title',''),f.difficulty,f.reveal_at,
 f.kind='new' AND $2<f.reveal_at, COALESCE(u.handle,''),
 ARRAY(SELECT u.handle FROM problem_testers t JOIN user_profiles u ON u.owner_id=t.owner_id WHERE t.problem_id=d.id AND d.published_draft IS NOT NULL ORDER BY u.handle)
 FROM featured_slots f LEFT JOIN problem_drafts d ON d.id=f.problem_id
 LEFT JOIN user_profiles u ON u.owner_id=d.owner_id AND d.published_draft IS NOT NULL
 WHERE f.scheduled_at IN (SELECT scheduled_at FROM featured_slots WHERE scheduled_at<=$2 GROUP BY scheduled_at ORDER BY scheduled_at DESC LIMIT 21 OFFSET $1)
 OR f.scheduled_at=(SELECT max(scheduled_at) FROM featured_slots WHERE scheduled_at<=$2)
 ORDER BY f.scheduled_at DESC,f.slot`, offset, now)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var at time.Time
		var slot FeaturedSlot
		if err = rows.Scan(&at, &slot.Slot, &slot.Kind, &slot.ProblemID, &slot.Title, &slot.Difficulty, &slot.RevealAt, &slot.EditorialHidden, &slot.Writer, &slot.Testers); err != nil {
			rows.Close()
			return page, err
		}
		if len(page.Items) == 0 || !page.Items[len(page.Items)-1].ScheduledAt.Equal(at) {
			page.Items = append(page.Items, FeaturedRound{ScheduledAt: at, Slots: []FeaturedSlot{}})
		}
		i := len(page.Items) - 1
		page.Items[i].Slots = append(page.Items[i].Slots, slot)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(page.Items) > 0 {
		latest := page.Items[0]
		if now.Before(latest.ScheduledAt.Add(23 * time.Hour)) {
			for _, slot := range latest.Slots {
				if slot.ProblemID != "" {
					page.Current = &latest
					break
				}
			}
		}
		if offset > 0 {
			page.Items = page.Items[1:]
		}
	}
	page.HasMore = len(page.Items) > 20
	if page.HasMore {
		page.Items = page.Items[:20]
	}
	return page, tx.Commit(ctx)
}
