import { expect, test } from './fixtures/account'
import type { Locator, Page } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

const problemId = '11111111-1111-4111-8111-111111111111'
const contestId = '88888888-8888-4888-8888-888888888888'
const submissionId = '33333333-3333-4333-8333-333333333331'
const updatedAt = '2026-09-10T00:00:00Z'
const submission = {
  id: submissionId, contestId, problemId, problemTitle: '長い問題名でも一覧の横幅に収まるかを確かめるための問題', problemVersion: 1, author: 'carol', runtime: 'cpp17', sourceBytes: 65536, status: 'DONE',
  createdAt: '2026-09-15T12:10:00Z', result: { verdict: 'WA', passed: 0, total: 1, cpuTimeMs: 1234, memoryBytes: 1024000000 },
}

// The lists sit below the fold on some pages, so this looks at the width alone.
async function inWidth(locator: Locator, width: number) {
  const box = await locator.boundingBox()
  expect(box!.x).toBeGreaterThanOrEqual(0)
  expect(box!.x + box!.width).toBeLessThanOrEqual(width)
}

// A card shows no column headings, and neither the list nor any of its cells scrolls sideways.
async function readsAsCards(page: Page, table: Locator, name: string) {
  for (const width of [320, 375]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(table).toBeVisible()
    await expect(table.locator('thead th:not(.select-cell)').first()).toBeHidden()
    const overflowing = await table.evaluate(node => [node.parentElement!, ...node.querySelectorAll('tbody th, tbody td')]
      .filter(element => element.scrollWidth > element.clientWidth).map(element => `${element.textContent}: ${element.scrollWidth} > ${element.clientWidth}`))
    expect(overflowing, `${width}px`).toEqual([])
    await table.screenshot({ path: test.info().outputPath(`${name}-${width}.png`) })
  }
}

test('a narrow problem list leads each card with the difficulty', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 900 })
  await page.goto('/problems?tag=pagination')
  const table = page.getByRole('region', { name: '問題一覧' }).getByRole('table')
  const row = table.locator('tbody tr').first()
  await expect(row.locator('.difficulty')).toBeInViewport()
  const difficulty = await row.locator('.difficulty').boundingBox()
  const title = await row.getByRole('link').first().boundingBox()
  expect(difficulty!.y).toBeLessThan(title!.y)
  await expect(row.getByRole('cell', { name: '正解者数 12', exact: true })).toBeInViewport()
  await readsAsCards(page, table, 'problems')
})

test('the problem list turns into cards just under 640px and stays a table from there up', async ({ page }) => {
  await page.setViewportSize({ width: 640, height: 900 })
  await page.goto('/problems')
  const heading = page.getByRole('columnheader', { name: '正解者数', exact: true })
  await expect(heading).toBeVisible()
  await page.setViewportSize({ width: 639, height: 900 })
  await expect(heading).toBeHidden()
})

test('narrow article lists read as cards', async ({ page }) => {
  await page.route('**/api/posts*', route => route.fulfill({ json: { items: [
    { id: problemId, title: '記事', author: 'alice', publishedAt: updatedAt, updatedAt, publishedVersion: 1, isOperator: true, tags: ['解説'] },
  ], nextCursor: '' } }))
  await page.goto('/')
  await page.getByRole('navigation', { name: 'メインナビゲーション' }).getByRole('link', { name: '記事', exact: true }).click()
  await readsAsCards(page, page.getByRole('region', { name: '記事一覧' }).getByRole('table'), 'posts')
})

test('narrow contest lists read as cards', async ({ page }) => {
  await page.goto('/contests')
  await readsAsCards(page, page.getByRole('region', { name: '過去のコンテスト' }).getByRole('table'), 'contests')
})

test('narrow public profile lists read as cards', async ({ page }) => {
  await page.goto('/users/alice')
  await readsAsCards(page, page.getByRole('region', { name: '公開済みの問題' }).getByRole('table'), 'user-problems')
  await page.getByRole('button', { name: '記事', exact: true }).click()
  await readsAsCards(page, page.getByRole('region', { name: '公開済みの記事' }).getByRole('table'), 'user-posts')
})

test('a narrow submission list leads each card with the result', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 900 })
  await page.route('**/api/problems/*/submissions?*', route => route.fulfill({ json: { items: [submission], hasMore: false } }))
  await page.goto(`/problems/${shortId(problemId)}?view=submissions`)
  const table = page.getByRole('region', { name: 'すべての提出' }).getByRole('table')
  const row = table.locator('tbody tr').first()
  await inWidth(row.locator('.verdict-badge'), 375)
  await inWidth(row.getByRole('link', { name: '詳細', exact: true }), 375)
  await readsAsCards(page, table, 'submissions')
  // With no title to push it down, the user still starts a line of its own under the result.
  const verdict = await row.locator('.verdict-badge').boundingBox()
  const user = await row.getByRole('link', { name: 'carol', exact: true }).boundingBox()
  expect(user!.y).toBeGreaterThanOrEqual(verdict!.y + verdict!.height)
  // A stat that wraps does not push the rule above its neighbours out of line.
  const tops = await row.locator('.stat-cell').evaluateAll(cells => cells.map(cell => cell.getBoundingClientRect().top))
  expect(new Set(tops).size).toBe(1)
})

