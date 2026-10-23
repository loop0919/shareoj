import { expect, test } from './fixtures/account'

test('post table replaces pages, retains data on failure, and goes back', async ({ page }, testInfo) => {
  const item = (id: string, title: string) => ({ id, title, author: 'alice', publishedAt: '2026-09-10T00:00:00Z', updatedAt: '2026-09-10T00:00:00Z', publishedVersion: 1, isOperator: true, tags: [] })
  let fail = true
  await page.route('**/api/posts*', route => {
    if (new URL(route.request().url()).searchParams.has('cursor')) {
      if (fail) return route.fulfill({ status: 502, json: {} })
      return route.fulfill({ json: { items: [item('22222222-2222-4222-8222-222222222222', '2ページ目')], nextCursor: '' } })
    }
    return route.fulfill({ json: { items: [item('11111111-1111-4111-8111-111111111111', '1ページ目')], nextCursor: 'second' } })
  })
  await page.goto('/')
  await page.getByRole('navigation', { name: 'メインナビゲーション' }).getByRole('link', { name: '記事', exact: true }).click()
  await expect(page).toHaveURL('/blog')
  await expect(page).toHaveTitle('記事 | ShareOJ')
  const table = page.getByRole('table')
  await expect(table.getByRole('link', { name: '1ページ目' })).toBeVisible()
  for (const width of [320, 375, 414, 768]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    if (width === 375 || width === 768) await page.screenshot({ path: testInfo.outputPath(`posts-${width}.png`), fullPage: true })
  }
  const pager = page.getByRole('navigation', { name: 'ページネーション' })
  await expect(pager.getByRole('button', { name: '前へ' })).toBeDisabled()
  await pager.getByRole('button', { name: '次へ' }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(table).toContainText('1ページ目')
  fail = false
  await pager.getByRole('button', { name: '次へ' }).click()
  await expect(table).toContainText('2ページ目')
  await expect(table).not.toContainText('1ページ目')
  await expect(pager.getByRole('button', { name: '次へ' })).toBeDisabled()
  await pager.getByRole('button', { name: '前へ' }).click()
  await expect(table).toContainText('1ページ目')
  await expect(table).not.toContainText('2ページ目')
})

// The fixture API lists 問題 001 to 問題 120 under the tag pagination, newest first.
const pagedId = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`

test('problem list pages by number and keeps the page in the URL', async ({ page, context }, testInfo) => {
  await page.route('**/api/my/solved-problems', route => route.fulfill({ json: { items: [pagedId(101)] } }))
  const requested: string[] = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (url.pathname === '/api/problems') requested.push(`offset=${url.searchParams.get('offset')}&limit=${url.searchParams.get('limit')}`)
  })
  const canonical = page.locator('link[rel="canonical"]')
  const table = page.getByRole('table')
  const rows = table.locator('tbody tr')
  const pages = page.getByRole('navigation', { name: 'ページ', exact: true }).first()
  await page.goto('/problems?tag=pagination&page=2')
  await expect(page.locator('.list-count', { hasText: '全 120 問中 51〜100 問目' })).toBeVisible()
  await expect(rows.first().locator('th')).toHaveText('問題 051')
  await expect(canonical).toHaveAttribute('href', 'https://judge.example/problems?tag=pagination&page=2')
  await expect(pages.getByRole('link')).toHaveText(['1', '3'])
  await expect(pages.locator('span[aria-current="page"]')).toHaveText('2')
  const first = rows.first()
  await expect(first.getByRole('cell', { name: '2 秒・256 MiB', exact: true })).toBeVisible()
  await expect(table.getByRole('columnheader', { name: '正解者数', exact: true })).toBeVisible()
  await expect(first.getByRole('cell', { name: '12', exact: true })).toBeVisible()
  await expect(first.locator('.difficulty')).toHaveText('Lv.10')
  await expect(first.locator('.difficulty-crown')).toHaveCount(1)
  await expect(first.locator('.difficulty-dot')).toHaveCount(0)
  await expect(first.getByRole('cell', { name: '3', exact: true })).toBeVisible()
  for (const width of [320, 375, 414, 768]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    if (width === 375 || width === 768) await page.screenshot({ path: testInfo.outputPath(`problems-${width}.png`), fullPage: true })
  }

  await pages.getByRole('link', { name: '3', exact: true }).click()
  await expect(page).toHaveURL('/problems?tag=pagination&page=3')
  await expect(page.locator('.list-count', { hasText: '全 120 問中 101〜120 問目' })).toBeVisible()
  await expect(rows).toHaveCount(20)
  await expect(rows.first().locator('th')).toHaveText('問題 101')
  await expect(rows.last().locator('th')).toHaveText('問題 120')
  // 問題 101 is solved.
  await expect(table.locator('tr.solved')).toHaveCount(1)
  await expect(rows.first()).toHaveClass('solved')
  await expect(rows.first()).toHaveCSS('background-color', 'rgb(237, 249, 241)')
  await expect(table.getByRole('link', { name: '問題 101（AC 済み）', exact: true })).toBeVisible()

  // Row 101 topped page 3, so 20 rows a page opens page 6. One request, with the new size and page together: page 3 at
  // 20 rows a page (offset=40) is never asked for.
  requested.length = 0
  await Promise.all([page.waitForResponse(response => response.url().includes('/api/problems') && response.url().includes('limit=20')), page.getByLabel('表示件数').selectOption('20')])
  await expect(page).toHaveURL('/problems?tag=pagination&page=6')
  await expect(page.locator('.list-count', { hasText: '全 120 問中 101〜120 問目' })).toBeVisible()
  await expect(rows).toHaveCount(20)
  await expect(rows.first().locator('th')).toHaveText('問題 101')
  await page.reload()
  await expect(page.getByLabel('表示件数')).toHaveValue('20')
  await expect(page.locator('.list-count', { hasText: '全 120 問中 101〜120 問目' })).toBeVisible()
  await expect(rows).toHaveCount(20)
  expect(requested).not.toContain('offset=40&limit=20')
  expect(requested).toEqual(['offset=100&limit=20'])

  // Without the cookie the list is 50 a page again, so page 99 is past the end and the server sends the last page instead.
  await context.clearCookies({ name: 'openoj-page-size-problems' })
  const response = await page.goto('/problems?tag=pagination&page=99')
  expect(response?.request().redirectedFrom()?.url()).toContain('page=99')
  await expect(page).toHaveURL('/problems?tag=pagination&page=3')
  await expect(canonical).toHaveAttribute('href', 'https://judge.example/problems?tag=pagination&page=3')
  await expect(page.locator('.list-count', { hasText: '全 120 問中 101〜120 問目' })).toBeVisible()
  await page.goto('/problems?tag=pagination&page=1', { waitUntil: 'networkidle' })
  await expect(canonical).toHaveAttribute('href', 'https://judge.example/problems?tag=pagination')
  await expect(page.locator('.list-count', { hasText: '全 120 問中 1〜50 問目' })).toBeVisible()
  // A move in the browser past the end lands on the last page too.
  await page.evaluate(path => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(path), '/problems?tag=pagination&page=99')
  await expect(page).toHaveURL('/problems?tag=pagination&page=3')
  await expect(rows.first().locator('th')).toHaveText('問題 101')
  // A page number beyond what the API takes as an offset ends on the last page, not on an error (the fixture refuses an offset above a million).
  for (const value of ['99999', '9'.repeat(400)]) {
    await page.goto(`/problems?tag=pagination&page=${value}`)
    await expect(page).toHaveURL('/problems?tag=pagination&page=3')
    await expect(page.locator('.list-count', { hasText: '全 120 問中 101〜120 問目' })).toBeVisible()
  }
  await page.evaluate(path => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(path), '/problems?tag=pagination&page=99999')
  await expect(page).toHaveURL('/problems?tag=pagination&page=3')
  await expect(rows.first().locator('th')).toHaveText('問題 101')
})

test('a failed page shows an alert and retries', async ({ page }) => {
  // $fetch retries a failed GET once by itself, so page 3 keeps failing until the viewer asks again.
  let fail = true
  await page.route('**/api/problems*', route => {
    if (fail && new URL(route.request().url()).searchParams.get('offset') === '100') return route.fulfill({ status: 502, json: {} })
    return route.continue()
  })
  await page.goto('/problems?tag=pagination&page=2')
  await page.getByRole('navigation', { name: 'ページ', exact: true }).first().getByRole('link', { name: '3', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('一覧を取得できませんでした。もう一度お試しください。')
  await expect(page.getByRole('table')).toHaveCount(0)
  fail = false
  await page.getByRole('button', { name: '再試行', exact: true }).click()
  await expect(page.getByRole('table').getByRole('link', { name: '問題 101', exact: true })).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

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
