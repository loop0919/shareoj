import { createServer } from 'node:http'

// Sits in front of tests/fixtures/public-api.mjs so the guide screenshots match production:
// the languages published in infra/api/runtime.auto.tfvars instead of the test-only C++17, and no hidden 定期便 labels.
const upstream = 'http://127.0.0.1:18080'
const runtimes = [
  ['c23-gcc', 'C23 (GCC)'], ['c23-clang', 'C23 (Clang)'], ['cpp23-gcc', 'C++23 (GCC)'], ['cpp23-clang', 'C++23 (Clang)'],
  ['python314', 'Python (CPython 3.14)'], ['pypy311', 'Python (PyPy 3.11)'], ['codon020', 'Python (Codon 0.20)'],
  ['rust2024', 'Rust (Edition 2024)'], ['java25', 'Java (Temurin 25)'], ['csharp14', 'C# 14 (.NET 10)'],
  ['nim22', 'Nim 2.2'], ['go127', 'Go 1.27'], ['haskell-ghc910', 'Haskell (GHC 9.10 LTS)'],
  ['javascript-node24', 'JavaScript (Node.js 24 LTS)'], ['typescript-node24', 'TypeScript (Node.js 24 LTS)'],
  ['javascript-deno29', 'JavaScript (Deno 2.9)'], ['typescript-deno29', 'TypeScript (Deno 2.9)'],
  ['javascript-bun14', 'JavaScript (Bun 1.4)'], ['typescript-bun14', 'TypeScript (Bun 1.4)'],
  ['ruby40', 'Ruby (CRuby 4.0)'], ['ruby-truffle40', 'Ruby (TruffleRuby 40)'],
].map(([id, label]) => ({ id, label }))

createServer(async (req, res) => {
  const path = new URL(req.url, 'http://localhost').pathname
  res.setHeader('Content-Type', 'application/json')
  if (path === '/runtimes') return res.end(JSON.stringify({ items: runtimes }))
  let body
  if (req.method !== 'GET' && req.method !== 'HEAD') {
    const chunks = []
    for await (const chunk of req) chunks.push(chunk)
    body = Buffer.concat(chunks)
  }
  const headers = { ...req.headers }
  delete headers.host
  const response = await fetch(upstream + req.url, { method: req.method, headers, body })
  let text = (await response.text()).replaceAll('"cpp17"', '"cpp23-gcc"')
  // Hard problems of Lv.9 and above get a label the guide does not announce.
  if (path === '/featured' && response.ok) text = text.replaceAll('"difficulty":9', '"difficulty":8')
  res.writeHead(response.status).end(text)
}).listen(18082, '127.0.0.1', () => console.log('docs screenshot API ready'))
