package httpapi

import (
	"strconv"
	"strings"
	"testing"

	"judge/api/internal/submissions"
)

func TestCodeRunsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	store := &submissions.Store{Pool: f.store.Pool()}
	f.fail("GET", "/my/code-runs/latest", "alice", nil, 404, "code_run_not_found")
	// Invalid input is refused before the limit counts it.
	for _, body := range []map[string]any{
		{"runtime": "cpp17", "source": "int main(){}", "stdin": strings.Repeat("a", 1<<20+1)},
		{"runtime": "cpp17", "source": "int main(){}", "stdin": "a\x00"},
		{"runtime": "cpp17", "source": "   ", "stdin": ""},
		{"runtime": "brainfuck", "source": "int main(){}", "stdin": ""},
	} {
		f.fail("POST", "/my/code-runs", "alice", body, 400, "invalid_code_run")
	}
	var first submissions.CodeRun
	f.decode(f.request("POST", "/my/code-runs", "alice", map[string]any{"runtime": "cpp17", "source": "int main(){}", "stdin": "1 2\r\n3\r"}, 202), &first)
	if first.Status != "QUEUED" || first.Result != nil {
		t.Fatalf("%+v", first)
	}
	if f.count(`SELECT count(*) FROM code_runs WHERE id=$1 AND owner_id='alice' AND runtime='cpp17-isolate' AND stdin=$2 AND image=$3`, first.ID, "1 2\n3\n", "sha256:"+strings.Repeat("a", 64)) != 1 {
		t.Fatal("run not stored as expected")
	}
	// Exactly the input limit is accepted.
	f.request("POST", "/my/code-runs", "alice", map[string]any{"runtime": "cpp17", "source": "int main(){}", "stdin": strings.Repeat("a", 1<<20)}, 202)
	var third submissions.CodeRun
	f.decode(f.request("POST", "/my/code-runs", "alice", map[string]any{"runtime": "cpp17", "source": "int main(){}", "stdin": ""}, 202), &third)
	// The fourth within 60 seconds is limited.
	w := f.do(f.newRequest("POST", "/my/code-runs", "alice", map[string]any{"runtime": "cpp17", "source": "int main(){}", "stdin": ""}))
	retry, _ := strconv.Atoi(w.Header().Get("Retry-After"))
	if w.Code != 429 || !strings.Contains(w.Body.String(), "code_run_rate_limited") || retry < 1 || retry > 60 {
		t.Fatalf("%d %q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
	}
	// Code runs stay out of the submission list and do not use the submission limit.
	if body := f.request("GET", "/my/submissions", "alice", nil, 200); !strings.Contains(body, `"items":[]`) {
		t.Fatal(body)
	}
	// The latest is the owner's newest; another owner sees none.
	var latest submissions.CodeRun
	f.decode(f.request("GET", "/my/code-runs/latest", "alice", nil, 200), &latest)
	if latest.ID != third.ID {
		t.Fatalf("latest %s want %s", latest.ID, third.ID)
	}
	f.fail("GET", "/my/code-runs/latest", "bob", nil, 404, "code_run_not_found")
	// An older attempt does nothing; the current one completes once.
	var attempt string
	if err := f.store.Pool().QueryRow(f.ctx, `SELECT judge_attempt::text FROM code_runs WHERE id=$1`, first.ID).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishCodeRun(f.ctx, first.ID, newSubmissionID(), submissions.CodeRunResult{Verdict: "AC"}); err != nil {
		t.Fatal(err)
	}
	if f.count(`SELECT count(*) FROM code_runs WHERE id=$1 AND status='QUEUED' AND result IS NULL`, first.ID) != 1 {
		t.Fatal("stale attempt changed the run")
	}
	if err := store.FinishCodeRun(f.ctx, first.ID, attempt, submissions.CodeRunResult{Verdict: "AC"}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishCodeRun(f.ctx, first.ID, attempt, submissions.CodeRunResult{Verdict: "RE"}); err != nil {
		t.Fatal(err)
	}
	if f.count(`SELECT count(*) FROM code_runs WHERE id=$1 AND status='DONE' AND result->>'verdict'='AC'`, first.ID) != 1 {
		t.Fatal("run not finished once with AC")
	}
}
