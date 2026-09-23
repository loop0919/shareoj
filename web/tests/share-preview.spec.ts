import { expect, test } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { shareImageSvg } from '../server/utils/share-image'

const problem = '11111111-1111-4111-8111-111111111111'
const running = '88888888-8888-4888-8888-888888888888'
const scheduled = '99999999-9999-4999-8999-999999999999'
const paths = ['/', '/problems', '/contests', '/blog', `/problems/${problem}`,
  `/contests/${running}`, `/contests/${scheduled}`, `/contests/${running}/problems/${problem}`,
  '/blog/77777777-7777-4777-8777-777777777777', '/blog/markdown-guide', '/blog/generator-guide', '/blog/language-guide', '/blog/contest-rules']

for (const path of paths) {
  test(`SSR exposes a working PNG preview: ${path}`, async ({ browser, request }, testInfo) => {
    const context = await browser.newContext({ javaScriptEnabled: false })
    const page = await context.newPage()
    await page.goto(`http://127.0.0.1:13000${path}?ref=ignored`)
    const imagePath = `/og${path === '/' ? '/index' : path}.png`
    for (const selector of ['meta[property="og:image"]', 'meta[name="twitter:image"]']) {
      await expect(page.locator(selector)).toHaveAttribute('content', `https://judge.example${imagePath}`)
    }
    await expect(page.locator('meta[name="twitter:card"]')).toHaveAttribute('content', 'summary_large_image')
    await expect(page.locator('meta[property="og:image:width"]')).toHaveAttribute('content', '1200')
    await expect(page.locator('meta[property="og:image:height"]')).toHaveAttribute('content', '630')
    const response = await request.get(imagePath, { headers: { 'user-agent': 'Twitterbot/1.0' } })
    expect(response.status()).toBe(200)
    expect(response.headers()['content-type']).toBe('image/png')
    const png = await response.body()
    expect(png.subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a')
    expect(png.readUInt32BE(16)).toBe(1200)
    expect(png.readUInt32BE(20)).toBe(630)
    expect(png.length).toBeGreaterThan(10000)
    await writeFile(testInfo.outputPath('share.png'), png)
    await context.close()
  })
}

test('private and pre-start images stay unavailable even with a session', async ({ context, page }) => {
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }])
  await page.goto('/problems/55555555-5555-4555-8555-555555555555')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('非公開の練習問題')
  await expect(page.locator('meta[property="og:image"], meta[name="twitter:image"]')).toHaveCount(0)
  for (const path of ['/problems/55555555-5555-4555-8555-555555555555', `/contests/${scheduled}/problems/${problem}`, '/blog/new', '/problems/missing', '/my/settings']) {
    const response = await context.request.get(`/og${path}.png`)
    expect(response.status()).toBe(404)
    expect(response.headers()['cache-control']).toMatch(/no-store|no-cache/)
  }
})

test('share card wraps English titles between words', () => {
  const svg = shareImageSvg('ShareOJ Programming Contest vol.1', 'CONTEST')
  const lines = [...svg.matchAll(/<text x="72" y="\d+" font-size="60">([^<]*)<\/text>/g)].map(match => match[1])
  expect(lines).toEqual(['ShareOJ Programming', 'Contest vol.1'])
})

test('titles are escaped, bounded and rendered as text', () => {
  const svg = shareImageSvg('日本語 & <image href="https://example.com"/>', 'ARTICLE')
  expect(svg).toContain('&amp;')
  expect(svg).toContain('&lt;image')
  expect(svg).not.toContain('<image')
  const long = shareImageSvg('あ'.repeat(200), 'PROBLEM')
  expect(long).toContain('あ'.repeat(16) + '…')
  expect(long).not.toContain('あ'.repeat(18))
})

test('upstream outages do not generate a successful image', async ({ request }) => {
  const response = await request.get('http://127.0.0.1:13001/og/problems/33333333-3333-4333-8333-333333333333.png')
  expect(response.status()).toBe(502)
  expect(response.headers()['cache-control']).toBe('no-store')
})
