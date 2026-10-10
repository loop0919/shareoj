import { expect, test } from './fixtures/account'

const updatedAt = '2026-10-01T00:00:00Z'
const ready = { publish: [], contest: [], featured: [] }
const problems = [
  { id: '11111111-aaaa-4aaa-8aaa-000000000001', title: '最短経路の数え上げ', updatedAt, readiness: ready },
  { id: '22222222-bbbb-4bbb-8bbb-000000000002', title: 'Range Sum Query', updatedAt, readiness: ready },
  { id: '33333333-cccc-4ccc-8ccc-000000000003', title: '難易度なしの問題', updatedAt, readiness: { ...ready, contest: ['difficulty_missing'] } },
]

test('contest problems are found by fuzzy title or UUID and added to the end of the order', async ({ page }) => {
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: problems, nextCursor: '' } }))
  await page.goto('/my/contests/new')
  const section = page.getByRole('button', { name: '問題・配点', exact: true })
  await section.click()
  const search = page.getByRole('searchbox', { name: '問題を検索' })
  const candidates = page.locator('#problem-candidates li')
  const selected = page.locator('.selected li')
  const onlyAddable = page.getByRole('checkbox', { name: '追加できる問題だけ表示', exact: true })
  // By default, problems that cannot be added stay out of sight.
  await expect(onlyAddable).toBeChecked()
  await expect(candidates).toHaveCount(2)
  await expect(page.getByText('2 件（追加済み・追加できない 1 件を非表示）', { exact: true })).toBeVisible()
  await expect(page.getByText('難易度なしの問題', { exact: true })).toHaveCount(0)
  // Letters in order still match when other letters sit between them.
  await search.fill('最短数え')
  await expect(candidates).toHaveCount(1)
  await expect(candidates).toContainText('最短経路の数え上げ')
  // Case and full-width letters are ignored, and every word must match.
  await search.fill('ｒａｎｇｅ　ｓｕｍ')
  await expect(candidates).toHaveCount(1)
  await expect(candidates).toContainText('Range Sum Query')
  await search.fill('range 最短')
  await expect(candidates).toHaveCount(0)
  // A UUID matches without hyphens, and Enter adds the best match instead of saving the contest.
  await search.fill('22222222bbbb')
  await expect(candidates).toHaveCount(1)
  await search.press('Enter')
  await expect(search).toHaveValue('')
  await expect(section).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(selected.locator('strong')).toHaveText(['Range Sum Query'])
  await expect(candidates).toHaveText([/最短経路の数え上げ/])
  // Showing everything marks added problems, and problems the server would reject say why.
  await onlyAddable.uncheck()
  await expect(page.getByRole('button', { name: 'Range Sum Queryは追加済み', exact: true })).toBeDisabled()
  await search.fill('難易度')
  await expect(candidates).toContainText('難易度が設定されていません')
  await expect(page.getByRole('button', { name: '難易度なしの問題を追加', exact: true })).toBeDisabled()
  await search.press('Enter')
  await expect(selected).toHaveCount(1)
  await search.fill('')
  await page.getByRole('button', { name: '最短経路の数え上げを追加', exact: true }).click()
  await expect(selected.locator('.problem-label')).toHaveText(['A', 'B'])
  await expect(selected.locator('strong')).toHaveText(['Range Sum Query', '最短経路の数え上げ'])
  await expect(page.getByRole('heading', { name: /出題順・配点/ })).toContainText('2 問 · 合計 200 点')
  // The handle moves a row with the arrow keys and keeps focus on the moved row.
  const titles = selected.locator('strong')
  await page.getByRole('button', { name: '最短経路の数え上げを並べ替え', exact: true }).press('ArrowUp')
  await expect(titles).toHaveText(['最短経路の数え上げ', 'Range Sum Query'])
  await expect(page.getByRole('button', { name: '最短経路の数え上げを並べ替え', exact: true })).toBeFocused()
  await expect(page.getByText('最短経路の数え上げをAに移動しました', { exact: true })).toBeAttached()
  // Dragging the handle carries the row; the order changes only when it is dropped.
  const handle = (await page.getByRole('button', { name: 'Range Sum Queryを並べ替え', exact: true }).boundingBox())!
  await page.mouse.move(handle.x + handle.width / 2, handle.y + handle.height / 2)
  await page.mouse.down()
  await page.mouse.move(handle.x + handle.width / 2, handle.y - 120, { steps: 6 })
  await expect(page.locator('.selected li.dragging')).toContainText('Range Sum Query')
  await expect(selected.locator('.problem-label')).toHaveText(['B', 'A'])
  await expect(titles).toHaveText(['最短経路の数え上げ', 'Range Sum Query'])
  await page.mouse.up()
  await expect(page.locator('.selected li.dragging')).toHaveCount(0)
  await expect(titles).toHaveText(['Range Sum Query', '最短経路の数え上げ'])
  for (const width of [320, 375, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await onlyAddable.check()
  await expect(candidates).toHaveCount(0)
  await page.getByRole('button', { name: 'Range Sum Queryを外す', exact: true }).click()
  await expect(selected.locator('strong')).toHaveText(['最短経路の数え上げ'])
  await expect(page.getByRole('button', { name: 'Range Sum Queryを追加', exact: true })).toBeEnabled()
})
