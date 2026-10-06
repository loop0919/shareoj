package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuthoringGuide(t *testing.T) {
	// Reading instructions must work without a usable API or credentials.
	t.Setenv("SHAREOJ_API_URL", "invalid")
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	var out strings.Builder
	if err := run(context.Background(), []string{"guide"}, &out); err != nil || out.String() != authoringGuide {
		t.Fatalf("guide: %v", err)
	}
	for _, args := range [][]string{{"guide", "--help"}, {"--help"}, {}} {
		out.Reset()
		if err := run(context.Background(), args, &out); err != nil || !strings.Contains(out.String(), "shareoj guide") {
			t.Fatalf("help %v: %s %v", args, out.String(), err)
		}
	}
	for _, args := range [][]string{{"guide", "extra"}, {"guide", "--unknown"}, {"guide", "--install-skill", "unsupported"}} {
		if err := run(context.Background(), args, io.Discard); err == nil {
			t.Fatalf("invalid guide args accepted: %v", args)
		}
	}
	// The guide's TOML example must remain usable with the generated files.
	_, exampleTOML, ok := strings.Cut(authoringGuide, "```toml\n")
	if !ok {
		t.Fatal("missing TOML example")
	}
	exampleTOML, _, ok = strings.Cut(exampleTOML, "\n```")
	if !ok {
		t.Fatal("unterminated TOML example")
	}
	dir := example(t)
	write(t, filepath.Join(dir, "problem.toml"), exampleTOML)
	write(t, filepath.Join(dir, "editorial.md"), "解説")
	if _, err := loadProblem(dir); err != nil {
		t.Fatal("guide example cannot be checked:", err)
	}
}

func TestInstallAuthoringSkills(t *testing.T) {
	for _, agent := range []string{"codex", "claude", "all"} {
		t.Run(agent, func(t *testing.T) {
			home := t.TempDir()
			for range 2 {
				if err := installAuthoringSkills(home, agent, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			for name, directory := range map[string]string{"codex": ".agents", "claude": ".claude"} {
				path := filepath.Join(home, directory, "skills", "shareoj-authoring", "SKILL.md")
				data, err := os.ReadFile(path)
				if agent != name && agent != "all" {
					if !os.IsNotExist(err) {
						t.Fatal("unselected agent changed")
					}
					continue
				}
				if err != nil || string(data) != authoringSkill {
					t.Fatalf("installed skill: %v", err)
				}
				write(t, path, "custom skill")
				if err := installAuthoringSkills(home, name, io.Discard); err == nil {
					t.Fatal("overwrote custom skill")
				}
				data, _ = os.ReadFile(path)
				if string(data) != "custom skill" {
					t.Fatal("custom skill changed")
				}
			}
		})
	}
	home := t.TempDir()
	if err := installAuthoringSkills(home, "../escape", io.Discard); err == nil {
		t.Fatal("unknown agent accepted")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid selection wrote files")
	}
	if runtime.GOOS != "windows" {
		target, link := filepath.Join(home, "custom.md"), filepath.Join(home, "SKILL.md")
		write(t, target, "custom skill")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := installAuthoringSkill(link); err == nil {
			t.Fatal("skill symlink accepted")
		}
		data, _ := os.ReadFile(target)
		if string(data) != "custom skill" {
			t.Fatal("symlink target changed")
		}
	}
}
