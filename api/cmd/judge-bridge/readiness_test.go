package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

// A submission and a rejudge accept the same drafts, so a rejudge never queues a run the judge would turn into JE,
// and a sample check skips only the scoring conditions. The drafts are written straight to the table to cover values
// that the validation on save refuses.
func TestSubmitAndRejudgeAgreeOnReadiness(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob'),('carol','carol')`)
	const problem = "11111111-1111-4111-8111-111111111111"
	difficulty := 1
	store := problems.New(db)
	saved, err := store.Save(ctx, "alice", problem, 0, problems.Draft{Title: "t", Markdown: "m", Difficulty: &difficulty, TimeLimitMS: "1000", MemoryLimitMB: "256",
		TestCases: []problems.TestCase{{IsSample: true, Input: "1", Output: "1"}, {Input: "2", Output: "2"}}})
	if err == nil {
		_, err = store.Publish(ctx, "alice", problem, saved.Version, true)
	}
	if err != nil {
		t.Fatal(err)
	}
	var base string
	if err = db.QueryRow(ctx, `SELECT published_draft::text FROM problem_drafts WHERE id=$1`, problem).Scan(&base); err != nil {
		t.Fatal(err)
	}
	finished := map[string]string{"cpp17-isolate": "aaaaaaaa-3333-4333-8333-333333333333", "cpp17-local": "bbbbbbbb-3333-4333-8333-333333333333"}
	for runtime, id := range finished {
		exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,status,job)
 VALUES ($1,'carol',$2,1,'t',$3,'source','DONE','{}'::jsonb)`, id, problem, runtime)
	}
	queue := &submissions.Store{Pool: db}
	digest := "sha256:" + strings.Repeat("a", 64)
	// The local setup offers only cpp17; python314 is listed too, so that only the cpp17 rule of cpp17-local refuses it.
	checkers := []string{"cpp17", "python314"}
	ready := func(runtime string, easyTest bool) bool {
		t.Helper()
		_, err := queue.CreateRun(ctx, submissions.RunInput{Owner: "bob", ID: "cccccccc-3333-4333-8333-333333333333", ProblemID: problem, Source: "source", Image: digest, Runtime: runtime, EasyTest: easyTest, CheckerRuntimes: checkers})
		if err != nil && !errors.Is(err, submissions.ErrNotReady) {
			t.Fatal(runtime, easyTest, err)
		}
		// Each check starts clear of the rate limit.
		exec(`DELETE FROM submissions WHERE owner_id='bob'`)
		return err == nil
	}
	rejudged := func(runtime string) bool {
		t.Helper()
		queued, err := queue.Rejudge(ctx, submissions.RejudgeInput{Actor: "alice", ProblemID: problem, SubmissionIDs: []string{finished[runtime]}, Image: digest, Runtimes: []string{runtime}, CheckerRuntimes: checkers})
		ok := (queued == 1 && err == nil) || (queued == 0 && errors.Is(err, submissions.ErrNotReady))
		if !ok {
			t.Fatal(runtime, queued, err)
		}
		exec(`UPDATE submissions SET status='DONE' WHERE owner_id='carol'`)
		return queued == 1
	}
	group := `[{"name":"g","points":100}]`
	for _, c := range []struct {
		name, patch                         string
		isolate, sample, local, localSample bool
	}{
		{"plain", `{}`, true, true, true, true},
		{"memory 63", `{"memoryLimitMb":"63"}`, false, false, false, false},
		{"memory 64", `{"memoryLimitMb":"64"}`, true, true, true, true},
		{"memory 512", `{"memoryLimitMb":"512"}`, true, true, true, true},
		{"memory 513", `{"memoryLimitMb":"513"}`, false, false, false, false},
		{"memory null", `{"memoryLimitMb":null}`, false, false, false, false},
		{"checker null", `{"checker":null}`, true, true, true, true},
		{"cpp17 checker", `{"checker":{"runtime":"cpp17","source":"x"}}`, true, true, true, true},
		{"python checker", `{"checker":{"runtime":"python314","source":"x"}}`, true, true, false, false},
		{"checker outside the languages", `{"checker":{"runtime":"java24","source":"x"}}`, false, false, false, false},
		{"blank checker", `{"checker":{"runtime":"cpp17","source":" \t\r\n"}}`, false, false, false, false},
		{"checker without source", `{"checker":{"runtime":"cpp17"}}`, false, false, false, false},
		{"interactor", `{"interactor":{"runtime":"python314","source":"x"}}`, true, true, false, false},
		{"interactor outside the languages", `{"interactor":{"runtime":"java24","source":"x"}}`, false, false, false, false},
		{"blank interactor", `{"interactor":{"runtime":"python314","source":" \n"}}`, false, false, false, false},
		{"checker and interactor", `{"checker":{"runtime":"cpp17","source":"x"},"interactor":{"runtime":"cpp17","source":"x"}}`, false, false, false, false},
		{"scoring null", `{"scoring":null}`, true, true, true, true},
		{"groups", `{"scoring":{"mode":"groups","groups":` + group + `}}`, true, true, false, true},
		{"no groups", `{"scoring":{"mode":"groups","groups":[]}}`, false, true, false, true},
		{"groups missing", `{"scoring":{"mode":"groups"}}`, false, true, false, true},
		{"groups object", `{"scoring":{"mode":"groups","groups":{"g":100}}}`, false, true, false, true},
		{"groups string", `{"scoring":{"mode":"groups","groups":"g"}}`, false, true, false, true},
		{"mode missing", `{"scoring":{"groups":` + group + `}}`, false, true, false, true},
		{"aggregator outside groups mode", `{"scoring":{"mode":"groups","groups":` + group + `,"aggregator":{"runtime":"java24","source":""}}}`, true, true, false, true},
		{"aggregate", `{"scoring":{"mode":"aggregate","groups":` + group + `,"aggregator":{"runtime":"cpp17","source":"x"}}}`, true, true, false, true},
		{"aggregate without aggregator", `{"scoring":{"mode":"aggregate","groups":` + group + `}}`, false, true, false, true},
		{"aggregator outside the languages", `{"scoring":{"mode":"aggregate","groups":` + group + `,"aggregator":{"runtime":"java24","source":"x"}}}`, false, true, false, true},
		{"blank aggregator", `{"scoring":{"mode":"aggregate","groups":` + group + `,"aggregator":{"runtime":"cpp17","source":"\t"}}}`, false, true, false, true},
	} {
		exec(`UPDATE problem_drafts SET published_draft=$2::jsonb || $3::jsonb WHERE id=$1`, problem, base, c.patch)
		got := [6]bool{ready("cpp17-isolate", false), ready("cpp17-isolate", true), ready("cpp17-local", false), ready("cpp17-local", true), rejudged("cpp17-isolate"), rejudged("cpp17-local")}
		// A rejudge accepts what a submission in the same language accepts.
		if want := [6]bool{c.isolate, c.sample, c.local, c.localSample, c.isolate, c.local}; got != want {
			t.Errorf("%s: got %v, want %v", c.name, got, want)
		}
	}
}
