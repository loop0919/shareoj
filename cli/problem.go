package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

type draft map[string]json.RawMessage

func (d draft) set(key string, value any) {
	// All callers pass JSON-compatible concrete values.
	d[key], _ = json.Marshal(value)
}

type codeConfig struct {
	Source   string `toml:"source"`
	Runtime  string `toml:"runtime"`
	Protocol string `toml:"protocol"`
}

type problemConfig struct {
	Title      string                `toml:"title"`
	Statement  string                `toml:"statement"`
	Editorial  *string               `toml:"editorial"`
	TimeMS     int                   `toml:"time_limit_ms"`
	MemoryMB   int                   `toml:"memory_limit_mb"`
	Difficulty *int                  `toml:"difficulty"`
	Generators map[string]codeConfig `toml:"generators"`
	Checker    *codeConfig           `toml:"checker"`
	Interactor *codeConfig           `toml:"interactor"`
	Scoring    *scoringConfig        `toml:"scoring"`
	Tests      *testConfig           `toml:"tests"`
}

// scoringConfig selects partial scoring (ADR 0025). Its groups live in [[tests.groups]], next to the cases they list.
type scoringConfig struct {
	Mode       string      `toml:"mode"`
	Aggregator *codeConfig `toml:"aggregator"`
}

type testConfig struct {
	Directory string            `toml:"directory"`
	Suffix    string            `toml:"output_suffix"`
	Samples   []string          `toml:"samples"`
	Names     map[string]string `toml:"names,omitempty"`
	Groups    []groupConfig     `toml:"groups,omitempty"`
}

// groupConfig lists a group's cases by file stem, like samples, because case names may be empty or repeated.
type groupConfig struct {
	Name   string   `toml:"name"`
	Points int      `toml:"points"`
	Cases  []string `toml:"cases"`
}

type program struct {
	Runtime  string `json:"runtime"`
	Source   string `json:"source"`
	Protocol string `json:"protocol,omitempty"`
}

