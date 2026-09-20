package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"judge/api/internal/submissions"
)

func TestRetiredCPP17IsHiddenAndRejected(t *testing.T) {
	p := submissionHandler{Service: submissions.Service{
		Config: submissions.Config{
			JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate",
			JudgeEnabledRuntimes: "c23-gcc,c23-clang,python314,pypy311,codon020,rust2024,cpp23-gcc,cpp23-clang",
		},
		Submissions: &submissions.Store{},
	}}
	catalog := p.AvailableRuntimes()
	if len(catalog) != 8 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	for _, runtime := range catalog {
		if runtime.ID == "cpp17" || runtime.ID == "java24" {
			t.Fatalf("unpublished runtime: %+v", runtime)
		}
	}
	for _, runtime := range []string{"cpp17", "cpp17-isolate", "cpp17-local"} {
		r := httptest.NewRequest("POST", "/my/submissions", strings.NewReader(`{"problemId":"11111111-1111-4111-8111-111111111111","runtime":"`+runtime+`","source":"int main(){}"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		p.submission(w, r, "alice")
		if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_submission") {
			t.Fatalf("%s accepted: %d %s", runtime, w.Code, w.Body.String())
		}
	}
}

func TestRuntimeCatalogFollowsAdmissionConfiguration(t *testing.T) {
	p := submissionHandler{Service: submissions.Service{Config: submissions.Config{JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate"}}}
	for _, enabled := range []string{"", "cpp17,c23-gcc", "none"} {
		p.JudgeEnabledRuntimes = enabled
		r := httptest.NewRecorder()
		p.runtimes(r, httptest.NewRequest("GET", "/runtimes", nil))
		if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(r)
		}
		if strings.Contains(r.Body.String(), "java24") {
			t.Fatal("unverified runtime published")
		}
		var response struct {
			Items       []submissions.Runtime
			Maintenance bool
		}
		if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil || response.Maintenance != (enabled == "none") || (enabled == "none" && len(response.Items) != 0) {
			t.Fatal(r.Body.String())
		}
		if strings.Contains(r.Body.String(), "c23-gcc") != (enabled == "cpp17,c23-gcc") {
			t.Fatal(r.Body.String())
		}
	}
	p.JudgeImage = ""
	if len(p.AvailableRuntimes()) != 0 {
		t.Fatal("disabled judge advertised runtimes")
	}
}

func TestMaintenanceRejectsAllJudgeRequestsBeforeCreatingJobs(t *testing.T) {
	p := submissionHandler{Service: submissions.Service{Config: submissions.Config{JudgeEnabledRuntimes: "none"}, Submissions: &submissions.Store{}}}
	// admission --pause also clears the digest. Maintenance must take priority.
	for _, body := range []string{
		`{"source":"normal"}`, `{"contestId":"contest"}`, `{"easyTest":true}`,
		`{"generation":{"mode":"input"}}`, `{"generation":{"mode":"output"}}`, `{"generation":{"mode":"validation"}}`,
	} {
		w := httptest.NewRecorder()
		p.submission(w, httptest.NewRequest("POST", "/my/submissions", strings.NewReader(body)), "alice")
		if w.Code != 503 || !strings.Contains(w.Body.String(), "judge_maintenance") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	// An unavailable configuration is not announced as planned maintenance.
	p.JudgeEnabledRuntimes = ""
	w := httptest.NewRecorder()
	p.runtimes(w, httptest.NewRequest("GET", "/runtimes", nil))
	if strings.Contains(w.Body.String(), `"maintenance":true`) {
		t.Fatal(w.Body.String())
	}
}
