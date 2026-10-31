package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const scoringTOML = `
[scoring]
mode = "aggregate"

[scoring.aggregator]
source = "aggregator.cpp"
runtime = "cpp23-gcc"

[[tests.groups]]
name = "小課題1"
points = 30
cases = ["01", "02"]

[[tests.groups]]
name = " 小課題2 "
points = 70
cases = ["sample1", "01", "02", "03"]
`

var noAggregator = strings.Replace(scoringTOML, "[scoring.aggregator]\nsource = \"aggregator.cpp\"\nruntime = \"cpp23-gcc\"\n", "", 1)

// scoringExample adds three cases and partial scoring with an aggregator to the init example.
func scoringExample(t *testing.T) string {
	t.Helper()
	dir := example(t)
	for _, name := range []string{"01", "02", "03"} {
		write(t, filepath.Join(dir, "tests", name+".in"), name+"\n")
		write(t, filepath.Join(dir, "tests", name+".out"), name+"\n")
	}
	write(t, filepath.Join(dir, "aggregator.cpp"), "int main() {}\n")
	write(t, filepath.Join(dir, "problem.toml"), template+scoringTOML)
	return dir
}

func caseGroups(t *testing.T, d draft) map[string][]string {
	t.Helper()
	var cases []testCase
	if err := json.Unmarshal(d["testCases"], &cases); err != nil {
		t.Fatal(err)
	}
	groups := map[string][]string{}
	for _, c := range cases {
		groups[c.Name] = c.Groups
	}
	return groups
}

