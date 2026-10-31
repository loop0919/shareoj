package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
)

var (
	//go:embed completions/shareoj.bash
	bashCompletion string
	//go:embed completions/_shareoj
	zshCompletion string
	//go:embed completions/shareoj.fish
	fishCompletion string
)

// showCompletions prints a script that completes commands, options, and directories; completion_test.go checks it
// against each command's --help.
func showCompletions(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("completions", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: shareoj completions <bash|zsh|fish>\nPrints the shell completion script; see the comment at its top for where to save or load it.")
	}
	if err := parseArgs(flags, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	script, ok := map[string]string{"bash": bashCompletion, "zsh": zshCompletion, "fish": fishCompletion}[flags.Arg(0)]
	if flags.NArg() != 1 || !ok {
		return errors.New("expected completions <bash|zsh|fish>")
	}
	_, err := io.WriteString(out, script)
	return err
}
