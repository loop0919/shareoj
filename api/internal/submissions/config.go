package submissions

import (
	"strings"
)

type Config struct {
	JudgeImage           string
	JudgeRuntime         string
	JudgeEnabledRuntimes string
}

func (p Config) AvailableRuntimes() []Runtime {
	if p.Maintenance() {
		return []Runtime{}
	}
	if !strings.HasPrefix(p.JudgeImage, "sha256:") || len(p.JudgeImage) != 71 {
		return []Runtime{}
	}
	if p.JudgeRuntime == "" || p.JudgeRuntime == "cpp17-local" {
		return PublishedRuntimes("cpp17")
	}
	if p.JudgeRuntime != "cpp17-isolate" {
		return []Runtime{}
	}
	return PublishedRuntimes(p.JudgeEnabledRuntimes)
}

func (p Config) Maintenance() bool {
	return strings.TrimSpace(p.JudgeEnabledRuntimes) == "none"
}

func (p Config) Ready() error {
	if p.Maintenance() {
		return ErrMaintenance
	}
	if !strings.HasPrefix(p.JudgeImage, "sha256:") || len(p.JudgeImage) != 71 || (p.JudgeRuntime != "" && p.JudgeRuntime != "cpp17-local" && p.JudgeRuntime != "cpp17-isolate") {
		return ErrUnavailable
	}
	return nil
}

func (p Config) RuntimeIDs() []string {
	ids := []string{}
	for _, rt := range p.AvailableRuntimes() {
		ids = append(ids, rt.ID)
	}
	return ids
}

func RuntimeIDs() []string {
	ids := make([]string, 0, len(Runtimes))
	for _, rt := range Runtimes {
		ids = append(ids, rt.ID)
	}
	return ids
}
