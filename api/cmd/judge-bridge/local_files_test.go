package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"judge/api/internal/problems"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

func TestLocalGenerationAndScoring(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE required")
	}
	ctx := context.Background()
	db := migratedDB(t)
	if _, err := db.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob')`); err != nil {
		t.Fatal(err)
	}
	const pid = "11111111-1111-4111-8111-111111111111"
	store := problems.New(db)
	draft := problems.Draft{Title: "Local generation", Markdown: "test", TimeLimitMS: "1000", MemoryLimitMB: "128", TestCases: []problems.TestCase{{Input: "1", Output: "1"}}}
	saved, err := store.Save(ctx, "alice", pid, 0, draft)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	objects, err := testfiles.NewLocalObjects(dir)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	files, handler, err := testfiles.NewLocal(db, dir, server.URL+"/local-test-files", "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/local-test-files", handler)
	raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	queue := &submissions.Store{Pool: db}
	svc := submissions.Service{Config: submissions.Config{JudgeImage: strings.TrimSpace(string(raw))}, Submissions: queue, Problems: store}
	run := func(input submissions.CreateInput, id string) submissions.Submission {
		t.Helper()
		item, err := svc.Create(ctx, "alice", id, input)
		if err != nil {
			t.Fatal(err)
		}
		claimed, job, err := queue.Claim(ctx)
		if err != nil || claimed.ID != item.ID {
			t.Fatal(claimed, err)
		}
		if err = queue.ResolveTestFiles(ctx, &job); err != nil {
			t.Fatal(err)
		}
		job.SaveOutput = func(ctx context.Context, data []byte) (*problems.TestFile, error) {
			return testfiles.WriteGenerated(ctx, objects, "local", job.GenerationPrefix, data)
		}
		job.LoadFile = func(ctx context.Context, file *problems.TestFile) (string, error) {
			return testfiles.ReadFile(ctx, objects, "local", file)
		}
		result := submissions.Judge(ctx, claimed.Source, job)
		if err = queue.Finish(ctx, item.ID, result); err != nil {
			t.Fatal(result, err)
		}
		item, err = queue.Get(ctx, "alice", item.ID)
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	item := run(submissions.CreateInput{ProblemID: pid, Runtime: "python314", Source: "print('x'*70000)", Generation: &submissions.Generation{Mode: "input", Start: 1, Count: 1}}, "22222222-2222-4222-8222-222222222222")
	if item.Result.Verdict != "AC" || item.Result.Cases[0].OutputFile == nil {
		t.Fatal(item.Result)
	}
	file, err := files.Complete(ctx, "alice", pid, item.Result.Cases[0].OutputFile.ID)
	if err != nil {
		t.Fatal(err)
	}
	download, err := files.Download(ctx, "alice", pid, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(download.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(data) != strings.Repeat("x", 70000)+"\n" {
		t.Fatal("generated download", err)
	}
	if _, err := files.Download(ctx, "bob", pid, file.ID); err == nil {
		t.Fatal("private file exposed")
	}
	// Save the generated reference as real test data and reload it through the worker.
	draft.TestCases = []problems.TestCase{{InputFile: &file, Output: "70000", Groups: []string{"large"}}}
	draft.Scoring = &problems.Scoring{Mode: "groups", Groups: []problems.ScoringGroup{{Name: "large", Points: 100}}}
	if _, err = store.Save(ctx, "alice", pid, saved.Version, draft); err != nil {
		t.Fatal(err)
	}
	item = run(submissions.CreateInput{ProblemID: pid, Runtime: "python314", Source: "print(len(input()))"}, "33333333-3333-4333-8333-333333333333")
	if item.Result.Verdict != "AC" || item.Result.Scoring == nil || item.Result.Scoring.Score != 10000 {
		t.Fatal(item.Result)
	}
}
