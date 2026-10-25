package submissions

import (
	"context"
	"io"
	"judge/api/internal/problems"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestOutputLimitWhileCopying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &boundedOutput{limit: 10, cancel: cancel}
	// Hide WriterTo so io.Copy also exercises the ReaderFrom optimization path.
	if _, err := io.Copy(out, struct{ io.Reader }{strings.NewReader(strings.Repeat("x", 100))}); err != nil {
		t.Fatal(err)
	}
	if out.buffer.Len() != 10 || !out.over || ctx.Err() == nil {
		t.Fatal("output limit bypassed")
	}
}

func TestCPPDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE is required for Docker integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	data, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	job := Job{GenerationPrefix: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/generated/", Runtime: "cpp23-gcc-local", Image: strings.TrimSpace(string(data)), TimeLimitMS: 300, MemoryLimitMB: 64, Cases: []Case{{Name: "sample", Input: "3 5\n", Output: "8\n"}, {Name: "large", Input: "1000000000 1000000000\n", Output: "2000000000\n"}}}
	for _, tc := range []struct{ name, source, verdict string }{
		{"AC", "#include <expected>\nstatic_assert(__cplusplus > 202002L);\nstd::expected<int,int> value;\n#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b;}", "AC"},
		{"partial", "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<(a==3?a+b:0);}", "WA"},
		{"wrong then accepted", "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<(a==3?0:a+b);}", "WA"},
		{"mixed failures", "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;if(a==3){std::cout<<0;return 0;}return 1;}", "WA"},
		{"WA", "#include <iostream>\nint main(){std::cout<<0;}", "WA"},
		{"CE", "int main( {", "CE"},
		{"RE", "int main(){return 1;}", "RE"},
		{"TLE", "int main(){for(;;){}}", "TLE"},
		{"OLE", "#include <unistd.h>\nint main(){char a[8192]={};for(;;)write(1,a,sizeof a);}", "OLE"},
		{"MLE", "#include <cstdlib>\n#include <cstring>\nint main(){for(;;){volatile char* p=(char*)malloc(1048576);if(!p)return 1;for(int i=0;i<1048576;i++)p[i]=1;}}", "MLE"},
		{"isolated cases", "#include <fstream>\n#include <iostream>\nint main(){if(std::ifstream(\"/tmp/previous\").good())return 1;std::ofstream(\"/tmp/previous\")<<1;long long a,b;std::cin>>a>>b;std::cout<<a+b;}", "AC"},
		{"read-only artifact", "#include <unistd.h>\n#include <fcntl.h>\n#include <iostream>\nint main(){if(open(\"/submission/main\",O_WRONLY)>=0)return 1;long long a,b;std::cin>>a>>b;std::cout<<a+b;}", "AC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caseJob := job
			if tc.verdict == "OLE" {
				caseJob.TimeLimitMS = 2000
			}
			result := Judge(ctx, tc.source, caseJob)
			if result.Verdict != tc.verdict {
				t.Fatalf("got %+v, want %s", result, tc.verdict)
			}
			if tc.verdict == "CE" {
				return
			}
			if len(result.Cases) != 2 || result.Cases[0].Name != "sample" || result.Cases[1].Name != "large" {
				t.Fatalf("missing case names: %+v", result)
			}
			want := []string{tc.verdict, tc.verdict}
			if tc.verdict == "AC" {
				want = []string{"AC", "AC"}
			}
			if tc.verdict == "CE" {
				want = []string{"SKIPPED", "SKIPPED"}
			}
			if tc.name == "partial" {
				want = []string{"AC", "WA"}
				if result.Passed != 1 {
					t.Fatalf("wrong passed count: %+v", result)
				}
			}
			if tc.name == "wrong then accepted" {
				want = []string{"WA", "AC"}
				if result.Passed != 1 {
					t.Fatalf("wrong passed count: %+v", result)
				}
			}
			if tc.name == "mixed failures" {
				want = []string{"WA", "RE"}
			}
			for i, verdict := range want {
				if result.Cases[i].Verdict != verdict {
					t.Fatalf("case %d: got %+v, want %s", i, result.Cases[i], verdict)
				}
			}
			if result.Verdict == "AC" && result.Passed != 2 {
				t.Fatal("did not execute all cases")
			}
		})
	}
}

func TestCaseDataDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE is required for Docker integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	data, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	// The program prints the number of CRs and whether the input ends with LF (2 when it is empty).
	job := Job{GenerationPrefix: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/generated/", Runtime: "cpp23-gcc-local", Image: strings.TrimSpace(string(data)), TimeLimitMS: 2000, MemoryLimitMB: 64,
		Cases:    []Case{{Input: "abc\r\nd\r", Output: "0 1"}, {Input: "abc\r\nd", Output: "0 1"}, {InputFile: &problems.TestFile{ID: "11111111-1111-4111-8111-111111111111", Size: 6, SHA256: strings.Repeat("a", 64), Key: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/11111111-1111-4111-8111-111111111111", Version: "1"}, Output: "0 1"}, {Output: "0 2"}},
		LoadFile: func(context.Context, *problems.TestFile) (string, error) { return "abc\r\nd", nil }}
	source := "#include <algorithm>\n#include <iostream>\n#include <iterator>\n#include <string>\nint main(){std::string s{std::istreambuf_iterator<char>(std::cin),{}};std::cout<<std::count(s.begin(),s.end(),'\\r')<<' '<<(s.empty()?2:s.back()=='\\n');}"
	if result := Judge(ctx, source, job); result.Verdict != "AC" || result.Passed != 4 {
		t.Fatalf("got %+v, want AC", result)
	}
}

func TestGeneratorDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	job := Job{GenerationPrefix: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/generated/", Runtime: "cpp23-gcc-local", Generate: true, Image: strings.TrimSpace(string(raw)), TimeLimitMS: 1000, MemoryLimitMB: 512, Cases: []Case{{Input: "7\n"}, {Input: "8\n"}}}
	var stored []string
	job.SaveOutput = func(_ context.Context, data []byte) (*problems.TestFile, error) {
		stored = append(stored, string(data))
		return &problems.TestFile{Size: int64(len(data))}, nil
	}
	result := Judge(ctx, "#include <iostream>\nint main(){int n;std::cin>>n;std::cout<<n<<\" \\n\\n\";}", job)
	if result.Verdict != "AC" || len(result.Cases) != 2 || len(stored) != 2 || stored[0] != "7 \n\n" || stored[1] != "8 \n\n" {
		t.Fatalf("generation: %+v", result)
	}
	result = Judge(ctx, "int main(){}", job)
	if result.Verdict != "AC" || result.Cases[0].Output == nil || *result.Cases[0].Output != "" {
		t.Fatalf("empty output: %+v", result)
	}
	job.Generate, job.Validate = false, true
	stored = nil
	result = Judge(ctx, "#include <iostream>\nint main(){int n;std::cin>>n;std::cout<<\"ignored\";return n==7?0:1;}", job)
	if result.Passed != 1 || result.Cases[0].Verdict != "AC" || result.Cases[1].Verdict != "RE" || len(stored) != 0 || result.Cases[0].Output != nil {
		t.Fatalf("input validation: %+v", result)
	}
	job.Generate, job.Validate = true, false
	job.GenerationBaseBytes = GenerationOutputLimit - 32768
	result = Judge(ctx, "#include <iostream>\nint main(){for(int i=0;i<20000;i++)std::cout<<'x';}", job)
	if result.Verdict != "OLE" {
		t.Fatalf("aggregate limit: %+v", result)
	}
	job.GenerationBaseBytes = GenerationOutputLimit - (32 << 20)
	job.TimeLimitMS = 5000
	job.Cases = []Case{{Input: "16777216"}, {Input: "16777216"}}
	stored = nil
	source := "#include <iostream>\n#include <string>\nint main(){int n;std::cin>>n;std::cout<<std::string(n,'x');}"
	result = Judge(ctx, source, job)
	if result.Verdict != "AC" || len(stored) != 2 || len(stored[0]) != 16<<20 || len(stored[1]) != 16<<20 {
		t.Fatalf("16 MiB boundary: %+v", result)
	}
	job.Cases = []Case{{Input: "16777217"}}
	result = Judge(ctx, source, job)
	if result.Verdict != "OLE" {
		t.Fatalf("file limit: %+v", result)
	}
}

func TestKnockoutDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	data, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	source := "#include <iostream>\nint main(){int n;std::cin>>n;if(n==1||n==3)for(;;){};}"
	for _, mode := range []string{"normal", "sample", "validation", "generation"} {
		t.Run(mode, func(t *testing.T) {
			job := Job{GenerationPrefix: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/generated/", Runtime: "cpp23-gcc-local", Image: strings.TrimSpace(string(data)), TimeLimitMS: 100, MemoryLimitMB: 64,
				EasyTest: mode == "sample", Validate: mode == "validation", Generate: mode == "generation",
				Cases: []Case{{Input: "1"}, {Input: "2"}, {Input: "3"}, {Input: "4"}}}
			r := Judge(ctx, source, job)
			last, passed := "AC", 2
			if mode == "normal" {
				last, passed = "SKIPPED", 1
			}
			if r.Verdict != "TLE" || r.Total != 4 || r.Passed != passed || len(r.Cases) != 4 {
				t.Fatalf("unexpected knockout result: %+v", r)
			}
			for i, want := range []string{"TLE", "AC", "TLE", last} {
				if r.Cases[i].Verdict != want {
					t.Fatalf("case %d: %+v, want %s", i, r.Cases[i], want)
				}
			}
		})
	}
}

func TestSpecialJudgeDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	checker := `#include <cassert>
#include <fstream>
#include <iostream>
#include <string>
int main(int argc,char** argv){
 assert(argc==5); int n,x; std::ifstream(argv[1])>>n;
 std::string expected; std::ifstream(argv[2])>>expected; assert(expected=="secret");
 std::string source; std::getline(std::ifstream(argv[3]) >> std::ws,source); assert(!source.empty());
 assert(std::cin>>x); assert(x>=0 && x<=n); std::string extra; assert(!(std::cin>>extra));
 assert(!std::ifstream("/tmp/marker").good()); std::ofstream("/tmp/marker")<<1;
 std::ofstream score(argv[4]); assert(score.good()); score<<123;
 std::cerr<<"private diagnostic";
}`
	for _, tc := range []struct{ name, source, checker, verdict string }{
		{"alternative answer", "#include <cstdio>\nint main(){puts(\"7\");}", checker, "AC"},
		{"assert", "#include <cstdio>\nint main(){puts(\"99\");}", checker, "WA"},
		{"trailing output", "#include <cstdio>\nint main(){puts(\"7 8\");}", checker, "WA"},
		{"submission RE", "int main(){return 1;}", "int main(){return 0;}", "RE"},
		{"checker CE", "int main(){}", "invalid syntax", "JE"},
		{"checker TLE", "int main(){}", "int main(){for(;;){}}", "JE"},
		{"exit 100", "int main(){}", "int main(){return 100;}", "WA"},
		{"private checker files", `#include <unistd.h>
int main(){return access("/submission/expected-output",F_OK)==-1 && access("/submission/test-input",F_OK)==-1 ? 0:1;}`, "int main(){return 0;}", "AC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := Job{GenerationPrefix: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/generated/", Runtime: "cpp23-gcc-local", Image: strings.TrimSpace(string(raw)), TimeLimitMS: 1000, MemoryLimitMB: 512,
				Checker: &problems.Generator{Runtime: "cpp23-gcc", Source: tc.checker}, Cases: []Case{{Input: "10", Output: "secret"}, {Input: "10", Output: "secret"}}}
			r := Judge(ctx, tc.source, job)
			if r.Verdict != tc.verdict {
				t.Fatalf("got %+v want %s", r, tc.verdict)
			}
			if tc.verdict == "AC" && r.Passed != 2 {
				t.Fatal(r)
			}
			if tc.verdict == "JE" && (r.Passed != 0 || len(r.Cases) != 0) {
				t.Fatal(r)
			}
		})
	}
}

func TestPythonDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	job := Job{GenerationPrefix: "test-files/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/11111111-1111-4111-8111-111111111111/generated/", Runtime: "python314-local", Image: strings.TrimSpace(string(raw)), TimeLimitMS: 1000, MemoryLimitMB: 64,
		Cases: []Case{{Input: "3 5\r", Output: "8"}, {Input: "10 11", Output: "21"}}}
	for _, tc := range []struct{ name, source, verdict string }{
		{"AC", "import sys\nassert sys.version_info[:2] == (3,14)\nprint(sum(map(int,input().split())))", "AC"},
		{"CE", "if :", "CE"},
		{"WA", "print(0)", "WA"},
		{"RE", "raise RuntimeError('failure')", "RE"},
		{"TLE", "while True: pass", "TLE"},
		{"OLE", "import sys\nwhile True: sys.stdout.write('x'*65536)", "OLE"},
		{"isolated cases", "from pathlib import Path\np=Path('/tmp/previous')\nassert not p.exists()\np.write_text('1')\nprint(sum(map(int,input().split())))", "AC"},
		{"read-only artifact", "import os\nassert not os.access('/submission/main',os.W_OK)\nprint(sum(map(int,input().split())))", "AC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Judge(ctx, tc.source, job)
			if result.Verdict != tc.verdict {
				t.Fatalf("got %+v want %s", result, tc.verdict)
			}
		})
	}
	job.Generate = true
	var saved []string
	job.SaveOutput = func(_ context.Context, data []byte) (*problems.TestFile, error) {
		saved = append(saved, string(data))
		return &problems.TestFile{Size: int64(len(data))}, nil
	}
	if r := Judge(ctx, "print(sum(map(int,input().split())))", job); r.Verdict != "AC" || len(saved) != 2 || saved[0] != "8\n" || saved[1] != "21\n" {
		t.Fatal(r, saved)
	}
	job.Generate, job.Validate = false, true
	if r := Judge(ctx, "assert sum(map(int,input().split())) == 8", job); r.Verdict != "RE" || r.Passed != 1 {
		t.Fatal(r)
	}
	job.Validate = false
	job.Checker = &problems.Generator{Runtime: "python314", Source: "import sys\nfrom pathlib import Path\nassert len(sys.argv)==5\nassert int(input())==int(Path(sys.argv[2]).read_text())\nassert Path(sys.argv[3]).read_text()\nPath(sys.argv[4]).write_text('0')"}
	for _, runtime := range []string{"cpp23-gcc-local", "python314-local"} {
		job.Runtime = runtime
		source := "print(sum(map(int,input().split())))"
		if runtime == "cpp23-gcc-local" {
			source = "#include <iostream>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b;}"
		}
		if r := Judge(ctx, source, job); r.Verdict != "AC" || r.Passed != 2 {
			t.Fatal(r)
		}
	}
}