func TestScoringValidation(t *testing.T) {
	dir := scoringExample(t)
	d, notes, err := readProblem(dir)
	if err != nil || len(notes) != 0 {
		t.Fatal(err, notes)
	}
	const want = `{"mode":"aggregate","groups":[{"name":"小課題1","points":30},{"name":" 小課題2 ","points":70}],"aggregator":{"runtime":"cpp23-gcc","source":"int main() {}\n"}}`
	if string(d["scoring"]) != want {
		t.Fatalf("scoring: %s", d["scoring"])
	}
	// Cases list their groups by exact name, in the order of the groups.
	if got := caseGroups(t, d); !reflect.DeepEqual(got, map[string][]string{
		"01": {"小課題1", " 小課題2 "}, "02": {"小課題1", " 小課題2 "}, "03": {" 小課題2 "}, "sample1": {" 小課題2 "},
	}) {
		t.Fatalf("membership: %v", got)
	}
	// Content issues are saved as drafts, so check only warns about them.
	for _, c := range []struct{ config, warning string }{
		{strings.Replace(scoringTOML, `"02", "03"]`, `"02"]`, 1), `non-sample cases in no group: "03"`},
		{strings.Replace(scoringTOML, `cases = ["01", "02"]`, ``, 1), `group "小課題1" lists no cases`},
		{strings.Split(scoringTOML, "[[tests.groups]]")[0], `[scoring] has no [[tests.groups]]`},
		{noAggregator, `mode = "aggregate" without [scoring.aggregator]`},
	} {
		write(t, filepath.Join(dir, "problem.toml"), template+c.config)
		var out strings.Builder
		if err := run(context.Background(), []string{"check", dir}, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "warning: "+c.warning) || strings.Count(out.String(), "warning:") != 1 || !strings.Contains(out.String(), "OK:") {
			t.Fatalf("missing warning %q: %s", c.warning, out.String())
		}
	}
	// A single group of 0 points is not, but a 0-point group next to others is, a valid structure.
	write(t, filepath.Join(dir, "problem.toml"), template+strings.Replace(scoringTOML, "points = 30", "points = 0", 1))
	if _, err := loadProblem(dir); err != nil {
		t.Fatal(err)
	}
	many := "[scoring]\nmode = \"groups\"\n"
	for i := range 101 {
		many += fmt.Sprintf("[[tests.groups]]\nname = \"g%d\"\npoints = 1\ncases = [\"01\"]\n", i)
	}
	for _, invalid := range []struct{ config, message string }{
		{strings.Replace(scoringTOML, `mode = "aggregate"`, `mode = "partial"`, 1), `scoring.mode must be "groups" or "aggregate"`},
		{strings.Replace(scoringTOML, `mode = "aggregate"`, `mode = "groups"`, 1), `[scoring.aggregator] requires mode = "aggregate"`},
		{strings.Replace(scoringTOML, `runtime = "cpp23-gcc"`, "runtime = \"cpp23-gcc\"\nprotocol = \"legacy\"", 1), "scoring aggregator: invalid runtime/protocol"},
		{strings.Replace(scoringTOML, `source = "aggregator.cpp"`, `source = "../aggregator.cpp"`, 1), "path must stay inside the problem directory"},
		{strings.Replace(scoringTOML, `" 小課題2 "`, `"小課題1 "`, 1), "group names must be non-blank, unique"},
		{strings.Replace(scoringTOML, `" 小課題2 "`, `"  "`, 1), "group names must be non-blank, unique"},
		{strings.Replace(scoringTOML, `" 小課題2 "`, `"a\tb"`, 1), "group names must be non-blank, unique"},
		{strings.Replace(scoringTOML, `" 小課題2 "`, `"`+strings.Repeat("あ", 65)+`"`, 1), "at most 64 characters"},
		{strings.Replace(scoringTOML, "points = 30", "points = -1", 1), `group "小課題1": points must be an integer 0..1000000`},
		{strings.Replace(scoringTOML, "points = 30", "points = 1000001", 1), `group "小課題1": points must be an integer 0..1000000`},
		{strings.Replace(scoringTOML, "points = 30", "points = 1.5", 1), "problem.toml:"},
		{strings.Replace(strings.Replace(scoringTOML, "points = 30", "points = 0", 1), "points = 70", "points = 0", 1), "the full score (sum of group points) must be 1..1000000, got 0"},
		{strings.Replace(scoringTOML, "points = 30", "points = 999931", 1), "the full score (sum of group points) must be 1..1000000, got 1000001"},
		{strings.Replace(scoringTOML, `["01", "02"]`, `["01", "04"]`, 1), `group "小課題1": no case matches "04"`},
		{strings.Replace(scoringTOML, `["01", "02"]`, `["01", "01.in"]`, 1), `group "小課題1": no case matches "01.in"`},
		{strings.Replace(scoringTOML, "points = 30", "points = 30\nscore = 1", 1), "problem.toml:"},
		{"\n[[tests.groups]]\nname = \"a\"\npoints = 1\ncases = [\"01\"]\n", `[[tests.groups]] requires [scoring]`},
		{many, "at most 100 [[tests.groups]]"},
	} {
		write(t, filepath.Join(dir, "problem.toml"), template+invalid.config)
		if _, err := loadProblem(dir); err == nil || !strings.Contains(err.Error(), invalid.message) {
			t.Errorf("want %q, got %v", invalid.message, err)
		}
	}
	// Groups list their cases in [tests], so [scoring] cannot replace the groups of remote tests.
	write(t, filepath.Join(dir, "problem.toml"), strings.Split(template, "[tests]")[0]+"[scoring]\nmode = \"groups\"\n")
	if _, err := loadProblem(dir); err == nil || !strings.Contains(err.Error(), "[scoring] requires [tests]") {
		t.Fatalf("scoring without tests: %v", err)
	}
}