type testFile struct {
	ID     string `json:"id"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type testCase struct {
	Name       string    `json:"name"`
	IsSample   bool      `json:"isSample"`
	Input      string    `json:"input"`
	Output     string    `json:"output"`
	InputFile  *testFile `json:"inputFile,omitempty"`
	OutputFile *testFile `json:"outputFile,omitempty"`
	Groups     []string  `json:"groups,omitempty"`
}

type scoring struct {
	Mode       string         `json:"mode"`
	Groups     []scoringGroup `json:"groups"`
	Aggregator *program       `json:"aggregator,omitempty"`
}

type scoringGroup struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

// The server's limits for partial scoring; the judge uses the same ones.
const (
	maxFullScore     = 1_000_000
	maxScoringGroups = 100
)

func validText(value string, limit int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= limit
}

// normalizeNewlines turns CRLF and lone CR into LF.
func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func readText(root *os.Root, name string, limit int) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("path must stay inside the problem directory: %q", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return "", fmt.Errorf("%s: expected a regular file of at most %d bytes", name, limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return "", err
	}
	if len(data) > limit || !validText(string(data), limit) {
		return "", fmt.Errorf("%s: requires UTF-8 without NUL, at most %d bytes", name, limit)
	}
	return string(data), nil
}

var runtimeID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func loadProgram(root *os.Root, config codeConfig, judge bool) (program, error) {
	p := program{Runtime: config.Runtime, Protocol: config.Protocol}
	if !runtimeID.MatchString(p.Runtime) || (!judge && p.Protocol != "") ||
		(judge && p.Protocol != "" && p.Protocol != "legacy" && p.Protocol != "testlib") ||
		(p.Protocol == "testlib" && p.Runtime != "cpp23-gcc" && p.Runtime != "cpp23-clang") {
		return p, errors.New("invalid runtime/protocol (testlib requires cpp23-gcc or cpp23-clang)")
	}
	var err error
	p.Source, err = readText(root, config.Source, 65536)
	return p, err
}

func loadProblem(directory string) (draft, error) {
	d, _, err := readProblem(directory)
	return d, err
}

// readProblem also returns warnings about partial scoring that the server reports only as readiness issues.
func readProblem(directory string) (draft, []string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	data, err := readText(root, "problem.toml", 65536)
	if err != nil {
		return nil, nil, err
	}
	c := problemConfig{TimeMS: 2000, MemoryMB: 512}
	if err := toml.NewDecoder(strings.NewReader(data)).DisallowUnknownFields().Decode(&c); err != nil {
		return nil, nil, fmt.Errorf("problem.toml: %w", err)
	}
	return buildDraft(root, c)
}

func buildDraft(root *os.Root, c problemConfig) (draft, []string, error) {
	if !validText(c.Title, 120) || c.TimeMS < 100 || c.TimeMS > 5000 || c.TimeMS%100 != 0 || c.MemoryMB < 64 || c.MemoryMB > 512 {
		return nil, nil, errors.New("title must be at most 120 characters; time_limit_ms: 100..5000 in steps of 100; memory_limit_mb: 64..512")
	}
	if c.Difficulty != nil && (*c.Difficulty < 1 || *c.Difficulty > 10) {
		return nil, nil, errors.New("difficulty must be 1..10")
	}
	d := draft{}
	d.set("title", c.Title)
	d.set("timeLimitMs", strconv.Itoa(c.TimeMS))
	d.set("memoryLimitMb", strconv.Itoa(c.MemoryMB))
	if c.Difficulty != nil {
		d.set("difficulty", *c.Difficulty)
	}
	texts := map[string]string{"markdown": c.Statement}
	if c.Editorial != nil {
		texts["editorial"] = *c.Editorial
	}
	for key, name := range texts {
		value, err := readText(root, name, 400000)
		if err != nil {
			return nil, nil, err
		}
		if !validText(value, 100000) {
			return nil, nil, fmt.Errorf("%s: exceeds 100000 characters", name)
		}
		d.set(key, value)
	}
	generators := map[string]program{}
	for role, config := range c.Generators {
		if !slices.Contains([]string{"input", "output", "validation"}, role) {
			return nil, nil, fmt.Errorf("unknown generator: %s", role)
		}
		p, err := loadProgram(root, config, false)
		if err != nil {
			return nil, nil, fmt.Errorf("%s generator: %w", role, err)
		}
		generators[role] = p
	}
	if len(generators) > 0 {
		d.set("generators", generators)
	}
	if c.Checker != nil && c.Interactor != nil {
		return nil, nil, errors.New("choose checker or interactor, not both")
	}
	for role, config := range map[string]*codeConfig{"checker": c.Checker, "interactor": c.Interactor} {
		if config != nil {
			p, err := loadProgram(root, *config, true)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", role, err)
			}
			d.set(role, p)
		}
	}
	if c.Scoring != nil {
		s, err := loadScoring(root, c)
		if err != nil {
			return nil, nil, err
		}
		d.set("scoring", s)
	} else if c.Tests != nil && len(c.Tests.Groups) > 0 {
		return nil, nil, errors.New(`[[tests.groups]] requires [scoring] with mode = "groups" or "aggregate"`)
	}
	var notes []string
	if c.Tests != nil {
		cases, caseNotes, err := loadCases(root, c)
		if err != nil {
			return nil, nil, err
		}
		d.set("testCases", cases)
		notes = caseNotes
	}
	if c.Scoring != nil && c.Scoring.Mode == "aggregate" && c.Scoring.Aggregator == nil {
		notes = append(notes, `mode = "aggregate" without [scoring.aggregator]: push keeps the remote aggregator, and publishing needs one`)
	}
	return d, notes, nil
}

// loadScoring mirrors the server's structural rules for partial scoring, which refuse the whole save otherwise.
func loadScoring(root *os.Root, c problemConfig) (scoring, error) {
	s := scoring{Mode: c.Scoring.Mode, Groups: []scoringGroup{}}
	if s.Mode != "groups" && s.Mode != "aggregate" {
		return s, errors.New(`scoring.mode must be "groups" or "aggregate"`)
	}
	// Membership is listed per group in [[tests.groups]], so groups cannot be checked without the cases.
	if c.Tests == nil {
		return s, errors.New("[scoring] requires [tests]; list each group and its cases in [[tests.groups]]")
	}
	if a := c.Scoring.Aggregator; a != nil {
		if s.Mode != "aggregate" {
			return s, errors.New(`[scoring.aggregator] requires mode = "aggregate"`)
		}
		// The aggregator runs in the judge's sandbox and has no protocol to choose.
		p, err := loadProgram(root, *a, false)
		if err != nil {
			return s, fmt.Errorf("scoring aggregator: %w", err)
		}
		s.Aggregator = &p
	}
	if len(c.Tests.Groups) > maxScoringGroups {
		return s, fmt.Errorf("at most %d [[tests.groups]]", maxScoringGroups)
	}
	names, total := map[string]bool{}, 0
	for _, g := range c.Tests.Groups {
		trimmed := strings.TrimSpace(g.Name)
		if trimmed == "" || !validText(g.Name, 64) || strings.ContainsFunc(g.Name, unicode.IsControl) || names[trimmed] {
			return s, fmt.Errorf("group names must be non-blank, unique after trimming spaces, at most 64 characters, without control characters: %q", g.Name)
		}
		if g.Points < 0 || g.Points > maxFullScore {
			return s, fmt.Errorf("group %q: points must be an integer 0..%d", g.Name, maxFullScore)
		}
		names[trimmed] = true
		total += g.Points
		s.Groups = append(s.Groups, scoringGroup{Name: g.Name, Points: g.Points})
	}
	if len(s.Groups) > 0 && (total < 1 || total > maxFullScore) {
		return s, fmt.Errorf("the full score (sum of group points) must be 1..%d, got %d", maxFullScore, total)
	}
	return s, nil
}

// loadCases also maps the stems in [[tests.groups]] to each case's groups, in the order of the groups.
func loadCases(root *os.Root, c problemConfig) ([]testCase, []string, error) {
	t := c.Tests
	if !filepath.IsLocal(t.Directory) {
		return nil, nil, errors.New("tests.directory must stay inside the problem directory")
	}
	if t.Suffix == "" {
		t.Suffix = ".out"
	}
	if t.Suffix != ".out" && t.Suffix != ".diff" {
		return nil, nil, errors.New("tests.output_suffix must be .out or .diff")
	}
	dir, err := root.OpenRoot(t.Directory)
	if err != nil {
		return nil, nil, err
	}
	defer dir.Close()
	entries, err := fs.ReadDir(dir.FS(), ".")
	if err != nil {
		return nil, nil, err
	}
	inputs, outputs := []string{}, []string{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".in") {
			inputs = append(inputs, strings.TrimSuffix(name, ".in"))
		}
		if strings.HasSuffix(name, t.Suffix) {
			outputs = append(outputs, strings.TrimSuffix(name, t.Suffix))
		}
	}
	slices.Sort(inputs)
	slices.Sort(outputs)
	if !slices.Equal(inputs, outputs) || len(inputs) > 100 {
		return nil, nil, errors.New("tests require paired input/output files, at most 100 cases")
	}
	for _, sample := range t.Samples {
		if !slices.Contains(inputs, sample) {
			return nil, nil, fmt.Errorf("sample case %q does not exist", sample)
		}
	}
	for name := range t.Names {
		if !slices.Contains(inputs, name) {
			return nil, nil, fmt.Errorf("named case %q does not exist", name)
		}
	}
	groups := map[string][]string{}
	var notes []string
	for _, g := range t.Groups {
		listed := map[string]bool{}
		for _, stem := range g.Cases {
			if !slices.Contains(inputs, stem) {
				return nil, nil, fmt.Errorf("group %q: case %q does not exist", g.Name, stem)
			}
			if listed[stem] {
				return nil, nil, fmt.Errorf("group %q lists case %q twice", g.Name, stem)
			}
			listed[stem] = true
			groups[stem] = append(groups[stem], g.Name)
		}
		// A group without cases gives its points to every judged submission, so the server refuses to publish it.
		if len(g.Cases) == 0 {
			notes = append(notes, fmt.Sprintf("group %q lists no cases; publishing and contests need a case in every group", g.Name))
		}
	}
	if c.Scoring != nil {
		var ungrouped []string
		for _, stem := range inputs {
			if !slices.Contains(t.Samples, stem) && len(groups[stem]) == 0 {
				ungrouped = append(ungrouped, strconv.Quote(stem))
			}
		}
		if len(t.Groups) == 0 {
			notes = append(notes, "[scoring] has no [[tests.groups]]; publishing and contests need at least one group")
		} else if len(ungrouped) > 0 {
			notes = append(notes, "non-sample cases in no group: "+strings.Join(ungrouped, ", ")+"; publishing and contests need every one in a group")
		}
	}
	cases, names, total := []testCase{}, map[string]bool{}, 0
	for _, name := range inputs {
		label := name
		if mapped, ok := t.Names[name]; ok {
			label = mapped
		}
		trimmed := strings.TrimSpace(label)
		if !validText(label, 64) || (label != "" && trimmed == "") || strings.ContainsFunc(label, unicode.IsControl) || (trimmed != "" && names[trimmed]) {
			return nil, nil, fmt.Errorf("invalid or duplicate test case name: %q", label)
		}
		names[trimmed] = true
		input, err := readText(dir, name+".in", 16<<20)
		if err != nil {
			return nil, nil, err
		}
		output, err := readText(dir, name+t.Suffix, 16<<20)
		if err != nil {
			return nil, nil, err
		}
		// Files written on Windows are CRLF, and Python's input() on Linux keeps the CR.
		input, output = normalizeNewlines(input), normalizeNewlines(output)
		total += len(input) + len(output)
		if total > 512<<20 {
			return nil, nil, errors.New("test data exceeds 512 MiB")
		}
		cases = append(cases, testCase{Name: label, IsSample: slices.Contains(t.Samples, name), Input: input, Output: output, Groups: groups[name]})
	}
	return cases, notes, nil
}

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected a single JSON value")
	}
	return nil
}
