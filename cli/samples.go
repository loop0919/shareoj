package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Samples in a statement, kept in step with the test cases. A sample sits between
//
//	<!-- shareoj-sample-start: NAME -->  and  <!-- shareoj-sample-end: NAME -->
//
// and everything between the two is rewritten from the test case; notes on a sample go after the end marker.
// web/app/utils/sample-sync.ts implements the same rules, and testdata/sample-sync.json holds the cases both pass.

type sampleText struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

var sampleStart = regexp.MustCompile(`<!-- shareoj-sample-start: (.+?) -->`)
var sampleHeading = regexp.MustCompile(`(?m)^## サンプル (\d{1,6})\s*$`)
var backticks = regexp.MustCompile("`+")

func sampleEnd(name string) string { return "<!-- shareoj-sample-end: " + name + " -->" }

// syncableSampleName tells whether a name can sit in a marker and be found again.
func syncableSampleName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.Contains(name, "-->") && !strings.ContainsAny(name, "\r\n")
}

func sampleBody(number int, s sampleText) string {
	fenced := func(value string) string {
		length := 3
		for _, run := range backticks.FindAllString(value, -1) {
			length = max(length, len(run)+1)
		}
		fence := strings.Repeat("`", length)
		if value != "" && !strings.HasSuffix(value, "\n") {
			value += "\n"
		}
		return fence + "text\n" + value + fence
	}
	return fmt.Sprintf("## サンプル %d\n\n### 入力\n%s\n\n### 出力\n%s\n", number, fenced(s.Input), fenced(s.Output))
}

// syncSamples rewrites the marked samples in place and appends the rest in order. A marked sample keeps its number;
// a sample whose name cannot sit in a marker is appended bare. A marker that names no sample is left alone.
func syncSamples(text string, samples []sampleText) (out string, updated, added int) {
	next := 0
	for _, m := range sampleHeading.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		next = max(next, n)
	}
	named := map[string]sampleText{}
	for _, s := range samples {
		if syncableSampleName(s.Name) {
			named[s.Name] = s
		}
	}
	seen := map[string]bool{}
	var b strings.Builder
	position := 0
	for _, m := range sampleStart.FindAllStringSubmatchIndex(text, -1) {
		name, from := text[m[2]:m[3]], m[1]
		s, ok := named[name]
		if from < position || !ok {
			continue
		}
		length := strings.Index(text[from:], sampleEnd(name))
		if length < 0 {
			continue
		}
		inner := text[from : from+length]
		var number int
		if h := sampleHeading.FindStringSubmatch(inner); h != nil {
			number, _ = strconv.Atoi(h[1])
		} else {
			next++
			number = next
		}
		fresh := "\n\n" + sampleBody(number, s) + "\n"
		if fresh != inner {
			updated++
		}
		b.WriteString(text[position:from])
		b.WriteString(fresh)
		position = from + length
		seen[name] = true
	}
	b.WriteString(text[position:])
	var blocks []string
	for _, s := range samples {
		marked := syncableSampleName(s.Name)
		if marked && seen[s.Name] {
			continue
		}
		next++
		block := sampleBody(next, s)
		if marked {
			block = "<!-- shareoj-sample-start: " + s.Name + " -->\n\n" + block + "\n" + sampleEnd(s.Name) + "\n"
			seen[s.Name] = true
		}
		blocks = append(blocks, block)
	}
	out = b.String()
	if len(blocks) > 0 {
		if out != "" {
			out += "\n\n"
		}
		out += strings.Join(blocks, "\n")
	}
	return out, updated, len(blocks)
}

// markedSamples returns the samples of the cases that a marker can name, and the names of the others.
func markedSamples(cases []testCase) (samples []sampleText, skipped []string) {
	for _, c := range cases {
		if !c.IsSample {
			continue
		}
		if syncableSampleName(c.Name) {
			samples = append(samples, sampleText{c.Name, c.Input, c.Output})
		} else {
			skipped = append(skipped, strconv.Quote(c.Name))
		}
	}
	return samples, skipped
}

// writeSamples brings the statement file of a problem directory in step with its sample cases.
func writeSamples(directory string) (string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	c, err := readConfig(root)
	if err != nil {
		return "", err
	}
	if c.Tests == nil {
		return "", errors.New("samples requires [tests] in problem.toml")
	}
	// The whole problem must be valid first, so that a statement is never rewritten from tests check would refuse.
	if _, _, err := buildDraft(root, c); err != nil {
		return "", err
	}
	cases, _, err := loadCases(root, c)
	if err != nil {
		return "", err
	}
	text, err := readText(root, c.Statement, 400000)
	if err != nil {
		return "", err
	}
	samples, skipped := markedSamples(cases)
	note := ""
	if len(skipped) > 0 {
		note = "; skipped names that cannot sit in a marker: " + strings.Join(skipped, ", ")
	}
	next, updated, added := syncSamples(text, samples)
	if next == text {
		return fmt.Sprintf("%s already matches the %d sample(s)%s", c.Statement, len(samples), note), nil
	}
	if !validText(next, 100000) {
		return "", fmt.Errorf("%s: would exceed 100000 characters", c.Statement)
	}
	// Written beside the statement and renamed, so an interrupted run leaves the old file whole.
	path := filepath.Join(directory, filepath.FromSlash(c.Statement))
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".shareoj-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.WriteString(next)
	if err := errors.Join(writeErr, f.Chmod(info.Mode().Perm()), f.Close()); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated %s: %d sample(s) rewritten, %d added%s", c.Statement, updated, added, note), nil
}
