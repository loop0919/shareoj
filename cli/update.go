package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const cliReleases = "https://github.com/loop0919/shareoj/releases"

var cliVersionPattern = regexp.MustCompile(`^cli-v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func parseCLIVersion(tag string) ([3]uint64, bool) {
	var numbers [3]uint64
	parts := cliVersionPattern.FindStringSubmatch(tag)
	if parts == nil {
		return numbers, false
	}
	for i := range numbers {
		var err error
		numbers[i], err = strconv.ParseUint(parts[i+1], 10, 64)
		if err != nil {
			return numbers, false
		}
	}
	return numbers, true
}

type updateCache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// Update checks are best effort and never change the command's result.
func notifyUpdate(ctx context.Context, args []string, out io.Writer) {
	current, ok := parseCLIVersion(version)
	if !ok || len(args) == 0 || os.Getenv("SHAREOJ_NO_UPDATE_CHECK") == "1" {
		return
	}
	online := false
	switch args[0] {
	case "login", "pull", "push":
		online = true
	case "guide", "init", "check", "logout":
	default:
		return
	}
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-help" || arg == "-h" || arg == "--h" {
			return
		}
	}
	directory, err := os.UserCacheDir()
	if err != nil {
		return
	}
	path := filepath.Join(directory, "shareoj", "update.json")
	var saved updateCache
	_ = readJSON(path, &saved)
	now := time.Now()
	if age := now.Sub(saved.CheckedAt); online && (age < 0 || age >= 24*time.Hour) {
		saved.CheckedAt = now
		if latest := latestCLIVersion(ctx); latest != "" {
			saved.Latest = latest
		}
		// Cache failed attempts too, so an outage does not slow every command.
		_ = atomicJSON(path, saved)
	}
	latest, ok := parseCLIVersion(saved.Latest)
	if !ok {
		return
	}
	for i := range current {
		if latest[i] < current[i] {
			return
		}
		if latest[i] > current[i] {
			fmt.Fprintf(out, "shareoj: Update available: %s (current: %s).\n", saved.Latest, version)
			if runtime.GOOS == "windows" {
				fmt.Fprintln(out, "Download:", cliReleases+"/latest")
			} else {
				fmt.Fprintln(out, "Update (use your original SHAREOJ_INSTALL_DIR if customized):\n  curl -fsSL https://www.share-oj.net/install.sh | bash")
			}
			return
		}
	}
}

func latestCLIVersion(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, cliReleases+"/latest", nil)
	if err != nil {
		return ""
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		return ""
	}
	location, err := response.Location()
	if err != nil {
		return ""
	}
	tag := strings.TrimPrefix(location.String(), cliReleases+"/tag/")
	if _, ok := parseCLIVersion(tag); !ok {
		return ""
	}
	return tag
}
