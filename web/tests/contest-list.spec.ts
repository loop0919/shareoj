import { expect, test, type Page } from '@playwright/test'

const titles = (page: Page, section: string) => page.getByRole('region', { name: section }).locator('tbody th')

test('the contest page stacks running, upcoming and past contests', async ({ page }, testInfo) => {
  await page.goto('/contests')
  await expect(page.getByRole('heading', { level: 2 })).toHaveText(['開催中のコンテスト', '開催予定のコンテスト', '過去のコンテスト'])
  await expect(titles(page, '開催中のコンテスト')).toHaveText(['先に始まった開催中', '後から始まった開催中'])
  await expect(titles(page, '開催予定のコンテスト')).toHaveText(['近い予定', '遠い予定'])
  await expect(titles(page, '過去のコンテスト')).toHaveText(['新しい過去', '古い過去'])
  await expect(page.getByRole('columnheader', { name: '状態' })).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('contest-list.png'), fullPage: true })
})

test('the stacked tables keep their columns in line', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/contests')
  for (const column of ['開始 (JST)', '終了 (JST)', '作成者']) {
    const starts = await page.getByRole('columnheader', { name: column }).evaluateAll(cells => cells.map(cell => Math.round(cell.getBoundingClientRect().left)))
    expect(new Set(starts).size, column).toBe(1)
  }
})

test('a page drops the states it has no contests for and keeps the paging', async ({ page }) => {
  await page.goto('/contests', { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '次へ' }).click()
  await expect(page.getByRole('heading', { level: 2 })).toHaveText(['過去のコンテスト'])
  await expect(titles(page, '過去のコンテスト')).toHaveText(['次のページの過去'])
})

test('an empty page still says there are no contests', async ({ page }) => {
  await page.route('**/api/contests?offset=50', route => route.fulfill({ json: { items: [], hasMore: false } }))
  await page.goto('/contests', { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '次へ' }).click()
  await expect(page.getByText('コンテストはまだありません。')).toBeVisible()
  await expect(page.getByRole('heading', { name: '過去のコンテスト' })).toHaveCount(0)
})
