import { expect, test, type Page } from '@playwright/test'
import { contestCountdown, contestRemaining } from '../shared/types/contest'
import { shortId } from '../shared/utils/short-id'

const section = (page: Page) => page.getByRole('region', { name: 'もうすぐ開催のコンテスト' })
const runningSection = (page: Page) => page.getByRole('region', { name: '開催中のコンテスト' })

test('the home page announces upcoming contests between the hero and the featured delivery', async ({ page }, testInfo) => {
  await page.goto('/')
  const rows = section(page).getByRole('listitem')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toContainText('あと5時間')
  await expect(rows.nth(1)).toContainText('あと2日')
  await expect(rows.nth(0).locator('time')).toHaveText(/^\d{4}\/\d{2}\/\d{2} \d{1,2}:\d{2}$/)
  await expect(section(page).getByRole('link', { name: 'ShareOJ 秋の腕試し' })).toHaveAttribute('href', `/contests/${shortId('99999999-9999-4999-8999-999999999999')}`)
  await expect(section(page).getByRole('link', { name: 'コンテスト一覧' })).toHaveAttribute('href', '/contests')
  const top = async (locator: ReturnType<Page['locator']>) => (await locator.boundingBox())!.y
  const [hero, running, upcoming, delivery] = await Promise.all([top(page.locator('.hero')), top(runningSection(page)), top(section(page)), top(page.getByRole('region', { name: 'ShareOJ定期便' }))])
  expect(hero).toBeLessThan(running)
  expect(running).toBeLessThan(upcoming)
  expect(upcoming).toBeLessThan(delivery)
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`upcoming-${width}.png`), fullPage: true })
  }
})

test('the home page lists the contests in progress above the upcoming ones, with the time left', async ({ page }) => {
  await page.goto('/')
  const rows = runningSection(page).getByRole('listitem')
  await expect(rows).toHaveCount(1)
  await expect(rows.first()).toContainText('残り1時間')
  await expect(rows.first().locator('time')).toHaveText(/^\d{4}\/\d{2}\/\d{2} \d{1,2}:\d{2} まで$/)
  await expect(runningSection(page).getByRole('link', { name: 'ShareOJ 夜ふかしコンテスト' })).toHaveAttribute('href', `/contests/${shortId('cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd')}`)
  // The two sections share the link to every contest, under the lower one.
  await expect(runningSection(page).getByRole('link', { name: 'コンテスト一覧' })).toHaveCount(0)
  await expect(section(page).getByRole('link', { name: 'コンテスト一覧' })).toHaveCount(1)
})

test('the contests in progress carry the link to every contest when none is about to start', async ({ page }) => {
  await page.clock.install()
  await page.goto('/', { waitUntil: 'networkidle' })
  await page.route('**/api/contests/upcoming', route => route.fulfill({ json: { items: [] } }))
  await page.clock.fastForward(60_000)
  await expect(section(page)).toHaveCount(0)
  await expect(runningSection(page).getByRole('link', { name: 'コンテスト一覧' })).toHaveAttribute('href', '/contests')
  await page.route('**/api/contests/running', route => route.fulfill({ json: { items: [] } }))
  await page.clock.fastForward(60_000)
  await expect(runningSection(page)).toHaveCount(0)
})

test('the announcement is in the server-rendered page', async ({ request }) => {
  const html = await (await request.get('/')).text()
  expect(html).toContain('ShareOJ 秋の腕試し')
  expect(html).toContain('ShareOJ 夜ふかしコンテスト')
})

test('the countdown keeps the time of the server render, so a browser clock that is off does not change it', async ({ page }) => {
  await page.clock.install({ time: new Date(Date.now() + 3 * 3_600_000) })
  const errors: string[] = []
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  await page.goto('/', { waitUntil: 'networkidle' })
  await expect(section(page).getByRole('listitem').first()).toContainText('あと5時間')
  expect(errors.filter(error => error.includes('Hydration'))).toEqual([])
})

test('the section leaves the page once no contest is about to start', async ({ page }) => {
  await page.clock.install()
  await page.goto('/', { waitUntil: 'networkidle' })
  await expect(section(page)).toBeVisible()
  await page.route('**/api/contests/upcoming', route => route.fulfill({ json: { items: [] } }))
  await page.clock.fastForward(60_000)
  await expect(section(page)).toHaveCount(0)
})

test('the home page shows nothing about contests when they cannot be read', async ({ page }) => {
  const response = await page.goto('http://127.0.0.1:13001/')
  expect(response?.status()).toBe(200)
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await expect(section(page)).toHaveCount(0)
  await expect(runningSection(page)).toHaveCount(0)
})

const now = Date.parse('2026-11-06T00:00:00Z')
for (const [startsAt, left] of [
  ['2026-11-06T00:00:30Z', 'まもなく'],
  ['2026-11-06T00:01:00Z', 'あと1分'],
  ['2026-11-06T00:59:59Z', 'あと59分'],
  ['2026-11-06T01:00:00Z', 'あと1時間'],
  ['2026-11-06T23:59:59Z', 'あと23時間'],
  ['2026-11-07T00:00:00Z', 'あと1日'],
  ['2026-11-13T00:00:00Z', 'あと7日'],
] as const) {
  test(`a start at ${startsAt} reads ${left}`, () => {
    expect(contestCountdown(startsAt, now)).toBe(left)
  })
}

for (const [endsAt, left] of [
  ['2026-11-06T00:00:30Z', 'まもなく終了'],
  ['2026-11-06T00:01:00Z', '残り1分'],
  ['2026-11-06T01:00:00Z', '残り1時間'],
  ['2026-11-07T00:00:00Z', '残り1日'],
] as const) {
  test(`an end at ${endsAt} reads ${left}`, () => {
    expect(contestRemaining(endsAt, now)).toBe(left)
  })
}
