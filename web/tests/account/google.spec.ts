import { test, expect } from '@playwright/test'
import { createHash } from 'node:crypto'
import { cliTicket, redeemCliTicket } from '../../server/utils/cli-oauth'

test('Google authorization uses PKCE and a private short-lived state cookie', async ({ page, context }) => {
  const providers = await context.request.get('/api/auth/providers')
  expect(await providers.json()).toEqual({ google: true, googleCLI: true })
  await page.goto('/login')
  await expect(page.getByRole('link', { name: 'Googleでログイン' })).toBeVisible()
  const response = await context.request.get('/auth/google', { maxRedirects: 0 })
  expect(response.status()).toBe(302)
  const url = new URL(response.headers().location!)
  expect(url.origin).toBe('https://example.auth.ap-northeast-1.amazoncognito.com')
  expect(url.pathname).toBe('/oauth2/authorize')
  expect(url.searchParams.get('identity_provider')).toBe('Google')
  expect(url.searchParams.get('redirect_uri')).toBe('http://127.0.0.1:13002/auth/google/callback')
  expect(url.searchParams.get('response_type')).toBe('code')
  expect(url.searchParams.get('code_challenge_method')).toBe('S256')
  const cookie = (await context.cookies()).find(c => c.name === 'openoj_google_flow')!
  expect(cookie.httpOnly).toBe(true)
  expect(cookie.sameSite).toBe('Lax')
  const [state, verifier] = decodeURIComponent(cookie.value).split('.')
  expect(url.searchParams.get('state')).toBe(state)
  expect(url.searchParams.get('code_challenge')).toBe(createHash('sha256').update(verifier!).digest('base64url'))
  expect(response.headers().location).not.toContain('browser-secret')
  expect(response.headers()['cache-control']).toBe('no-store')
})

test('unrequested, mismatched and cancelled callbacks cannot establish a session', async ({ context }) => {
  for (const kind of ['missing', 'mismatched', 'cancelled']) {
    let state = 'invalid'
    if (kind !== 'missing') {
      const start = await context.request.get('/auth/google', { maxRedirects: 0 })
      state = new URL(start.headers().location!).searchParams.get('state')!
    }
    const query = kind === 'cancelled' ? `state=${state}&error=access_denied` : 'state=invalid&code=forged'
    const response = await context.request.get(`/auth/google/callback?${query}`, { maxRedirects: 0 })
    expect(response.status()).toBe(303)
    expect(response.headers().location).toBe('/login?socialError=failed')
    expect((await context.cookies()).some(c => ['openoj_access', 'openoj_google_flow'].includes(c.name))).toBe(false)
  }
})


test('Google login preserves a tester invitation and rejects external return paths', async ({ context }) => {
  const invitation = `/my/tester-invitations/${'A'.repeat(32)}`
  for (const [next, expected] of [[invitation, invitation], ['https://attacker.example/', '/my'], ['//attacker.example/', '/my']]) {
    const response = await context.request.get(`/auth/google?${new URLSearchParams({ next: next! })}`, { maxRedirects: 0 })
    expect(response.status()).toBe(302)
    const flow = (await context.cookies()).find(cookie => cookie.name === 'openoj_google_flow')!
    const encoded = decodeURIComponent(flow.value).split('.')[2]!
    expect(Buffer.from(encoded, 'base64url').toString()).toBe(expected)
  }
})

test('CLI Google login uses the CLI PKCE challenge and returns only a signed code ticket to loopback', async ({ context }) => {
  const verifier = 'v'.repeat(43)
  const challenge = createHash('sha256').update(verifier).digest('base64url')
  const cliState = 's'.repeat(43)
  const query = new URLSearchParams({ cli_port: '54321', cli_state: cliState, cli_challenge: challenge })
  const start = await context.request.get(`/auth/google?${query}`, { maxRedirects: 0 })
  expect(start.status()).toBe(302)
  const authorize = new URL(start.headers().location!)
  expect(authorize.searchParams.get('code_challenge')).toBe(challenge)
  expect(authorize.searchParams.get('redirect_uri')).toBe('http://127.0.0.1:13002/auth/google/callback')
  const callback = await context.request.get(`/auth/google/callback?${new URLSearchParams({ state: authorize.searchParams.get('state')!, code: 'fake-code' })}`, { maxRedirects: 0 })
  expect(callback.status()).toBe(303)
  expect(callback.headers()['referrer-policy']).toBe('no-referrer')
  const local = new URL(callback.headers().location!)
  expect(local.origin).toBe('http://127.0.0.1:54321')
  expect(local.pathname).toBe('/callback')
  expect(local.searchParams.get('state')).toBe(cliState)
  const ticket = local.searchParams.get('ticket')!
  expect(redeemCliTicket(ticket, verifier, 'browser-secret')).toBe('fake-code')
  expect((await context.cookies()).some(c => ['openoj_access', 'openoj_refresh', 'openoj_google_flow'].includes(c.name))).toBe(false)
  const invalid = await context.request.post('/api/auth/cli/exchange', { data: { ticket, verifier: 'x'.repeat(43) } })
  expect(invalid.status()).toBe(400)
  expect(invalid.headers()['cache-control']).toBe('no-store')
})

test('CLI rejects invalid destinations and propagates cancellation after verifying OAuth state', async ({ context }) => {
  for (const port of ['80', '65536', '54321/path', 'https://attacker.example']) {
    const response = await context.request.get(`/auth/google?${new URLSearchParams({ cli_port: port, cli_state: 's'.repeat(43), cli_challenge: 'c'.repeat(43) })}`, { maxRedirects: 0 })
    expect(response.status()).toBe(400)
  }
  const start = await context.request.get(`/auth/google?${new URLSearchParams({ cli_port: '54321', cli_state: 's'.repeat(43), cli_challenge: 'c'.repeat(43) })}`, { maxRedirects: 0 })
  const state = new URL(start.headers().location!).searchParams.get('state')!
  const cancelled = await context.request.get(`/auth/google/callback?${new URLSearchParams({ state, error: 'access_denied' })}`, { maxRedirects: 0 })
  expect(new URL(cancelled.headers().location!).searchParams.get('error')).toBe('login_failed')
})

test('CLI tickets require an unmodified signature, matching PKCE verifier, and a live expiry', () => {
  const verifier = 'v'.repeat(43)
  const challenge = createHash('sha256').update(verifier).digest('base64url')
  const ticket = cliTicket('authorization-code', challenge, 'server-secret', 100000)
  expect(redeemCliTicket(ticket, verifier, 'server-secret', 100001)).toBe('authorization-code')
  for (const [value, proof, secret, now] of [
    [ticket, verifier, 'wrong-secret', 100001],
    [`${ticket}x`, verifier, 'server-secret', 100001],
    [ticket, 'x'.repeat(43), 'server-secret', 100001],
    [ticket, verifier, 'server-secret', 160000],
  ] as const) expect(() => redeemCliTicket(value, proof, secret, now)).toThrow()
})
