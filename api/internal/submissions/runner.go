package submissions

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"judge/api/internal/problems"
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

// Judge is a local Docker adapter. It never runs submitted code on the worker host.
func Judge(ctx context.Context, source string, job Job) Result {
	r := Result{Verdict: "JE", Total: len(job.Cases)}
	if job.Interactor != nil {
		return Result{Verdict: "JE", Total: len(job.Cases), CheckerLog: "対話形式はisolateワーカーで実行してください。"}
	}
	// The adapter keeps its own copy of the judging rules and does not copy partial scoring (ADR 0025). CreateRun
	// already refuses these jobs; a JE without scores is what FinishAttempt accepts for one.
	if job.Scoring != nil {
		return Result{Verdict: "JE", Total: len(job.Cases), CheckerLog: "部分点の問題はisolateワーカーで実行してください。"}
	}
	if job.Checker != nil && (job.Checker.Runtime != "cpp17" || !job.Checker.ValidJudgeProtocol() || strings.TrimSpace(job.Checker.Source) == "" || len(job.Checker.Source) > 65536 || !utf8.ValidString(job.Checker.Source) || strings.ContainsRune(job.Checker.Source, 0) || job.Generate || job.Validate) {
		return r
	}
	if len(job.Cases) == 0 || job.TimeLimitMS < 100 || job.TimeLimitMS > 5000 || job.TimeLimitMS%100 != 0 || len(job.Cases) > 100 || job.MemoryLimitMB < 64 || job.MemoryLimitMB > 1024 || !strings.HasPrefix(job.Image, "sha256:") {
		return r
	}
	for i, c := range job.Cases {
		name := c.Name
		if name == "" {
			name = fmt.Sprintf("ケース%d", i+1)
		}
		r.Cases = append(r.Cases, CaseResult{Name: name, Verdict: "SKIPPED"})
	}
	dir, err := os.MkdirTemp("", "openoj-source-")
	if err != nil {
		return r
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if os.Chmod(dir, 0755) != nil || os.WriteFile(filepath.Join(dir, "main.cpp"), []byte(source), 0644) != nil {
		return r
	}
	compiled := container(ctx, job.Image, dir, "", 1536, 30*time.Second, 32<<20,
		"sh", "-c", "g++ -std=c++17 -O2 -pipe /submission/main.cpp -o /tmp/main && cat /tmp/main")
	if compiled.err != nil {
		return r
	}
	if compiled.code != 0 || compiled.timedOut || compiled.oom || compiled.overflow {
		r.Verdict = "CE"
		r.CompileLog = strings.ToValidUTF8(string(compiled.diagnostic), "�")
		if compiled.timedOut {
			r.CompileLog = "コンパイルの制限時間を超えました。"
		}
		if compiled.oom {
			r.CompileLog = "コンパイルのメモリ上限を超えました。"
		}
		if compiled.overflow {
			r.CompileLog = "コンパイルの出力上限を超えました。"
		}
		return r
	}
	if len(compiled.output) == 0 {
		return r
	}
	if os.Remove(filepath.Join(dir, "main.cpp")) != nil || os.WriteFile(filepath.Join(dir, "main"), compiled.output, 0555) != nil {
		return r
	}
	checkerDir := ""
	checkerError := func(log string) Result {
		return Result{Verdict: "JE", Total: len(job.Cases), CheckerLog: checkerLog(r.CheckerLog, log)}
	}
	if job.Checker != nil {
		checkerDir, err = os.MkdirTemp("", "openoj-checker-")
		if err != nil {
			return r
		}
		defer func() { _ = os.RemoveAll(checkerDir) }()
		if os.Chmod(checkerDir, 0755) != nil || os.WriteFile(filepath.Join(checkerDir, "main.cpp"), []byte(job.Checker.Source), 0644) != nil {
			return r
		}
		checked := container(ctx, job.Image, checkerDir, "", 1536, 30*time.Second, 32<<20,
			"sh", "-c", "g++ -std=c++17 -O2 -pipe /submission/main.cpp -o /tmp/main && cat /tmp/main")
		if checked.err != nil || checked.code != 0 || checked.timedOut || checked.oom || checked.overflow || len(checked.output) == 0 {
			return checkerError("検証コードのコンパイル失敗\n" + string(checked.diagnostic))
		}
		if os.WriteFile(filepath.Join(checkerDir, "main"), checked.output, 0555) != nil || os.WriteFile(filepath.Join(checkerDir, "submission-source"), []byte(source), 0644) != nil {
			return checkerError("検証コードの準備失敗")
		}
	}
	r.Verdict = "AC"
	tleCount := 0
	generatedBytes := job.GenerationBaseBytes
	if generatedBytes < 0 || generatedBytes > GenerationOutputLimit {
		r.Verdict = "JE"
		return r
	}
	for i, c := range job.Cases {
		if ctx.Err() != nil {
			r.Verdict = "JE"
			return r
		}
		if c.InputFile != nil {
			if job.LoadFile == nil {
				r.Verdict = "JE"
				return r
			}
			c.Input, err = job.LoadFile(ctx, c.InputFile)
			if err != nil {
				r.Verdict = "JE"
				return r
			}
		}
		if c.OutputFile != nil && !job.Generate {
			if job.LoadFile == nil {
				r.Verdict = "JE"
				return r
			}
			c.Output, err = job.LoadFile(ctx, c.OutputFile)
			if err != nil {
				r.Verdict = "JE"
				return r
			}
		}
		// Cases saved before normalization may still be CRLF or lack the final newline.
		c.Input, c.Output = problems.NormalizeTestData(c.Input), problems.NormalizeTestData(c.Output)
		limit := time.Duration(job.TimeLimitMS) * time.Millisecond
		actual := container(ctx, job.Image, dir, c.Input, job.MemoryLimitMB, limit+10*time.Second, 16<<20,
			"timeout", "--signal=TERM", "--kill-after=0.1s", fmt.Sprintf("%.3fs", limit.Seconds()), "/submission/main")
		verdict := "AC"
		switch {
		case actual.err != nil || actual.timedOut:
			verdict = "JE"
		case actual.overflow:
			verdict = "OLE"
		case actual.oom:
			verdict = "MLE"
		case (actual.code == 124 || actual.code == 137) && actual.elapsed >= limit:
			verdict = "TLE"
		case actual.code != 0:
			verdict = "RE"
		case job.Generate && (generatedBytes+int64(len(actual.output)) > GenerationOutputLimit):
			verdict = "OLE"
		case job.Generate && (!utf8.Valid(actual.output) || bytes.ContainsRune(actual.output, 0)):
			verdict = "RE"
		case job.Checker == nil && !job.Generate && !job.Validate && !equalTokens(actual.output, []byte(c.Output)):
			verdict = "WA"
		}
		if job.Checker != nil && verdict == "AC" {
			if os.WriteFile(filepath.Join(checkerDir, "test-input"), []byte(c.Input), 0644) != nil || os.WriteFile(filepath.Join(checkerDir, "expected-output"), []byte(c.Output), 0644) != nil {
				return checkerError("検証データの準備失敗")
			}
			checked := container(ctx, job.Image, checkerDir, string(actual.output), 512, 15*time.Second, 16<<20,
				"sh", "-c", "touch /tmp/score && exec timeout --signal=TERM --kill-after=0.1s 5s /submission/main /submission/test-input /submission/expected-output /submission/submission-source /tmp/score")
			r.CheckerLog = checkerLog(r.CheckerLog, fmt.Sprintf("ケース%d: exit=%d\n%s\n", i+1, checked.code, checked.diagnostic))
			if checked.err != nil || checked.timedOut || checked.oom || checked.overflow || ((checked.code == 124 || checked.code == 137) && checked.elapsed >= 5*time.Second) {
				return checkerError("検証コードの実行失敗・制限超過")
			}
			if checked.code != 0 {
				verdict = "WA"
			}
		}
		if verdict == "AC" {
			r.Passed++
		}
		if job.Generate && verdict == "AC" {
			if len(actual.output) == 0 {
				value := ""
				r.Cases[i].Output = &value
			} else {
				if job.SaveOutput == nil {
					r.Verdict = "JE"
					return r
				}
				r.Cases[i].OutputFile, err = job.SaveOutput(ctx, actual.output)
				if err != nil {
					r.Verdict = "JE"
					return r
				}
			}
			generatedBytes += int64(len(actual.output))
		}
		if job.EasyTest {
			limit := min(4096, SamplePreviewBudget/len(job.Cases)/3)
			r.Cases[i].SampleDetails = &SampleDetails{
				Input:          samplePreview([]byte(c.Input), limit),
				ExpectedOutput: samplePreview([]byte(c.Output), limit),
				ActualOutput:   samplePreview(actual.output, limit),
			}
		}
		r.Cases[i].Verdict = verdict
		if verdict == "JE" {
			r.Verdict = "JE"
			return r
		}
		if r.Verdict == "AC" && verdict != "AC" {
			r.Verdict = verdict
		}
		if verdict == "TLE" {
			tleCount++
		}
		if tleCount >= 2 && !job.EasyTest && !job.Validate && !job.Generate {
			break
		}
	}
	return r
}

func checkerLog(previous, message string) string {
	log := strings.ToValidUTF8(previous+message, "�")
	if len(log) > 16384 {
		log = strings.ToValidUTF8(log[:16384], "")
	}
	return log
}

func equalTokens(a, b []byte) bool {
	space := func(r rune) bool { return r == ' ' || (r >= '\t' && r <= '\r') }
	x, y := bytes.FieldsFunc(a, space), bytes.FieldsFunc(b, space)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if !bytes.Equal(x[i], y[i]) {
			return false
		}
	}
	return true
}

func samplePreview(data []byte, limit int) TextPreview {
	text := strings.ReplaceAll(strings.ToValidUTF8(string(data), "�"), "\x00", "�")
	truncated := len(text) > limit
	if truncated {
		text = strings.ToValidUTF8(text[:limit], "")
	}
	return TextPreview{Text: text, Truncated: truncated}
}
