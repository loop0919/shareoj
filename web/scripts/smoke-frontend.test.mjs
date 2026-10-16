import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { test } from 'node:test'
import { smokeFrontend } from './smoke-frontend.mjs'

async function site(t, overrides = {}) {
  const paths = []
  const server = createServer((req, res) => {
    paths.push(req.url)
    if (overrides[req.url]) return overrides[req.url](res)
    if (req.url === '/problems') return res.end(`<title>ShareOJ</title><link rel="canonical" href="http://${req.headers.host}/problems"><script src="/app.js"></script><link href="/app.css">`)
    if (req.url === '/blog') return res.end('ShareOJ: 公開された記事はまだありません。')
    if (req.url === '/api/problems' || req.url === '/api/posts') return res.end(JSON.stringify({ items: [], nextCursor: '' }))
    if (req.url === '/docs/markdown') return res.end('<span class="katex-html">math</span>')
    if (['/problems/new', '/blog/new', '/login', '/signup'].includes(req.url)) return res.end('<meta name="robots" content="noindex, nofollow">')
    if (req.url === '/app.js' || req.url === '/app.css') return res.end('/* asset */')
    res.writeHead(404).end()
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => new Promise(resolve => { server.closeAllConnections(); server.close(resolve) }))
  return { url: `http://127.0.0.1:${server.address().port}`, paths }
}

test('a fresh database passes without a seeded problem', async t => {
  const { url, paths } = await site(t)
  await smokeFrontend(url, { attempts: 1 })
  assert.ok(!paths.includes('/problems/a-plus-b'))
  assert.ok(paths.includes('/api/posts'))
})

test('a database failure cannot pass as an empty catalogue', async t => {
  const { url } = await site(t, { '/api/posts': res => res.writeHead(503).end('{}') })
  await assert.rejects(smokeFrontend(url, { attempts: 1 }), /\/api\/posts: HTTP 503/)
})

test('a missing JavaScript bundle fails deployment verification', async t => {
  const { url } = await site(t, { '/app.js': res => res.writeHead(404).end() })
  await assert.rejects(smokeFrontend(url, { attempts: 1 }), /\/app.js: HTTP 404/)
})
