import { expect, test, type Locator, type Page } from '@playwright/test'
import type { Contest, Standing } from '../shared/types/contest'
import { test as signedIn } from './fixtures/account'

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
  await expect(alice.locator('td').nth(3)).toHaveText('100(1)誤答 1 回10:00')
  await expect(alice.locator('td').nth(4)).toHaveText('10015:00')
  await expect(table.getByText('(0)', { exact: true })).toHaveCount(0)
  // Screen readers read the count as words instead of the bracketed number.
  const wrong = table.locator('.standing-wrong').filter({ hasText: '誤答 2 回' })
  await expect(wrong.locator('[aria-hidden="true"]')).toHaveText('(2)')
  await expect(wrong.getByText('誤答 2 回', { exact: true })).toHaveClass('visually-hidden')
  await expect(wrong).toHaveCSS('color', 'rgb(161, 38, 34)')
  await expect(table.getByRole('img', { name: '判定待ち' })).toHaveCount(1)
  await expect(table.locator('svg[aria-label="判定待ち"]')).toBeVisible()
  const fa = table.locator('tfoot tr').nth(0)
  await expect(fa.locator('td').nth(0)).toHaveText('bobcarol5:00')
  await expect(fa.locator('td').nth(1)).toHaveText('alice15:00')
  await expect(fa.locator('td').nth(2)).toHaveText('—')
  await expect(table.locator('tfoot tr').nth(1).locator('td')).toHaveText(['3 / 3', '1 / 2', '0 / 1', '0 / 0'])
  // A single page still says how many rows there are, but has no page numbers.
  await expect(page.getByText('全 3 件中 1〜3 件目', { exact: true })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'ページ', exact: true })).toHaveCount(0)
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
  await expect(page.getByLabel('表示件数')).toHaveCount(0)
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