test('narrow rejudge lists keep the checkbox for the whole page', async ({ page }) => {
  await page.route('**/api/my/problems/*/rejudge', route => route.fulfill({ json: { pending: 0 } }))
  await page.route('**/api/problems/*/submissions?*', route => route.fulfill({ json: { items: [submission], hasMore: false } }))
  await page.goto(`/problems/${shortId(problemId)}?view=submissions`)
  const list = page.getByRole('region', { name: 'すべての提出' })
  await expect(list.getByRole('checkbox', { name: /の提出を選択$/ })).toBeVisible()
  await readsAsCards(page, list.getByRole('table'), 'rejudge')
  await inWidth(list.getByRole('checkbox', { name: 'このページの提出をすべて選択' }), 375)
  await inWidth(list.getByRole('checkbox', { name: /の提出を選択$/ }), 375)
})

test('narrow contest submission lists read as cards', async ({ page }) => {
  const contest = {
    id: contestId, title: '一覧の表示テスト', author: 'setter', description: '', status: 'running',
    startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5,
    version: 1, official: false, canEdit: false, canViewSubmissions: true,
    problems: [{ id: problemId, title: submission.problemTitle, points: 100 }],
  }
  await page.route(`**/api/contests/${contestId}**`, route => {
    const path = new URL(route.request().url()).pathname
    if (path.includes('/submissions')) return route.fulfill({ json: { items: [submission], hasMore: false } })
    if (path.endsWith('/standings')) return route.fulfill({ json: [] })
    return route.fulfill({ json: contest })
  })
  await page.goto(`/contests/${contestId}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.site-header')).toContainText(contest.title)
  await page.evaluate(to => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(to), `/contests/${shortId(contestId)}?view=submissions`)
  const table = page.getByRole('region', { name: '提出一覧のスクロール領域' }).getByRole('table')
  await expect(table.locator('tbody tr')).toHaveCount(1)
  await readsAsCards(page, table, 'contest-submissions')
})

test('narrow lists on the my page read as cards', async ({ page }) => {
  await page.route('**/api/my/problems', route => route.fulfill({ json: { items: [
    { id: problemId, title: 'A + B', difficulty: 7, publishedVersion: 1, updatedAt },
  ], nextCursor: '' } }))
  await page.route('**/api/my/posts', route => route.fulfill({ json: { items: [
    { id: problemId, title: '公開記事', publishedVersion: 1, publishedAt: updatedAt, updatedAt, isOperator: false },
  ], nextCursor: '' } }))
  await page.route(url => url.pathname === '/api/my/contests/drafts', route => route.fulfill({ json: { items: [
    { id: contestId, title: '下書きコンテスト', author: 'ui_test', startsAt: null, updatedAt },
  ], hasMore: false } }))
  await page.route(url => url.pathname === '/api/my/contests', route => route.fulfill({ json: { items: [
    { id: contestId, author: 'ui_test', problemAuthors: [], testers: [], title: '予定のコンテスト', description: '', startsAt: '2026-09-01T00:00:00Z', endsAt: '2026-09-01T02:00:00Z', status: 'scheduled' },
    { id: problemId, author: 'ui_test', problemAuthors: [], testers: [], title: '終了したコンテスト', description: '', startsAt: '2026-09-01T00:00:00Z', endsAt: '2026-09-01T02:00:00Z', status: 'ended' },
  ], hasMore: false } }))
  await page.goto('/my')
  await readsAsCards(page, page.getByRole('table', { name: '自分の問題' }), 'my-problems')
  await page.getByRole('button', { name: '記事', exact: true }).click()
  await readsAsCards(page, page.getByRole('table', { name: '作成した記事' }), 'my-posts')
  await page.getByRole('button', { name: 'コンテスト', exact: true }).click()
  await readsAsCards(page, page.getByRole('region', { name: 'コンテストの下書き' }).getByRole('table'), 'my-contest-drafts')
  const published = page.getByRole('region', { name: 'コンテスト一覧' }).getByRole('table')
  await readsAsCards(page, published, 'my-contests')
  await inWidth(published.getByRole('link', { name: '予定のコンテストを編集' }), 375)
  await expect(published.locator('tbody tr').nth(1).getByText('—', { exact: true })).toBeHidden()
})
