package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrationsFollowFileNumbers checks that the .sql files are numbered 001..N and that migrations()[i] is file i+1.
func TestMigrationsFollowFileNumbers(t *testing.T) {
	files, err := filepath.Glob("*.sql")
	if err != nil {
		t.Fatal(err)
	}
	list := migrations()
	for i, file := range files {
		if want := fmt.Sprintf("%03d_", i+1); !strings.HasPrefix(file, want) {
			t.Errorf("%s is file %d; number the files from 001 without gaps or duplicates", file, i+1)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if i < len(list) && list[i] != string(data) {
			t.Errorf("migrations()[%d] (version %d) is not %s", i, i+1, file)
		}
	}
	if len(list) != len(files) {
		t.Errorf("migrations() lists %d schemas for %d .sql files", len(list), len(files))
	}
}
