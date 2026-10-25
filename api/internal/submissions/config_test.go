package submissions

import (
	"reflect"
	"strings"
	"testing"
)

func TestLocalRuntimeCatalog(t *testing.T) {
	for _, backend := range []string{"", "docker-local"} {
		config := Config{JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: backend}
		if err := config.Ready(); err != nil {
			t.Fatal(err)
		}
		if got := config.RuntimeIDs(); !reflect.DeepEqual(got, []string{"cpp23-gcc", "python314"}) {
			t.Fatal(got)
		}
		service := Service{Config: config}
		if got := service.submissionRuntimes(); !reflect.DeepEqual(got, []string{"cpp23-gcc-local", "python314-local"}) {
			t.Fatal(got)
		}
		config.JudgeEnabledRuntimes = "none"
		if config.Ready() != ErrMaintenance || len(config.AvailableRuntimes()) != 0 {
			t.Fatal("local maintenance bypassed")
		}
	}
}