// Samples and group cases are patterns over the file stems, so new files join without editing problem.toml.
func TestCasePatterns(t *testing.T) {
	dir := example(t)
	for _, name := range []string{"sample2", "a1", "a2", "b1"} {
		write(t, filepath.Join(dir, "tests", name+".in"), name+"\n")
		write(t, filepath.Join(dir, "tests", name+".out"), name+"\n")
	}
	tests := `samples = ["sample*"]

[scoring]
mode = "groups"

[[tests.groups]]
name = "A"
points = 40
cases = ["a?", "a1"]

[[tests.groups]]
name = "All"
points = 60
cases = ["[ab]*", "sample[^2]"]
`
	config := strings.Replace(template, `samples = ["sample1"]`+"\n", tests, 1)
	write(t, filepath.Join(dir, "problem.toml"), config)
	d, notes, err := readProblem(dir)
	if err != nil || len(notes) != 0 {
		t.Fatal(err, notes)
	}
	// A case that two patterns of one group match belongs to it once; sample2 is a sample in no group.
	if got := caseGroups(t, d); !reflect.DeepEqual(got, map[string][]string{
		"a1": {"A", "All"}, "a2": {"A", "All"}, "b1": {"All"}, "sample1": {"All"}, "sample2": nil,
	}) {
		t.Fatalf("membership: %v", got)
	}
	var cases []testCase
	_ = json.Unmarshal(d["testCases"], &cases)
	samples := []string{}
	for _, c := range cases {
		if c.IsSample {
			samples = append(samples, c.Name)
		}
	}
	if !reflect.DeepEqual(samples, []string{"sample1", "sample2"}) {
		t.Fatalf("samples: %v", samples)
	}
	// A pattern that matches nothing is likely a typo, so it stops check and push.
	for _, invalid := range []struct{ old, new, message string }{
		{`["sample*"]`, `["sample*", "x*"]`, `samples: no case matches "x*"`},
		{`["a?", "a1"]`, `["a?", "c*"]`, `group "A": no case matches "c*"`},
		{`["a?", "a1"]`, `["a["]`, `group "A": invalid pattern "a["`},
	} {
		write(t, filepath.Join(dir, "problem.toml"), strings.Replace(config, invalid.old, invalid.new, 1))
		if _, err := loadProblem(dir); err == nil || err.Error() != invalid.message {
			t.Errorf("want %q, got %v", invalid.message, err)
		}
	}
}

