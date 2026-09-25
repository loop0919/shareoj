import { expect, test } from './fixtures/account'

const id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const hidden = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const updatedAt = '2026-09-26T00:00:00Z'

test('featured editions show two slots, revivals, vacancies and the Ultimate label', async ({ page }, testInfo) => {
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: [] } }))
  await page.goto('/featured')
  await expect(page.getByRole('heading', { name: '定期便', exact: true })).toBeVisible()
  await expect(page.locator('.round')).toHaveCount(2)
  await expect(page.locator('.round').first().locator('.slot')).toHaveCount(2)
  await expect(page.getByRole('heading', { name: 'Ultimate', exact: true })).toBeVisible()
  await expect(page.getByText('欠番', { exact: true })).toBeVisible()
  await expect(page.getByText('解説公開済み', { exact: false }).first()).toBeVisible()
  await expect(page.getByRole('link', { name: '定期便の新作', exact: true })).toHaveAttribute('href', `/problems/${hidden}`)
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`featured-${width}.png`), fullPage: true })
  }
})

test('authors choose a preference, see validation failures, apply and withdraw', async ({ page }) => {
  let applied = false
  let attempts = 0
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: applied ? [{ problemId: id, title: '応募する問題', preference: 'later', enteredAt: updatedAt }] : [] } }))
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items: [
    { id, title: '応募する問題', updatedAt },
    { id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', title: '過去に公開済み', everPublished: true, updatedAt },
    { id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', title: 'コンテスト用', contestId: 'contest', updatedAt },
  ], nextCursor: '' } }))
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
    id, version: 3, updatedAt, draft: { title: '応募する問題', markdown: '本文', editorial: '解説', difficulty: 4, timeLimitMs: '2000', memoryLimitMb: '256', testCases: [] },
  } }))
  await page.route(`**/api/my/problems/${id}/featured`, route => {
    const body = route.request().postDataJSON()
    expect(body.version).toBe(3)
    if (body.preference === '') { applied = false; return route.fulfill({ status: 204 }) }
    expect(body.preference).toBe('later')
    if (++attempts === 1) return route.fulfill({ status: 400, json: { data: { code: 'featured_ineligible' } } })
    applied = true
    return route.fulfill({ status: 204 })
  })
  await page.goto('/featured')
  await page.getByRole('button', { name: '新作を応募' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('投稿する問題').locator('option')).toHaveText(['問題を選択してください', '応募する問題'])
  await expect(dialog).not.toContainText('Ultimate')
  await dialog.getByLabel('投稿する問題').selectOption(id)
  await dialog.getByLabel('公開の希望').selectOption('later')
  await dialog.getByRole('button', { name: '応募する', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('難易度・解説')
  await dialog.getByRole('button', { name: '応募する', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.locator('.applications')).toContainText('あとからでもよい')
  await page.getByRole('button', { name: '取り下げる' }).click()
  await expect(page.getByText('応募中の問題はありません。')).toBeVisible()
})

test('hidden editorial and other submissions show the unlock time while own results stay accessible', async ({ page }) => {
  await page.goto(`/problems/${hidden}?view=editorial`)
  await expect(page.getByText('解説は公開待ちです。')).toBeVisible()
  await expect(page.getByText(/解法のネタバレを含めないでください/)).toBeVisible()
  await page.getByRole('link', { name: 'すべての提出', exact: true }).click()
  await expect(page.getByText(/他者の提出は解禁後に閲覧できます/)).toBeVisible()
  await expect(page.locator('.problem-submissions')).toHaveCount(0)
  await page.getByRole('link', { name: '自分の提出', exact: true }).click()
  await expect(page.getByRole('heading', { name: '自分の提出', exact: true })).toBeVisible()
})

test('featured history has a recoverable error state', async ({ page }) => {
  await page.goto('/problems')
  await page.route('**/api/featured**', route => route.fulfill({ status: 502, json: {} }))
  await page.locator('header').getByRole('link', { name: '定期便', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: '定期便を取得できませんでした' })).toBeVisible()
  await expect(page.getByRole('button', { name: '再試行', exact: true }).first()).toBeVisible()
})
