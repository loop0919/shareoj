package problems

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const featuredPlanRounds = 8

// FeaturedPlanSlot never carries a problem ID. Title is set only for the viewer's own problems.
type FeaturedPlanSlot struct {
	Slot       string `json:"slot"`
	Kind       string `json:"kind"`
	Writer     string `json:"writer"`
	Difficulty *int   `json:"difficulty"`
	Mine       bool   `json:"mine,omitempty"`
	Title      string `json:"title,omitempty"`
	Trial      bool   `json:"trial,omitempty"`
}

type FeaturedPlanRound struct {
	ScheduledAt time.Time          `json:"scheduledAt"`
	Confirmed   bool               `json:"confirmed"`
	Slots       []FeaturedPlanSlot `json:"slots"`
}

type FeaturedPlan struct {
	Waiting FeaturedWaiting     `json:"waiting"`
	Rounds  []FeaturedPlanRound `json:"rounds"`
}

type featuredEntry struct {
	featuredCandidate
	writer, title, slot string
	difficulty          int
	trial               bool
}

// FeaturedPlan lays out the coming rounds as if the current applications stayed unchanged.
// The first round is the saved preview; later rounds replay selection with featuredBefore.
// With trialID, the viewer's problem joins the queue as if applied with trialPreference.
func (s *Store) FeaturedPlan(ctx context.Context, viewer, trialID, trialPreference string, known, enabled []string) (FeaturedPlan, error) {
	plan := FeaturedPlan{Rounds: []FeaturedPlanRound{}}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plan, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize with release and previews, as FeaturedList does.
	var at, now time.Time
	if err = tx.QueryRow(ctx, `SELECT next_at FROM featured_schedule FOR UPDATE`).Scan(&at); err != nil {
		return plan, err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return plan, err
	}
	previews, ids, waiting, err := s.previewFeatured(ctx, tx, at, now, known, enabled)
	if err != nil {
		return plan, err
	}
	plan.Waiting = waiting
	queue, err := s.featuredQueue(ctx, tx, known, enabled)
	if err != nil {
		return plan, err
	}
	if trialID != "" {
		if queue, err = s.featuredTrial(ctx, tx, queue, viewer, trialID, trialPreference, now, known, enabled); err != nil {
			return plan, err
		}
	}
	counts := map[string]int{}
	revivals := map[string]bool{}
	var easyCount, hardCount int
	var easyRevival, hardRevival bool
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE slot='easy' AND kind='new'),count(*) FILTER (WHERE slot='hard' AND kind='new'),
 EXISTS(SELECT 1 FROM problem_drafts WHERE published_draft IS NOT NULL AND (published_draft->>'difficulty')::int BETWEEN 1 AND 4),
 EXISTS(SELECT 1 FROM problem_drafts WHERE published_draft IS NOT NULL AND (published_draft->>'difficulty')::int BETWEEN 5 AND 10)
 FROM featured_slots`).Scan(&easyCount, &hardCount, &easyRevival, &hardRevival); err != nil {
		return plan, err
	}
	counts["easy"], counts["hard"] = easyCount, hardCount
	revivals["easy"], revivals["hard"] = easyRevival, hardRevival
	for i := 0; i < featuredPlanRounds; i++ {
		round := FeaturedPlanRound{ScheduledAt: at, Confirmed: i == 0, Slots: []FeaturedPlanSlot{}}
		for j, name := range []string{"easy", "hard"} {
			var entry *featuredEntry
			preview := i == 0 && previews[j].Kind == "new"
			if preview {
				queue, entry = takeFeatured(queue, slices.IndexFunc(queue, func(e featuredEntry) bool { return e.id == ids[j] }))
			} else {
				// A new problem replaces a previewed revival, so the first round also takes from the queue.
				queue, entry = nextFeatured(queue, name, at, featuredPreference(counts[name]))
			}
			slot := FeaturedPlanSlot{Slot: name, Kind: "missing"}
			switch {
			case entry != nil:
				slot = FeaturedPlanSlot{Slot: name, Kind: "new", Writer: entry.writer, Difficulty: &entry.difficulty, Trial: entry.trial}
				if entry.owner == viewer {
					slot.Mine, slot.Title = true, entry.title
				}
			case i == 0:
				slot = FeaturedPlanSlot{Slot: name, Kind: previews[j].Kind, Writer: previews[j].Writer, Difficulty: previews[j].Difficulty}
			case revivals[name]:
				slot.Kind = "revival"
			}
			if slot.Kind == "new" {
				counts[name]++
			}
			round.Slots = append(round.Slots, slot)
		}
		plan.Rounds = append(plan.Rounds, round)
		at = NextFeatured(at)
	}
	return plan, tx.Commit(ctx)
}

// featuredQueue loads, in entry order, every application that selection would accept now.
func (s *Store) featuredQueue(ctx context.Context, tx pgx.Tx, known, enabled []string) ([]featuredEntry, error) {
	rows, err := tx.Query(ctx, `SELECT d.id,d.owner_id,d.draft,a.preference,a.entered_at,COALESCE(u.handle,'') FROM problem_drafts d
 JOIN featured_applications a ON a.problem_id=d.id LEFT JOIN user_profiles u ON u.owner_id=d.owner_id
 WHERE NOT d.ever_published AND NOT EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.problem_id=d.id) ORDER BY a.entered_at,d.id`)
	if err != nil {
		return nil, err
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (featuredEntry, error) {
		var e featuredEntry
		err := row.Scan(&e.id, &e.owner, &e.raw, &e.preference, &e.entered, &e.writer)
		return e, err
	})
	if err != nil {
		return nil, err
	}
	queue := []featuredEntry{}
	for _, e := range entries {
		ready, err := featuredReady(ctx, tx, &e, known, enabled)
		if err != nil {
			return nil, err
		}
		if ready {
			queue = append(queue, e)
		}
	}
	return queue, nil
}

// featuredTrial adds the owner's problem as if applied now; an existing application keeps its entry time.
// A problem that cannot be applied is left out rather than reported, as the readiness check already explains it.
func (s *Store) featuredTrial(ctx context.Context, tx pgx.Tx, queue []featuredEntry, owner, id, preference string, now time.Time, known, enabled []string) ([]featuredEntry, error) {
	if i := slices.IndexFunc(queue, func(e featuredEntry) bool { return e.id == id }); i >= 0 {
		if queue[i].owner != owner {
			return nil, ErrNotFound
		}
		queue[i].preference, queue[i].trial = preference, true
		return queue, nil
	}
	e := featuredEntry{featuredCandidate: featuredCandidate{preference: preference, entered: now}, trial: true}
	var unavailable bool
	err := tx.QueryRow(ctx, `SELECT d.id,d.owner_id,d.draft,COALESCE(u.handle,''),
 d.ever_published OR d.published_draft IS NOT NULL OR d.scheduled_at IS NOT NULL
 OR EXISTS(SELECT 1 FROM contest_problems WHERE problem_id=d.id) OR EXISTS(SELECT 1 FROM featured_applications WHERE problem_id=d.id)
 FROM problem_drafts d LEFT JOIN user_profiles u ON u.owner_id=d.owner_id WHERE d.id=$1 AND d.owner_id=$2`, id, owner).Scan(&e.id, &e.owner, &e.raw, &e.writer, &unavailable)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || unavailable {
		return queue, err
	}
	ready, err := featuredReady(ctx, tx, &e, known, enabled)
	if err != nil || !ready {
		return queue, err
	}
	return append(queue, e), nil
}

// featuredReady applies selectFeatured's content checks and fills in the slot.
func featuredReady(ctx context.Context, tx pgx.Tx, e *featuredEntry, known, enabled []string) (bool, error) {
	var draft Draft
	if json.Unmarshal(e.raw, &draft) != nil || !featuredEligible(draft, known, enabled) {
		return false, nil
	}
	if err := validateTestFiles(ctx, tx.Query, e.owner, e.id, draft.TestCases); errors.Is(err, ErrTestFile) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	e.title, e.difficulty, e.slot = draft.Title, *draft.Difficulty, "hard"
	if e.difficulty <= 4 {
		e.slot = "easy"
	}
	return true, nil
}

// nextFeatured removes the entry selection would pick for slot at the given round.
func nextFeatured(queue []featuredEntry, slot string, at time.Time, preference string) ([]featuredEntry, *featuredEntry) {
	best := -1
	for i, e := range queue {
		if e.slot != slot || e.entered.After(at) {
			continue
		}
		if best < 0 || featuredBefore(e.featuredCandidate, queue[best].featuredCandidate, "", preference) {
			best = i
		}
	}
	return takeFeatured(queue, best)
}

func takeFeatured(queue []featuredEntry, i int) ([]featuredEntry, *featuredEntry) {
	if i < 0 {
		return queue, nil
	}
	e := queue[i]
	return append(queue[:i:i], queue[i+1:]...), &e
}
