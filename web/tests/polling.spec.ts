import { expect, test, type Page } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

// The clock stands still between steps, so every poll falls at a known time. The first step ends 15 seconds after
// the page loaded and later ones are whole intervals, which keeps every poll well inside a step.
async function load(page: Page, path: string) {
  await page.clock.install()
  await page.goto(path, { waitUntil: 'networkidle' })
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  await page.clock.runFor(15_000)
}
// A poll sets its next one only once it is answered, so each step waits for the answer in real time.
async function run(page: Page, seconds: number, step = 30) {
  for (let passed = 0; passed < seconds; passed += step) {
    await page.clock.runFor(step * 1000)
    await page.waitForTimeout(200)
  }
}
// The layout reads the judge catalogue every 30 seconds on every page.
async function countCatalog(page: Page) {
  const counter = { count: 0 }
  await page.route('**/api/runtimes', (route) => {
    counter.count++
    return route.fulfill({ json: { items: [{ id: 'cpp17', label: 'C++17 (GCC)' }], maintenance: false } })
  })
  return counter
}

test('a page nobody touches for three minutes polls a quarter as often, and reads again at the first input', async ({ page }) => {
  const counter = await countCatalog(page)
  await load(page, '/')
  await page.mouse.move(10, 10)
  let start = counter.count
  // In use: every 30 seconds. A step can end before its answer arrives, so the counts allow one poll either way.
  await run(page, 180)
  expect(counter.count - start).toBeGreaterThanOrEqual(5)
  // Untouched for three minutes by now: every 2 minutes, where the usual pace would read 16 times.
  await run(page, 60)
  start = counter.count
  await run(page, 480)
  expect(counter.count - start).toBeGreaterThanOrEqual(3)
  expect(counter.count - start).toBeLessThanOrEqual(5)
  // Once the last read is over 30 seconds old, the first input reads at once instead of waiting for the slow timer.
  start = counter.count
  while (counter.count === start) await run(page, 30)
  await run(page, 60)
  start = counter.count
  await page.keyboard.press('Shift')
  await expect.poll(() => counter.count - start).toBe(1)
  // Back in use: every 30 seconds again.
  await page.waitForTimeout(200)
  await run(page, 120)
  expect(counter.count - start).toBeGreaterThanOrEqual(4)
})

test('input on a page in use does not read early', async ({ page }) => {
  const counter = await countCatalog(page)
  await load(page, '/')
  const start = counter.count
  await page.keyboard.press('Shift')
  await page.mouse.move(40, 40)
  await page.waitForTimeout(200)
  expect(counter.count - start).toBe(0)
  await run(page, 30)
  expect(counter.count - start).toBe(1)
})

test('a tab that comes back into view reads what it skipped while hidden', async ({ page }) => {
  // The home page reads the upcoming contests every 60 seconds.
  let reads = 0
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/contests/upcoming') reads++ })
  await load(page, '/')
  const hide = (hidden: boolean) => page.evaluate((value) => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => value })
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => value ? 'hidden' : 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
  }, hidden)
  await hide(true)
  const start = reads
  await run(page, 150)
  expect(reads - start).toBe(0)
  await hide(false)
  await expect.poll(() => reads - start).toBe(1)
  // The timer starts over from that read.
  await run(page, 30)
  expect(reads - start).toBe(1)
  await run(page, 90)
  expect(reads - start).toBeGreaterThanOrEqual(2)
})

test('a submission being judged is followed every 2 seconds however long the page is left alone', async ({ page }) => {
  const problemId = '11111111-1111-4111-8111-111111111111'
  const submissionId = '22222222-2222-4222-8222-222222222222'
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
  let reads = 0
  await page.route(`**/api/my/submissions/${submissionId}`, (route) => {
    reads++
    return route.fulfill({ json: { id: submissionId, problemId, problemVersion: 2, problemTitle: 'A + B', runtime: 'cpp17-local', source: 'int main(){}', status: 'QUEUED', result: null, createdAt: '2026-09-01T00:00:00Z' } })
  })
  await load(page, `/my/submissions/${shortId(submissionId)}`)
  await expect.poll(() => reads).toBeGreaterThan(0)
  await run(page, 240, 60)
  const start = reads
  // The slowed pace would read once or twice.
  await run(page, 10, 2)
  expect(reads - start).toBeGreaterThanOrEqual(4)
})
