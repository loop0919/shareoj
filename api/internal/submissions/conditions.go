package submissions

import "fmt"

const (
	SortNewest   = "newest"
	SortOldest   = "oldest"
	SortShortest = "shortest"
	SortFastest  = "fastest"
)

// Verdicts lists the verdicts a list can be narrowed to.
var Verdicts = []string{"AC", "WA", "TLE", "MLE", "OLE", "RE", "CE", "JE"}

// ListConditions narrows and orders a submission list inside what the viewer may see.
type ListConditions struct {
	Sort     string   // SortNewest, SortOldest, SortShortest or SortFastest; "" reads as SortNewest
	Verdict  string   // "" or one of Verdicts
	Runtimes []string // stored runtime IDs to keep; nil keeps every runtime
	User     string   // a handle; "" keeps every submitter
}

// where gives the SQL to AND onto a visibility filter and the arguments it adds, numbered from next. Every value is an
// argument; the SQL text comes from constants only.
func (c ListConditions) where(next int) (sql string, args []any) {
	add := func(format string, value any) {
		args = append(args, value)
		sql += " AND " + fmt.Sprintf(format, next+len(args)-1)
	}
	if c.Verdict != "" {
		// result->>'verdict' is the expression of the submissions_verdict statistics in 052_submission_list_indexes.sql.
		add("(status='DONE' AND result->>'verdict'=$%d)", c.Verdict)
	}
	if len(c.Runtimes) > 0 {
		add("runtime=ANY($%d)", c.Runtimes)
	}
	if c.User != "" {
		// An unknown handle selects no owner, so the list is empty.
		add("owner_id IN (SELECT owner_id FROM user_profiles WHERE handle=$%d)", c.User)
	}
	return sql, args
}

// order gives the ORDER BY text; it comes from constants only. Every order ends in the newest run first, so that pages
// of equal values neither repeat nor skip a row. The code length and time orders are word for word the keys of the
// indexes in 052_submission_list_indexes.sql after the problem or the contest, which let a page read only its own runs;
// change them together. A migration that replaces submission_cpu_time_ms must also REINDEX submissions_problem_fastest
// and submissions_contest_fastest, which hold the times the old function computed.
func (c ListConditions) order() string {
	return c.orderBy(c.key())
}

// key gives the value that order sorts by before the time of the run, or "" when it sorts by the time alone.
func (c ListConditions) key() string {
	switch c.Sort {
	case SortShortest:
		return "octet_length(source)"
	case SortFastest:
		return "submission_cpu_time_ms(result)"
	}
	return ""
}

// orderBy is order with key in place of the value key gives, such as the name of a column that holds it.
func (c ListConditions) orderBy(key string) string {
	switch c.Sort {
	case SortOldest:
		return "created_at ASC,id ASC"
	case SortShortest:
		return key + " ASC,created_at DESC,id DESC"
	case SortFastest:
		return key + " ASC NULLS LAST,created_at DESC,id DESC"
	}
	return "created_at DESC,id DESC"
}
