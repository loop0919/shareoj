import { expect, test } from '@playwright/test'
import type { Contest, Standing } from '../shared/types/contest'

const id = '88888888-8888-4888-8888-888888888888'
const startsAt = '2026-09-15T12:00:00Z'
const contest: Contest = {
  id, title: '順位表テスト', author: 'setter', description: '', startsAt,
  endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5, version: 1,
  status: 'running', participating: false, canEdit: false, official: true, canViewSubmissions: false,
  problems: ['a', 'b', 'c', 'd'].map(id => ({ id, title: id, points: 100 })),
}
const rows: Standing[] = [
  { rank: 1, handle: 'alice', points: 200, timeMs: 1200000, problems: {
    a: { points: 100, wrong: 1, pending: 0, acceptedAt: '2026-09-15T12:10:00Z' },
    b: { points: 100, wrong: 0, pending: 0, acceptedAt: '2026-09-15T12:15:00Z' },
  } },
  { rank: 2, handle: 'bob', points: 100, timeMs: 300000, problems: {
    a: { points: 100, wrong: 0, pending: 0, acceptedAt: '2026-09-15T12:05:00Z' },
    b: { points: 0, wrong: 2, pending: 3 },
    c: { points: 0, wrong: 0, pending: 0 }, // A CE-only submitter still counts.
  } },
  { rank: 2, handle: 'carol', points: 100, timeMs: 300000, problems: {
    a: { points: 100, wrong: 0, pending: 0, acceptedAt: '2026-09-15T12:05:00Z' },
  } },
]

test('standings show compact results, participant counts and earliest AC including ties', async ({ page }) => {
  let standings = structuredClone(rows)
  await page.route(`**/api/contests/${id}**`, route => route.fulfill({ json: route.request().url().includes('/standings') ? standings : contest }))
  await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  const table = page.locator('.standings-table')
  await expect(table.locator('thead th')).toHaveText(['順位', 'ユーザー', '得点', '時間', 'A', 'B', 'C', 'D'])
  const alice = table.locator('tbody tr').filter({ hasText: 'alice' })
  await expect(alice.locator('td').nth(3)).toHaveText('100(1)10:00')
  await expect(alice.locator('td').nth(4)).toHaveText('10015:00')
  await expect(table.getByText('(0)', { exact: true })).toHaveCount(0)
  const wrong = table.getByLabel('誤答 2 回', { exact: true })
  await expect(wrong).toHaveText('(2)')
  await expect(wrong).toHaveCSS('color', 'rgb(161, 38, 34)')
  await expect(table.getByRole('img', { name: '判定待ち' })).toHaveCount(1)
  await expect(table.locator('svg[aria-label="判定待ち"]')).toBeVisible()
  const fa = table.locator('tfoot tr').nth(0)
  await expect(fa.locator('td').nth(0)).toHaveText('bobcarol5:00')
  await expect(fa.locator('td').nth(1)).toHaveText('alice15:00')
  await expect(fa.locator('td').nth(2)).toHaveText('—')
  await expect(table.locator('tfoot tr').nth(1).locator('td')).toHaveText(['3 / 3', '1 / 2', '0 / 1', '0 / 0'])
  // Refresh must recompute counts and FA when a pending result becomes AC.
  standings[1]!.problems.b = { points: 100, wrong: 0, pending: 0, acceptedAt: '2026-09-15T12:04:00Z' }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(fa.locator('td').nth(1)).toHaveText('bob4:00')
  await expect(table.locator('tfoot tr').nth(1).locator('td').nth(1)).toHaveText('2 / 2')
  await expect(table.getByRole('img', { name: '判定待ち' })).toHaveCount(0)
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const scroll = page.getByRole('region', { name: '順位表のスクロール領域' })
  expect(await scroll.evaluate(el => el.scrollWidth > el.clientWidth)).toBe(true)
  await scroll.focus()
  await page.keyboard.press('End')
  await expect(table.locator('tfoot')).toBeVisible()
})

test('problem labels continue after Z and match the problem list', async ({ page }) => {
  const many = { ...contest, problems: Array.from({ length: 28 }, (_, i) => ({ id: `p${i}`, points: 100, title: `問題${i}` })) }
  await page.route(`**/api/contests/${id}**`, route => route.fulfill({ json: route.request().url().includes('/standings') ? rows : many }))
  await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.standings-table thead th').nth(30)).toHaveText('AA')
  await expect(page.locator('.standings-table thead th').nth(31)).toHaveText('AB')
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
  await expect(page.locator('.contest-problems tbody tr').nth(0).locator('td').first()).toHaveText('A')
  await expect(page.locator('.contest-problems tbody tr').nth(27).locator('td').first()).toHaveText('AB')
})

test('standings retain empty and error states', async ({ page }) => {
  let failed = false
  await page.route(`**/api/contests/${id}/standings`, route => route.fulfill(failed ? { status: 502, json: {} } : { json: [] }))
  await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
  await expect(page.getByText('参加者はまだいません。')).toBeVisible()
  failed = true
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.getByRole('alert')).toHaveText('順位表を取得できませんでした。')
  await expect(page.locator('.standings-table')).toHaveCount(0)
})


test('contest problem rows highlight own ACs and refresh after judging', async ({ page }, testInfo) => {
  const current = { ...contest, title: 'ShareOJ Beginner Contest', problems: [
    { id: 'a', title: 'A + B', points: 100, solved: true },
    { id: 'b', title: 'Range Sum Query', points: 200, solved: false },
    { id: 'c', title: 'Shortest Path', points: 300, solved: false },
  ] }
  await page.clock.install()
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: startsAt } } }))
  await page.route(`**/api/contests/${id}**`, route => route.fulfill({ json: route.request().url().includes('/standings') ? rows : current }))
  await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
  const problems = page.locator('.contest-problems tbody tr')
  await expect(problems.nth(0)).toHaveClass('solved')
  await expect(problems.nth(1)).not.toHaveClass('solved')
  current.problems[1]!.solved = true
  await page.clock.fastForward(15000)
  await expect(problems.nth(1)).toHaveClass('solved')
  await expect(problems.nth(1)).toHaveCSS('background-color', 'rgb(237, 249, 241)')
  await expect(problems.nth(2)).not.toHaveClass('solved')
  await expect(page.getByRole('link', { name: 'Range Sum Query（AC 済み）', exact: true })).toBeVisible()
  await page.setViewportSize({ width: 1280, height: 1000 })
  await page.screenshot({ path: testInfo.outputPath('contest-solved-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('contest-solved-mobile.png'), fullPage: true })
})


test('scheduled problems show points and labels without links', async ({ page }) => {
  const scheduled: Contest = { ...contest, status: 'scheduled', problems: [
    { id: '', points: 100 },
    { id: '', points: 200 },
  ] }
  await page.route(`**/api/contests/${id}**`, route => route.fulfill({ json: route.request().url().includes('/standings') ? [] : scheduled }))
  await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
  const problems = page.locator('.contest-problems tbody tr')
  await expect(problems).toHaveCount(2)
  await expect(problems.locator('th')).toHaveText(['???', '???'])
  await expect(problems.locator('td:last-child')).toHaveText(['100 点', '200 点'])
  await expect(problems.locator('a')).toHaveCount(0)
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '順位表', exact: true }).click()
  await expect(page.locator('.standings-table thead th')).toHaveText(['順位', 'ユーザー', '得点', '時間', 'A', 'B'])
  await expect(page.locator('.standings-table thead a')).toHaveCount(0)
})
