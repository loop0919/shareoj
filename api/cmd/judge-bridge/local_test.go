package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

func TestLocalDockerQueue(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	const pid = "11111111-1111-4111-8111-111111111111"
	problemsStore := problems.New(db)
	draft := problems.Draft{Title: "A+B", Markdown: "Add", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1 2", Output: "3", IsSample: true}}}
	saved, err := problemsStore.Save(ctx, "alice", pid, 0, draft)
	if err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	dockerImage := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if dockerImage != "" {
		raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", dockerImage).Output()
		if err != nil {
			t.Fatal(err)
		}
		image = strings.TrimSpace(string(raw))
	}
	queue := &submissions.Store{Pool: db}
	svc := submissions.Service{Config: submissions.Config{JudgeImage: image}, Submissions: queue, Problems: problemsStore,
		DispatchJudge: func(context.Context) error { t.Fatal("local job dispatched to cloud"); return nil }}
	next := 0
	create := func(runtime, source string, generation *submissions.Generation) (submissions.Submission, error) {
		next++
		if _, err := db.Exec(ctx, `UPDATE submissions SET created_at=clock_timestamp()-interval '61 seconds'`); err != nil {
			t.Fatal(err)
		}
		return svc.Create(ctx, "alice", fmt.Sprintf("%08d-3333-4333-8333-333333333333", next), submissions.CreateInput{ProblemID: pid, Runtime: runtime, Source: source, Generation: generation})
	}
	for _, runtime := range []string{"cpp17", "cpp17-local", "cpp23-gcc-isolate", "python314-isolate", "java24"} {
		if _, err := create(runtime, "source", nil); !errors.Is(err, submissions.ErrInvalid) {
			t.Fatal(runtime, err)
		}
	}
	for _, runtime := range []string{"cpp23-gcc", "python314"} {
		source := "print(sum(map(int,input().split())))"
		if runtime == "cpp23-gcc" {
			source = "#include <iostream>\nstatic_assert(__cplusplus > 202002L);\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b;}"
		}
		item, err := create(runtime, source, nil)
		if err != nil || item.Runtime != runtime+"-local" {
			t.Fatal(item, err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			claimed, job, err := queue.Claim(ctx)
			if err != nil || claimed.ID != item.ID || job.Runtime != item.Runtime || job.Image != image {
				t.Fatal(claimed, job, err)
			}
			result := submissions.Result{Verdict: "AC", Passed: 1, Total: 1}
			if dockerImage != "" {
				result = submissions.Judge(ctx, claimed.Source, job)
			}
			if result.Verdict != "AC" {
				t.Fatal(result)
			}
			if err := queue.Finish(ctx, item.ID, result); err != nil {
				t.Fatal(err)
			}
			finished, err := queue.Get(ctx, "alice", item.ID)
			if err != nil || finished.Status != "DONE" || finished.Result == nil || finished.Result.Verdict != "AC" {
				t.Fatal(finished, err)
			}
			if attempt == 0 {
				count, err := svc.Rejudge(ctx, "alice", pid, []string{item.ID}, false)
				if err != nil || count != 1 {
					t.Fatal(count, err)
				}
			}
		}
	}
	// Generation uses a different insertion path but must retain the Python runtime on claim.
	item, err := create("python314-local", "print(input())", &submissions.Generation{Mode: "input", Start: 5, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	claimed, job, err := queue.Claim(ctx)
	if err != nil || claimed.ID != item.ID || job.Runtime != "python314-local" || !job.Generate || job.Cases[0].Input != "5\n" {
		t.Fatal(claimed, job, err)
	}
	if err := queue.Finish(ctx, item.ID, submissions.Result{Verdict: "JE", Total: 1}); err != nil {
		t.Fatal(err)
	}
	// Interactive judging still requires isolate.
	draft.Interactor = &problems.Generator{Runtime: "python314", Source: "print(1)"}
	if _, err = problemsStore.Save(ctx, "alice", pid, saved.Version, draft); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{"cpp23-gcc", "python314"} {
		if _, err := create(runtime, "source", nil); !errors.Is(err, submissions.ErrNotReady) {
			t.Fatal(runtime, err)
		}
	}
}