test('the refresh button is an icon on the heading row at every width', async ({ page }, testInfo) => {
  await page.route(`**/api/contests/${id}**`, route => route.fulfill({ json: route.request().url().includes('/standings') ? rows : contest }))
  await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
  const refresh = page.getByRole('button', { name: '今すぐ更新' })
  await expect(refresh).toHaveText('')
  await expect(refresh).toHaveAttribute('title', '今すぐ更新')
  const heading = page.getByRole('heading', { name: '公式順位表' })
  for (const width of [320, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    const [title, button] = [await heading.boundingBox(), await refresh.boundingBox()]
    expect(Math.abs(title!.y + title!.height / 2 - (button!.y + button!.height / 2))).toBeLessThan(8)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await page.setViewportSize({ width: 375, height: 900 })
  await page.locator('.contest-section-heading').screenshot({ path: testInfo.outputPath('standings-heading.png') })
})

// Every row tried problem a and the even positions solved it, one second apart, so its footer reads 500 / 1000 on any page.
const thousand: Standing[] = Array.from({ length: 1000 }, (_, i) => ({ rank: i + 1, handle: `user${i}`, points: i % 2 ? 0 : 100, timeMs: 0, problems: {
  a: i % 2 ? { points: 0, wrong: 1, pending: 0 } : { points: 100, wrong: 0, pending: 0, acceptedAt: new Date(Date.parse(startsAt) + (i + 1) * 1000).toISOString() },
} }))
async function routeThousand(page: Page, current: Contest = contest, rows = () => thousand) {
  await page.route(`**/api/contests/${id}**`, route => route.fulfill({ json: route.request().url().includes('/standings') ? rows() : current }))
}
// The server renders the fixture's empty table; the refresh reads the routed rows.
async function openStandings(page: Page, query: string) {
  await page.goto(`/contests/${id}?view=standings${query}`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
}
async function expectPages(nav: Locator, links: string[], current: string) {
  await expect(nav.getByRole('link')).toHaveText(links)
  await expect(nav.locator('[aria-current]')).toHaveCount(1)
  await expect(nav.locator('span[aria-current="page"]')).toHaveText(current)
}

test('standings are split into pages with AtCoder spacing', async ({ page }, testInfo) => {
  await routeThousand(page)
  await openStandings(page, '&page=20')
  const table = page.locator('.standings-table')
  const navs = page.getByRole('navigation', { name: 'ページ', exact: true })
  const first = table.locator('tbody tr').first().locator('th')
  await expect(page.getByText('全 1,000 件中 381〜400 件目', { exact: true })).toBeVisible()
  await expectPages(navs.first(), ['1', '5', '13', '17', '18', '19', '21', '22', '23', '27', '35', '50'], '20')
  // The numbers come again below the table.
  await expect(navs).toHaveCount(2)
  await expect(table.locator('tbody tr')).toHaveCount(20)
  // Signed out, no row is anyone's own.
  await expect(page.locator('.standing-mine')).toHaveCount(0)
  await expect(first).toHaveText('user380')
  // The footer counts every row, not only the page's.
  await expect(table.locator('tfoot tr').nth(0).locator('td').nth(0)).toHaveText('user00:01')
  await expect(table.locator('tfoot tr').nth(1).locator('td')).toHaveText(['500 / 1000', '0 / 0', '0 / 0', '0 / 0'])
  // The count, the size and the numbers share one line on a wide screen and wrap without a sideways scroll on a narrow one.
  const boxes = await Promise.all([page.getByText('全 1,000 件中 381〜400 件目', { exact: true }), page.getByLabel('表示件数'), navs.first()].map(item => item.boundingBox()))
  for (const box of boxes) expect(Math.abs(box!.y + box!.height / 2 - (boxes[0]!.y + boxes[0]!.height / 2))).toBeLessThan(8)
  await page.screenshot({ path: testInfo.outputPath('standings-pages-1280.png') })
  await page.setViewportSize({ width: 320, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('standings-pages-320.png'), fullPage: true })
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.setViewportSize({ width: 1280, height: 720 })
  await navs.first().getByRole('link', { name: '35', exact: true }).click()
  await expect(page).toHaveURL(/[?&]page=35$/)
  await expect(first).toHaveText('user680')
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(first).toHaveText('user680')
  await navs.first().getByRole('link', { name: '1', exact: true }).click()
  await expect(page).toHaveURL(/\?view=standings$/)
  await expectPages(navs.first(), ['2', '3', '4', '8', '16', '32', '50'], '1')
  await expect(first).toHaveText('user0')
  await navs.last().getByRole('link', { name: '50', exact: true }).click()
  await expect(page).toHaveURL(/[?&]page=50$/)
  await expectPages(navs.first(), ['1', '19', '35', '43', '47', '48', '49'], '50')
  await expect(first).toHaveText('user980')
  // A page past the end shows the last one, and anything but a whole number from 1 the first.
  await openStandings(page, '&page=999')
  await expect(page.getByText('全 1,000 件中 981〜1,000 件目', { exact: true })).toBeVisible()
  await expect(navs.first().locator('span[aria-current="page"]')).toHaveText('50')
  for (const value of ['abc', '0']) {
    await openStandings(page, `&page=${value}`)
    await expect(page.getByText('全 1,000 件中 1〜20 件目', { exact: true })).toBeVisible()
    await expect(first).toHaveText('user0')
  }
})

test('the page size is remembered per browser and keeps the first row', async ({ page, context }) => {
  await routeThousand(page)
  await openStandings(page, '&page=3')
  const size = page.getByLabel('表示件数')
  await expect(page.getByText('全 1,000 件中 41〜60 件目', { exact: true })).toBeVisible()
  await expect(size).toHaveValue('20')
  // Row 41 topped the page, so 50 rows a page opens the first page. The size and the page change together: page 3 at 50
  // rows a page (from user100) never renders on the way.
  await page.locator('.standings-table tbody').evaluate(tbody => {
    const added: string[] = (window as unknown as { addedRows: string[] }).addedRows = []
    new MutationObserver(records => { for (const record of records) for (const node of record.addedNodes) if (node instanceof HTMLTableRowElement) added.push(node.querySelector('th')?.textContent?.trim() ?? '') }).observe(tbody, { childList: true })
  })
  await size.selectOption('50')
  await expect(page).toHaveURL(/\?view=standings$/)
  await expect(page.getByText('全 1,000 件中 1〜50 件目', { exact: true })).toBeVisible()
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(50)
  const added = await page.evaluate(() => (window as unknown as { addedRows: string[] }).addedRows)
  expect(added).toContain('user0')
  expect(added).not.toContain('user100')
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(size).toHaveValue('50')
  await expect(page.getByText('全 1,000 件中 1〜50 件目', { exact: true })).toBeVisible()
  expect((await context.cookies()).find(cookie => cookie.name === 'openoj-page-size-standings')?.value).toBe('50')
  // Row 51 tops the second page; at 20 rows a page it is on the third.
  await page.getByRole('navigation', { name: 'ページ', exact: true }).first().getByRole('link', { name: '2', exact: true }).click()
  await expect(page.getByText('全 1,000 件中 51〜100 件目', { exact: true })).toBeVisible()
  await size.selectOption('20')
  await expect(page).toHaveURL(/[?&]page=3$/)
  await expect(page.getByText('全 1,000 件中 41〜60 件目', { exact: true })).toBeVisible()
  // A size that is not offered reads as the initial 20.
  await context.addCookies([{ name: 'openoj-page-size-standings', value: '7', url: new URL(page.url()).origin }])
  await openStandings(page, '')
  await expect(page.getByText('全 1,000 件中 1〜20 件目', { exact: true })).toBeVisible()
  await expect(size).toHaveValue('20')
})

test('switching the scope or the freeze returns to the first page', async ({ page }) => {
  await routeThousand(page, { ...contest, onsiteDivision: true, freezeAt: '2026-09-15T13:00:00Z', canViewLiveStandings: true })
  await openStandings(page, '&page=3')
  await expect(page.getByText('全 1,000 件中 41〜60 件目', { exact: true })).toBeVisible()
  await page.getByRole('switch', { name: 'オンサイトのみ表示' }).click()
  await expect(page).toHaveURL(/\?view=standings&scope=onsite$/)
  await expect(page.getByText('全 1,000 件中 1〜20 件目', { exact: true })).toBeVisible()
  await openStandings(page, '&page=3')
  await expect(page.getByText('全 1,000 件中 41〜60 件目', { exact: true })).toBeVisible()
  await page.getByRole('switch', { name: '凍結なしで表示' }).click()
  await expect(page).toHaveURL(/\?view=standings$/)
  await expect(page.getByText('全 1,000 件中 1〜20 件目', { exact: true })).toBeVisible()
})

test('a page chosen under the table opens at its first row', async ({ page }) => {
  await routeThousand(page)
  await openStandings(page, '&page=20')
  const navs = page.getByRole('navigation', { name: 'ページ', exact: true })
  await navs.last().scrollIntoViewIfNeeded()
  await expect(page.getByText('全 1,000 件中 381〜400 件目', { exact: true })).not.toBeInViewport()
  await navs.last().getByRole('link', { name: '21', exact: true }).click()
  await expect(page).toHaveURL(/[?&]page=21$/)
  await expect(page.getByText('全 1,000 件中 401〜420 件目', { exact: true })).toBeInViewport()
  await expect(page.locator('.standings-table tbody tr').first().locator('th')).toHaveText('user400')
})

test('the refresh keeps the page, and shows the last page once rows drop', async ({ page }) => {
  let rows = thousand
  await routeThousand(page, contest, () => rows)
  const current = page.getByRole('navigation', { name: 'ページ', exact: true }).first().locator('span[aria-current="page"]')
  await openStandings(page, '&page=50')
  await expect(page.getByText('全 1,000 件中 981〜1,000 件目', { exact: true })).toBeVisible()
  rows = thousand.slice(0, 990)
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.getByText('全 990 件中 981〜990 件目', { exact: true })).toBeVisible()
  await expect(current).toHaveText('50')
  rows = thousand.slice(0, 970)
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.getByText('全 970 件中 961〜970 件目', { exact: true })).toBeVisible()
  await expect(current).toHaveText('49')
  // The URL keeps the page asked for, so rows coming back return to it.
  await expect(page).toHaveURL(/[?&]page=50$/)
  await openStandings(page, '&page=20')
  await expect(page.getByText('全 970 件中 381〜400 件目', { exact: true })).toBeVisible()
  await Promise.all([page.waitForResponse(response => response.url().includes('/standings')), page.getByRole('button', { name: '今すぐ更新' }).click()])
  await expect(page.getByText('全 970 件中 381〜400 件目', { exact: true })).toBeVisible()
  await expect(current).toHaveText('20')
  await expect(page.locator('.standings-table tbody tr').first().locator('th')).toHaveText('user380')
})

// The fixture's account is ui_test; the hundred rows put it at the given position.
const hundred = (mine: number, patch: (row: Standing, i: number) => Standing = row => row): Standing[] => Array.from({ length: 100 }, (_, i) => patch({ rank: i + 1, handle: i === mine ? 'ui_test' : `user${i}`, points: 0, timeMs: 0, problems: {} }, i))

signedIn('own row is highlighted and pinned above or below the page', async ({ page }) => {
  let rows = hundred(11)
  await routeThousand(page, contest, () => rows)
  await openStandings(page, '&page=3')
  const lines = page.locator('.standings-table tbody tr')
  await expect(page.getByText('全 100 件中 41〜60 件目', { exact: true })).toBeVisible()
  await expect(lines).toHaveCount(22)
  const mine = lines.first()
  await expect(mine).toHaveClass(/standing-mine/)
  await expect(mine.locator('td').first()).toHaveText('12')
  await expect(mine).toHaveCSS('background-color', 'rgb(237, 249, 241)')
  await expect(mine.locator('th .visually-hidden')).toHaveText('（自分）')
  await expect(lines.nth(1)).toHaveText('⋮')
  await expect(lines.nth(1)).toHaveAttribute('aria-hidden', 'true')
  await expect(lines.nth(1).locator('td')).toHaveAttribute('colspan', '8')
  await expect(lines.nth(2).locator('th')).toHaveText('user40')
  await expect(page.locator('.standing-mine')).toHaveCount(1)
  // After the page it follows the rows, above the footer.
  rows = hundred(86)
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(lines.nth(21)).toHaveClass(/standing-mine/)
  await expect(lines.nth(20)).toHaveText('⋮')
  await expect(lines).toHaveCount(22)
  await expect(page.locator('.standings-table tfoot')).toBeVisible()
  // Inside the page it is highlighted in place, with nothing added.
  rows = hundred(45)
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(lines).toHaveCount(20)
  await expect(lines.filter({ hasText: '⋮' })).toHaveCount(0)
  await expect(lines.nth(5)).toHaveClass(/standing-mine/)
  await expect(page.locator('.standing-mine')).toHaveCount(1)
  await expect(lines.nth(5)).toHaveCSS('background-color', 'rgb(237, 249, 241)')
})

signedIn('a tie across the page boundary is placed by position', async ({ page }) => {
  await routeThousand(page, contest, () => hundred(60, (row, i) => i >= 59 ? { ...row, rank: 60 } : row))
  await openStandings(page, '&page=3')
  const lines = page.locator('.standings-table tbody tr')
  await expect(lines).toHaveCount(22)
  await expect(lines.last()).toHaveClass(/standing-mine/)
  await expect(lines.nth(20)).toHaveText('⋮')
})

signedIn('own team row is highlighted in a team contest', async ({ page }) => {
  const teams = hundred(-1, (row, i) => i === 1 ? { ...row, handle: '', team: { id: 't1', name: 'チームA', members: ['bob', 'ui_test'], onsite: false } } : row)
  await routeThousand(page, { ...contest, teamMode: true }, () => teams)
  await openStandings(page, '')
  const mine = page.locator('.standings-table tbody tr.standing-mine')
  await expect(mine).toHaveCount(1)
  await expect(mine.locator('th')).toContainText('チームA')
})
