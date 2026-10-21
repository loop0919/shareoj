package problems

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"judge/api/internal/tags"
)

var (
	problemID      = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
	testFileDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func ValidID(id string) bool { return problemID.MatchString(id) }

// Whitespace programs consist of spaces, tabs and newlines, so trimming loses the program.
func HasSource(runtime, source string) bool {
	if strings.TrimSuffix(runtime, "-isolate") == "whitespace13" {
		return source != ""
	}
	return strings.TrimSpace(source) != ""
}

func ValidDraft(d Draft, knownRuntimes []string) bool {
	if d.Difficulty != nil && (*d.Difficulty < 1 || *d.Difficulty > 10) {
		return false
	}
	if (d.Checker != nil && d.Interactor != nil) || !tags.Valid(d.Tags) {
		return false
	}
	if !validScoring(d, knownRuntimes) {
		return false
	}
	for _, code := range []*Generator{d.Checker, d.Interactor} {
		if code == nil {
			continue
		}

		if !slices.Contains(knownRuntimes, code.Runtime) || !code.ValidJudgeProtocol() || len(code.Source) > 65536 || !utf8.ValidString(code.Source) || strings.ContainsRune(code.Source, 0) {
			return false
		}
	}
	if d.Generators != nil {
		for _, g := range []Generator{d.Generators.Input, d.Generators.Output, d.Generators.Validation} {
			if g.Protocol != "" || len(g.Runtime) > 64 || len(g.Source) > 65536 || !utf8.ValidString(g.Source) || strings.ContainsRune(g.Source+g.Runtime, 0) {
				return false
			}
		}
	}

	timeMS, e1 := strconv.Atoi(d.TimeLimitMS)
	if e1 != nil || timeMS < 100 || timeMS > 5000 || timeMS%100 != 0 || len(d.TestCases) > 100 {
		return false
	}
	var total, inline int64
	names := make(map[string]bool)
	for _, c := range d.TestCases {
		name := strings.TrimSpace(c.Name)
		if utf8.RuneCountInString(c.Name) > 64 || strings.IndexFunc(c.Name, unicode.IsControl) >= 0 || (c.Name != "" && name == "") || (name != "" && names[name]) {
			return false
		}
		if name != "" {
			names[name] = true
		}
		for _, value := range []struct {
			text string
			file *TestFile
		}{{c.Input, c.InputFile}, {c.Output, c.OutputFile}} {
			if value.file == nil {
				if len(value.text) > 64<<10 || !utf8.ValidString(value.text) || strings.ContainsRune(value.text, 0) {
					return false
				}
				total += int64(len(value.text))
				inline += int64(len(value.text))
				continue
			}
			if value.text != "" || !problemID.MatchString(value.file.ID) || value.file.Size <= 0 || value.file.Size > 16<<20 ||
				!testFileDigest.MatchString(value.file.SHA256) || value.file.Key != "" || value.file.Version != "" {
				return false
			}
			total += value.file.Size
		}
	}
	if total > 512<<20 || inline > 256<<10 {
		return false
	}
	memory, e2 := strconv.Atoi(d.MemoryLimitMB)
	return utf8.RuneCountInString(d.Title) <= 120 && utf8.RuneCountInString(d.Markdown) <= 100000 && utf8.RuneCountInString(d.Editorial) <= 100000 &&
		!strings.ContainsRune(d.Title+d.Markdown+d.Editorial, '\x00') && len(d.TimeLimitMS) <= 10 && len(d.MemoryLimitMB) <= 10 &&
		e2 == nil && memory >= 64 && memory <= 512
}

// MaxFullScore bounds a problem's full score like a contest's points, and MaxScoringGroups the number of its groups.
const (
	MaxFullScore     = 1_000_000
	MaxScoringGroups = 100
)

// validScoring checks the structure of partial scoring with the judge's limits (ADR 0025). A draft without groups, a
// group without cases and a case in no group are content issues instead, so that the editor can save on the way.
func validScoring(d Draft, knownRuntimes []string) bool {
	s := d.Scoring
	if s == nil {
		return !slices.ContainsFunc(d.TestCases, func(c TestCase) bool { return len(c.Groups) > 0 })
	}
	if (s.Mode != ScoringGroups && s.Mode != ScoringAggregate) || len(s.Groups) > MaxScoringGroups || (s.Aggregator != nil && s.Mode != ScoringAggregate) {
		return false
	}
	// Cases refer to groups by name, so names follow the rules of case names and are never blank.
	trimmed, exact := map[string]bool{}, map[string]bool{}
	total := 0
	for _, g := range s.Groups {
		name := strings.TrimSpace(g.Name)
		if name == "" || utf8.RuneCountInString(g.Name) > 64 || strings.IndexFunc(g.Name, unicode.IsControl) >= 0 || trimmed[name] || g.Points < 0 || g.Points > MaxFullScore {
			return false
		}
		trimmed[name], exact[g.Name] = true, true
		total += g.Points
	}
	if len(s.Groups) > 0 && (total < 1 || total > MaxFullScore) {
		return false
	}
	for _, c := range d.TestCases {
		seen := map[string]bool{}
		for _, name := range c.Groups {
			if !exact[name] || seen[name] {
				return false
			}
			seen[name] = true
		}
	}
	// The aggregator is judge code: it runs in the judge's sandbox and has no protocol to choose.
	if a := s.Aggregator; a != nil && (!slices.Contains(knownRuntimes, a.Runtime) || a.Protocol != "" || len(a.Source) > 65536 || !utf8.ValidString(a.Source) || strings.ContainsRune(a.Source, 0)) {
		return false
	}
	return true
}

// Publishable adds publication requirements to the rules for a saved draft.
func Publishable(d Draft, knownRuntimes, enabledRuntimes []string) bool {
	return len(ContentIssues(d, knownRuntimes, enabledRuntimes)) == 0
}
