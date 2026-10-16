import { expect, test } from './fixtures/account'

test('guide opens in a separate tab while the editor keeps its draft', async ({ page, request }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const response = await request.get('/docs/markdown')
  expect(response.status()).toBe(200)
  const html = await response.text()
  expect(html).toContain('katex-html')
  expect(html).toContain('rel="canonical" href="https://judge.example/docs/markdown"')
  await page.goto('/problems/new')
  await page.locator('#problem-title').fill('記事を読んでも残る下書き')
  await page.locator('#problem-source').fill('## 保存した本文\n\n$A+B$')
  const popupPromise = page.waitForEvent('popup')
  await page.getByRole('link', { name: 'Markdown・数式の書き方' }).click()
  const guide = await popupPromise
  await expect(guide.getByRole('heading', { level: 1 })).toHaveText('Markdown と数式の書き方')
  expect(await guide.evaluate(() => window.opener)).toBeNull()
  await guide.close()
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.locator('#problem-title')).toHaveValue('記事を読んでも残る下書き')
  await expect(page.locator('#problem-source')).toHaveText('## 保存した本文\n\n$A+B$', { useInnerText: true })
  const workspace = await page.locator('.author-workspace').boundingBox()
  expect(workspace!.width).toBeGreaterThan(1440 * .9)
  await expect(page.locator('.source-pane')).toBeVisible()
  await expect(page.locator('.preview-pane')).toBeVisible()
  await page.locator('#problem-source').fill(Array.from({ length: 50 }, (_, i) => `## 節 ${i}\n\n本文`).join('\n\n'))
  await expect(page.locator('.preview-document h2')).toHaveCount(50)
  expect(await page.locator('.preview-document').evaluate(element => {
    element.scrollTop = 100
    return element.scrollTop
  })).toBeGreaterThan(0)
  expect(await page.locator('.markdown-source-editor .cm-scroller').evaluate(element => {
    element.scrollTop = 100
    return element.scrollTop
  })).toBeGreaterThan(0)
  expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true)
  await page.getByRole('button', { name: '編集', exact: true }).click()
  await expect(page.locator('.preview-pane')).toBeHidden()
  await page.getByRole('button', { name: 'プレビュー', exact: true }).click()
  await expect(page.locator('.source-pane')).toBeHidden()
  await page.getByRole('button', { name: '分割', exact: true }).click()
  await expect(page.locator('.source-pane')).toBeVisible()
  await expect(page.locator('.preview-pane')).toBeVisible()
  await page.setViewportSize({ width: 1280, height: 600 })
  expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true)
})

for (const width of [320, 375, 768, 1440]) {
  test(`Markdown guide is readable at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/blog')
    await page.getByRole('main').getByRole('link', { name: 'ガイド', exact: true }).click()
    await page.locator('.docs-index').getByRole('link', { name: 'Markdown と数式の書き方', exact: true }).click()
    await expect(page.locator('.blog-article .input-format .katex')).toHaveCount(7)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await page.locator('.input-format .math-expression').evaluateAll(elements =>
      elements.every(element => getComputedStyle(element).overflowX === 'visible'),
    )).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`guide-${width}.png`), fullPage: true })
  })
}
