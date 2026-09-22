package contests

import (
	"strings"
	"unicode/utf8"

	"judge/api/internal/problems"
)

// JudgePolicy is captured from the current admission configuration.
type JudgePolicy struct {
	KnownRuntimes, EnabledRuntimes []string
}

func (p JudgePolicy) validProblem(d problems.Draft) bool {
	return problems.Publishable(d, p.KnownRuntimes, p.EnabledRuntimes) && len(d.TestCases) > 0
}

func ValidInput(in Input) bool {
	if strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Description) > 100000 || strings.ContainsRune(in.Title+in.Description, 0) || !utf8.ValidString(in.Title+in.Description) || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) || in.EndsAt.Year() > 9999 || in.Version < 0 || in.Version > 9007199254740990 || in.PenaltyMinutes == nil || *in.PenaltyMinutes < 0 || *in.PenaltyMinutes > 1440 || len(in.Problems) == 0 || len(in.Problems) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, p := range in.Problems {
		if !problems.ValidID(p.ID) || seen[p.ID] || p.Points < 1 || p.Points > 1000000 {
			return false
		}
		seen[p.ID] = true
	}
	return true
}
