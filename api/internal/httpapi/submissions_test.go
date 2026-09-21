package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

func TestDraftTestCaseLimits(t *testing.T) {
	draft := problems.Draft{TimeLimitMS: "2000", MemoryLimitMB: "512"}
	file := &problems.TestFile{ID: "11111111-1111-4111-8111-111111111111", Size: 16 << 20, SHA256: strings.Repeat("a", 64)}
	atSetLimit := make([]problems.TestCase, 16)
	for i := range atSetLimit {
		atSetLimit[i] = problems.TestCase{InputFile: file, OutputFile: file}
	}
	for _, tc := range []struct {
		name  string
		cases []problems.TestCase
		valid bool
	}{
		{"legacy", nil, true},
		{"empty input and output", []problems.TestCase{{}}, true},
		{"at case limit", make([]problems.TestCase, 100), true},
		{"over case limit", make([]problems.TestCase, 101), false},
		{"byte limit", []problems.TestCase{{Input: strings.Repeat("あ", 22000)}}, false},
		{"16 MiB file", []problems.TestCase{{InputFile: &problems.TestFile{ID: "11111111-1111-4111-8111-111111111111", Size: 16 << 20, SHA256: strings.Repeat("a", 64)}}}, true},
		{"over 16 MiB file", []problems.TestCase{{InputFile: &problems.TestFile{ID: "11111111-1111-4111-8111-111111111111", Size: 16<<20 + 1, SHA256: strings.Repeat("a", 64)}}}, false},
		{"file and inline data", []problems.TestCase{{Input: "x", InputFile: &problems.TestFile{ID: "11111111-1111-4111-8111-111111111111", Size: 1, SHA256: strings.Repeat("a", 64)}}}, false},
		{"at test set limit", atSetLimit, true},
		{"over test set limit", append(atSetLimit, problems.TestCase{Input: "x"}), false},
		{"NUL", []problems.TestCase{{Output: "\x00"}}, false},
		{"named case", []problems.TestCase{{Name: "最大値のケース"}}, true},
		{"long name", []problems.TestCase{{Name: strings.Repeat("あ", 65)}}, false},
		{"duplicate name", []problems.TestCase{{Name: "sample"}, {Name: " sample "}}, false},
		{"blank name", []problems.TestCase{{Name: "   "}}, false},
		{"control in name", []problems.TestCase{{Name: "a\nb"}}, false},
		{"total limit", []problems.TestCase{{Input: strings.Repeat("x", 65536), Output: strings.Repeat("x", 65536)}, {Input: strings.Repeat("x", 65536), Output: strings.Repeat("x", 65536)}, {Input: "x"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft.TestCases = tc.cases
			if validDraft(draft) != tc.valid {
				t.Fatal("unexpected validation result")
			}
		})
	}
}

func TestDraftMemoryLimit(t *testing.T) {
	for memory, valid := range map[string]bool{"63": false, "64": true, "512": true, "513": false, "1024": false, "invalid": false} {
		draft := problems.Draft{TimeLimitMS: "2000", MemoryLimitMB: memory}
		if actual := validDraft(draft); actual != valid {
			t.Errorf("memory %s: got valid=%v, want %v", memory, actual, valid)
		}
	}
}

func TestDraftEditorialLimit(t *testing.T) {
	draft := problems.Draft{TimeLimitMS: "2000", MemoryLimitMB: "512", Editorial: strings.Repeat("あ", 100000)}
	if !validDraft(draft) {
		t.Fatal("editorial at character limit should be valid")
	}
	draft.Editorial += "あ"
	if validDraft(draft) {
		t.Fatal("editorial over character limit should be invalid")
	}
	draft.Editorial = "解説\x00"
	if validDraft(draft) {
		t.Fatal("editorial containing NUL should be invalid")
	}
}

func TestDraftCaseLimitIndependentOfTL(t *testing.T) {
	for _, tl := range []int{100, 1000, 2000, 2300, 5000} {
		for _, extra := range []int{0, 1} {
			count := 100 + extra
			t.Run(fmt.Sprintf("%dms/%dcases", tl, count), func(t *testing.T) {
				draft := problems.Draft{TimeLimitMS: fmt.Sprint(tl), MemoryLimitMB: "512", TestCases: make([]problems.TestCase, count)}
				if validDraft(draft) != (extra == 0) {
					t.Fatal("unexpected case limit validation")
				}
			})
		}
	}
}

func TestSubmissionsPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	schema := fmt.Sprintf("test_submissions_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) }()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool().Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob')`); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	_, err = store.Save(ctx, "alice", id, 0, problems.Draft{Title: "A+B", Markdown: "Add", TimeLimitMS: "2000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "1 2", Output: "3", IsSample: true}}})
	if err != nil {
		t.Fatal(err)
	}
	published, err := store.Publish(ctx, "alice", id, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	f := newSigningFixture(t)
	queue := &submissions.Store{Pool: store.Pool()}
	image := "sha256:" + strings.Repeat("a", 64)
	if configured := os.Getenv("TEST_JUDGE_CPP_IMAGE"); configured != "" {
		data, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", configured).Output()
		if err != nil {
			t.Fatal(err)
		}
		image = strings.TrimSpace(string(data))
	}
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: queue, JudgeImage: image, Verifier: newCognitoVerifier(f.server.URL, "client")})
	request := func(method, path, owner, body string, want int) string {
		t.Helper()
		// These fixture scenarios represent independent judging sessions.
		if method == "POST" && path == "/my/submissions" {
			if _, err := store.Pool().Exec(ctx, `UPDATE submissions SET created_at=clock_timestamp()-interval '91 seconds' WHERE created_at>clock_timestamp()-interval '90 seconds'`); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if owner != "" {
			r.Header.Set("Authorization", "Bearer "+f.token(t, owner, nil))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response cacheable")
		}
		return w.Body.String()
	}
	t.Run("account rate limit", func(t *testing.T) {
		body := `{"problemId":"` + id + `","runtime":"cpp17","source":"source"}`
		token := f.token(t, "alice", nil)
		codes := make(chan int, 8)
		for i := 0; i < 8; i++ {
			go func() {
				r := httptest.NewRequest("POST", "/my/submissions", strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code == 429 && (w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "submission_rate_limited")) {
					codes <- 0
					return
				}
				codes <- w.Code
			}()
		}
		accepted, limited := 0, 0
		for i := 0; i < 8; i++ {
			switch code := <-codes; code {
			case 202:
				accepted++
			case 429:
				limited++
			default:
				t.Fatalf("unexpected status: %d", code)
			}
		}
		if accepted != 2 || limited != 6 {
			t.Fatalf("accepted=%d limited=%d", accepted, limited)
		}
		if _, err := queue.CreateRun(ctx, submissions.RunInput{Owner: "bob", ID: newSubmissionID(), ProblemID: id, Source: "source", Image: image, Runtime: "cpp17-local"}); err != nil {
			t.Fatal(err)
		}
		// Only one slot expires: accept one more, then reject generation too.
		if _, err := store.Pool().Exec(ctx, `UPDATE submissions SET created_at=clock_timestamp()-interval '91 seconds' WHERE id=(SELECT id FROM submissions WHERE owner_id='alice' ORDER BY created_at LIMIT 1)`); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.CreateRun(ctx, submissions.RunInput{Owner: "alice", ID: newSubmissionID(), ProblemID: id, Source: "source", Image: image, Runtime: "cpp17-local"}); err != nil {
			t.Fatal(err)
		}
		_, err := queue.CreateGeneration(ctx, submissions.GenerationInput{RunInput: submissions.RunInput{Owner: "alice", ID: newSubmissionID(), ProblemID: id, Source: "source", Runtime: "cpp17-local"}, ProblemVersion: 1, Job: submissions.Job{Generate: true}})
		var limitedErr *submissions.RateLimitError
		if !errors.As(err, &limitedErr) {
			t.Fatalf("generation bypassed shared limit: %v", err)
		}
		// A full normal quota leaves all three sample slots available.
		for i := 0; i < 4; i++ {
			_, err := queue.CreateRun(ctx, submissions.RunInput{Owner: "alice", ID: newSubmissionID(), ProblemID: id, Source: "source", Image: image, Runtime: "cpp17-local", EasyTest: true})
			if i < 3 && err != nil {
				t.Fatal(err)
			}
			if i == 3 && !errors.As(err, &limitedErr) {
				t.Fatalf("sample quota: %v", err)
			}
		}
		// At 61 seconds sample slots reopen, but the normal quota remains full.
		if _, err := store.Pool().Exec(ctx, `UPDATE submissions SET created_at=clock_timestamp()-interval '61 seconds' WHERE owner_id='alice'`); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.CreateRun(ctx, submissions.RunInput{Owner: "alice", ID: newSubmissionID(), ProblemID: id, Source: "source", Image: image, Runtime: "cpp17-local", EasyTest: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.CreateRun(ctx, submissions.RunInput{Owner: "alice", ID: newSubmissionID(), ProblemID: id, Source: "source", Image: image, Runtime: "cpp17-local"}); !errors.As(err, &limitedErr) || limitedErr.RetryAfter < 1 || limitedErr.RetryAfter > 30 {
			t.Fatalf("normal quota expired too early: %v", err)
		}
		if _, err := store.Pool().Exec(ctx, `DELETE FROM submissions`); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := store.Pool().Exec(ctx, `UPDATE problem_drafts SET draft=jsonb_set(draft,'{testCases,0,isSample}','false'),published_draft=jsonb_set(published_draft,'{testCases,0,isSample}','false') WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// Easy Test uses explicit flags regardless of names, preserves order, and never enters submission history.
	const easyProblem = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	_, err = store.Save(ctx, "alice", easyProblem, 0, problems.Draft{Title: "Easy", TimeLimitMS: "2000", MemoryLimitMB: "512", TestCases: []problems.TestCase{
		{Name: "example.txt", IsSample: true, Input: "2", Output: "2"}, {Name: "sampleX1"}, {Name: "hidden"}, {Name: "sample_1", IsSample: true, Input: "1", Output: "1"}, {Name: "sample_unchecked"}, {},
	}})
	if err != nil {
		t.Fatal(err)
	}
	easyBody := `{"problemId":"` + easyProblem + `","runtime":"cpp17","source":"source","easyTest":true}`
	request("POST", "/my/submissions", "bob", easyBody, 409)
	request("POST", "/my/submissions", "alice", strings.TrimSuffix(easyBody, "}")+`,"generation":{"mode":"input","start":1,"count":1}}`, 400)
	request("POST", "/my/submissions", "alice", strings.Replace(easyBody, easyProblem, id, 1), 409)
	var easy submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", easyBody, 202)), &easy); err != nil || !easy.EasyTest {
		t.Fatalf("easy test: %+v %v", easy, err)
	}
	var rawJob []byte
	if err := store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, easy.ID).Scan(&rawJob); err != nil {
		t.Fatal(err)
	}
	var easyJob submissions.Job
	if err := json.Unmarshal(rawJob, &easyJob); err != nil || !easyJob.EasyTest || len(easyJob.Cases) != 2 || easyJob.Cases[0].Name != "example.txt" || easyJob.Cases[1].Name != "sample_1" {
		t.Fatalf("wrong sample selection: %s %v", rawJob, err)
	}
	if got := request("GET", "/my/submissions", "alice", "", 200); strings.Contains(got, easy.ID) {
		t.Fatal("easy test in submission history")
	}
	request("GET", "/my/submissions/"+easy.ID, "alice", "", 200)
	request("GET", "/my/submissions/"+easy.ID, "bob", "", 404)
	// Remove this queued test so the existing worker assertions below retain their ordering.
	if _, err := store.Pool().Exec(ctx, `DELETE FROM submissions WHERE id=$1`, easy.ID); err != nil {
		t.Fatal(err)
	}
	body := `{"problemId":"` + id + `","runtime":"cpp17-local","source":"#include <cstdio>\nint main(){puts(\"3\");}"}`
	request("POST", "/my/submissions", "", body, 401)
	request("POST", "/my/submissions", "alice", strings.Replace(body, "cpp17-local", "python", 1), 400)
	request("POST", "/my/submissions", "alice", strings.TrimSuffix(body, "}")+`,"owner_id":"bob"}`, 400)
	var item submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", body, 202)), &item); err != nil {
		t.Fatal(err)
	}
	request("GET", "/my/submissions/"+item.ID, "bob", "", 404)
	if got := request("GET", "/my/submissions", "bob", "", 200); strings.Contains(got, item.ID) {
		t.Fatal("owner leaked")
	}
	if got := request("GET", "/my/submissions", "alice", "", 200); strings.Contains(got, "int main") {
		t.Fatal("list leaked source")
	}
	// The same UI language selects a pinned cloud runtime, never the local worker.
	localHandler := h
	dispatches := 0
	h = newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: queue, JudgeImage: image, JudgeRuntime: "cpp17-isolate", Verifier: newCognitoVerifier(f.server.URL, "client"), DispatchJudge: func(ctx context.Context) error {
		dispatches++
		var count int
		if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM submissions WHERE runtime='cpp17-isolate' AND status='QUEUED'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("wake-up before durable commit: %d %v", count, err)
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("unbounded dispatch")
		}
		return fmt.Errorf("simulated dispatch outage")
	}})
	request("POST", "/my/submissions", "alice", body, 400)
	var cloud submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", strings.Replace(body, "cpp17-local", "cpp17", 1), 202)), &cloud); err != nil || cloud.Runtime != "cpp17-isolate" {
		t.Fatalf("cloud runtime not pinned: %+v %v", cloud, err)
	}
	request("POST", "/my/submissions", "alice", strings.Replace(body, "cpp17-local", "c23-gcc-isolate", 1), 400)
	if dispatches != 1 {
		t.Fatalf("dispatches=%d; accepted submission must wake once, rejected ones never", dispatches)
	}
	h = newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Submissions: queue, JudgeImage: image, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17,c23-gcc", Verifier: newCognitoVerifier(f.server.URL, "client")})
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", strings.Replace(body, "cpp17-local", "c23-gcc", 1), 202)), &cloud); err != nil || cloud.Runtime != "c23-gcc-isolate" {
		t.Fatalf("C runtime not pinned: %+v %v", cloud, err)
	}
	request("POST", "/my/submissions", "alice", strings.Replace(body, "cpp17-local", "java24", 1), 400)
	// Published memory limits must survive both normal and sample cloud submissions.
	if _, err := store.Pool().Exec(ctx, `UPDATE problem_drafts SET published_draft=jsonb_set(published_draft,'{testCases,0,isSample}','true') WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	for _, memory := range []int{63, 513, 64, 315, 512} {
		if _, err := store.Pool().Exec(ctx, `UPDATE problem_drafts SET published_draft=jsonb_set(published_draft,'{memoryLimitMb}',to_jsonb($2::text)) WHERE id=$1`, id, fmt.Sprint(memory)); err != nil {
			t.Fatal(err)
		}
		for _, sample := range []bool{false, true} {
			input := fmt.Sprintf(`{"problemId":%q,"runtime":"c23-gcc","source":"int main(){}","easyTest":%t}`, id, sample)
			if memory < 64 || memory > 512 {
				request("POST", "/my/submissions", "alice", input, 409)
				continue
			}
			var accepted submissions.Submission
			if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", input, 202)), &accepted); err != nil {
				t.Fatal(err)
			}
			var pinnedMemory int
			if err := store.Pool().QueryRow(ctx, `SELECT (job->>'memoryLimitMb')::int FROM submissions WHERE id=$1`, accepted.ID).Scan(&pinnedMemory); err != nil || pinnedMemory != memory {
				t.Fatalf("memory limit was not pinned: got %d, want %d: %v", pinnedMemory, memory, err)
			}
		}
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE problem_drafts SET published_draft=jsonb_set(published_draft,'{testCases,0,isSample}','false') WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	h = localHandler
	// Draft changes must not affect published tests or accepted submissions.
	changed := published.Draft
	changed.TestCases = []problems.TestCase{{Input: "secret-input", Output: "999"}}
	updated, err := store.Save(ctx, "alice", id, published.Version, changed)
	if err != nil {
		t.Fatal(err)
	}
	public, err := store.PublicGet(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	publicJSON, _ := json.Marshal(public)
	if strings.Contains(string(publicJSON), "testCases") || strings.Contains(string(publicJSON), "secret-input") {
		t.Fatal("public API leaked tests")
	}
	claimed, job, err := queue.Claim(ctx)
	if err != nil || claimed.ID != item.ID || job.Cases[0].Output != "3" || job.TimeLimitMS != 2000 {
		t.Fatalf("claim: %+v %+v %v", claimed, job, err)
	}
	if _, _, err = queue.Claim(ctx); err != pgx.ErrNoRows {
		t.Fatal("job claimed twice", err)
	}
	result := submissions.Result{Verdict: "AC", Passed: 1, Total: 1}
	if os.Getenv("TEST_JUDGE_CPP_IMAGE") != "" {
		result = submissions.Judge(ctx, claimed.Source, job)
		if result.Verdict != "AC" {
			t.Fatalf("real C++ submission: %+v", result)
		}
	}
	if err := queue.Finish(ctx, item.ID, result); err != nil {
		t.Fatal(err)
	}
	if err := queue.Finish(ctx, item.ID, submissions.Result{Verdict: "WA"}); err != nil {
		t.Fatal(err)
	}
	got := request("GET", "/my/submissions/"+item.ID, "alice", "", 200)
	if !strings.Contains(got, `"verdict":"AC"`) || strings.Contains(got, `"cases"`) {
		t.Fatal(got)
	}
	// Submitting before publication still uses the old cases.
	var before submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", body, 202)), &before); err != nil {
		t.Fatal(err)
	}
	_, oldJob, err := queue.Claim(ctx)
	if err != nil || oldJob.Cases[0].Output != "3" {
		t.Fatalf("draft leaked into judging: %+v %v", oldJob, err)
	}
	if _, err := store.Publish(ctx, "alice", id, updated.Version, true); err != nil {
		t.Fatal(err)
	}
	request("POST", "/my/submissions", "alice", body, 202)
	_, newJob, err := queue.Claim(ctx)
	if err != nil || newJob.Cases[0].Output != "999" {
		t.Fatalf("published tests not used: %+v %v", newJob, err)
	}
	current, err := store.Get(ctx, "alice", id)
	if err != nil {
		t.Fatal(err)
	}

	// A published set over the case limit cannot enqueue a job.
	for _, tc := range []struct{ tl, count, status int }{{100, 101, 409}, {5000, 100, 202}} {
		data, err := json.Marshal(problems.Draft{Title: "Budget", TimeLimitMS: fmt.Sprint(tc.tl), MemoryLimitMB: "512", TestCases: make([]problems.TestCase, tc.count)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Pool().Exec(ctx, `UPDATE problem_drafts SET published_draft=$2 WHERE id=$1`, id, data); err != nil {
			t.Fatal(err)
		}
		request("POST", "/my/submissions", "alice", body, tc.status)
	}
	current.Draft.TestCases = nil
	saved, err := store.Save(ctx, "alice", id, current.Version, current.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, "alice", id, saved.Version, true); err != nil {
		t.Fatal(err)
	}
	request("POST", "/my/submissions", "alice", body, 409)

	// Generation works without published tests and remains scoped to the draft owner.
	generation := strings.TrimSuffix(body, "}") + `,"generation":{"mode":"input","start":7,"count":2}}`
	request("POST", "/my/submissions", "bob", generation, 404)
	for _, invalid := range []string{
		strings.Replace(generation, `"count":2`, `"count":0`, 1),
		strings.Replace(generation, `"start":7`, `"start":2147483647`, 1),
		strings.Replace(generation, `"mode":"input"`, `"mode":"output"`, 1),
	} {
		request("POST", "/my/submissions", "alice", invalid, 400)
	}
	var generated submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", generation, 202)), &generated); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, generated.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var generationJob submissions.Job
	if json.Unmarshal(raw, &generationJob) != nil || !generationJob.Generate || len(generationJob.Cases) != 2 || generationJob.Cases[0].Input != "7\n" || generationJob.Cases[1].Input != "8\n" {
		t.Fatalf("generation job: %s", raw)
	}
	if got := request("GET", "/my/submissions", "alice", "", 200); strings.Contains(got, generated.ID) {
		t.Fatal("generation appeared in submission history")
	}
	request("GET", "/my/submissions/"+generated.ID, "bob", "", 404)
	current, err = store.Get(ctx, "alice", id)
	if err != nil {
		t.Fatal(err)
	}
	current.Draft.TestCases = []problems.TestCase{{Input: " 1  2\n", Output: "secret"}}
	if _, err = store.Save(ctx, "alice", id, current.Version, current.Draft); err != nil {
		t.Fatal(err)
	}
	generation = strings.Replace(generation, `"mode":"input","start":7,"count":2`, `"mode":"output","start":1,"count":1`, 1)
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", generation, 202)), &generated); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, generated.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &generationJob) != nil || generationJob.Cases[0].Input != " 1  2\n" || generationJob.Cases[0].Output != "" {
		t.Fatalf("output generation job: %s", raw)
	}

	validation := strings.Replace(generation, `"mode":"output"`, `"mode":"validation"`, 1)
	request("POST", "/my/submissions", "bob", validation, 404)
	request("POST", "/my/submissions", "alice", strings.Replace(validation, `"start":1`, `"start":2`, 1), 400)
	var validated submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", validation, 202)), &validated); err != nil {
		t.Fatal(err)
	}
	var validationRaw []byte
	if err := store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, validated.ID).Scan(&validationRaw); err != nil {
		t.Fatal(err)
	}
	var validationJob submissions.Job
	if json.Unmarshal(validationRaw, &validationJob) != nil || !validationJob.Validate || validationJob.Generate || len(validationJob.Cases) != 1 || validationJob.Cases[0].Input != " 1  2\n" || validationJob.Cases[0].Output != "" || validationJob.Cases[0].OutputFile != nil {
		t.Fatalf("validation job: %s", validationRaw)
	}
	if strings.Contains(request("GET", "/my/submissions", "alice", "", 200), validated.ID) {
		t.Fatal("validation appeared in submission history")
	}
	if generationJob.GenerationBaseBytes != int64(len(" 1  2\n")) {
		t.Fatalf("replaced output still counted: %+v", generationJob)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE submissions SET status='RUNNING' WHERE id=$1`, generated.ID); err != nil {
		t.Fatal(err)
	}
	file := problems.TestFile{ID: "44444444-4444-4444-8444-444444444444", Size: 16 << 20, SHA256: strings.Repeat("a", 64), Key: testfiles.GenerationPrefix("bob", id) + "44444444-4444-4444-8444-444444444444", Version: "generated-version"}
	generatedResult := submissions.Result{Verdict: "AC", Passed: 1, Total: 1, Cases: []submissions.CaseResult{{Verdict: "AC", OutputFile: &file}}}
	if err = queue.Finish(ctx, generated.ID, generatedResult); err == nil {
		t.Fatal("cross-owner generated file registered")
	}
	file.Key = testfiles.GenerationPrefix("alice", id) + file.ID
	// A result cannot bypass the complete set budget even if the worker is wrong.
	if _, err = store.Pool().Exec(ctx, `UPDATE submissions SET job=jsonb_set(job,'{generationBaseBytes}',to_jsonb($2::bigint)) WHERE id=$1`, generated.ID, int64(submissions.GenerationOutputLimit)-(16<<20)+1); err != nil {
		t.Fatal(err)
	}
	if err = queue.Finish(ctx, generated.ID, generatedResult); err == nil {
		t.Fatal("over-budget generated file registered")
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE submissions SET job=jsonb_set(job,'{generationBaseBytes}',to_jsonb($2::bigint)) WHERE id=$1`, generated.ID, int64(submissions.GenerationOutputLimit)-(16<<20)); err != nil {
		t.Fatal(err)
	}
	if err = queue.Finish(ctx, generated.ID, generatedResult); err != nil {
		t.Fatal(err)
	}
	if err = queue.Finish(ctx, generated.ID, generatedResult); err != nil {
		t.Fatal("duplicate result not ignored", err)
	}
	got = request("GET", "/my/submissions/"+generated.ID, "alice", "", 200)
	if strings.Contains(got, "generated-version") || strings.Contains(got, "test-files/") || !strings.Contains(got, `"size":16777216`) {
		t.Fatal("invalid generated reference response", got)
	}
	var pinned string
	var ready bool
	if err = store.Pool().QueryRow(ctx, `SELECT upload_version_id,ready FROM test_files WHERE id=$1 AND owner_id='alice'`, file.ID).Scan(&pinned, &ready); err != nil || pinned != "generated-version" || ready {
		t.Fatal("generated file skipped validation", pinned, ready, err)
	}

	// Unpublished problems are judgeable only by their owner, with immutable inputs.
	const privateID = "55555555-5555-4555-8555-555555555555"
	privateDraft := problems.Draft{Title: "Private", Markdown: "Private statement", TimeLimitMS: "1000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "private-input", Output: "private-output"}}}
	privateProblem, err := store.Save(ctx, "alice", privateID, 0, privateDraft)
	if err != nil {
		t.Fatal(err)
	}
	privateBody := strings.Replace(body, id, privateID, 1)
	request("GET", "/problems/"+privateID, "", "", 404)
	request("GET", "/my/problems/"+privateID, "bob", "", 404)
	request("GET", "/my/problems/"+privateID, "alice", "", 200)
	request("POST", "/my/submissions", "", privateBody, 401)
	request("POST", "/my/submissions", "bob", privateBody, 409)
	var privateItem submissions.Submission
	if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", privateBody, 202)), &privateItem); err != nil {
		t.Fatal(err)
	}
	if privateItem.ProblemVersion != privateProblem.Version {
		t.Fatal("draft version not pinned")
	}
	privateDraft.TestCases[0].Output = "changed"
	if _, err := store.Save(ctx, "alice", privateID, privateProblem.Version, privateDraft); err != nil {
		t.Fatal(err)
	}
	var privateRaw []byte
	if err := store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, privateItem.ID).Scan(&privateRaw); err != nil {
		t.Fatal(err)
	}
	var privateJob submissions.Job
	if json.Unmarshal(privateRaw, &privateJob) != nil || privateJob.Cases[0].Output != "private-output" || privateJob.TimeLimitMS != 1000 {
		t.Fatalf("private snapshot: %s", privateRaw)
	}
	currentPrivate, err := store.Get(ctx, "alice", privateID)
	if err != nil {
		t.Fatal(err)
	}
	currentPrivate.Draft.TestCases = nil
	if _, err := store.Save(ctx, "alice", privateID, currentPrivate.Version, currentPrivate.Draft); err != nil {
		t.Fatal(err)
	}
	request("POST", "/my/submissions", "alice", privateBody, 409)

	// Checker source and language are pinned with the selected problem version.
	const checkerID = "77777777-7777-4777-8777-777777777777"
	checkerDraft := problems.Draft{Title: "Checker", Markdown: "Construct", TimeLimitMS: "1000", MemoryLimitMB: "512", Checker: &problems.Generator{Runtime: "cpp17", Source: "checker-original"}, TestCases: []problems.TestCase{{Input: "10", Output: ""}}}
	cp, err := store.Save(ctx, "alice", checkerID, 0, checkerDraft)
	if err != nil {
		t.Fatal(err)
	}
	checkerBody := strings.Replace(body, id, checkerID, 1)
	var cs submissions.Submission
	if err = json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", checkerBody, 202)), &cs); err != nil {
		t.Fatal(err)
	}
	checkerDraft.Checker = &problems.Generator{Runtime: "python314", Source: "assert True"}
	cp, err = store.Save(ctx, "alice", checkerID, cp.Version, checkerDraft)
	if err != nil {
		t.Fatal(err)
	}
	var checkerRaw []byte
	if err = store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, cs.ID).Scan(&checkerRaw); err != nil {
		t.Fatal(err)
	}
	var pinnedChecker submissions.Job
	if json.Unmarshal(checkerRaw, &pinnedChecker) != nil || pinnedChecker.Checker == nil || pinnedChecker.Checker.Source != "checker-original" || pinnedChecker.Checker.Runtime != "cpp17" {
		t.Fatalf("checker changed: %s", checkerRaw)
	}
	request("POST", "/my/submissions", "alice", checkerBody, 409) // Python is not published locally.
	request("PUT", "/my/problems/"+checkerID+"/publication", "alice", fmt.Sprintf(`{"version":%d,"publish":true}`, cp.Version), 400)
	h = newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Submissions: queue, JudgeImage: image, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17,python314", Verifier: newCognitoVerifier(f.server.URL, "client")})
	checkerBody = strings.Replace(checkerBody, "cpp17-local", "cpp17", 1)
	request("POST", "/my/submissions", "alice", checkerBody, 202)
	cp, err = store.Publish(ctx, "alice", checkerID, cp.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	checkerDraft.Checker = nil
	if _, err = store.Save(ctx, "alice", checkerID, cp.Version, checkerDraft); err != nil {
		t.Fatal(err)
	}
	publicChecker := request("GET", "/problems/"+checkerID, "", "", 200)
	if !strings.Contains(publicChecker, `"specialJudge":true`) || strings.Contains(publicChecker, "assert True") {
		t.Fatal(publicChecker)
	}
	for _, submitter := range []string{"alice", "bob"} {
		if err = json.Unmarshal([]byte(request("POST", "/my/submissions", submitter, checkerBody, 202)), &cs); err != nil {
			t.Fatal(err)
		}
		if err = store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, cs.ID).Scan(&checkerRaw); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(checkerRaw), "assert True") {
			t.Fatal("published checker not pinned")
		}
		if _, err = store.Pool().Exec(ctx, `UPDATE submissions SET status='DONE',result='{"verdict":"WA","passed":0,"total":1,"checkerLog":"private-checker-diagnostic"}' WHERE id=$1`, cs.ID); err != nil {
			t.Fatal(err)
		}
		detail := request("GET", "/my/submissions/"+cs.ID, submitter, "", 200)
		if strings.Contains(detail, "private-checker-diagnostic") != (submitter == "alice") {
			t.Fatal("checker diagnostic authorization", detail)
		}
		if strings.Contains(request("GET", "/my/submissions", submitter, "", 200), "private-checker-diagnostic") {
			t.Fatal("checker diagnostic leaked through list")
		}
	}

	const interactiveID = "88888888-8888-4888-8888-888888888888"
	draft := problems.Draft{Title: "Interactive", Markdown: "Protocol", TimeLimitMS: "1000", MemoryLimitMB: "512", Interactor: &problems.Generator{Runtime: "python314", Source: "private-interactor"}, TestCases: []problems.TestCase{{Input: "secret", Output: ""}}}
	ip, err := store.Save(ctx, "alice", interactiveID, 0, draft)
	if err != nil {
		t.Fatal(err)
	}
	ibody := strings.Replace(checkerBody, checkerID, interactiveID, 1)
	// Draft submissions pin the same interactor and cases before subsequent edits.
	var isub submissions.Submission
	if err = json.Unmarshal([]byte(request("POST", "/my/submissions", "alice", ibody, 202)), &isub); err != nil {
		t.Fatal(err)
	}
	draft.Interactor.Source = "edited-interactor"
	ip, err = store.Save(ctx, "alice", interactiveID, ip.Version, draft)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, isub.ID).Scan(&checkerRaw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(checkerRaw), "private-interactor") || strings.Contains(string(checkerRaw), "edited-interactor") {
		t.Fatal(string(checkerRaw))
	}
	ip, err = store.Publish(ctx, "alice", interactiveID, ip.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	draft.Interactor = nil
	if _, err = store.Save(ctx, "alice", interactiveID, ip.Version, draft); err != nil {
		t.Fatal(err)
	}
	detail := request("GET", "/problems/"+interactiveID, "", "", 200)
	if !strings.Contains(detail, `"interactive":true`) || strings.Contains(detail, "interactor") {
		t.Fatal(detail)
	}
	for _, owner := range []string{"alice", "bob"} {
		if err = json.Unmarshal([]byte(request("POST", "/my/submissions", owner, ibody, 202)), &isub); err != nil {
			t.Fatal(err)
		}
		if err = store.Pool().QueryRow(ctx, `SELECT job FROM submissions WHERE id=$1`, isub.ID).Scan(&checkerRaw); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(checkerRaw), "edited-interactor") {
			t.Fatal("published interactor not pinned")
		}
		if _, err = store.Pool().Exec(ctx, `UPDATE submissions SET status='DONE',result='{"verdict":"AC","passed":1,"total":1,"checkerLog":"private-dialogue"}' WHERE id=$1`, isub.ID); err != nil {
			t.Fatal(err)
		}
		detail = request("GET", "/my/submissions/"+isub.ID, owner, "", 200)
		if strings.Contains(detail, "private-dialogue") != (owner == "alice") {
			t.Fatal("dialogue authorization", detail)
		}
		if strings.Contains(request("GET", "/my/submissions", owner, "", 200), "private-dialogue") {
			t.Fatal("dialogue leaked through list")
		}
		if _, err = store.Pool().Exec(ctx, `UPDATE submissions SET job=jsonb_set(job,'{easyTest}','true') WHERE id=$1`, isub.ID); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(request("GET", "/my/submissions/"+isub.ID, owner, "", 200), "private-dialogue") {
			t.Fatal("sample dialogue unavailable to submitter")
		}
	}
	if _, err = queue.CreateRun(ctx, submissions.RunInput{Owner: "alice", ID: "99999999-9999-4999-8999-999999999999", ProblemID: interactiveID, Source: "source", Image: image, Runtime: "cpp17-local", CheckerRuntimes: []string{"python314"}}); !errors.Is(err, submissions.ErrNotReady) {
		t.Fatal("local worker accepted interactive job", err)
	}
	t.Run("testlib protocol is pinned with judge source", func(t *testing.T) {
		h = newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Submissions: queue, JudgeImage: image, JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp23-gcc", Verifier: newCognitoVerifier(f.server.URL, "client")})
		for index, field := range []string{"checker", "interactor"} {
			pid := fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012d", index)
			code := &problems.Generator{Runtime: "cpp23-gcc", Source: "testlib-original", Protocol: "testlib"}
			d := problems.Draft{Title: "Protocol snapshot", Markdown: "Check", TimeLimitMS: "1000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "3", Output: "3"}}}
			if field == "checker" {
				d.Checker = code
			} else {
				d.Interactor = code
			}
			payload, _ := json.Marshal(map[string]any{"version": 0, "draft": d})
			request("PUT", "/my/problems/"+pid, "alice", string(payload), 200)
			var published problems.Problem
			if err := json.Unmarshal([]byte(request("PUT", "/my/problems/"+pid+"/publication", "alice", `{"version":1,"publish":true}`, 200)), &published); err != nil {
				t.Fatal(err)
			}
			var submission submissions.Submission
			body := fmt.Sprintf(`{"problemId":%q,"runtime":"cpp23-gcc","source":"int main(){}"}`, pid)
			if err := json.Unmarshal([]byte(request("POST", "/my/submissions", "bob", body, 202)), &submission); err != nil {
				t.Fatal(err)
			}
			code.Protocol, code.Source = "legacy", "edited-legacy"
			payload, _ = json.Marshal(map[string]any{"version": published.Version, "draft": d})
			request("PUT", "/my/problems/"+pid, "alice", string(payload), 200)
			var raw []byte
			if err := store.Pool().QueryRow(ctx, `SELECT job->$2 FROM submissions WHERE id=$1`, submission.ID, field).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var pinned problems.Generator
			if json.Unmarshal(raw, &pinned) != nil || pinned.Protocol != "testlib" || pinned.Source != "testlib-original" {
				t.Fatalf("judge protocol snapshot changed: %s", raw)
			}
		}
	})
	t.Run("generation rejects a draft edited after job construction", func(t *testing.T) {
		const owner = "generation-version"
		if _, err := store.Pool().Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES($1,'generation_version')`, owner); err != nil {
			t.Fatal(err)
		}
		pid := newSubmissionID()
		draft := problems.Draft{Title: "Versioned generation", TimeLimitMS: "1000", MemoryLimitMB: "512", TestCases: []problems.TestCase{{Input: "old"}}}
		before, err := store.Save(ctx, owner, pid, 0, draft)
		if err != nil {
			t.Fatal(err)
		}
		draft.TestCases[0].Input = "new"
		after, err := store.Save(ctx, owner, pid, before.Version, draft)
		if err != nil {
			t.Fatal(err)
		}
		in := submissions.GenerationInput{
			RunInput:       submissions.RunInput{Owner: owner, ID: newSubmissionID(), ProblemID: pid, Source: "source", Runtime: "cpp17-local"},
			ProblemVersion: before.Version,
			Job:            submissions.Job{Generate: true, Cases: before.Draft.TestCases},
		}
		if _, err = queue.CreateGeneration(ctx, in); !errors.Is(err, problems.ErrConflict) {
			t.Fatalf("stale job accepted: %v", err)
		}
		var count int
		if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM submissions WHERE id=$1`, in.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("stale job persisted: %d %v", count, err)
		}
		in.ProblemVersion, in.Job.Cases = after.Version, after.Draft.TestCases
		got, err := queue.CreateGeneration(ctx, in)
		if err != nil || got.ProblemVersion != after.Version {
			t.Fatalf("current job rejected: %+v %v", got, err)
		}
	})
}

