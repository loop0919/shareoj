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
