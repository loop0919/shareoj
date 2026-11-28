package submissions

import "strings"

type Runtime struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Language string `json:"language"` // the key of its Language
}

type Language struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Runtimes []Runtime `json:"runtimes"` // every runtime of the language, retired ones included, in catalogue order
}

// languageLabels is the only place that names the languages. Each label is the
// leading name shared by the labels of its runtimes.
var languageLabels = map[string]string{
	"c": "C", "cpp": "C++", "python": "Python", "rust": "Rust", "java": "Java",
	"csharp": "C#", "nim": "Nim", "go": "Go", "goose": "Goose", "haskell": "Haskell",
	"commonlisp": "Common Lisp", "fsharp": "F#", "gleam": "Gleam", "fix": "Fix",
	"javascript": "JavaScript", "typescript": "TypeScript", "ruby": "Ruby",
	"clay": "cLay", "dc": "dc", "nibbles": "Nibbles", "brainfuck": "Brainfuck",
	"whitespace": "Whitespace", "text": "Text",
}

// Installation and publication are separate. The API defaults to C++17 only.
var Runtimes = []Runtime{
	{"cpp17", "C++17 (GCC)", "cpp"},
	{"c23-gcc", "C23 (GCC)", "c"}, {"c23-clang", "C23 (Clang)", "c"},
	{"cpp23-gcc", "C++23 (GCC)", "cpp"}, {"cpp23-clang", "C++23 (Clang)", "cpp"},
	{"python314", "Python (CPython 3.14)", "python"}, {"pypy311", "Python (PyPy 3.11)", "python"},
	{"codon020", "Python (Codon 0.20)", "python"}, {"rust2024", "Rust (Edition 2024)", "rust"},
	{"java24", "Java (OpenJDK 24)", "java"},
	{"java25", "Java (Temurin 25)", "java"}, {"csharp14", "C# 14 (.NET 10)", "csharp"},
	{"nim22", "Nim 2.2", "nim"}, {"go127", "Go 1.27", "go"},
	{"goose20263", "Goose 2026.3", "goose"},
	{"haskell-ghc910", "Haskell (GHC 9.10 LTS)", "haskell"},
	{"commonlisp-sbcl26", "Common Lisp (SBCL 2.6)", "commonlisp"},
	{"fsharp10", "F# 10 (.NET 10)", "fsharp"},
	{"gleam119", "Gleam 1.19 (Erlang/OTP 29)", "gleam"},
	{"fix16", "Fix 1.6", "fix"},
	{"javascript-node24", "JavaScript (Node.js 24 LTS)", "javascript"},
	{"typescript-node24", "TypeScript (Node.js 24 LTS)", "typescript"},
	{"javascript-deno29", "JavaScript (Deno 2.9)", "javascript"},
	{"typescript-deno29", "TypeScript (Deno 2.9)", "typescript"},
	{"javascript-bun14", "JavaScript (Bun 1.4)", "javascript"},
	{"typescript-bun14", "TypeScript (Bun 1.4)", "typescript"},
	{"ruby40", "Ruby (CRuby 4.0)", "ruby"},
	{"ruby-truffle40", "Ruby (TruffleRuby 40)", "ruby"},
	{"clay20250308", "cLay (20250308-1)", "clay"},
	{"dc152", "dc (GNU dc 1.5.2)", "dc"},
	{"nibbles101", "Nibbles (1.01+)", "nibbles"},
	{"brainfuck-tritium", "Brainfuck (Tritium)", "brainfuck"},
	{"whitespace13", "Whitespace (whitespacers 1.3)", "whitespace"},
	{"text", "Text (cat 9.4)", "text"},
}

// Languages lists every language in catalogue order.
func Languages() []Language {
	languages := make([]Language, 0, len(languageLabels))
	index := map[string]int{}
	for _, runtime := range Runtimes {
		i, ok := index[runtime.Language]
		if !ok {
			i = len(languages)
			index[runtime.Language] = i
			languages = append(languages, Language{Key: runtime.Language, Label: languageLabels[runtime.Language], Runtimes: []Runtime{}})
		}
		languages[i].Runtimes = append(languages[i].Runtimes, runtime)
	}
	return languages
}

// LanguageRuntimeIDs gives the catalogue IDs of a language's runtimes; ok is false for an unknown key.
func LanguageRuntimeIDs(key string) (ids []string, ok bool) {
	for _, runtime := range Runtimes {
		if runtime.Language == key {
			ids = append(ids, runtime.ID)
		}
	}
	return ids, len(ids) > 0
}

// KnownRuntime tells whether id is a catalogue ID (without a suffix).
func KnownRuntime(id string) bool {
	for _, runtime := range Runtimes {
		if runtime.ID == id {
			return true
		}
	}
	return false
}

// StoredRuntimeIDs expands catalogue IDs to every form a stored submission may carry (bare, -isolate, -local).
func StoredRuntimeIDs(ids []string) []string {
	stored := make([]string, 0, len(ids)*3)
	for _, id := range ids {
		stored = append(stored, id, id+"-isolate", id+"-local")
	}
	return stored
}

func IsolateRuntimeIDs() []string {
	ids := make([]string, 0, len(Runtimes))
	for _, runtime := range Runtimes {
		ids = append(ids, runtime.ID+"-isolate")
	}
	return ids
}

func PublishedRuntimes(enabled string) []Runtime {
	if enabled == "" {
		enabled = "cpp17"
	}
	selected := make(map[string]bool)
	for _, id := range strings.Split(enabled, ",") {
		selected[strings.TrimSpace(id)] = true
	}
	result := make([]Runtime, 0)
	for _, runtime := range Runtimes {
		if selected[runtime.ID] {
			result = append(result, runtime)
		}
	}
	return result
}
