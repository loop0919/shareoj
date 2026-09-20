package problems

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	problemID      = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
	testFileDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func ValidID(id string) bool { return problemID.MatchString(id) }
func ValidDraft(d Draft, knownRuntimes []string) bool {
	if d.Difficulty != nil && (*d.Difficulty < 1 || *d.Difficulty > 10) {
		return false
	}
	if d.Checker != nil && d.Interactor != nil {
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

// Publishable adds publication requirements to the rules for a saved draft.
func Publishable(d Draft, knownRuntimes, enabledRuntimes []string) bool {
	if !ValidDraft(d, knownRuntimes) || strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Markdown) == "" {
		return false
	}
	for _, code := range []*Generator{d.Checker, d.Interactor} {
		if code != nil && (!slices.Contains(enabledRuntimes, code.Runtime) || strings.TrimSpace(code.Source) == "") {
			return false
		}
	}
	return true
}
