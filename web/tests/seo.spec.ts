import { expect, test } from '@playwright/test'

test('home serves site identity and crawlable navigation before JavaScript', async ({ request }) => {
  const response = await request.get('/')
  expect(response.status()).toBe(200)
  const html = await response.text()
  expect(html).toContain('<title>ShareOJ — プログラミング問題を解く・作る・共有する</title>')
  expect(html).toContain('name="description" content="ShareOJ（Share Online Judge）は、')
  expect(html).toContain('rel="canonical" href="https://judge.example/"')
  const json = html.match(/<script[^>]*type="application\/ld\+json"[^>]*>(.*?)<\/script>/s)?.[1]
  expect(JSON.parse(json!)).toMatchObject({ '@type': 'WebSite', name: 'ShareOJ', alternateName: 'Share Online Judge', url: 'https://judge.example/' })
  for (const path of ['/problems', '/contests', '/blog', '/blog/contest-rules', '/blog/language-guide', '/blog/markdown-guide']) {
    expect(html).toContain(`href="${path}"`)
  }
  expect(html).not.toContain('noindex')
  const icon = await request.get('/favicon.ico')
  expect(icon.status()).toBe(200)
  const bytes = await icon.body()
  expect(bytes.readUInt16LE(2)).toBe(1)
  expect(bytes[6]).toBeGreaterThanOrEqual(48)
})

test('robots and sitemap expose canonical public entry points', async ({ request }) => {
  const robots = await request.get('/robots.txt')
  expect(robots.status()).toBe(200)
  expect(robots.headers()['content-type']).toContain('text/plain')
  expect(await robots.text()).toBe('User-agent: *\nAllow: /\n\nSitemap: https://judge.example/sitemap.xml\n')
  const sitemap = await request.get('/sitemap.xml')
  expect(sitemap.status()).toBe(200)
  expect(sitemap.headers()['content-type']).toContain('application/xml')
  const xml = await sitemap.text()
  expect(xml).toContain('xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"')
  const locations = [...xml.matchAll(/<loc>(.*?)<\/loc>/g)].map(match => match[1]!)
  expect(locations).toEqual([
    '/', '/problems', '/contests', '/blog',
    '/blog/contest-rules', '/blog/language-guide', '/blog/generator-guide',
    '/blog/difficulty-guide', '/blog/markdown-guide',
  ].map(path => `https://judge.example${path}`))
  for (const location of locations) {
    expect(location).toMatch(/^https:\/\/judge\.example\//)
    const response = await request.get(new URL(location).pathname)
    expect(response.status()).toBe(200)
    const html = await response.text()
    expect(html).toContain(`rel="canonical" href="${location}"`)
    expect(html).not.toContain('noindex')
  }
})

test('site identity and metadata follow client navigation', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('navigation', { name: 'メインナビゲーション' }).getByRole('link', { name: 'コンテスト', exact: true }).click()
  await expect(page).toHaveTitle('コンテスト | ShareOJ')
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://judge.example/contests')
  await expect(page.locator('meta[name="description"]')).toHaveAttribute('content', /ShareOJ のプログラミングコンテスト一覧/)
  await expect(page.locator('script[type="application/ld+json"]')).toHaveCount(0)
})
