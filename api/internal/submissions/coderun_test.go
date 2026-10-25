package submissions

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCreateCodeRunNeedsAReadyJudge(t *testing.T) {
	image := "sha256:" + strings.Repeat("a", 64)
	input := CodeRunInput{Runtime: "cpp17", Source: "int main(){}"}
	for name, test := range map[string]struct {
		service Service
		want    error
	}{
		"maintenance":   {Service{Config: Config{JudgeImage: image, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "none"}, Submissions: &Store{}}, ErrMaintenance},
		"no image":      {Service{Config: Config{JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17"}, Submissions: &Store{}}, ErrUnavailable},
		"no submission": {Service{Config: Config{JudgeImage: image, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17"}}, ErrUnavailable},
	} {
		if _, err := test.service.CreateCodeRun(context.Background(), "alice", "id", input); !errors.Is(err, test.want) {
			t.Errorf("%s: got %v want %v", name, err, test.want)
		}
	}
}
