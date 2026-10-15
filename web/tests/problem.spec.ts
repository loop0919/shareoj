import { expect, test } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

test('initial HTTP response includes the problem and SEO metadata', async ({ request, page }) => {
  const response = await request.get('/problems/11111111-1111-4111-8111-111111111111?ref=test')
  expect(response.status()).toBe(200)
  const html = await response.text()
  // Ignore script contents: serialized hydration data is not SSR content.
  const markup = html.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
  expect(markup).toMatch(/<html\s+lang="ja"/)
  expect(markup).toContain('<title>A + B | ShareOJ</title>')
  expect(markup).toContain('name="description"')
  expect(markup).toContain('property="og:title"')
  expect(markup).toContain(`rel="canonical" href="https://judge.example/problems/${shortId('11111111-1111-4111-8111-111111111111')}"`)
  await page.setContent(markup)
  const testers = page.locator('p').filter({ hasText: /^テスター / })
  await expect(testers).toHaveText('テスター bob、carol')
  await expect(testers.getByRole('link', { name: 'bob', exact: true })).toHaveAttribute('href', '/users/bob')
  await expect(testers.getByRole('link', { name: 'carol', exact: true })).toHaveAttribute('href', '/users/carol')
  expect(markup).toContain('2 つの整数 ')
  expect(markup).toContain('class="katex"')
  expect(markup).toContain('<math xmlns="http://www.w3.org/1998/Math/MathML"')
  expect(markup.slice(markup.indexOf('<body'))).not.toContain('$A$')
  expect(markup).toContain('2000000000')
  expect(html).not.toContain('127.0.0.1:18080')
})

test('problem is readable with JavaScript disabled', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false })
  const page = await context.newPage()
  await page.goto('http://127.0.0.1:13000/problems/11111111-1111-4111-8111-111111111111')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('A + B')
  await expect(page.getByRole('heading', { name: '制約', exact: true })).toBeVisible()
  await expect(page.locator('.markdown-body .katex').first()).toBeVisible()
  await expect(page.locator('.markdown-body pre').first()).toHaveText('3 5\n')
  await context.close()
})

test('published editorial is available from the problem menu', async ({ page }) => {
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  await page.getByRole('link', { name: '解説', exact: true }).click()
  await expect(page).toHaveURL(/view=editorial/)
  await expect(page.getByRole('heading', { name: '解説', exact: true })).toBeVisible()
  await expect(page.locator('.problem-body .katex')).toBeVisible()
  await expect(page.getByRole('link', { name: '解説', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.locator('form')).toHaveCount(0)
})

test('client navigation and hydration work without errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => {
    if (message.type() === 'error' || /hydration/i.test(message.text())) errors.push(message.text())
  })
  await page.goto('/problems')
  await page.getByRole('table').getByRole('link', { name: 'A + B', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('A + B')
  await expect(page).toHaveTitle('A + B | ShareOJ')
  await page.locator('.breadcrumb').getByRole('link', { name: '問題' }).click()
  await expect(page).toHaveTitle('問題 | ShareOJ')
  expect(errors).toEqual([])
})

for (const path of ['/problems/missing', '/problems/INVALID', '/unknown-page']) {
  test(`missing page returns a real 404 and noindex: ${path}`, async ({ request }) => {
    const response = await request.get(path)
    expect(response.status()).toBe(404)
    const html = await response.text()
    expect(html).toContain('ページが見つかりません')
    expect(html).toContain('name="robots" content="noindex, nofollow"')
  })
}

for (const id of ['33333333-3333-4333-8333-333333333333', '44444444-4444-4444-8444-444444444444']) {
  test(`upstream failures return 502 rather than a successful empty page: ${id}`, async ({ request }) => {
    const response = await request.get(`http://127.0.0.1:13001/problems/${id}`)
    expect(response.status()).toBe(502)
    const html = await response.text()
    expect(html).toContain('ページを表示できませんでした')
    expect(html).toContain('name="robots" content="noindex, nofollow"')
    expect(html).not.toContain('private-upstream-diagnostic')
  })
}

for (const width of [320, 375, 414, 768, 1280]) {
  test(`problem layout fits ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/problems/11111111-1111-4111-8111-111111111111')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('A + B')
    const overflow = await page.evaluate(() => {
      const viewport = document.documentElement.clientWidth
      return [...document.querySelectorAll('body *')].filter(element => {
        // Closed details can retain layout boxes even though their contents are not rendered.
        if (!element.checkVisibility()) return false
        const rect = element.getBoundingClientRect()
        return rect.width > 0 && (rect.right > viewport + 1 || rect.left < -1)
      }).map(element => element.tagName + '.' + element.className)
    })
    expect(overflow).toEqual([])
    await page.screenshot({ path: testInfo.outputPath(`problem-${width}.png`), fullPage: true })
  })
}