func TestCheckerDraftValidation(t *testing.T) {
	for _, tc := range []struct {
		runtime, source string
		valid           bool
	}{
		{"cpp17", "", true},
		{"python314", "assert True", true},
		{"java24", "class Main {}", true},
		{"sh", "exit 0", false},
		{"cpp17-isolate", "int main(){}", false},
		{"cpp17", "\x00", false},
		{"cpp17", strings.Repeat("あ", 22000), false},
	} {
		draft := problems.Draft{TimeLimitMS: "1000", MemoryLimitMB: "512", Checker: &problems.Generator{Runtime: tc.runtime, Source: tc.source}}
		for _, interactive := range []bool{false, true} {
			if interactive {
				draft.Interactor, draft.Checker = draft.Checker, nil
			}
			if validDraft(draft) != tc.valid {
				t.Fatalf("runtime=%s interactive=%v valid=%v", tc.runtime, interactive, tc.valid)
			}
		}
	}
}

func TestConflictingJudgeModes(t *testing.T) {
	code := &problems.Generator{Runtime: "python314", Source: "print(1)"}
	if validDraft(problems.Draft{TimeLimitMS: "1000", MemoryLimitMB: "512", Checker: code, Interactor: code}) {
		t.Fatal("accepted two judge modes")
	}
}

func TestJudgeProtocolsAreExplicitAndBoundToSupportedRuntimes(t *testing.T) {
	for _, tc := range []struct {
		runtime, protocol string
		valid             bool
	}{
		{"cpp23-gcc", "testlib", true},
		{"cpp23-clang", "testlib", true},
		{"cpp17", "", true},
		{"python314", "legacy", true},
		{"python314", "testlib", false},
		{"cpp23-gcc", "guess", false},
	} {
		code := &problems.Generator{Runtime: tc.runtime, Protocol: tc.protocol, Source: "code"}
		for _, interactive := range []bool{false, true} {
			d := problems.Draft{TimeLimitMS: "1000", MemoryLimitMB: "512", Checker: code}
			if interactive {
				d.Checker, d.Interactor = nil, code
			}
			if validDraft(d) != tc.valid {
				t.Fatalf("%+v interactive=%v", tc, interactive)
			}
		}
	}
}
