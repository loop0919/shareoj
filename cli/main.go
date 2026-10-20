package main

import (
	"bufio"
	"context"
	"crypto/rand"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"
)

const stateName = ".shareoj.json"
const maxVersion = 9007199254740990

// Set by the release build with -ldflags "-X main.version=cli-vX.Y.Z".
var version = "dev"

var problemID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type state struct {
	API     string `json:"api"`
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

type problem struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Draft   draft  `json:"draft"`
}

func push(ctx context.Context, directory string, c *apiClient, expected int64) (state, error) {
	local, err := loadProblem(directory)
	if err != nil {
		return state{}, err
	}
	lockPath := filepath.Join(directory, ".shareoj.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return state{}, errors.New("another push is active; if it crashed, remove .shareoj.lock after checking the process")
	}
	if err != nil {
		return state{}, err
	}
	defer os.Remove(lockPath)
	_, _ = fmt.Fprintln(lock, os.Getpid())
	if err := lock.Close(); err != nil {
		return state{}, err
	}
	s := state{}
	statePath := filepath.Join(directory, stateName)
	err = readJSON(statePath, &s)
	if errors.Is(err, os.ErrNotExist) {
		if expected != -1 {
			return s, errors.New("--expect-version requires an existing .shareoj.json")
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return s, err
		}
		id[6], id[8] = id[6]&0x0f|0x40, id[8]&0x3f|0x80
		s = state{API: c.base, ID: fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])}
		// Persist identity before uploads or saves so retries cannot create duplicates.
		if err := atomicJSON(statePath, s); err != nil {
			return s, err
		}
	} else if err != nil {
		return s, err
	}
	if s.API != c.base {
		return s, errors.New("problem belongs to another API; use the original --api URL")
	}
	if !problemID.MatchString(s.ID) || s.Version < 0 || s.Version > maxVersion || expected < -1 || expected > maxVersion {
		return s, errors.New("invalid problem ID/version in state or --expect-version")
	}
	version := s.Version
	if expected != -1 {
		version = expected
	}
	path := "/my/problems/" + s.ID
	var remote problem
	err = c.call(ctx, "GET", path, nil, &remote, true)
	var apiErr *httpError
	if errors.As(err, &apiErr) && apiErr.status == 404 && version == 0 {
		remote = problem{ID: s.ID, Draft: draft{}}
	} else if err != nil {
		return s, err
	}
	if remote.ID != s.ID || remote.Draft == nil {
		return s, errors.New("invalid problem response from server")
	}
	if remote.Version != version {
		return s, fmt.Errorf("version conflict: local %d, remote %d; review the web draft before using --expect-version %d", version, remote.Version, remote.Version)
	}
	d := remote.Draft
	if err := mergeDraft(local, d); err != nil {
		return s, err
	}
	if local["testCases"] != nil {
		var cases []testCase
		if err := decodeJSON(local["testCases"], &cases); err != nil {
			return s, err
		}
		inline := 0
		for i := range cases {
			for _, side := range []struct {
				value *string
				ref   **testFile
			}{
				{&cases[i].Input, &cases[i].InputFile}, {&cases[i].Output, &cases[i].OutputFile},
			} {
				if len(*side.value) > 64<<10 || inline+len(*side.value) > 256<<10 {
					ref, err := c.upload(ctx, s.ID, *side.value)
					if err != nil {
						return s, err
					}
					*side.ref, *side.value = ref, ""
				} else {
					inline += len(*side.value)
				}
			}
		}
		d.set("testCases", cases)
	}
	var result problem
	if err := c.call(ctx, "PUT", path, struct {
		Version int64 `json:"version"`
		Draft   draft `json:"draft"`
	}{version, d}, &result, true); err != nil {
		return s, err
	}
	if result.ID != s.ID || result.Version != version+1 {
		return s, errors.New("unexpected save response; check the remote draft before retrying")
	}
	s.Version = result.Version
	if err := atomicJSON(statePath, s); err != nil {
		return s, fmt.Errorf("remote save succeeded at version %d, but local state could not be saved: %w", s.Version, err)
	}
	return s, nil
}