// fakeDrafts serves one problem's draft and its uploaded test files like the API and storage.
func fakeDrafts(t *testing.T, remote *problem, puts *int) *httptest.Server {
	files, pending, next := map[string]string{}, map[string]testFile{}, 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/my/problems/"), "/")
		switch {
		case strings.HasPrefix(r.URL.Path, "/object/"):
			fileID := strings.TrimPrefix(r.URL.Path, "/object/")
			if r.Method == http.MethodPut {
				data, _ := io.ReadAll(r.Body)
				files[fileID] = string(data)
				return
			}
			fmt.Fprint(w, files[fileID])
		case path == "" && r.Method == http.MethodGet:
			if remote.ID != id {
				w.WriteHeader(404)
				fmt.Fprint(w, `{"error":"problem_not_found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(remote)
		case path == "" && r.Method == http.MethodPut:
			*puts++
			var input problem
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Version != remote.Version {
				t.Errorf("save: %v %d", err, input.Version)
			}
			*remote = problem{ID: id, Version: input.Version + 1, Draft: input.Draft}
			_ = json.NewEncoder(w).Encode(remote)
		case path == "test-files":
			var ref testFile
			_ = json.NewDecoder(r.Body).Decode(&ref)
			next++
			ref.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", next)
			pending[ref.ID] = ref
			_ = json.NewEncoder(w).Encode(map[string]any{"id": ref.ID, "url": server.URL + "/object/" + ref.ID})
		case strings.HasSuffix(path, "/complete"):
			_ = json.NewEncoder(w).Encode(pending[strings.TrimSuffix(strings.TrimPrefix(path, "test-files/"), "/complete")])
		case strings.HasPrefix(path, "test-files/"):
			ref := pending[strings.TrimPrefix(path, "test-files/")]
			_ = json.NewEncoder(w).Encode(map[string]any{"url": server.URL + "/object/" + ref.ID, "size": ref.Size, "sha256": ref.SHA256})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestScoringPushAndPull(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "author")
	ctx := context.Background()
	var remote problem
	puts := 0
	dir := scoringExample(t)
	// An uploaded side must keep its case's groups when push rebuilds the cases.
	large := strings.Repeat("3\n", 40<<10)
	write(t, filepath.Join(dir, "tests/03.in"), large)
	server := fakeDrafts(t, &remote, &puts)
	client, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	var s state
	if s, err = push(ctx, dir, client, -1); err != nil || s.Version != 1 {
		t.Fatalf("create: %+v %v", s, err)
	}
	local, err := loadProblem(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(remote.Draft["scoring"]) != string(local["scoring"]) {
		t.Fatalf("scoring: %s", remote.Draft["scoring"])
	}
	var cases []testCase
	_ = json.Unmarshal(remote.Draft["testCases"], &cases)
	if len(cases) != 4 || cases[2].InputFile == nil || !reflect.DeepEqual(cases[2].Groups, []string{" 小課題2 "}) {
		t.Fatalf("uploaded case lost its groups: %+v", cases[2])
	}
	if !reflect.DeepEqual(caseGroups(t, remote.Draft), caseGroups(t, local)) {
		t.Fatal("membership changed on push")
	}
	// An omitted aggregator keeps the remote one, like other optional programs.
	aggregator := `{"runtime":"cpp23-gcc","source":"int main() {}\n"}`
	write(t, filepath.Join(dir, "problem.toml"), template+noAggregator)
	if _, err := push(ctx, dir, client, -1); err != nil || !strings.Contains(string(remote.Draft["scoring"]), `"aggregator":`+aggregator) {
		t.Fatalf("aggregator not kept: %s %v", remote.Draft["scoring"], err)
	}
	// Groups mode drops the aggregator, which the server allows only in aggregate mode.
	write(t, filepath.Join(dir, "problem.toml"), template+strings.Replace(noAggregator, `"aggregate"`, `"groups"`, 1))
	if _, err := push(ctx, dir, client, -1); err != nil || string(remote.Draft["scoring"]) != `{"mode":"groups","groups":[{"name":"小課題1","points":30},{"name":" 小課題2 ","points":70}]}` {
		t.Fatalf("groups mode: %s %v", remote.Draft["scoring"], err)
	}
	// Without [scoring], replacing the tests would leave every case ungrouped, so push stops before any request.
	write(t, filepath.Join(dir, "problem.toml"), template)
	before := puts
	if _, err := push(ctx, dir, client, -1); err == nil || !strings.Contains(err.Error(), "remote draft uses partial scoring") || puts != before {
		t.Fatalf("ungrouping push: %v", err)
	}
	// Without [tests] either, the remote scoring and membership stay.
	previous := string(remote.Draft["scoring"]) + string(remote.Draft["testCases"])
	write(t, filepath.Join(dir, "problem.toml"), strings.Split(template, "[tests]")[0])
	if s, err = push(ctx, dir, client, -1); err != nil || s.Version != 4 || string(remote.Draft["scoring"])+string(remote.Draft["testCases"]) != previous {
		t.Fatalf("keep remote scoring: %+v %v", s, err)
	}

	// Pull writes the scoring, the aggregator and each group's cases by stem, and push saves them unchanged.
	remote.Draft.set("scoring", scoring{Mode: "aggregate", Groups: []scoringGroup{{"小課題1", 30}, {" 小課題2 ", 70}, {"空", 0}}, Aggregator: &program{Runtime: "python3", Source: "print(0)\n"}})
	pulled := filepath.Join(t.TempDir(), "pulled")
	if _, err := pull(ctx, s.ID, pulled, client); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(pulled, "problem.toml"))
	for _, part := range []string{"[scoring]", "[scoring.aggregator]", "aggregator.txt", "[[tests.groups]]", "case001", "cases = []"} {
		if !strings.Contains(string(text), part) {
			t.Errorf("pulled problem.toml lacks %q:\n%s", part, text)
		}
	}
	got, notes, err := readProblem(pulled)
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0], `group "空" lists no cases`) {
		t.Fatalf("pulled: %v %v", notes, err)
	}
	if string(got["scoring"]) != string(remote.Draft["scoring"]) || !reflect.DeepEqual(caseGroups(t, got), caseGroups(t, remote.Draft)) {
		t.Fatalf("pulled scoring: %s", got["scoring"])
	}
	previous = string(remote.Draft["scoring"])
	membership := caseGroups(t, remote.Draft)
	if _, err := push(ctx, pulled, client, -1); err != nil || string(remote.Draft["scoring"]) != previous || !reflect.DeepEqual(caseGroups(t, remote.Draft), membership) {
		t.Fatalf("push pulled scoring: %s %v", remote.Draft["scoring"], err)
	}
	// Drafts the CLI cannot represent leave no directory behind.
	for _, invalid := range []any{
		nil, // cases still list groups
		scoring{Mode: "groups", Groups: []scoringGroup{{"小課題1", 30}}},
		scoring{Mode: "groups", Groups: []scoringGroup{{"小課題1", 30}, {" 小課題2 ", 70}, {"小課題1", 1}}},
	} {
		remote.Draft.set("scoring", invalid)
		target := filepath.Join(t.TempDir(), "invalid")
		if _, err := pull(ctx, s.ID, target, client); err == nil {
			t.Fatalf("invalid scoring pulled: %v", invalid)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("invalid draft left output")
		}
	}
}

func TestScoringPushMerge(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "author")
	ctx := context.Background()
	var remote problem
	puts, uploads := 0, 0
	dir := scoringExample(t)
	write(t, filepath.Join(dir, "tests/03.in"), strings.Repeat("3\n", 40<<10))
	server := fakeDrafts(t, &remote, &puts)
	client, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	next := server.Client().Transport
	client.http = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/test-files") {
			uploads++
		}
		return next.RoundTrip(r)
	})}
	if _, err := push(ctx, dir, client, -1); err != nil || uploads != 1 {
		t.Fatalf("create: %v uploads=%d", err, uploads)
	}
	// Ungrouping the remote cases stops push before it uploads or saves anything.
	write(t, filepath.Join(dir, "problem.toml"), template)
	beforePuts, beforeUploads := puts, uploads
	if _, err := push(ctx, dir, client, -1); err == nil || err.Error() != "the remote draft uses partial scoring: add [scoring] and [[tests.groups]] to problem.toml (shareoj pull writes them) or omit [tests]; switching back to all-or-nothing is done on the website" || puts != beforePuts || uploads != beforeUploads {
		t.Fatalf("ungrouping push: %v puts=%d uploads=%d", err, puts-beforePuts, uploads-beforeUploads)
	}
	// A version conflict is reported before the merge errors.
	remote.Version++
	_, err = push(ctx, dir, client, -1)
	remote.Version--
	if err == nil || err.Error() != "version conflict: local 1, remote 2; review the web draft before using --expect-version 2" {
		t.Fatalf("conflict: %v", err)
	}
	// An omitted aggregator needs a valid remote scoring, so an invalid one stops push before any upload or save.
	remote.Draft["scoring"] = json.RawMessage(`"x"`)
	write(t, filepath.Join(dir, "problem.toml"), template+noAggregator)
	if _, err := push(ctx, dir, client, -1); err == nil || err.Error() != "invalid scoring in the remote draft" || puts != beforePuts || uploads != beforeUploads {
		t.Fatalf("invalid remote scoring: %v puts=%d uploads=%d", err, puts-beforePuts, uploads-beforeUploads)
	}
	// A null remote scoring is all-or-nothing, so [tests] without [scoring] is saved.
	remote.Draft["scoring"] = json.RawMessage("null")
	write(t, filepath.Join(dir, "problem.toml"), template)
	if _, err := push(ctx, dir, client, -1); err != nil || string(remote.Draft["scoring"]) != "null" || puts != beforePuts+1 {
		t.Fatalf("null scoring: %s %v", remote.Draft["scoring"], err)
	}
	// An invalid remote generators value stops push before any upload or save.
	remote.Draft["generators"] = json.RawMessage(`"x"`)
	write(t, filepath.Join(dir, "problem.toml"), template+scoringTOML)
	beforePuts, beforeUploads = puts, uploads
	var typeErr *json.UnmarshalTypeError
	if _, err := push(ctx, dir, client, -1); !errors.As(err, &typeErr) || puts != beforePuts || uploads != beforeUploads {
		t.Fatalf("invalid remote generators: %v puts=%d uploads=%d", err, puts-beforePuts, uploads-beforeUploads)
	}
}
