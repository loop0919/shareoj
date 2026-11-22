import { test, expect, type APIRequestContext } from '@playwright/test'

// Each contest's reveal answers with one refusal from tests/fixtures/public-api.mjs.
const cases = [
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f001', name: 'drops a code the screen does not know', status: 400, data: undefined },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f002', name: 'drops a rate-limit code that came without 429', status: 400, data: undefined },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f003', name: 'passes a listed code on a 429 without the wait', status: 429, data: { code: 'team_full' } },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f004', name: 'drops an unknown code and the wait on a 429', status: 429, data: undefined },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f005', name: 'answers 502 for an unexpected status and keeps a listed code', status: 502, data: { code: 'team_full' } },
]
for (const { id, name, status, data } of cases) {
  test(`the web server ${name}`, async ({ request }) => {
    const response = await request.post(`/api/my/contests/${id}/reveal`, { headers: { Cookie: 'openoj_access=valid-access', Origin: 'https://judge.example' } })
    expect(response.status()).toBe(status)
    expect(response.headers()['retry-after']).toBeUndefined()
    const text = await response.text()
    expect(text).not.toContain('private-upstream-diagnostic')
    expect(JSON.parse(text).data).toEqual(data)
  })
}

// The fixture API plays the gateway in front of the API: `busy_once` refuses the first request and answers the next,
// `busy_always` refuses every one, and `busy_coded` refuses as the API itself would. Each test asks under its own name.
const token = () => Math.random().toString(36).slice(2, 10).padEnd(8, '0')
// A contest ID under one of the fixture's two prefixes: 0001 refuses once, 0002 always.
const busyContest = (kind: '0001' | '0002') => `b0b0b0b0-${kind}-4000-8000-${Math.random().toString(16).slice(2, 14).padEnd(12, '0')}`
const hits = async (request: APIRequestContext, key: string) => (await (await request.get(`http://127.0.0.1:18080/fixture/hits?key=${key}`)).json()).hits as number
const session = { Cookie: 'openoj_access=valid-access', Origin: 'https://judge.example' }

test('a read the gateway refused once is sent again and answered', async ({ request }) => {
  const handle = `busy_once_${token()}`
  const response = await request.get(`/api/users?handles=${handle}`)
  expect(response.status()).toBe(200)
  expect(await response.json()).toEqual({ items: [] })
  expect(await hits(request, handle)).toBe(2)
})

test('a read the gateway keeps refusing answers 503 after one more try', async ({ request }) => {
  const handle = `busy_always_${token()}`
  const response = await request.get(`/api/users?handles=${handle}`)
  expect(response.status()).toBe(503)
  expect(await response.text()).not.toContain('Too Many Requests')
  expect(await hits(request, handle)).toBe(2)
})

test('a read the API itself refused is not sent again and still answers 502', async ({ request }) => {
  const handle = `busy_coded_${token()}`
  expect((await request.get(`/api/users?handles=${handle}`)).status()).toBe(502)
  expect(await hits(request, handle)).toBe(1)
})

test('a signed-in read the gateway refused once is sent again and answered', async ({ request }) => {
  const id = busyContest('0001')
  const response = await request.get(`/api/contests/${id}`, { headers: session })
  expect(response.status()).toBe(200)
  expect(await response.json()).toEqual({ id })
  expect(await hits(request, id)).toBe(2)
})

test('a write the gateway refused is not sent twice', async ({ request }) => {
  const id = busyContest('0002')
  const response = await request.post(`/api/my/contests/${id}/reveal`, { headers: session })
  expect(response.status()).toBe(429)
  expect(await hits(request, id)).toBe(1)
})
