package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// commandOptions reads the commands from shareoj --help and each command's options from its --help, so a new command
// or option without a completion fails the tests.
func commandOptions(t *testing.T) map[string][]string {
	t.Helper()
	var out strings.Builder
	if err := run(context.Background(), []string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	usage := regexp.MustCompile(`shareoj <([a-z|]+)>`).FindStringSubmatch(out.String())
	if usage == nil {
		t.Fatalf("no commands in usage: %s", out.String())
	}
	options := map[string][]string{}
	for _, command := range strings.Split(usage[1], "|") {
		// version prints the version for any arguments, so it has no options.
		if command == "version" {
			options[command] = []string{}
			continue
		}
		var help strings.Builder
		if err := run(context.Background(), []string{command, "--help"}, &help); err != nil {
			t.Fatal(command, err)
		}
		names := []string{"--help"}
		for _, m := range regexp.MustCompile(`(?m)^  -([a-z-]+)`).FindAllStringSubmatch(help.String(), -1) {
			names = append(names, "--"+m[1])
		}
		slices.Sort(names)
		options[command] = names
	}
	return options
}

// completionScript writes a shell's script and a directory to complete in, with a problem directory and a file.
func completionScript(t *testing.T, shell string) (script, dir string) {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s is not installed", shell)
	}
	var out strings.Builder
	if err := run(context.Background(), []string{"completions", shell}, &out); err != nil {
		t.Fatal(err)
	}
	script, dir = filepath.Join(t.TempDir(), "completion"), t.TempDir()
	write(t, script, out.String())
	if err := os.Mkdir(filepath.Join(dir, "prob"), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "notes.txt"), "")
	return script, dir
}

func sortedLines(output []byte) []string {
	lines := []string{}
	for line := range strings.Lines(string(output)) {
		if word, _, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "\t"); word != "" {
			lines = append(lines, word)
		}
	}
	slices.Sort(lines)
	return slices.Compact(lines)
}

// completionCases are command lines whose last word is being completed; directory is the answer for "prob".
func completionCases(t *testing.T, directory string) map[string][]string {
	options := commandOptions(t)
	commands := []string{}
	for command := range options {
		commands = append(commands, command)
	}
	slices.Sort(commands)
	cases := map[string][]string{
		"shareoj ":                          commands,
		"shareoj -":                         {"--help", "--version"},
		"shareoj guide --install-skill ":    {"all", "claude", "codex"},
		"shareoj completions ":              {"bash", "fish", "zsh"},
		"shareoj push ":                     {directory},
		"shareoj init p":                    {directory},
		"shareoj pull --api https://x/ id ": {directory},
		"shareoj push prob --api ":          {},
		"shareoj push prob --expect-v":      {"--expect-version"},
	}
	for command, names := range options {
		cases["shareoj "+command+" -"] = names
	}
	return cases
}

func TestBashCompletion(t *testing.T) {
	// Some builds of bash, such as Nix's default one, leave out programmable completion.
	if out, _ := exec.Command("bash", "-c", "compgen -W ok ok").Output(); string(out) != "ok\n" {
		t.Skip("bash without programmable completion")
	}
	script, dir := completionScript(t, "bash")
	for line, want := range completionCases(t, "prob") {
		cmd := exec.Command("bash", "--norc", "--noprofile", "-c",
			`source "$1"; shift; COMP_WORDS=("$@"); COMP_CWORD=$(($# - 1)); _shareoj; printf '%s\n' "${COMPREPLY[@]}"`,
			"bash", script)
		cmd.Args = append(cmd.Args, strings.Split(line, " ")...)
		cmd.Dir = dir
		output, err := cmd.Output()
		if got := sortedLines(output); err != nil || !slices.Equal(got, want) {
			t.Errorf("%q: got %q, want %q (%v)", line, got, want, err)
		}
	}
}

func TestFishCompletion(t *testing.T) {
	script, dir := completionScript(t, "fish")
	for line, want := range completionCases(t, "prob/") {
		cmd := exec.Command("fish", "--no-config", "-c", `source $argv[1]; complete -C $argv[2]`, script, line)
		cmd.Dir = dir
		output, err := cmd.Output()
		if got := sortedLines(output); err != nil || !slices.Equal(got, want) {
			t.Errorf("%q: got %q, want %q (%v)", line, got, want, err)
		}
	}
}

// zsh completes only inside its line editor, so this checks the syntax and that every command and option has a spec.
func TestZshCompletion(t *testing.T) {
	script, _ := completionScript(t, "zsh")
	if output, err := exec.Command("zsh", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("syntax: %v %s", err, output)
	}
	data, _ := os.ReadFile(script)
	for command, names := range commandOptions(t) {
		if !strings.Contains(string(data), "'"+command+":") {
			t.Errorf("no description for %s", command)
		}
		for _, name := range names {
			if !strings.Contains(string(data), "'"+name+"[") {
				t.Errorf("no spec for %s %s", command, name)
			}
		}
	}
}

func TestCompletionsCommand(t *testing.T) {
	for _, args := range [][]string{{"completions"}, {"completions", "pwsh"}, {"completions", "bash", "zsh"}} {
		if err := run(context.Background(), args, io.Discard); err == nil || err.Error() != "expected completions <bash|zsh|fish>" {
			t.Errorf("%v: %v", args, err)
		}
	}
}
