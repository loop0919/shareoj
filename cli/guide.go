package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed guide.md
var authoringGuide string

//go:embed skills/shareoj-authoring/SKILL.md
var authoringSkill string

func showGuide(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("guide", flag.ContinueOnError)
	flags.SetOutput(out)
	agent := flags.String("install-skill", "", "Install the authoring skill for codex, claude, or all (user scope)")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: shareoj guide [--install-skill <codex|claude|all>]\nWithout options, prints the offline authoring guide.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("guide takes no positional arguments")
	}
	if *agent == "" {
		_, err := io.WriteString(out, authoringGuide)
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return installAuthoringSkills(home, *agent, out)
}

func installAuthoringSkills(home, agent string, out io.Writer) error {
	if agent != "codex" && agent != "claude" && agent != "all" {
		return errors.New("--install-skill must be codex, claude, or all")
	}
	for _, target := range []struct{ name, directory string }{{"codex", ".agents"}, {"claude", ".claude"}} {
		if agent != target.name && agent != "all" {
			continue
		}
		path := filepath.Join(home, target.directory, "skills", "shareoj-authoring", "SKILL.md")
		if err := installAuthoringSkill(path); err != nil {
			return err
		}
		fmt.Fprintln(out, "Skill ready:", path)
	}
	_, err := fmt.Fprintln(out, "Start a new agent session, then ask: ShareOJで作問して")
	return err
}

func installAuthoringSkill(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(path)
		if statErr == nil && info.Mode().IsRegular() {
			data, readErr := os.ReadFile(path)
			if readErr == nil && string(data) == authoringSkill {
				return nil
			}
		}
		return fmt.Errorf("skill already exists; review it before replacing: %s", path)
	}
	if err != nil {
		return err
	}
	_, writeErr := io.WriteString(f, authoringSkill)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
}
