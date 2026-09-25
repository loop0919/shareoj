import { expect, test } from './fixtures/account'

const at = '2099-09-28T14:00:00Z'
const path = `/featured/standings?at=${encodeURIComponent(at)}`

test('edition standings show tied ranks, checks, elapsed times and non-penalizing wrong counts', async ({ page }, testInfo) => {
  await page.goto('/featured')
  await page.locator('.round').first().getByRole('link', { name: '順位表', exact: true }).click()
  await expect(page.getByRole('heading', { name: '定期便の順位表', exact: true })).toBeVisible()
  const table = page.getByRole('table')
  await expect(table.locator('thead th')).toHaveText(['順位', 'ユーザー', '時間', 'Easy', 'Hard'])
  const rows = table.locator('tbody tr')
  await expect(rows).toHaveCount(6)
  await expect(rows.locator('td:first-child')).toHaveText(['1', '1', '3', '4', '5', '5'])
  await expect(rows.locator('th')).toHaveText(['carol', 'alice', 'bob', 'yuki', 'henry', 'mika'])
  await expect(rows.first().getByRole('img', { name: 'Easy 正解' })).toBeVisible()
  await expect(rows.first().getByRole('img', { name: 'Hard 正解' })).toBeVisible()
  await expect(rows.first()).toContainText('0:20:00')
  await expect(rows.first().getByLabel('不正解 2 回')).toHaveText('(2)')
  await expect(rows.nth(2).getByRole('img', { name: 'Easy 正解' })).toHaveCount(0)
  await expect(rows.nth(2).getByRole('img', { name: 'Hard 正解' })).toBeVisible()
  await expect(rows.nth(4).getByRole('img')).toHaveCount(0)
  await expect(table).not.toContainText('得点')
  await expect(page.locator('a[href*="submissions"]')).toHaveCount(0)
  const rules = page.locator('.standings-rules')
  await expect(rules.locator('ul')).toBeHidden()
  await rules.locator('summary').focus()
  await page.keyboard.press('Enter')
  await expect(rules).toContainText('誤答ペナルティはなく')
  await page.keyboard.press('Enter')
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`featured-standings-${width}.png`), fullPage: true })
  }
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.screenshot({ path: testInfo.outputPath('featured-standings-dark.png'), fullPage: true })
})

test('home links to the last edition standings and refresh can recover from an error', async ({ page }) => {
  await page.goto('/')
  const payload = await (await page.request.get(`/api/featured/standings?at=${encodeURIComponent(at)}`)).json()
  let failing = true
  await page.route('**/api/featured/standings**', route => failing ? route.fulfill({ status: 502, json: {} }) : route.fulfill({ json: payload }))
  await page.getByRole('link', { name: '前回の順位表', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('順位表を取得できませんでした')
  failing = false
  await page.getByRole('button', { name: '再試行', exact: true }).click()
  await expect(page.getByRole('table').locator('tbody tr')).toHaveCount(6)
  await page.getByRole('navigation', { name: 'パンくずリスト' }).getByRole('link', { name: '定期便', exact: true }).click()
  await expect(page.getByRole('heading', { name: '定期便', exact: true })).toBeVisible()
})

test('standings handle pagination, empty editions and polling after the deadline', async ({ page }) => {
  await page.clock.install()
  await page.goto('/featured')
  const payload = await (await page.request.get(`/api/featured/standings?at=${encodeURIComponent(at)}`)).json()
  payload.closed = true
  let offset = 0
  await page.route('**/api/featured/standings**', route => {
    offset = Number(new URL(route.request().url()).searchParams.get('offset') ?? 0)
    return route.fulfill({ json: { ...payload, hasMore: offset === 0, items: offset ? [] : payload.items } })
  })
  await page.locator('.round').first().getByRole('link', { name: '順位表', exact: true }).click()
  await expect(page.getByText('集計期間終了', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '次へ', exact: true }).click()
  await expect(page.getByText('このページに参加者はいません。')).toBeVisible()
  expect(offset).toBe(50)
  await page.getByRole('button', { name: '前へ', exact: true }).click()
  payload.items = []
  await page.clock.fastForward(16000)
  await expect(page.getByText('集計対象の提出はまだありません。')).toBeVisible()
})

test('standings are rendered on the server and unknown editions return no results', async ({ page }) => {
  const html = await (await page.request.get(path)).text()
  expect(html).toContain('定期便の順位表')
  expect(html).toContain('carol')
  await page.goto('/featured/standings?at=2099-01-01T00%3A00%3A00Z')
  await expect(page.getByRole('alert')).toContainText('この回の順位表は見つかりません')
  await expect(page.getByRole('table')).toHaveCount(0)
})
