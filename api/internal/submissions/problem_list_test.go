package submissions

import (
	"fmt"
	"slices"
	"testing"
)

// referenceProblemListQuery builds ProblemList's page query as it was before the page was cut by id: one scan of
// submissions under the filter, which selects the columns of every run it reads. countArgs and count count what it
// lists, for the offsets of the last page.
func referenceProblemListQuery(problemID, contestID, viewer string, scope Scope, onsite bool, offset int, conditions ListConditions) (page, count string, args []any) {
	narrow, extra := conditions.where(7)
	// The count leaves $5, the offset, unused, so PostgreSQL cannot type it unless the query refers to it.
	return `SELECT ` + columns + ` FROM submissions WHERE ` + problemSubmissionVisible + narrow + ` ORDER BY ` + conditions.order() + ` LIMIT 51 OFFSET $5`,
		`SELECT count(*) FROM submissions WHERE ` + problemSubmissionVisible + narrow + ` AND $5::integer>=0`,
		append([]any{problemID, contestID, viewer, string(scope), offset, onsite}, extra...)
}

// TestProblemListMatchesReferencePostgres compares ProblemList's page with the reference, for the list of each
// problem and of each contest problem, viewer by viewer, scope by scope and with and without onsite, for every sort
// with each kind of condition, on the first page, pages that start inside the list, its last page and past its end.
func TestProblemListMatchesReferencePostgres(t *testing.T) {
	ctx, pool := newListTestPool(t, "problem_list_reference")
	seedListReference(t, ctx, pool)

	cpp, _ := LanguageRuntimeIDs("cpp")
	languages := StoredRuntimeIDs(cpp)
	sorts := []string{SortNewest, SortOldest, SortShortest, SortFastest}
	var rotation []ListConditions
	for _, sort := range sorts {
		rotation = append(rotation,
			ListConditions{Sort: sort},
			ListConditions{Sort: sort, Verdict: "AC"},
			ListConditions{Sort: sort, Runtimes: languages},
			ListConditions{Sort: sort, User: "part1"},
			ListConditions{Sort: sort, Verdict: "AC", Runtimes: languages},
		)
	}
	// Each problem is listed as such, and each contest problem also in its contest.
	type list struct{ problem, contest string }
	var lists []list
	for n := 1; n <= 9; n++ {
		problem := fmt.Sprintf("a0000000-0000-4000-8000-00000000000%d", n)
		lists = append(lists, list{problem, ""})
		if n <= 8 {
			lists = append(lists, list{problem, fmt.Sprintf("c0000000-0000-4000-8000-00000000000%d", (n+1)/2)})
		}
	}
	type view struct {
		scope  Scope
		onsite bool
	}
	var compared, empty, deep int
	compare := func(l list, viewer string, v view, offset int, conditions ListConditions) {
		t.Helper()
		want, _, wantArgs := referenceProblemListQuery(l.problem, l.contest, viewer, v.scope, v.onsite, offset, conditions)
		got, gotArgs := problemListQuery(l.problem, l.contest, viewer, v.scope, v.onsite, offset, conditions)
		gotItems, wantItems := listedIDs(t, ctx, pool, got, gotArgs), listedIDs(t, ctx, pool, want, wantArgs)
		if !slices.Equal(gotItems, wantItems) {
			t.Errorf("problem %s in contest %q, viewer %q, scope %q onsite %v, %+v offset %d: got %v, want %v",
				l.problem, l.contest, viewer, v.scope, v.onsite, conditions, offset, gotItems, wantItems)
		}
		compared++
		switch {
		case len(wantItems) == 0:
			empty++
		case offset >= 20:
			deep++
		}
	}
	// Each list and viewer gets a page of the next conditions in turn, from the first run or 3 runs in, so that every
	// condition is read in some list by some viewer. A participant, a team mate and an outsider also read each list
	// through the other views.
	turn := 0
	for _, l := range lists {
		views := []view{{ScopeMine, false}}
		if l.contest != "" {
			views = append(views, view{ScopeAll, true}, view{ScopeTeam, false})
		}
		for _, viewer := range append(referenceUsers, "") {
			compare(l, viewer, view{ScopeAll, false}, turn%2*3, rotation[turn%len(rotation)])
			turn++
			if !slices.Contains([]string{"part1", "mate1", "outsider"}, viewer) {
				continue
			}
			for _, v := range views {
				compare(l, viewer, v, turn%2*3, rotation[turn%len(rotation)])
				turn++
			}
		}
	}
	// Every sort, as it is and narrowed, is read deep into the list too, on its last page and past its end, by the
	// owner of the contests and of the ninth problem, and by an outsider.
	for _, l := range lists {
		for _, viewer := range []string{"alice", "outsider"} {
			for _, sort := range sorts {
				for _, conditions := range []ListConditions{{Sort: sort}, {Sort: sort, Verdict: "AC"}} {
					v := view{ScopeAll, false}
					_, count, args := referenceProblemListQuery(l.problem, l.contest, viewer, v.scope, v.onsite, 0, conditions)
					total := countedRuns(t, ctx, pool, count, args)
					for _, offset := range []int{max(total-1, 0) / 50 * 50, total} {
						compare(l, viewer, v, offset, conditions)
					}
				}
			}
		}
	}
	// The comparisons are not vacuous: some pages hold runs from run 20 on, and some viewers see nothing.
	if empty == 0 || deep == 0 || empty == compared {
		t.Fatalf("seed too uniform: of %d pages, %d are empty and %d start at run 20 or later and hold runs", compared, empty, deep)
	}
	t.Logf("compared %d pages: %d are empty, %d start at run 20 or later and hold runs", compared, empty, deep)
}
