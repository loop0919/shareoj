package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstaller(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash installer targets Linux and macOS")
	}
	script, err := os.ReadFile("../web/public/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, system, arch, tag, failure string
		ok                               bool
	}{
		{"latest-linux", "Linux", "x86_64", "", "", true},
		{"pinned-linux-arm", "Linux", "aarch64", "cli-v0.1.0", "", true},
		{"mac-intel", "Darwin", "x86_64", "", "", true},
		{"mac-arm", "Darwin", "arm64", "", "", true},
		{"corrupt", "Linux", "x86_64", "", "checksum", false},
		{"download-error", "Linux", "x86_64", "", "download", false},
		{"missing-checksum", "Linux", "x86_64", "", "missing", false},
		{"bad-tag", "Linux", "x86_64", "../../bad", "", false},
		{"unsupported", "Linux", "riscv64", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "install path")
			if err := os.Mkdir(dest, 0755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(dest, "shareoj"), "old binary")
			binary := "new binary\n"
			write(t, filepath.Join(dir, "binary"), binary)
			platform, arch := strings.ToLower(tc.system), "amd64"
			if tc.arch == "arm64" || tc.arch == "aarch64" {
				arch = "arm64"
			}
			asset := "shareoj-" + platform + "-" + arch
			manifest := fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(binary)), asset)
			if tc.failure == "checksum" {
				manifest = strings.Repeat("0", 64) + "  " + asset + "\n"
			} else if tc.failure == "missing" {
				manifest = ""
			}
			write(t, filepath.Join(dir, "SHA256SUMS"), manifest)
			mocks := map[string]string{
				"uname": "#!/bin/bash\nif [[ $1 == -s ]]; then echo \"$MOCK_SYSTEM\"; else echo \"$MOCK_ARCH\"; fi\n",
				"curl": `#!/bin/bash
set -eu
output=''
for ((i=1; i<=$#; i++)); do
  if [[ ${!i} == --output ]]; then ((i+=1)); output=${!i}; fi
done
url=${!#}
base=https://github.com/loop0919/shareoj/releases
case "$url" in
  "$base/latest")
    [[ -z "$MOCK_TAG" ]] || exit 91
    printf '%s/tag/cli-v0.1.0' "$base" ;;
  "$base/download/cli-v0.1.0/$MOCK_ASSET")
    [[ "$MOCK_FAILURE" != download ]] || exit 22
    cp "$MOCK_DIR/binary" "$output" ;;
  "$base/download/cli-v0.1.0/SHA256SUMS") cp "$MOCK_DIR/SHA256SUMS" "$output" ;;
  *) echo "unexpected URL: $url" >&2; exit 92 ;;
esac
`,
			}
			for name, data := range mocks {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("SHAREOJ_INSTALL_DIR", dest)
			t.Setenv("MOCK_DIR", dir)
			t.Setenv("MOCK_SYSTEM", tc.system)
			t.Setenv("MOCK_ARCH", tc.arch)
			t.Setenv("MOCK_TAG", tc.tag)
			t.Setenv("MOCK_ASSET", asset)
			t.Setenv("MOCK_FAILURE", tc.failure)
			args := []string{"-s", "--"}
			if tc.tag != "" {
				args = append(args, tc.tag)
			}
			cmd := exec.Command("bash", args...)
			cmd.Stdin = strings.NewReader(string(script))
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.ok {
				t.Fatalf("installer: %v\n%s", err, output)
			}
			want := "old binary"
			if tc.ok {
				want = binary
				info, err := os.Stat(filepath.Join(dest, "shareoj"))
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatalf("executable permissions: %v", err)
				}
			}
			got, err := os.ReadFile(filepath.Join(dest, "shareoj"))
			if err != nil || string(got) != want {
				t.Fatalf("installed binary: %q, %v", got, err)
			}
			entries, _ := os.ReadDir(dest)
			if len(entries) != 1 {
				t.Fatal("temporary files not cleaned up")
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for _, arg := range []string{"version", "--version"} {
		var out strings.Builder
		if err := run(context.Background(), []string{arg}, &out); err != nil || out.String() != "shareoj "+version+"\n" {
			t.Fatalf("version output: %q, %v", out.String(), err)
		}
	}
}
