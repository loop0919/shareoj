package submissions

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestPublishedRuntimes(t *testing.T) {
	if got := PublishedRuntimes(""); len(got) != 1 || got[0].ID != "cpp17" {
		t.Fatal(got)
	}
	if got := PublishedRuntimes("none"); len(got) != 0 {
		t.Fatal(got)
	}
	got := PublishedRuntimes("cpp17,c23-gcc,java24,unknown,c23-gcc")
	if len(got) != 3 || got[1].ID != "c23-gcc" || got[2].ID != "java24" {
		t.Fatal(got)
	}
}

// TestCatalogueKeysMatchTheWebShape checks every language key and runtime ID against the shape the web accepts in a
// list's query (keyShape, isLanguageKey and isRuntimeId in web/shared/utils/submission-conditions.ts), so that a key
// or ID the web could not narrow a list to fails here. Change the two together.
func TestCatalogueKeysMatchTheWebShape(t *testing.T) {
	webShape := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	for _, language := range Languages() {
		if !webShape.MatchString(language.Key) {
			t.Errorf("language key %q does not match %s", language.Key, webShape)
		}
	}
	for _, runtime := range Runtimes {
		if !webShape.MatchString(runtime.ID) || strings.HasSuffix(runtime.ID, "-isolate") || strings.HasSuffix(runtime.ID, "-local") {
			t.Errorf("runtime ID %q does not match %s, or ends in -isolate or -local", runtime.ID, webShape)
		}
	}
}

func TestLanguagesGroupRuntimes(t *testing.T) {
	keyPattern := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	seen := map[string]string{}
	multi := []string{}
	for _, language := range Languages() {
		if !keyPattern.MatchString(language.Key) || strings.HasSuffix(language.Key, "-isolate") || strings.HasSuffix(language.Key, "-local") {
			t.Fatalf("bad language key %q", language.Key)
		}
		if language.Label == "" || len(language.Runtimes) == 0 {
			t.Fatalf("empty language: %+v", language)
		}
		if len(language.Runtimes) > 1 {
			multi = append(multi, language.Key)
		}
		for _, runtime := range language.Runtimes {
			if runtime.Language != language.Key {
				t.Fatalf("%s listed under %s but says %s", runtime.ID, language.Key, runtime.Language)
			}
			if previous, ok := seen[runtime.ID]; ok {
				t.Fatalf("%s listed under %s and %s", runtime.ID, previous, language.Key)
			}
			seen[runtime.ID] = language.Key
		}
	}
	if len(seen) != len(Runtimes) {
		t.Fatalf("languages cover %d of %d runtimes", len(seen), len(Runtimes))
	}
	for _, runtime := range Runtimes {
		if seen[runtime.ID] != runtime.Language {
			t.Fatalf("%s missing from its language", runtime.ID)
		}
	}
	want := []string{"c", "cpp", "java", "javascript", "python", "ruby", "typescript"}
	sort.Strings(multi)
	if !reflect.DeepEqual(multi, want) {
		t.Fatalf("multi-runtime languages: %v", multi)
	}

	cpp, ok := LanguageRuntimeIDs("cpp")
	if !ok || !reflect.DeepEqual(cpp, []string{"cpp17", "cpp23-gcc", "cpp23-clang"}) {
		t.Fatalf("cpp: %v %v", cpp, ok)
	}
	c, _ := LanguageRuntimeIDs("c")
	if !reflect.DeepEqual(c, []string{"c23-gcc", "c23-clang"}) {
		t.Fatalf("c: %v", c)
	}
	python, _ := LanguageRuntimeIDs("python")
	if !reflect.DeepEqual(python, []string{"python314", "pypy311", "codon020"}) {
		t.Fatalf("python: %v", python)
	}
	js, _ := LanguageRuntimeIDs("javascript")
	ts, _ := LanguageRuntimeIDs("typescript")
	if len(js) != 3 || len(ts) != 3 || js[0] == ts[0] {
		t.Fatalf("javascript %v typescript %v", js, ts)
	}
	if ids, ok := LanguageRuntimeIDs("cobol"); ok || ids != nil {
		t.Fatalf("cobol: %v %v", ids, ok)
	}
	if !KnownRuntime("cpp23-gcc") || KnownRuntime("cpp23-gcc-isolate") || KnownRuntime("cobol") {
		t.Fatal("KnownRuntime")
	}
	if got := StoredRuntimeIDs([]string{"cpp17"}); !reflect.DeepEqual(got, []string{"cpp17", "cpp17-isolate", "cpp17-local"}) {
		t.Fatalf("stored: %v", got)
	}
	labels := map[string]string{}
	for _, language := range Languages() {
		labels[language.Key] = language.Label
	}
	if labels["rust"] != "Rust" || labels["cpp"] != "C++" || labels["commonlisp"] != "Common Lisp" || labels["csharp"] != "C#" {
		t.Fatalf("labels: %v", labels)
	}
	if rust, _ := LanguageRuntimeIDs("rust"); !reflect.DeepEqual(rust, []string{"rust2024"}) {
		t.Fatalf("rust: %v", rust)
	}
}
