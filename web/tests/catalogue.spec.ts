import { expect, test } from './fixtures/account'

for (const [path, api, label] of [['/problems', 'problems', '問題'], ['/blog', 'posts', '記事']] as const) {
  test(`${label} table replaces pages, retains data on failure, and goes back`, async ({ page }, testInfo) => {
    const item = (id: string, title: string) => ({ id, title, author: 'alice', publishedAt: '2026-09-10T00:00:00Z', updatedAt: '2026-09-10T00:00:00Z', publishedVersion: 1, isOperator: true, timeLimitMs: 2000, memoryLimitMb: 256, difficulty: 10, favoriteCount: 3, solverCount: 12, tags: [] })
    await page.route('**/api/my/solved-problems', route => route.fulfill({ json: { items: ['22222222-2222-4222-8222-222222222222'] } }))
    let fail = true
    await page.route(`**/api/${api}*`, route => {
      if (new URL(route.request().url()).searchParams.has('cursor')) {
        if (fail) return route.fulfill({ status: 502, json: {} })
        return route.fulfill({ json: { items: [item('22222222-2222-4222-8222-222222222222', '2ページ目')], nextCursor: '' } })
      }
      return route.fulfill({ json: { items: [item('11111111-1111-4111-8111-111111111111', '1ページ目')], nextCursor: 'second' } })
    })
    await page.goto('/')
    await page.getByRole('navigation', { name: 'メインナビゲーション' }).getByRole('link', { name: label, exact: true }).click()
    await expect(page).toHaveURL(path)
    await expect(page).toHaveTitle(`${label} | ShareOJ`)
    const table = page.getByRole('table')
    await expect(table.getByRole('link', { name: '1ページ目' })).toBeVisible()
    if (api === 'problems') {
      await expect(table.getByRole('cell', { name: '2 秒・256 MiB', exact: true })).toBeVisible()
      await expect(table.getByRole('columnheader', { name: '正解者数', exact: true })).toBeVisible()
      await expect(table.getByRole('cell', { name: '12', exact: true })).toBeVisible()
      await expect(table.locator('.difficulty')).toHaveText('Lv.10')
      await expect(table.locator('.difficulty-crown')).toHaveCount(1)
      await expect(table.locator('.difficulty-dot')).toHaveCount(0)
      await expect(table.getByRole('cell', { name: '3', exact: true })).toBeVisible()
    }
    for (const width of [320, 375, 414, 768]) {
      await page.setViewportSize({ width, height: 900 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      if (width === 375 || width === 768) await page.screenshot({ path: testInfo.outputPath(`${api}-${width}.png`), fullPage: true })
    }
    const pager = page.getByRole('navigation', { name: 'ページネーション' })
    await expect(pager.getByRole('button', { name: '前へ' })).toBeDisabled()
    await pager.getByRole('button', { name: '次へ' }).click()
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(table).toContainText('1ページ目')
    fail = false
    await pager.getByRole('button', { name: '次へ' }).click()
    await expect(table).toContainText('2ページ目')
    if (api === 'problems') {
      await expect(table.locator('tbody tr')).toHaveClass('solved')
      await expect(table.locator('tbody tr')).toHaveCSS('background-color', 'rgb(237, 249, 241)')
      await expect(table.getByRole('link', { name: '2ページ目（AC 済み）', exact: true })).toBeVisible()
    }
    await expect(table).not.toContainText('1ページ目')
    await expect(pager.getByRole('button', { name: '次へ' })).toBeDisabled()
    await pager.getByRole('button', { name: '前へ' }).click()
    await expect(table).toContainText('1ページ目')
    await expect(table).not.toContainText('2ページ目')
    await expect(table.locator('tr.solved')).toHaveCount(0)
  })
}

test('favorite status survives reload and a failed update preserves the count', async ({ page }) => {
  let favorited = false
  let fail = false
  await page.route('**/api/my/favorites/*', route => {
    if (route.request().method() === 'PUT') {
      if (fail) return route.fulfill({ status: 503, json: {} })
      favorited = route.request().postDataJSON().favorited
    }
    return route.fulfill({ json: { favorited, favoriteCount: favorited ? 1 : 0 } })
  })
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  const button = page.getByRole('button', { name: /お気に入り/ })
  await expect(button).toBeEnabled()
  await button.click()
  await expect(button).toHaveAttribute('aria-pressed', 'true')
  await page.reload()
  await expect(button).toHaveAttribute('aria-pressed', 'true')
  fail = true
  await button.click()
  await expect(page.getByRole('alert')).toContainText('お気に入りを更新できませんでした')
  await expect(button).toHaveAttribute('aria-pressed', 'true')
  await expect(button).toContainText('1')
  fail = false
  await button.click()
  await expect(button).toHaveAttribute('aria-pressed', 'false')
  await expect(button).toContainText('0')
})


test('solved highlighting retries after failure and is absent when signed out', async ({ page }) => {
  let fail = true
  let requests = 0
  await page.route('**/api/my/solved-problems', route => {
    requests++
    return route.fulfill(fail ? { status: 503, json: {} } : { json: { items: ['11111111-1111-4111-8111-111111111111'] } })
  })
  await page.goto('/problems')
  await expect(page.getByRole('status').filter({ hasText: '正解状況を取得できませんでした' })).toBeVisible()
  await expect(page.locator('tr.solved')).toHaveCount(0)
  fail = false
  await page.getByRole('button', { name: '再試行', exact: true }).click()
  await expect(page.locator('tr.solved')).toHaveCount(1)
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  const before = requests
  await page.reload()
  await expect(page.getByRole('link', { name: 'ログイン', exact: true }).first()).toBeVisible()
  await expect(page.locator('tr.solved')).toHaveCount(0)
  expect(requests).toBe(before)
})
