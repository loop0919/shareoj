import { readFileSync, readdirSync } from 'node:fs'
import { join, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { APIRequestContext } from '@playwright/test'
import { expect, test } from '@playwright/test'

const problemId = '11111111-1111-4111-8111-111111111111'
const submissionId = '22222222-2222-4222-8222-222222222222'
const featuredId = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const imageId = '99999999-9999-4999-8999-999999999999'
const origin = 'https://judge.example'
const on = 'openoj_access=valid-access; openoj-streaming=1'
const off = 'openoj_access=valid-access'

test('a streaming request is refused on every route not marked', async ({ request }) => {
  for (const path of ['/api/my/contests/drafts', '/api/my/notifications', '/api/my/problems', '/api/my/contest-schedule', '/api/my/account']) {
    const response = await request.get(path, { headers: { cookie: on } })
    expect(response.status(), path).toBe(403)
    expect((await response.json()).data).toEqual({ code: 'streaming_mode' })
    expect((await request.get(path, { headers: { cookie: off } })).status(), path).not.toBe(403)
  }
  const update = await request.put('/api/my/profile', { headers: { cookie: on, origin }, data: { handle: 'alice' } })
  expect(update.status()).toBe(403)
  // The origin check answers 403 too, without the code.
  expect((await update.json()).data).toEqual({ code: 'streaming_mode' })
})

test('a closed call falls back to the public view', async ({ request }) => {
  // The fixture session is the author of the featured problem, whose editorial is still hidden from everyone else.
  const usual = await (await request.get(`/api/problems/${featuredId}`, { headers: { cookie: off } })).json()
  expect(usual.editorial).toBe('定期便の解説本文')
  const response = await request.get(`/api/problems/${featuredId}`, { headers: { cookie: on } })
  expect(response.status()).toBe(200)
  const streaming = await response.json()
  expect(streaming.title).toBe('定期便の新作')
  expect(JSON.stringify(streaming)).not.toContain('定期便の解説本文')
})

// The Set-Cookie headers of a response for the streaming cookie, each split into its attributes.
async function streamingSetCookies(response: Awaited<ReturnType<APIRequestContext['get']>>) {
  return response.headersArray().filter(({ name, value }) => name.toLowerCase() === 'set-cookie' && value.startsWith('openoj-streaming=')).map(({ value }) => value.split(';').map(part => part.trim()))
}

test('a page in streaming mode renews the cookie for a year', async ({ request }) => {
  // A public page, a closed page and a signed-in page: every document the server renders.
  for (const [path, cookie] of [['/', 'openoj-streaming=1'], ['/my', `${off}; openoj-streaming=1`], ['/problems', `${off}; openoj-streaming=1`]] as const) {
    const renewed = await streamingSetCookies(await request.get(path, { headers: { cookie } }))
    expect(renewed, path).toHaveLength(1)
    const [value, ...attributes] = renewed[0]!
    expect(value, path).toBe('openoj-streaming=1')
    expect(attributes, path).toEqual(expect.arrayContaining(['Max-Age=31536000', 'Path=/', 'SameSite=Lax', 'Secure']))
    expect(attributes.map(attribute => attribute.toLowerCase()), path).not.toContain('httponly')
  }
})

test('the server never sets the streaming cookie on its own', async ({ request }) => {
  for (const cookie of ['', off, 'openoj-streaming=0', `${off}; openoj-streaming=true`]) {
    expect(await streamingSetCookies(await request.get('/', { headers: { cookie } })), cookie).toEqual([])
  }
  // API responses leave it alone even in streaming mode.
  for (const path of ['/api/auth/me', '/api/my/profile', `/api/problems/${problemId}`]) {
    expect(await streamingSetCookies(await request.get(path, { headers: { cookie: on } })), path).toEqual([])
  }
})

test('a streaming request keeps the marked routes', async ({ request }) => {
  for (const path of ['/api/auth/me', '/api/my/profile', '/api/my/solved-problems', '/api/my/code-runs/latest', `/api/my/favorites/${problemId}`, `/api/my/difficulty-votes/${problemId}`, `/api/my/submissions/${submissionId}`]) {
    expect((await request.get(path, { headers: { cookie: on } })).status(), path).not.toBe(403)
  }
  const submitted = await request.post('/api/my/submissions', { headers: { cookie: on, origin }, data: { problemId, runtime: 'cpp23-gcc', source: 'int main(){}' } })
  expect(submitted.status()).toBe(202)
})

test('only the value 1 turns streaming mode on', async ({ request }) => {
  for (const value of ['0', 'true', '']) {
    const response = await request.get('/api/my/notifications', { headers: { cookie: `${off}; openoj-streaming=${value}` } })
    expect(response.status(), value).not.toBe(403)
  }
})

test('a session with only the refresh cookie still answers in streaming mode', async ({ request }) => {
  const response = await request.get('/api/auth/me', { headers: { cookie: 'openoj_refresh=valid-refresh; openoj-streaming=1' } })
  expect(response.status()).toBe(200)
  expect(await response.json()).toEqual({ user: { id: 'session-user' } })
})

test('a submission with only the refresh cookie still goes through in streaming mode', async ({ request }) => {
  const response = await request.post('/api/my/submissions', { headers: { cookie: 'openoj_refresh=valid-refresh; openoj-streaming=1', origin }, data: { problemId, runtime: 'cpp23-gcc', source: 'int main(){}' } })
  expect(response.status()).toBe(202)
  // The web server refreshed the session to send it.
  expect(response.headersArray().some(({ name, value }) => name.toLowerCase() === 'set-cookie' && value.startsWith('openoj_access=valid-access;'))).toBe(true)
})

test('login works in streaming mode', async ({ request }) => {
  const response = await request.post('/api/auth/login', {
    headers: { origin, cookie: 'openoj-streaming=1' }, data: { username: 'user', password: 'password' },
  })
  expect(response.ok()).toBe(true)
  expect(await response.json()).toEqual({ user: { id: 'session-user' } })
})

test('the own list of a problem stays private, the whole list and a detail turn public', async ({ request }) => {
  const detail = `/api/problems/${featuredId}/submissions/33333333-3333-4333-8333-333333333333?mine=1`
  const own = await request.get(`/api/problems/${featuredId}/submissions?mine=1`, { headers: { cookie: on } })
  expect(own.status()).toBe(200)
  expect(JSON.stringify(await own.json())).toContain('carol')
  // Without the cookie the detail is the private one, so the public answers below are not the fixture's default.
  expect(await (await request.get(detail, { headers: { cookie: off } })).text()).toContain('carol')
  for (const path of [`/api/problems/${featuredId}/submissions?mine=0`, detail]) {
    const response = await request.get(path, { headers: { cookie: on } })
    expect(response.status(), path).toBe(200)
    const text = await response.text()
    expect(text, path).toContain('"route":"public"')
    expect(text, path).not.toContain('carol')
  }
})

test('an image is read from the public route in streaming mode', async ({ request }) => {
  const draft = await request.get(`/api/images/${imageId}`, { headers: { cookie: off } })
  expect((await draft.body()).toString('latin1')).toContain('private')
  const response = await request.get(`/api/images/${imageId}`, { headers: { cookie: on } })
  expect(response.status()).toBe(200)
  expect(response.headers()['content-type']).toBe('image/png')
  expect((await response.body()).toString('latin1')).toContain('public')
})

test('a signed-out visitor still reads images in streaming mode', async ({ request }) => {
  const response = await request.get(`/api/images/${imageId}`, { headers: { cookie: 'openoj-streaming=1' } })
  expect(response.status()).toBe(200)
  expect(response.headers()['content-type']).toBe('image/png')
  expect((await response.body()).toString('latin1')).toContain('public')
})

const staffContest = 'bcbcbcbc-bcbc-4cbc-8cbc-bcbcbcbcbcbc'
const brokenContest = 'bdbdbdbd-bdbd-4dbd-8dbd-bdbdbdbdbdbd'

test('staff of a contest get the public answers in streaming mode', async ({ request }) => {
  for (const path of ['', '/submissions', `/submissions/${submissionId}`, `/problems/${problemId}`, `/problems/${problemId}/submissions`, '/standings?live=1']) {
    const streaming = await (await request.get(`/api/contests/${staffContest}${path}`, { headers: { cookie: on } })).json()
    expect(streaming.route, path).toBe('public')
    const usual = await (await request.get(`/api/contests/${staffContest}${path}`, { headers: { cookie: off } })).json()
    expect(usual.route ?? 'private', path).toBe('private')
  }
  for (const mine of ['1', 'team']) {
    const own = await request.get(`/api/contests/${staffContest}/submissions?mine=${mine}`, { headers: { cookie: on } })
    expect(own.status()).toBe(403)
    expect((await own.json()).data).toEqual({ code: 'streaming_mode' })
  }
})

test('a participant keeps the private answers in streaming mode, but never the live standings', async ({ request }) => {
  const contest = '66666666-6666-4666-8666-666666666666'
  for (const path of ['', '/submissions', '/submissions?mine=1', '/submissions?mine=team', `/problems/${problemId}`]) {
    expect((await (await request.get(`/api/contests/${contest}${path}`, { headers: { cookie: on } })).json()).route, path).toBe('private')
  }
  // Without the cookie the same call goes to the private route, so the public answer below comes from streaming mode.
  expect((await (await request.get(`/api/contests/${contest}/standings?live=1`, { headers: { cookie: off } })).json()).route).toBe('private')
  expect((await (await request.get(`/api/contests/${contest}/standings?live=1`, { headers: { cookie: on } })).json()).route).toBe('public')
})

test('a failed staff check uses neither route', async ({ request }) => {
  const response = await request.get(`/api/contests/${brokenContest}/submissions`, { headers: { cookie: on } })
  expect(response.status()).toBe(502)
  // The public route would have answered, so the 502 comes from the failed check alone.
  expect((await (await request.get(`/api/contests/${brokenContest}/submissions`, { headers: { cookie: 'openoj-streaming=1' } })).json()).route).toBe('public')
})

test('an expired session reads the public route', async ({ request }) => {
  const response = await request.get(`/api/contests/${staffContest}/submissions`, { headers: { cookie: 'openoj_access=expired; openoj-streaming=1' } })
  expect(response.status()).toBe(200)
  expect((await response.json()).route).toBe('public')
})

test('the own submission detail drops the checker diagnostics in streaming mode', async ({ request }) => {
  const hidden = 'c1c1c1c1-c1c1-4c1c-8c1c-c1c1c1c1c1c1'
  const sample = 'c2c2c2c2-c2c2-4c2c-8c2c-c2c2c2c2c2c2'
  const usual = await (await request.get(`/api/my/submissions/${hidden}`, { headers: { cookie: off } })).json()
  expect(usual.result.checkerLog).toBe('secret-checker-log')
  const streaming = await (await request.get(`/api/my/submissions/${hidden}`, { headers: { cookie: on } })).json()
  expect(streaming.result.verdict).toBe('AC')
  expect(streaming.result).not.toHaveProperty('checkerLog')
  const easy = await (await request.get(`/api/my/submissions/${sample}`, { headers: { cookie: on } })).json()
  expect(easy.result.checkerLog).toBe('sample-diagnostics')
  // A sample check of a plain problem carries what only its manager may read, so it goes too.
  const manager = 'c3c3c3c3-c3c3-4c3c-8c3c-c3c3c3c3c3c3'
  expect((await (await request.get(`/api/my/submissions/${manager}`, { headers: { cookie: off } })).json()).result.checkerLog).toBe('manager-diagnostics')
  const plain = await (await request.get(`/api/my/submissions/${manager}`, { headers: { cookie: on } })).json()
  expect(plain.easyTest).toBe(true)
  expect(plain.result).not.toHaveProperty('checkerLog')
})

test('the own submission detail drops the generated outputs in streaming mode', async ({ request }) => {
  const generated = 'c4c4c4c4-c4c4-4c4c-8c4c-c4c4c4c4c4c4'
  const usual = await (await request.get(`/api/my/submissions/${generated}`, { headers: { cookie: off } })).json()
  expect(usual.result.cases[0].output).toBe('secret-output')
  expect(usual.result.cases[0].outputFile.url).toBe('https://download.example/secret')
  const streaming = await (await request.get(`/api/my/submissions/${generated}`, { headers: { cookie: on } })).json()
  expect(streaming.result.cases).toEqual([{ name: 'gen_1', verdict: 'AC' }])
})

test('the standings and the editorials answer publicly without a staff check', async ({ request }) => {
  // This contest's check fails, so a check here would turn the answer into a 502.
  for (const path of ['/standings', '/standings?live=1', '/editorials']) {
    const response = await request.get(`/api/contests/${brokenContest}${path}`, { headers: { cookie: on } })
    expect(response.status(), path).toBe(200)
    expect((await response.json()).route, path).toBe('public')
  }
  // A request for the own submissions still needs the check, which fails closed.
  expect((await request.get(`/api/contests/${brokenContest}/standings?mine=1`, { headers: { cookie: on } })).status()).toBe(502)
})

// Every call under web/server that goes through in streaming mode, counted per file. Before an entry is added or raised,
// check what the API returns on that path to staff, authors and testers: only what anyone may see can go through.
const streamingCalls: Record<string, number> = {
  'api/auth/[action].ts': 3,
  'api/contests/[...path].get.ts': 1,
  'api/my/code-runs/index.post.ts': 1,
  'api/my/code-runs/latest.get.ts': 1,
  'api/my/difficulty-votes/[id].ts': 1,
  'api/my/favorites/[id].ts': 1,
  'api/my/profile.ts': 1,
  'api/my/solved-problems.get.ts': 1,
  'api/my/submissions/[id].get.ts': 1,
  'api/my/submissions/index.ts': 1,
  'api/problems/index.get.ts': 1,
  'utils/problem-submissions.ts': 1,
  'utils/streaming-mode.ts': 1,
}

test('only the listed calls go through in streaming mode', () => {
  const root = fileURLToPath(new URL('../server/', import.meta.url))
  const found: Record<string, number> = {}
  for (const file of readdirSync(root, { recursive: true, encoding: 'utf8' })) {
    if (!/\.[cm]?[jt]s$/.test(file)) continue
    // Any streaming property counts, whatever its value; the option's own type is written streaming?:.
    const count = readFileSync(join(root, file), 'utf8').match(/\bstreaming\s*:/g)?.length ?? 0
    if (count) found[file.split(sep).join('/')] = count
  }
  expect(found).toEqual(streamingCalls)
})
