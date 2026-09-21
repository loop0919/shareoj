import { test, expect } from '@playwright/test'

test('public samples are available without a session and preserve input bytes', async ({ request }) => {
  const response = await request.get('/api/problems/11111111-1111-4111-8111-111111111111/samples')
  expect(response.status()).toBe(200)
  expect(response.headers()['cache-control']).toBe('no-store')
  expect(await response.json()).toEqual({ items: [
    { name: 'sample_1', input: ' 3 5\n\n', output: '8\n' },
    { name: 'empty', input: '', output: '' },
    { name: 'large', input: '', output: 'ok\n', inputFile: { url: 'https://download.example/input', size: 100000, sha256: 'a'.repeat(64) } },
  ] })
  for (const id of ['invalid', '55555555-5555-4555-8555-555555555555']) {
    const missing = await request.get(`/api/problems/${id}/samples`, { headers: { Cookie: 'openoj_access=valid-access' } })
    expect(missing.status()).toBe(404)
  }
  const unavailable = await request.get('http://127.0.0.1:13001/api/problems/11111111-1111-4111-8111-111111111111/samples')
  expect(unavailable.status()).toBe(502)
})
