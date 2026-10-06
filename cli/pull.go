package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

// pull reserves a new directory and removes partial output on failure.
func pull(ctx context.Context, id, directory string, c *apiClient) (saved state, err error) {
	if !problemID.MatchString(id) {
		return saved, errors.New("expected a problem UUID")
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		return saved, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(directory))
		}
	}()
	var remote problem
	if err = c.call(ctx, "GET", "/my/problems/"+id, nil, &remote, true); err != nil {
		return saved, err
	}
	if remote.ID != id || remote.Version <= 0 || remote.Version > maxVersion || remote.Draft == nil {
		return saved, errors.New("invalid problem response from server")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return saved, err
	}
	defer root.Close()
	if err = exportDraft(ctx, root, remote, c); err != nil {
		return saved, err
	}
	if _, err = loadProblem(directory); err != nil {
		return saved, fmt.Errorf("draft cannot be represented by the CLI: %w", err)
	}
	saved = state{API: c.base, ID: id, Version: remote.Version}
	err = atomicJSON(filepath.Join(directory, stateName), saved)
	return saved, err
}

func exportDraft(ctx context.Context, root *os.Root, remote problem, c *apiClient) error {
	var d struct {
		Title, Markdown, Editorial string
		TimeLimitMS, MemoryLimitMB string
		Difficulty                 *int
		Generators                 map[string]program
		Checker, Interactor        *program
		TestCases                  []testCase
	}
	data, err := json.Marshal(remote.Draft)
	if err != nil {
		return err
	}
	if err := decodeJSON(data, &d); err != nil {
		return err
	}
	config := problemConfig{Title: d.Title, Statement: "statement.md", Difficulty: d.Difficulty}
	if config.TimeMS, err = strconv.Atoi(d.TimeLimitMS); err != nil {
		return errors.New("invalid time limit from server")
	}
	if config.MemoryMB, err = strconv.Atoi(d.MemoryLimitMB); err != nil {
		return errors.New("invalid memory limit from server")
	}
	editorial := "editorial.md"
	config.Editorial = &editorial
	for name, content := range map[string]string{
		"statement.md": d.Markdown, editorial: d.Editorial,
		".gitignore": ".shareoj.json\n.shareoj.lock\n",
	} {
		if err := root.WriteFile(name, []byte(content), 0600); err != nil {
			return err
		}
	}
	config.Generators = map[string]codeConfig{}
	for role, p := range d.Generators {
		if role != "input" && role != "output" && role != "validation" {
			return fmt.Errorf("unknown generator: %s", role)
		}
		// Absent legacy generators can be returned as zero-valued structs.
		if p == (program{}) {
			continue
		}
		code, err := exportProgram(root, role, p)
		if err != nil {
			return err
		}
		config.Generators[role] = code
	}
	for role, p := range map[string]*program{"checker": d.Checker, "interactor": d.Interactor} {
		if p == nil {
			continue
		}
		code, err := exportProgram(root, role, *p)
		if err != nil {
			return err
		}
		if role == "checker" {
			config.Checker = &code
		} else {
			config.Interactor = &code
		}
	}
	if len(d.TestCases) > 100 {
		return errors.New("too many test cases from server")
	}
	config.Tests = &testConfig{Directory: "tests", Suffix: ".out", Samples: []string{}, Names: map[string]string{}}
	if err := root.Mkdir("tests", 0700); err != nil {
		return err
	}
	total := 0
	for i, test := range d.TestCases {
		// Never use server-supplied case names as paths; numbering preserves order.
		name := fmt.Sprintf("case%03d", i+1)
		config.Tests.Names[name] = test.Name
		if test.IsSample {
			config.Tests.Samples = append(config.Tests.Samples, name)
		}
		for _, side := range []struct {
			suffix, text string
			ref          *testFile
		}{{".in", test.Input, test.InputFile}, {".out", test.Output, test.OutputFile}} {
			size := len(side.text)
			if side.ref != nil {
				if side.text != "" || side.ref.Size <= 0 {
					return errors.New("invalid test file reference")
				}
				size = side.ref.Size
			}
			if size > 16<<20 || total+size > 512<<20 {
				return errors.New("test data exceeds size limits")
			}
			total += size
			content := []byte(side.text)
			if side.ref != nil {
				content, err = c.downloadTestFile(ctx, remote.ID, *side.ref)
				if err != nil {
					return err
				}
			}
			if err := root.WriteFile("tests/"+name+side.suffix, content, 0600); err != nil {
				return err
			}
		}
	}
	data, err = toml.Marshal(config)
	if err != nil {
		return err
	}
	return root.WriteFile("problem.toml", data, 0600)
}

func exportProgram(root *os.Root, role string, p program) (codeConfig, error) {
	// Source paths are explicit in TOML, so they need no language-specific extension.
	code := codeConfig{Source: role + ".txt", Runtime: p.Runtime, Protocol: p.Protocol}
	return code, root.WriteFile(code.Source, []byte(p.Source), 0600)
}

func (c *apiClient) downloadTestFile(ctx context.Context, id string, ref testFile) ([]byte, error) {
	digest, err := hex.DecodeString(ref.SHA256)
	if !problemID.MatchString(ref.ID) || ref.Size <= 0 || ref.Size > 16<<20 || err != nil || len(digest) != sha256.Size {
		return nil, errors.New("invalid test file reference")
	}
	var download struct {
		URL, SHA256 string
		Size        int
	}
	if err := c.call(ctx, "GET", "/my/problems/"+id+"/test-files/"+ref.ID, nil, &download, true); err != nil {
		return nil, err
	}
	if download.Size != ref.Size || download.SHA256 != ref.SHA256 {
		return nil, errors.New("test file metadata mismatch")
	}
	if err := validateURL(download.URL); err != nil {
		return nil, err
	}
	// Storage gets no API token. The client's redirect policy also applies here.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download.URL, nil)
	if err != nil {
		return nil, errors.New("could not create test file request")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("test file download failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("test file download: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(ref.Size)+1))
	if err != nil {
		return nil, errors.New("could not read test file")
	}
	if len(data) != ref.Size || fmt.Sprintf("%x", sha256.Sum256(data)) != ref.SHA256 || !validText(string(data), 16<<20) {
		return nil, errors.New("downloaded test file integrity check failed")
	}
	return data, nil
}
