import assert from 'node:assert/strict'
import { pathToFileURL } from 'node:url'

export async function smokeFrontend(siteUrl, { attempts = 12, retryDelayMs = 5000 } = {}) {
  const origin = new URL(siteUrl)
  async function get(path) {
    let response
    for (let attempt = 0; attempt < attempts; attempt++) {
      try {
        response = await fetch(new URL(path, origin), { signal: AbortSignal.timeout(30000), redirect: 'error' })
        if (response.ok) return response
      } catch (error) {
        if (attempt === attempts - 1) throw error
      }
      if (attempt < attempts - 1) await new Promise(resolve => setTimeout(resolve, retryDelayMs))
    }
    throw new Error(`${path}: HTTP ${response?.status}`)
  }

  // A newly migrated database legitimately has no published problems or posts.
  // Check the DB-backed catalogues, rather than assuming a seeded problem exists.
  const html = await (await get('/problems')).text()
  assert.match(html, /ShareOJ/)
  assert.ok(html.includes(`${origin.origin}/problems`), 'Public canonical URL missing')
  for (const extension of ['js', 'css']) {
    const path = html.match(new RegExp(`(?:src|href)="([^" ]+\\.${extension})"`))?.[1]
    assert.ok(path, `${extension} asset missing`)
    assert.equal(new URL(path, origin).origin, origin.origin, 'Unexpected asset origin')
    assert.ok((await (await get(path)).arrayBuffer()).byteLength > 0)
  }
  assert.match(await (await get('/blog')).text(), /ShareOJ/)
  for (const path of ['/api/problems', '/api/posts']) {
    const catalogue = await (await get(path)).json()
    assert.ok(Array.isArray(catalogue.items), `${path}: items missing`)
    assert.equal(typeof catalogue.nextCursor, 'string', `${path}: cursor missing`)
  }
  // The guide exercises server-side math rendering without depending on user data.
  assert.match(await (await get('/docs/markdown')).text(), /katex-html/)
  for (const path of ['/problems/new', '/blog/new', '/login', '/signup']) {
    assert.match(await (await get(path)).text(), /noindex/)
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await smokeFrontend(process.argv[2])
  console.log(`Frontend SSR, database catalogues, assets, and editors verified: ${new URL(process.argv[2]).origin}`)
}
