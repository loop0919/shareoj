import { expect, test } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

const problem = '11111111-1111-4111-8111-111111111111'
const submission = '22222222-2222-4222-8222-222222222222'
const running = '88888888-8888-4888-8888-888888888888'
const scheduled = '99999999-9999-4999-8999-999999999999'
const post = '77777777-7777-4777-8777-777777777777'

for (const [from, to, status] of [
  [`/problems/${problem}?view=editorial`, `/problems/${shortId(problem)}?view=editorial`, 301],
  [`/problems/${problem}/submissions/${submission}`, `/problems/${shortId(problem)}/submissions/${shortId(submission)}`, 301],
  [`/contests/${running}?view=standings`, `/contests/${shortId(running)}?view=standings`, 301],
  [`/blog/${post}`, `/blog/${shortId(post)}`, 301],
  [`/contests/${running}/problems/${problem}?view=editorial`, `/contests/${shortId(running)}/A?view=editorial`, 301],
  [`/contests/${shortId(running)}/problems/${shortId(problem)}`, `/contests/${shortId(running)}/A`, 301],
  // Before the start the order may still change, so clients must not remember the redirect.
  [`/contests/${scheduled}/problems/${problem}`, `/contests/${shortId(scheduled)}/A`, 302],
] as const) {
  test(`old URL redirects with ${status}: ${from}`, async ({ request }) => {
    const response = await request.get(from, { maxRedirects: 0 })
    expect(response.status()).toBe(status)
    const location = new URL(response.headers().location!, 'http://127.0.0.1:13000')
    expect(location.pathname + location.search).toBe(to)
  })
}

test('an old contest problem link opens the problem at its letter', async ({ page }) => {
  await page.goto(`/contests/${running}/problems/${problem}`)
  await expect(page).toHaveURL(`/contests/${shortId(running)}/A`)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('A - A + B')
})

for (const path of [`/contests/${shortId(running)}/B`, `/contests/${shortId(running)}/a`, `/contests/${shortId(running)}/problems/${shortId('33333333-3333-4333-8333-333333333333')}`, `/problems/${'z'.repeat(22)}`]) {
  test(`unknown short URL is not found: ${path}`, async ({ request }) => {
    expect((await request.get(path, { maxRedirects: 0 })).status()).toBe(404)
  })
}
