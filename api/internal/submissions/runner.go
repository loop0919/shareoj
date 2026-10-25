package submissions

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// JudgeTimeout covers up to 100 cases, including container startup, inspection and cleanup.
const JudgeTimeout = time.Hour

// boundedOutput drains pipes after the limit and stops the container through cancel.
type boundedOutput struct {
	buffer bytes.Buffer
	limit  int
	over   bool
	cancel context.CancelFunc
	once   sync.Once
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	left := b.limit - b.buffer.Len()
	if n > left {
		_, _ = b.buffer.Write(p[:left])
		b.over = true
		b.once.Do(b.cancel)
	} else {
		_, _ = b.buffer.Write(p)
	}
	return n, nil
}

type containerResult struct {
	output, diagnostic      []byte
	code                    int
	elapsed                 time.Duration
	oom, timedOut, overflow bool
	err                     error
}

func container(ctx context.Context, image, dir, input string, memory int, deadline time.Duration, outputLimit int, command ...string) containerResult {
	var token [12]byte
	_, _ = rand.Read(token[:])
	name := "openoj-" + hex.EncodeToString(token[:])
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
	}()
	limited, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	out := &boundedOutput{limit: outputLimit, cancel: cancel}
	diag := &boundedOutput{limit: 64 << 10, cancel: cancel}
	args := []string{"run", "--name", name, "--pull=never", "--network=none", "--read-only",
		"--cap-drop=ALL", "--security-opt=no-new-privileges", "--user=65534:65534",
		"--cpus=1", fmt.Sprintf("--memory=%dm", memory), fmt.Sprintf("--memory-swap=%dm", memory),
		"--pids-limit=64", "--ulimit=nofile=64:64", "--ulimit=fsize=33554432:33554432", "--ulimit=core=0:0",
		"--log-driver=none", "--tmpfs=/tmp:rw,nosuid,nodev,size=128m,mode=1777", "--workdir=/tmp",
		"--mount", "type=bind,src=" + dir + ",dst=/submission,readonly", "-i", image}
	args = append(args, command...)
	cmd := exec.CommandContext(limited, "docker", args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout, cmd.Stderr = out, diag
	err := cmd.Run()
	result := containerResult{output: out.buffer.Bytes(), diagnostic: diag.buffer.Bytes(), overflow: out.over || diag.over}
	if limited.Err() != nil {
		result.timedOut = !result.overflow && ctx.Err() == nil
		if ctx.Err() != nil {
			result.err = ctx.Err()
		}
		return result
	}
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 5*time.Second)
	defer inspectCancel()
	state, inspectErr := exec.CommandContext(inspectCtx, "docker", "inspect", "--format={{json .State}}", name).Output()
	var status struct {
		ExitCode   int
		OOMKilled  bool
		Error      string
		Running    bool
		StartedAt  time.Time
		FinishedAt time.Time
	}
	if inspectErr != nil || json.Unmarshal(state, &status) != nil || status.Error != "" || status.Running {
		result.err = fmt.Errorf("container state unavailable")
		return result
	}
	result.code, result.oom = status.ExitCode, status.OOMKilled
	result.elapsed = status.FinishedAt.Sub(status.StartedAt)
	if err != nil && status.ExitCode == 0 {
		result.err = err
	}
	return result
}

// localCommands keeps saved C++17 submissions on their original command.
func localCommands(runtime string) (filename, compile string, run []string) {
	switch runtime {
	case "cpp17-local":
		return "main.cpp", "g++ -std=c++17 -O2 -pipe /submission/main.cpp -o /tmp/main && cat /tmp/main", []string{"/submission/main"}
	case "cpp23-gcc-local":
		return "main.cpp", "g++ -std=c++23 -O2 -pipe -I/submission/include /submission/main.cpp -o /tmp/main && cat /tmp/main", []string{"/submission/main"}
	case "python314-local":
		return "main.py", `python3 -I -c 'import py_compile; py_compile.compile("/submission/main.py", cfile="/tmp/main", doraise=True)' && cat /tmp/main`, []string{"python3", "-I", "-B", "/submission/main"}
	}
	return "", "", nil
}
