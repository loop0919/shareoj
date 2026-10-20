import { test, expect } from '@playwright/test'

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
