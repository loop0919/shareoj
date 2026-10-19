package contests

import (
	"strings"
	"time"
	"unicode/utf8"

	"judge/api/internal/problems"
)

// JudgePolicy is captured from the current admission configuration.
type JudgePolicy struct {
	KnownRuntimes, EnabledRuntimes []string
}

func (p JudgePolicy) validProblem(d problems.Draft) bool {
	return len(problems.ContentIssues(d, p.KnownRuntimes, p.EnabledRuntimes)) == 0
}

func ValidInput(in Input) bool {
	if strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Description) > 100000 || strings.ContainsRune(in.Title+in.Description, 0) || !utf8.ValidString(in.Title+in.Description) || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) || in.EndsAt.Year() > 9999 || in.Version < 0 || in.Version > 9007199254740990 || in.PenaltyMinutes == nil || *in.PenaltyMinutes < 0 || *in.PenaltyMinutes > 1440 || len(in.Problems) == 0 || len(in.Problems) > 100 {
		return false
	}
	if f := in.FreezeMinutes.Value; f != nil && (*f < 1 || int64(*f) > int64(in.EndsAt.Sub(in.StartsAt)/time.Minute)) {
		return false
	}
	if size := in.MaxTeamSize.Value; size != nil && (*size < MinTeamSize || *size > MaxTeamMembers) {
		return false
	}
	// Turning the onsite division on needs its passphrase; otherwise the passphrase is not read.
	if in.OnsiteDivision != nil && *in.OnsiteDivision && (in.OnsitePassphrase == nil || !ValidPassphrase(*in.OnsitePassphrase)) {
		return false
	}
	seen := map[string]bool{}
	// Points 0 leave them unset: a partially scored problem takes its full score (ADR 0025), and save refuses the
	// others without points.
	for _, p := range in.Problems {
		if !problems.ValidID(p.ID) || seen[p.ID] || p.Points < 0 || p.Points > problems.MaxFullScore {
			return false
		}
		seen[p.ID] = true
	}
	return true
}
