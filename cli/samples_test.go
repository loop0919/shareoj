package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The cases are shared with web/app/utils/sample-sync.ts, which must answer the same.
func TestSyncSamples(t *testing.T) {
	data, err := os.ReadFile("testdata/sample-sync.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Statement, Want string
		Samples               []sampleText
		Updated, Added        int
	}
	if err := json.Unmarshal(data, &cases); err != nil || len(cases) == 0 {
		t.Fatal(err)
	}
	for _, c := range cases {
		got, updated, added := syncSamples(c.Statement, c.Samples)
		if got != c.Want || updated != c.Updated || added != c.Added {
			t.Errorf("%s: updated=%d added=%d\n%q", c.Name, updated, added, got)
		}
	}
}

// Issue #18: shareoj samples rewrites the statement from the sample cases, and check says when they drift.
func TestSamplesCommand(t *testing.T) {
	dir := example(t)
	statement := filepath.Join(dir, "statement.md")
	write(t, statement, "## 問題文\n")
	command := func(name string) string {
		t.Helper()
		var out strings.Builder
		if err := run(context.Background(), []string{name, dir}, &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	read := func() string {
		t.Helper()
		data, err := os.ReadFile(statement)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	// A statement without markers has not asked for syncing, so check stays quiet about it.
	if out := command("check"); strings.Contains(out, "marked samples") {
		t.Fatal(out)
	}
	input, _ := os.ReadFile(filepath.Join(dir, "tests", "sample1.in"))
	output, _ := os.ReadFile(filepath.Join(dir, "tests", "sample1.out"))
	block := func(in, out string) string {
		return "<!-- shareoj-sample-start: sample1 -->\n\n## サンプル 1\n\n### 入力\n```text\n" + in + "```\n\n### 出力\n```text\n" + out + "```\n\n<!-- shareoj-sample-end: sample1 -->\n"
	}
	if out := command("samples"); out != "Updated statement.md: 0 sample(s) rewritten, 1 added\n" || read() != "## 問題文\n\n\n"+block(string(input), string(output)) {
		t.Fatalf("%s\n%q", out, read())
	}
	if out := command("samples"); out != "statement.md already matches the 1 sample(s)\n" {
		t.Fatal(out)
	}
	// Notes after the end marker survive a change of the test case; non-sample cases never enter the statement.
	write(t, statement, read()+"\n説明\n")
	write(t, filepath.Join(dir, "tests", "sample1.in"), "10 20\n")
	write(t, filepath.Join(dir, "tests", "sample1.out"), "30\n")
	write(t, filepath.Join(dir, "tests", "hidden.in"), "1 1\n")
	write(t, filepath.Join(dir, "tests", "hidden.out"), "2\n")
	if out := command("check"); !strings.Contains(out, "warning: statement.md: the marked samples differ from the sample test cases; run shareoj samples <directory>") {
		t.Fatal(out)
	}
	if out := command("samples"); out != "Updated statement.md: 1 sample(s) rewritten, 0 added\n" || read() != "## 問題文\n\n\n"+block("10 20\n", "30\n")+"\n説明\n" {
		t.Fatalf("%s\n%q", out, read())
	}
	if out := command("check"); strings.Contains(out, "warning:") {
		t.Fatal(out)
	}
	// A problem that check refuses is not rewritten.
	write(t, filepath.Join(dir, "tests", "sample1.in"), "1 2\n")
	if err := os.Remove(filepath.Join(dir, "tests", "hidden.out")); err != nil {
		t.Fatal(err)
	}
	before := read()
	if err := run(context.Background(), []string{"samples", dir}, &strings.Builder{}); err == nil || read() != before {
		t.Fatal("rewrote the statement of an invalid problem", err)
	}
}
