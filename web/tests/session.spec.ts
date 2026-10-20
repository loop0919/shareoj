import { test, expect } from '@playwright/test'
import { loginDestination } from '../shared/utils/login-destination'

const origin = 'https://judge.example'
const base = 'http://127.0.0.1:13000'

test('login retains private refresh credentials and logout removes both cookies', async ({ context }) => {
  const response = await context.request.post('/api/auth/login', { headers: { Origin: origin }, data: { username: 'user', password: 'password' } })
  expect(response.ok()).toBe(true)
  expect(await response.json()).toEqual({ user: { id: 'session-user' } })
  const cookies = await context.cookies()
  for (const name of ['openoj_access', 'openoj_refresh']) {
    const cookie = cookies.find(c => c.name === name)!
    expect(cookie.httpOnly).toBe(true)
    expect(cookie.secure).toBe(true)
    expect(cookie.sameSite).toBe('Lax')
  }
  expect(cookies.find(c => c.name === 'openoj_refresh')!.expires).toBeGreaterThan(Date.now() / 1000 + 29 * 86400)
  const logout = await context.request.post('/api/auth/logout', { headers: { Origin: origin } })
  expect(logout.ok()).toBe(true)
  expect((await context.cookies()).filter(c => c.name.startsWith('openoj_'))).toEqual([])
})

for (const access of ['', 'expired-access']) {
  test(`submission refreshes ${access ? 'rejected' : 'expired cookie'} access without losing source`, async ({ context }) => {
    await context.addCookies([
      { name: 'openoj_refresh', value: 'valid-refresh', url: base },
      ...(access ? [{ name: 'openoj_access', value: access, url: base }] : []),
    ])
    const source = { problemId: '11111111-1111-4111-8111-111111111111', sourceCode: 'int main() { return 0; }', runtime: 'cpp17' }
    const response = await context.request.post('/api/my/submissions', { headers: { Origin: origin }, data: source })
    expect(response.status()).toBe(202)
    expect(await response.json()).toEqual(source)
    expect((await context.cookies()).find(c => c.name === 'openoj_access')?.value).toBe('valid-access')
    expect((await context.cookies()).find(c => c.name === 'openoj_refresh')?.value).toBe('valid-refresh')
  })
}

test('account and content requests recover when the access cookie has expired', async ({ context }) => {
  for (const path of ['/api/auth/me', '/api/my/posts', '/api/my/problems']) {
    await context.clearCookies()
    await context.addCookies([{ name: 'openoj_refresh', value: 'valid-refresh', url: base }])
    const response = await context.request.get(path)
    expect(response.status()).toBe(200)
    expect((await context.cookies()).find(c => c.name === 'openoj_access')?.value).toBe('valid-access')
  }
})

test('invalid refresh expires the session but a temporary outage retains it', async ({ context }) => {
  for (const token of ['invalid-refresh', 'unavailable']) {
    await context.clearCookies()
    await context.addCookies([{ name: 'openoj_refresh', value: token, url: base }])
    const response = await context.request.get('/api/auth/me')
    if (token === 'invalid-refresh') {
      expect(await response.json()).toEqual({ user: null })
      expect((await context.cookies()).find(c => c.name === 'openoj_refresh')).toBeUndefined()
    } else {
      expect(response.status()).toBe(503)
      expect((await context.cookies()).find(c => c.name === 'openoj_refresh')?.value).toBe(token)
    }
  }
})

test('refresh credentials do not bypass the origin check on submissions', async ({ context }) => {
  await context.addCookies([{ name: 'openoj_refresh', value: 'valid-refresh', url: base }])
  const response = await context.request.post('/api/my/submissions', { headers: { Origin: 'https://other.example' }, data: {} })
  expect(response.status()).toBe(403)
  expect((await context.cookies()).find(c => c.name === 'openoj_access')).toBeUndefined()
})

test('login returns to any path of this site and sends anything else to my page', () => {
  for (const path of ['/', '/my', '/problems/post', '/featured/apply', `/problems/${'A'.repeat(22)}`, '/my/contests', '/my/contests/new',
    `/my/contests/${'A'.repeat(22)}`, '/my/submissions', '/my?tab=testing', `/my/tester-invitations/${'A'.repeat(32)}`, '/problems?tag=%E3%82%B0']) {
    expect(loginDestination(path), path).toBe(path)
  }
  for (const value of [undefined, null, 42, ['/problems/post'], '', 'problems', 'https://attacker.example/', '//attacker.example/', '/\\attacker.example/',
    '/problems//post', '/problems?next=https://attacker.example/', '/\t/attacker.example', '/my\n', '/my\r\nLocation: https://attacker.example/', '/my\u0000', '/my\u007f', '/my\u0085']) {
    expect(loginDestination(value), JSON.stringify(value)).toBe('/my')
  }
})
