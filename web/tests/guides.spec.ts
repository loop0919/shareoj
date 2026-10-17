import { expect, test } from '@playwright/test'
import { docPages, legacyDocPaths } from '../shared/docs'

for (const [slug, codeBlocks, tables] of [['generator', 3, 1], ['checkers', 3, 1], ['languages', 0, 11]] as const) {
  test(`${slug} renders Markdown, highlighted code and stable section links`, async ({ page, request }, testInfo) => {
    const path = `/docs/${slug}`
    const response = await request.get(path)
    expect(response.status()).toBe(200)
    if (codeBlocks) expect(await response.text()).toContain('class="hljs-')
    await page.goto(path)
    const body = page.locator('article .markdown-body')
    await expect(body.locator('pre.highlighted-code')).toHaveCount(codeBlocks)
    await expect(body.locator('table')).toHaveCount(tables)
    for (const code of await body.locator('pre code').all()) {
      await expect(code).toHaveClass('language-cpp')
      expect(await code.locator('[class^="hljs-"]').count()).toBeGreaterThan(0)
      await expect(code).toContainText('#include')
    }
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', `https://judge.example${path}`)
    for (const link of await page.getByRole('navigation', { name: 'ガイドの一覧' }).getByRole('list', { name: 'このページの目次' }).getByRole('link').all()) {
      const target = (await link.getAttribute('href'))!
      await expect(page.locator(target)).toHaveCount(1)
      await link.click()
      await expect(page).toHaveURL(new RegExp(`${target}$`))
      await expect(page.locator(target)).toBeInViewport()
    }
    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 900 })
      await body.locator(codeBlocks ? 'pre' : 'table').first().scrollIntoViewIfNeeded()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: testInfo.outputPath(`${slug}-${width}.png`) })
    }
  })
}

test('the guide index lists every guide and the sidebar follows the reader', async ({ page, request }) => {
  expect((await request.get('/docs/unknown')).status()).toBe(404)
  await page.goto('/docs')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('ガイド')
  const index = page.locator('.docs-index')
  for (const doc of docPages) await expect(index.getByRole('link', { name: doc.title, exact: true })).toHaveAttribute('href', `/docs/${doc.slug}`)
  await index.getByRole('link', { name: 'コンテストのルール', exact: true }).click()
  await expect(page).toHaveURL('/docs/contest-rules')
  const sidebar = page.getByRole('navigation', { name: 'ガイドの一覧' })
  await expect(sidebar.getByRole('link', { name: 'コンテストのルール', exact: true })).toHaveAttribute('aria-current', 'page')
  // Only the open guide lists its headings, right under its link.
  const contents = sidebar.getByRole('list', { name: 'このページの目次' })
  await expect(contents).toHaveCount(1)
  await expect(sidebar.getByRole('listitem').filter({ has: page.getByRole('link', { name: 'コンテストのルール', exact: true }) }).getByRole('list', { name: 'このページの目次' })).toBeVisible()
  await expect(contents.getByRole('link', { name: '参加と開催時間' })).toHaveAttribute('href', '#participation')
  // The sidebar switches guides and their headings follow.
  await sidebar.getByRole('link', { name: '提出と判定結果', exact: true }).click()
  await expect(page).toHaveURL('/docs/submissions')
  await expect(contents.getByRole('link', { name: '解答を提出する' })).toHaveAttribute('href', '#submit')
  await expect(contents.getByRole('link', { name: '参加と開催時間' })).toHaveCount(0)
  // A link in the Markdown body moves within the app, keeping the sidebar.
  await page.locator('article .markdown-body').getByRole('link', { name: '使える言語と実行環境の仕様' }).click()
  await expect(page).toHaveURL('/docs/languages')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('使える言語と実行環境の仕様')
  await expect(sidebar.getByRole('link', { name: '使える言語', exact: true })).toHaveAttribute('aria-current', 'page')
  const next = docPages[docPages.findIndex(doc => doc.slug === 'languages') + 1]!
  await page.getByRole('navigation', { name: '前後のガイド' }).getByRole('link', { name: new RegExp(`次のガイド\\s*${next.label}`) }).click()
  await expect(page).toHaveURL(`/docs/${next.slug}`)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(next.title)
})

test('every screenshot in the guides is served and has a description', async ({ page, request }) => {
  for (const doc of docPages) {
    await page.goto(`/docs/${doc.slug}`)
    for (const image of await page.locator('article .markdown-body img').all()) {
      expect(await image.getAttribute('alt')).toBeTruthy()
      const src = (await image.getAttribute('src'))!
      expect(src).toMatch(/^\/images\/docs\/[a-z-]+\.png$/)
      expect((await request.get(src)).status()).toBe(200)
    }
  }
})

test('old guide URLs redirect permanently to the guides', async ({ request }) => {
  for (const [path, slug] of Object.entries(legacyDocPaths)) {
    const response = await request.get(path, { maxRedirects: 0 })
    expect(response.status()).toBe(301)
    expect(response.headers().location).toBe(`/docs/${slug}`)
  }
})

test('narrow screens fold the guide list behind a button and never scroll sideways', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 375, height: 800 })
  await page.goto('/docs/markdown')
  const toggle = page.getByRole('button', { name: 'ガイドの一覧' })
  const sidebar = page.getByRole('navigation', { name: 'ガイドの一覧' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(sidebar.getByRole('link', { name: '使える言語', exact: true })).toBeHidden()
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await sidebar.getByRole('link', { name: '使える言語', exact: true }).click()
  await expect(page).toHaveURL('/docs/languages')
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  for (const path of ['/docs', ...docPages.map(doc => `/docs/${doc.slug}`)]) {
    await page.goto(path)
    for (const width of [320, 768]) {
      await page.setViewportSize({ width, height: 800 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${path} at ${width}px`).toBe(true)
    }
  }
  await page.setViewportSize({ width: 1280, height: 900 })
  await expect(toggle).toBeHidden()
  await expect(sidebar.getByRole('link', { name: '使える言語', exact: true })).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('docs-1280.png') })
})

test('links between guides point at existing pages and headings', async ({ page }) => {
  const targets = new Set<string>()
  for (const doc of docPages) {
    await page.goto(`/docs/${doc.slug}`)
    for (const href of await page.locator('article .markdown-body a[href^="/docs/"], article .markdown-body a[href^="#"]').evaluateAll(links => links.map(link => link.getAttribute('href')!))) {
      targets.add(href.startsWith('#') ? `/docs/${doc.slug}${href}` : href)
    }
  }
  expect(targets.size).toBeGreaterThan(10)
  for (const target of targets) {
    const [path, hash] = target.split('#')
    const response = await page.goto(path!)
    expect(response?.status(), target).toBe(200)
    if (hash) await expect(page.locator(`#${hash}`), target).toHaveCount(1)
  }
})
