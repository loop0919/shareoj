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
	Tests      *struct {
		Directory string   `toml:"directory"`
		Suffix    string   `toml:"output_suffix"`
		Samples   []string `toml:"samples"`
	} `toml:"tests"`
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
}

func validText(value string, limit int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= limit
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
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := readText(root, "problem.toml", 65536)
	if err != nil {
		return nil, err
	}
	c := problemConfig{TimeMS: 2000, MemoryMB: 512}
	if err := toml.NewDecoder(strings.NewReader(data)).DisallowUnknownFields().Decode(&c); err != nil {
		return nil, fmt.Errorf("problem.toml: %w", err)
	}
	if !validText(c.Title, 120) || c.TimeMS < 100 || c.TimeMS > 5000 || c.TimeMS%100 != 0 || c.MemoryMB < 64 || c.MemoryMB > 512 {
		return nil, errors.New("title must be at most 120 characters; time_limit_ms: 100..5000 in steps of 100; memory_limit_mb: 64..512")
	}
	if c.Difficulty != nil && (*c.Difficulty < 1 || *c.Difficulty > 10) {
		return nil, errors.New("difficulty must be 1..10")
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
			return nil, err
		}
		if !validText(value, 100000) {
			return nil, fmt.Errorf("%s: exceeds 100000 characters", name)
		}
		d.set(key, value)
	}
	generators := map[string]program{}
	for role, config := range c.Generators {
		if !slices.Contains([]string{"input", "output", "validation"}, role) {
			return nil, fmt.Errorf("unknown generator: %s", role)
		}
		p, err := loadProgram(root, config, false)
		if err != nil {
			return nil, fmt.Errorf("%s generator: %w", role, err)
		}
		generators[role] = p
	}
	if len(generators) > 0 {
		d.set("generators", generators)
	}
	if c.Checker != nil && c.Interactor != nil {
		return nil, errors.New("choose checker or interactor, not both")
	}
	for role, config := range map[string]*codeConfig{"checker": c.Checker, "interactor": c.Interactor} {
		if config != nil {
			p, err := loadProgram(root, *config, true)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", role, err)
			}
			d.set(role, p)
		}
	}
	if c.Tests != nil {
		cases, err := loadCases(root, c)
		if err != nil {
			return nil, err
		}
		d.set("testCases", cases)
	}
	return d, nil
}

func loadCases(root *os.Root, c problemConfig) ([]testCase, error) {
	t := c.Tests
	if !filepath.IsLocal(t.Directory) {
		return nil, errors.New("tests.directory must stay inside the problem directory")
	}
	if t.Suffix == "" {
		t.Suffix = ".out"
	}
	if t.Suffix != ".out" && t.Suffix != ".diff" {
		return nil, errors.New("tests.output_suffix must be .out or .diff")
	}
	dir, err := root.OpenRoot(t.Directory)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := fs.ReadDir(dir.FS(), ".")
	if err != nil {
		return nil, err
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
		return nil, errors.New("tests require paired input/output files, at most 100 cases")
	}
	for _, sample := range t.Samples {
		if !slices.Contains(inputs, sample) {
			return nil, fmt.Errorf("sample case %q does not exist", sample)
		}
	}
	cases, names, total := []testCase{}, map[string]bool{}, 0
	for _, name := range inputs {
		trimmed := strings.TrimSpace(name)
		if !validText(name, 64) || trimmed == "" || strings.ContainsFunc(name, unicode.IsControl) || names[trimmed] {
			return nil, fmt.Errorf("invalid or duplicate test case name: %q", name)
		}
		names[trimmed] = true
		input, err := readText(dir, name+".in", 16<<20)
		if err != nil {
			return nil, err
		}
		output, err := readText(dir, name+t.Suffix, 16<<20)
		if err != nil {
			return nil, err
		}
		total += len(input) + len(output)
		if total > 512<<20 {
			return nil, errors.New("test data exceeds 512 MiB")
		}
		cases = append(cases, testCase{Name: name, IsSample: slices.Contains(t.Samples, name), Input: input, Output: output})
	}
	return cases, nil
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
