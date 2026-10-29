// Keep historical submissions readable even when a runtime is no longer published.
const runtimeLabels = new Map([
  ['cpp17', 'C++17 (GCC)'],
  ['c23-gcc', 'C23 (GCC)'],
  ['c23-clang', 'C23 (Clang)'],
  ['cpp23-gcc', 'C++23 (GCC)'],
  ['cpp23-clang', 'C++23 (Clang)'],
  ['python314', 'Python (CPython 3.14)'],
  ['pypy311', 'Python (PyPy 3.11)'],
  ['codon020', 'Python (Codon 0.20)'],
  ['rust2024', 'Rust (Edition 2024)'],
  ['java24', 'Java (OpenJDK 24)'],
  ['java25', 'Java (Temurin 25)'],
  ['csharp14', 'C# 14 (.NET 10)'],
  ['nim22', 'Nim 2.2'],
  ['go127', 'Go 1.27'],
  ['goose20263', 'Goose 2026.3'],
  ['haskell-ghc910', 'Haskell (GHC 9.10 LTS)'],
  ['javascript-node24', 'JavaScript (Node.js 24 LTS)'],
  ['typescript-node24', 'TypeScript (Node.js 24 LTS)'],
  ['javascript-deno29', 'JavaScript (Deno 2.9)'],
  ['typescript-deno29', 'TypeScript (Deno 2.9)'],
  ['javascript-bun14', 'JavaScript (Bun 1.4)'],
  ['typescript-bun14', 'TypeScript (Bun 1.4)'],
  ['ruby40', 'Ruby (CRuby 4.0)'],
  ['ruby-truffle40', 'Ruby (TruffleRuby 40)'],
  ['clay20250308', 'cLay (20250308-1)'],
  ['dc152', 'dc (GNU dc 1.5.2)'],
  ['nibbles101', 'Nibbles (1.01+)'],
  ['brainfuck-tritium', 'Brainfuck (Tritium)'],
  ['whitespace13', 'Whitespace (whitespacers 1.3)'],
  ['text', 'Text (cat 9.4)'],
])

export function runtimeLabel(runtime: string) {
  return runtimeLabels.get(runtime.replace(/-(isolate|local)$/, '')) ?? runtime
}
