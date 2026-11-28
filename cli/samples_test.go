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
	// Issue #29: check names the samples the statement does not show. Nothing is marked yet, so nothing has drifted.
	if out := command("check"); strings.Contains(out, "marked samples") || !strings.Contains(out, `warning: statement.md: the statement does not show the sample(s) "sample1"; write them in or run shareoj samples <directory>`) {
		t.Fatal(out)
	}
	// A sample written by hand counts, and so does any sample of a problem whose input goes to the judge code.
	write(t, statement, "## 問題文\n\n```\n3 5\n```\n\n```\n8\n```\n")
	if out := command("check"); strings.Contains(out, "does not show") {
		t.Fatal(out)
	}
	write(t, statement, "## 問題文\n")
	write(t, filepath.Join(dir, "judge.py"), "print(1)\n")
	for _, judge := range []string{"[interactor]\nsource = \"judge.py\"\nruntime = \"python3\"\nprotocol = \"legacy\"\n", "[run_twice]\nsource = \"judge.py\"\nruntime = \"python3\"\n"} {
		write(t, filepath.Join(dir, "problem.toml"), template+"\n"+judge)
		if out := command("check"); strings.Contains(out, "does not show") {
			t.Fatal(judge, out)
		}
	}
	write(t, filepath.Join(dir, "problem.toml"), template)
	input, _ := os.ReadFile(filepath.Join(dir, "tests", "sample1.in"))
	output, _ := os.ReadFile(filepath.Join(dir, "tests", "sample1.out"))
	block := func(in, out string) string {
		return "<!-- sample-begin: sample1 -->\n## サンプル 1\n### 入力\n```text\n" + in + "```\n### 出力\n```text\n" + out + "```\n<!-- sample-end: sample1 -->\n"
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

// The same cases as TestSampleInStatement in api/internal/problems, which must answer the same.
func TestSampleInStatement(t *testing.T) {
	sample := testCase{Name: "sample1", IsSample: true, Input: "1 2\n", Output: "3\n"}
	for _, tc := range []struct {
		name, statement string
		c               testCase
		want            bool
	}{
		{"nothing", "## 問題文\n1 2 を足すと 3 です。", sample, false},
		{"between the markers", "<!-- sample-begin: sample1 -->\n古い内容\n<!-- sample-end: sample1 -->\n", sample, true},
		{"the markers of another sample", "<!-- sample-begin: sample2 -->\n<!-- sample-end: sample2 -->\n", sample, false},
		{"a start marker alone", "<!-- sample-begin: sample1 -->\n", sample, false},
		{"written by hand", "### 入力\n```\n1 2\n```\n### 出力\n```text\n3\n```\n", sample, true},
		{"tilde fences and CRLF", "~~~\r\n1 2\r\n~~~\r\n\r\n~~~~text\r\n3\r\n~~~~\r\n", sample, true},
		{"the input alone", "```\n1 2\n```\n", sample, false},
		{"another output", "```\n1 2\n```\n```\n4\n```\n", sample, false},
		{"outside a code block", "入力は\n1 2\nで、出力は\n3\nです。", sample, false},
		{"a longer fence holds a shorter one", "````\n```\n1 2\n```\n````\n```\n3\n```\n", sample, false},
		{"an empty input needs no block", "```\n3\n```\n", testCase{Name: "s", IsSample: true, Output: "3\n"}, true},
	} {
		if got := sampleInStatement(tc.statement, tc.c); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}