// mergeDraft applies the update rules of cli/README.md to remote: set fields replace remote ones, omitted optional
// fields keep them, generators are replaced per role, and a checker and an interactor replace each other. The scoring
// and generator merges read remote values that the copy from local overwrites, so they run first and write their
// results into local, which is modified too. push calls it before uploading test files, so that a refusal uploads or
// saves nothing and the uploaded references then replace the inline cases copied from local.
func mergeDraft(local, remote draft) error {
	if err := mergeScoring(local, remote); err != nil {
		return err
	}
	if local["generators"] != nil {
		generators, updates := map[string]program{}, map[string]program{}
		if remote["generators"] != nil && string(remote["generators"]) != "null" {
			if err := decodeJSON(remote["generators"], &generators); err != nil {
				return err
			}
		}
		if err := decodeJSON(local["generators"], &updates); err != nil {
			return err
		}
		for role, p := range updates {
			generators[role] = p
		}
		local.set("generators", generators)
	}
	for key, value := range local {
		remote[key] = value
	}
	if local["checker"] != nil {
		delete(remote, "interactor")
	}
	if local["interactor"] != nil {
		delete(remote, "checker")
	}
	return nil
}

// mergeScoring keeps a remote aggregator that problem.toml omits, like other optional programs. It refuses to replace
// the tests of a partially scored problem without [scoring], since the rebuilt cases would belong to no group.
func mergeScoring(local, remote draft) error {
	partial := remote["scoring"] != nil && string(remote["scoring"]) != "null"
	if local["scoring"] == nil {
		if partial && local["testCases"] != nil {
			return errors.New("the remote draft uses partial scoring: add [scoring] and [[tests.groups]] to problem.toml (shareoj pull writes them) or omit [tests]; switching back to all-or-nothing is done on the website")
		}
		return nil
	}
	var s scoring
	if err := decodeJSON(local["scoring"], &s); err != nil {
		return err
	}
	if s.Mode != "aggregate" || s.Aggregator != nil || !partial {
		return nil
	}
	var r scoring
	if err := decodeJSON(remote["scoring"], &r); err != nil {
		return errors.New("invalid scoring in the remote draft")
	}
	s.Aggregator = r.Aggregator
	local.set("scoring", s)
	return nil
}

const template = `title = "A + B"
statement = "statement.md"
editorial = "editorial.md"
time_limit_ms = 2000
memory_limit_mb = 512

[generators.input]
source = "input.cpp"
runtime = "cpp23-gcc"

[generators.output]
source = "output.cpp"
runtime = "cpp23-gcc"

[generators.validation]
source = "validate.cpp"
runtime = "cpp23-gcc"

[tests]
directory = "tests"
output_suffix = ".out"
samples = ["sample1"]
`

// The statement and editorial a new problem starts with. The website's problem editor imports the same file
// (web/app/utils/problem-draft.ts), so both start from the same text.
//
//go:embed problem-template.json
var markdownTemplate []byte

func initProblem(directory string) error {
	var text struct {
		Statement string `json:"statement"`
		Editorial string `json:"editorial"`
	}
	if err := decodeJSON(markdownTemplate, &text); err != nil {
		return err
	}
	// Mkdir deliberately refuses existing directories; init never overwrites work.
	if err := os.Mkdir(directory, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"problem.toml":     template,
		".gitignore":       ".shareoj.json\n.shareoj.lock\n",
		"statement.md":     text.Statement,
		"editorial.md":     text.Editorial,
		"input.cpp":        "#include <iostream>\nint main() { long long n; std::cin >> n; n = (n % 1000000000 + 1000000000) % 1000000000; std::cout << n << \" \" << n + 1 << \"\\n\"; }\n",
		"output.cpp":       "#include <iostream>\nint main() { long long a, b; std::cin >> a >> b; std::cout << a + b << \"\\n\"; }\n",
		"validate.cpp":     "#include <iostream>\nint main() { long long a, b; if (!(std::cin >> a >> b) || a < 0 || a > 1000000000 || b < 0 || b > 1000000000) return 1; std::cin >> std::ws; return std::cin.eof() ? 0 : 1; }\n",
		"tests/sample1.in": "3 5\n", "tests/sample1.out": "8\n",
	}
	for name, value := range files {
		path := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return err
		}
	}
	return nil
}

