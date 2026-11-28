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
// of equal values neither repeat nor skip a row.
func (c ListConditions) order() string {
	switch c.Sort {
	case SortOldest:
		return "created_at ASC,id ASC"
	case SortShortest:
		return "octet_length(source) ASC,created_at DESC,id DESC"
	case SortFastest:
		return "submission_cpu_time_ms(result) ASC NULLS LAST,created_at DESC,id DESC"
	}
	return "created_at DESC,id DESC"
}