func TestLocalJudgeRejectsUnknownRuntime(t *testing.T) {
	for _, runtime := range []string{"", "python314-isolate", "java24-local"} {
		job := Job{Runtime: runtime, Image: "sha256:invalid", TimeLimitMS: 1000, MemoryLimitMB: 64, Cases: []Case{{Input: "1", Output: "1"}}}
		if r := Judge(context.Background(), "print(1)", job); r.Verdict != "JE" || len(r.Cases) != 0 {
			t.Fatal(runtime, r)
		}
	}
}

func TestLocalScoringDocker(t *testing.T) {
	image := os.Getenv("TEST_JUDGE_CPP_IMAGE")
	if image == "" {
		t.Skip("TEST_JUDGE_CPP_IMAGE required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Runtime: "python314-local", Image: strings.TrimSpace(string(raw)), TimeLimitMS: 100, MemoryLimitMB: 128,
		Scoring: &problems.Scoring{Mode: "groups", Groups: []problems.ScoringGroup{{Name: "slow", Points: 40}, {Name: "fast", Points: 60}}},
		Cases:   []Case{{Input: "1", Output: "1", Groups: []string{"slow"}}, {Input: "1", Output: "1", Groups: []string{"slow"}}, {Input: "1", Output: "1", Groups: []string{"slow"}}, {Input: "2", Output: "2", Groups: []string{"fast"}}}}
	source := "n=int(input())\nwhile n==1: pass\nprint(n)"
	r := Judge(ctx, source, job)
	if r.Verdict != "TLE" || r.Scoring == nil || r.Scoring.Score != 6000 || r.Cases[2].Verdict != "SKIPPED" || r.Cases[3].Verdict != "AC" {
		t.Fatal(r)
	}
	if err := scoreResult(job, &r); err != nil {
		t.Fatal(err)
	}
	r = Judge(ctx, "if :", job)
	if r.Verdict != "CE" || r.Scoring == nil || r.Scoring.Score != 0 || r.Scoring.MaxScore != 10000 {
		t.Fatal(r)
	}
	job.TimeLimitMS = 1000
	job.Cases = []Case{{Input: "1", Output: "1", Groups: []string{"slow"}, IsSample: true}, {Input: "2", Output: "2", Groups: []string{"fast"}}}
	job.Scoring.Mode = "aggregate"
	job.Scoring.Aggregator = &problems.Generator{Runtime: "python314", Source: "import sys\nlines=sys.stdin.read().splitlines()\nassert lines==['2 2','40','60','AC 7 1 1 0','AC 7 0 1 1']\nprint('12.349 50')"}
	job.Checker = &problems.Generator{Runtime: "python314", Source: "import sys\nopen(sys.argv[4],'w').write('7')"}
	r = Judge(ctx, "print(input())", job)
	if r.Verdict != "AC" || r.Scoring == nil || r.Scoring.Score != 6234 || r.Cases[0].Score == nil || *r.Cases[0].Score != 7 {
		t.Fatal(r)
	}
	if err := scoreResult(job, &r); err != nil {
		t.Fatal(err)
	}
	// The same pinned testlib header and result parser handle quitp locally.
	job.Checker = &problems.Generator{Runtime: "cpp23-gcc", Protocol: "testlib", Source: "#include \"testlib.h\"\nint main(int argc,char**argv){registerTestlibCmd(argc,argv);ouf.readInt();quitp(7,\"partial\");}"}
	r = Judge(ctx, "print(input())", job)
	if r.Verdict != "AC" || r.Scoring == nil || r.Scoring.Score != 6234 {
		t.Fatal(r)
	}
	job.Scoring.Aggregator.Source = "print('40.001 50')"
	r = Judge(ctx, "print(input())", job)
	if r.Verdict != "JE" || r.Scoring != nil || r.CheckerLog == "" {
		t.Fatal(r)
	}
	job.Scoring = nil
	job.EasyTest = true
	job.AcceptPoints = true
	r = Judge(ctx, "print(input())", job)
	if r.Verdict != "AC" || r.Scoring != nil || r.Cases[0].Score != nil {
		t.Fatal(r)
	}
}