func terminalPrompt(ctx context.Context, label string, secret bool) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	var original *term.State
	if secret {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return "", errors.New("login needs an interactive terminal; use SHAREOJ_ACCESS_TOKEN for automation")
		}
		var err error
		original, err = term.GetState(int(os.Stdin.Fd()))
		if err != nil {
			return "", err
		}
	}
	type answer struct {
		value string
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		if secret {
			value, err := term.ReadPassword(int(os.Stdin.Fd()))
			done <- answer{string(value), err}
		} else {
			value, err := bufio.NewReader(os.Stdin).ReadString('\n')
			done <- answer{strings.TrimSpace(value), err}
		}
	}()
	select {
	case result := <-done:
		if secret {
			fmt.Fprintln(os.Stderr)
		}
		return result.value, result.err
	case <-ctx.Done():
		if original != nil {
			_ = term.Restore(int(os.Stdin.Fd()), original)
		}
		return "", ctx.Err()
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		fmt.Fprintln(out, "shareoj", version)
		return nil
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "Usage: shareoj <guide|init|check|pull|push|login|logout|version> [options] [arguments]\nAuthoring with an AI agent? Start with: shareoj guide\nAutomatic discovery: shareoj guide --install-skill <codex|claude|all>\nUse shareoj <command> --help for options.")
		return nil
	}
	command := args[0]
	if command == "guide" {
		return showGuide(args[1:], out)
	}
	if command != "init" && command != "check" && command != "pull" && command != "push" && command != "login" && command != "logout" {
		return fmt.Errorf("unknown command: %s", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(out)
	api, username, expected := defaultAPI, "", int64(-1)
	google, noBrowser, site := false, false, ""
	if value := os.Getenv("SHAREOJ_API_URL"); value != "" {
		api = value
	}
	if command == "pull" || command == "push" || command == "login" || command == "logout" {
		flags.StringVar(&api, "api", api, "ShareOJ API URL")
	}
	if command == "pull" {
		flags.Usage = func() {
			fmt.Fprintln(out, "Usage: shareoj pull [--api URL] <problem-id> <directory>")
			flags.PrintDefaults()
		}
	}
	if command == "push" {
		flags.Int64Var(&expected, "expect-version", -1, "Save against a reviewed remote version after a conflict")
	}
	if command == "login" {
		flags.StringVar(&username, "username", "", "Login email address")
		flags.BoolVar(&google, "google", false, "Sign in with Google in a browser")
		flags.BoolVar(&noBrowser, "no-browser", false, "Print the Google login URL without opening a browser")
		flags.StringVar(&site, "site", "", "ShareOJ website origin for Google login")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	needsDirectory := command == "init" || command == "check" || command == "push"
	if command == "pull" && flags.NArg() != 2 {
		return errors.New("expected pull <problem-id> <directory>; place options before the problem ID")
	}
	if needsDirectory && flags.NArg() != 1 {
		return errors.New("expected one directory for init/check/push; place options before the directory")
	}
	if (command == "login" || command == "logout") && flags.NArg() != 0 {
		return fmt.Errorf("%s takes no positional arguments", command)
	}
	if expected < -1 || expected > maxVersion {
		return errors.New("invalid --expect-version")
	}
	if (google && username != "") || (!google && (noBrowser || site != "")) {
		return errors.New("use --google without --username; --site and --no-browser require --google")
	}
	directory := flags.Arg(0)
	switch command {
	case "init":
		if err := initProblem(directory); err != nil {
			return err
		}
		fmt.Fprintln(out, "Created", directory)
	case "check":
		_, notes, err := readProblem(directory)
		if err != nil {
			return err
		}
		// Drafts are saved with these, but publishing and unfinished contests refuse them.
		for _, note := range notes {
			fmt.Fprintln(out, "warning:", note)
		}
		fmt.Fprintln(out, "OK: configuration and files checked; code was not compiled or executed")
	default:
		client, err := newClient(api)
		if err != nil {
			return err
		}
		switch command {
		case "pull":
			saved, err := pull(ctx, flags.Arg(0), flags.Arg(1), client)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Pulled draft %s (version %d) into %s\n", saved.ID, saved.Version, flags.Arg(1))
		case "login":
			if google {
				var opener func(string) error
				if !noBrowser {
					opener = openBrowser
				}
				err = client.browserLogin(ctx, site, out, opener)
			} else {
				err = client.login(ctx, username, func(label string, secret bool) (string, error) {
					return terminalPrompt(ctx, label, secret)
				})
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "Logged in")
		case "logout":
			if err := os.Remove(client.credential); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			fmt.Fprintln(out, "Local credentials removed (SHAREOJ_ACCESS_TOKEN, if set, is unchanged)")
		case "push":
			saved, err := push(ctx, directory, client, expected)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Saved draft %s (version %d)\n", saved.ID, saved.Version)
		}
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "shareoj:", err)
		os.Exit(1)
	}
	notifyUpdate(ctx, os.Args[1:], os.Stderr)
}
