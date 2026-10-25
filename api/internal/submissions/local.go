package submissions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"judge/api/internal/problems"
)

// Judge uses the production rules; only sandbox execution and file I/O differ locally.
func Judge(ctx context.Context, source string, job Job) Result {
	failed := Result{Verdict: "JE", Total: len(job.Cases)}
	if !strings.HasPrefix(job.Image, "sha256:") || len(job.Image) != 71 {
		return failed
	}
	raw, _ := json.Marshal(job)
	var initial map[string]any
	_ = json.Unmarshal(raw, &initial)
	initial["source"], initial["runtime"], initial["runtimeDigest"] = source, job.Runtime, job.Image
	value, message := serveLocal(ctx, job.Image, initial, job.LoadFile, job.SaveOutput)
	if value == nil {
		failed.CheckerLog = message
		return failed
	}
	var result Result
	if json.Unmarshal(value, &result) != nil {
		return failed
	}
	if result.Total == 0 {
		result.Total = len(job.Cases)
	}
	return result
}

// serveLocal runs judge/local.py on one job and answers its RPC calls until it prints the result.
// It returns the raw result value, or nil and a message (possibly empty) when the run failed.
func serveLocal(ctx context.Context, image string, initial map[string]any,
	loadFile func(context.Context, *problems.TestFile) (string, error),
	saveOutput func(context.Context, []byte) (*problems.TestFile, error)) (json.RawMessage, string) {
	script := os.Getenv("JUDGE_LOCAL_RUNNER")
	if script == "" {
		cwd, _ := os.Getwd()
		for dir := cwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			candidate := filepath.Join(dir, "judge", "local.py")
			if _, err := os.Stat(candidate); err == nil {
				script = candidate
				break
			}
		}
	}
	if script == "" {
		return nil, "ローカル採点スクリプトが見つかりません。"
	}
	dir, err := os.MkdirTemp("", "openoj-local-")
	if err != nil {
		return nil, ""
	}
	defer os.RemoveAll(dir)
	// Docker runs as nobody and must traverse the controller-owned staging directory.
	if os.Chmod(dir, 0755) != nil {
		return nil, ""
	}
	cmd := exec.CommandContext(ctx, "python3", script)
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, ""
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, ""
	}
	if err = cmd.Start(); err != nil {
		return nil, err.Error()
	}
	defer func() { input.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	encoder, decoder := json.NewEncoder(input), json.NewDecoder(output)
	if err = encoder.Encode(initial); err != nil {
		return nil, ""
	}
	for {
		var call struct {
			Action  string `json:"action"`
			Request struct {
				Source, Runtime, Input, Protocol string
				TimeLimitMS, MemoryLimitMB       int
			} `json:"request"`
			Compile      bool               `json:"compile"`
			Artifact     string             `json:"artifact"`
			CheckerFiles map[string][]byte  `json:"checkerFiles"`
			ScoreFile    bool               `json:"scoreFile"`
			File         *problems.TestFile `json:"file"`
			Data         []byte             `json:"data"`
			Value        json.RawMessage    `json:"value"`
		}
		if err = decoder.Decode(&call); err != nil {
			return nil, "ローカル採点処理との通信に失敗しました。"
		}
		var value any
		switch call.Action {
		case "result":
			input.Close()
			if err = cmd.Wait(); err != nil || call.Value == nil {
				return nil, ""
			}
			return call.Value, ""
		case "load":
			if loadFile == nil {
				err = fmt.Errorf("テストファイルの保存先が未設定です。")
			} else {
				value, err = loadFile(ctx, call.File)
			}
		case "save":
			if saveOutput == nil {
				err = fmt.Errorf("生成結果の保存先が未設定です。")
			} else {
				value, err = saveOutput(ctx, call.Data)
			}
		case "execute":
			req := call.Request
			runtime := strings.TrimSuffix(req.Runtime, "-isolate") + "-local"
			filename, compile, run := localCommands(runtime)
			if filename == "" || (call.Artifact != "main" && call.Artifact != "checker" && call.Artifact != "aggregator") {
				return nil, ""
			}
			program := filepath.Join(dir, call.Artifact)
			if call.Compile {
				if err = os.MkdirAll(program, 0755); err == nil {
					err = os.WriteFile(filepath.Join(program, filename), []byte(req.Source), 0644)
				}
				if err != nil {
					return nil, ""
				}
				header := filepath.Join(filepath.Dir(script), ".build", "extension-inputs", "testlib.h")
				if data, readErr := os.ReadFile(header); readErr == nil {
					if os.MkdirAll(filepath.Join(program, "include"), 0755) != nil || os.WriteFile(filepath.Join(program, "include", "testlib.h"), data, 0644) != nil {
						return nil, ""
					}
				}
				result := container(ctx, image, program, "", 1536, 30*time.Second, 32<<20, "sh", "-c", compile)
				if result.err != nil {
					err = result.err
					break
				}
				ok := result.code == 0 && !result.timedOut && !result.oom && !result.overflow && len(result.output) > 0
				log := strings.ToValidUTF8(string(result.diagnostic), "�")
				if result.timedOut {
					log = "コンパイルの制限時間を超えました。"
				}
				if result.oom {
					log = "コンパイルのメモリ上限を超えました。"
				}
				if result.overflow {
					log = "コンパイルの出力上限を超えました。"
				}
				value = map[string]any{"compiled": ok, "compileLog": log}
				if ok {
					err = os.WriteFile(filepath.Join(program, "main"), result.output, 0555)
				}
				if err == nil {
					err = os.Remove(filepath.Join(program, filename))
				}
				break
			}
			data, decodeErr := base64.StdEncoding.DecodeString(req.Input)
			if decodeErr != nil {
				return nil, ""
			}
			for name, content := range call.CheckerFiles {
				if name != "test-input" && name != "expected-output" && name != "submission-source" {
					return nil, ""
				}
				if err = os.WriteFile(filepath.Join(program, name), content, 0644); err != nil {
					return nil, ""
				}
			}
			limit := time.Duration(req.TimeLimitMS) * time.Millisecond
			command := run
			if len(call.CheckerFiles) > 0 {
				args := []string{"/submission/test-input", "/submission/expected-output", "/submission/submission-source", "/tmp/score"}
				if req.Protocol == "testlib" {
					args = []string{"/submission/test-input", "/tmp/actual", "/submission/expected-output", "/tmp/result", "-appes"}
				}
				command = append([]string{"python3", "-I", "-c", localChecker, req.Protocol}, append(run, args...)...)
			}
			command = append([]string{"timeout", "--signal=TERM", "--kill-after=0.1s", fmt.Sprintf("%.3fs", limit.Seconds())}, command...)
			result := container(ctx, image, program, string(data), req.MemoryLimitMB, limit+10*time.Second, 16<<20, command...)
			if result.err != nil || result.timedOut {
				err = fmt.Errorf("Dockerでの実行に失敗しました。")
				break
			}
			status := ""
			if (result.code == 124 || result.code == 137) && result.elapsed >= limit {
				status = "TO"
			}
			metrics := map[string]any{"output": base64.StdEncoding.EncodeToString(result.output), "exitCode": result.code, "signal": 0, "status": status, "oom": result.oom, "overflow": result.overflow,
				"cpuTimeMs": 0, "wallTimeMs": result.elapsed.Seconds() * 1000, "memoryBytes": 0, "checkerLog": strings.ToValidUTF8(string(result.diagnostic), "�")}
			if len(call.CheckerFiles) > 0 && status == "" && !result.oom && !result.overflow && result.code == 0 {
				var checked struct {
					ExitCode  int     `json:"exitCode"`
					ScoreFile *string `json:"scoreFile"`
					Overflow  bool    `json:"overflow"`
				}
				if err = json.Unmarshal(result.output, &checked); err != nil {
					break
				}
				metrics["exitCode"], metrics["output"], metrics["overflow"] = checked.ExitCode, "", checked.Overflow
				if checked.ExitCode < 0 {
					metrics["signal"] = -checked.ExitCode
					metrics["exitCode"] = 0
				}
				if call.ScoreFile {
					metrics["scoreFile"] = checked.ScoreFile
				}
			} else if call.ScoreFile {
				metrics["scoreFile"] = nil
			}
			value = metrics
		default:
			return nil, ""
		}
		reply := map[string]any{"value": value}
		if err != nil {
			reply["error"] = err.Error()
			err = nil
		}
		if encoder.Encode(reply) != nil {
			return nil, ""
		}
	}
}

// Runs inside the checker container; score files never become writable host mounts.
const localChecker = `import base64,json,os,stat,subprocess,sys
protocol=sys.argv[1]
if protocol=='testlib':
    with open('/tmp/actual','wb') as f: f.write(sys.stdin.buffer.read())
else:
    open('/tmp/score','wb').close()
with open('/tmp/checker-stdout','wb') as out:
    code=subprocess.run(sys.argv[2:],stdout=out).returncode
limit=262144 if protocol=='testlib' else 4096
score=None
try:
    fd=os.open('/tmp/result' if protocol=='testlib' else '/tmp/score',os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
    with os.fdopen(fd,'rb') as f:
        if stat.S_ISREG(os.fstat(f.fileno()).st_mode):
            data=f.read(limit+1)
            if len(data)<=limit: score=base64.b64encode(data).decode()
except OSError: pass
print(json.dumps(dict(exitCode=code,scoreFile=score,overflow=os.path.getsize("/tmp/checker-stdout")>16777216)))
`
