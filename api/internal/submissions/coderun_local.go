package submissions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// RunCodeLocally runs a code run through the production host.run_code with the local Docker sandbox.
func RunCodeLocally(ctx context.Context, job CodeRunJob) CodeRunResult {
	failed := CodeRunResult{Verdict: "JE"}
	if !strings.HasPrefix(job.Image, "sha256:") || len(job.Image) != 71 {
		return failed
	}
	value, _ := serveLocal(ctx, job.Image, map[string]any{
		"codeRun": true, "runtime": job.Runtime, "runtimeDigest": job.Image, "source": job.Source,
		"stdin": base64.StdEncoding.EncodeToString([]byte(job.Stdin)), "timeLimitMs": CodeRunTimeLimitMS, "memoryLimitMb": CodeRunMemoryLimitMB,
	}, nil, nil)
	var result CodeRunResult
	if value == nil || json.Unmarshal(value, &result) != nil || result.Verdict == "" {
		return failed
	}
	return result
}
