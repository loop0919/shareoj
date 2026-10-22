import type { BrowserContext, Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { fakeNotifications, shown } from './fixtures/notifications'
import { shortId } from '../shared/utils/short-id'

const cookieName = 'openoj-browser-notifications'
const kindLabels = ['質問と回答（全体公開を含む）', 'コンテストの開始と終了', '定期便の出題', '定期便の解説公開']
const denied = 'ブラウザで通知が拒否されています。ブラウザのサイトの設定で通知を許可してください。'
const receive = (page: Page) => page.getByRole('switch', { name: 'このブラウザで通知を受け取る', exact: true })
const kind = (page: Page, index: number) => page.getByRole('switch', { name: kindLabels[index], exact: true })

test('allowing notifications turns every kind on', async ({ page }) => {
  await fakeNotifications(page, { permission: 'default', answer: 'granted' })
  await page.goto('/my/settings?tab=notifications')
  await expect(page.getByText('ShareOJを開いているあいだ、このブラウザに通知を出します。閉じているときは届きません。')).toBeVisible()
  await expect(page.getByText('質問と回答と結果の公開は、ベルにも残ります。')).toBeVisible()
  await expect(receive(page)).not.toBeChecked()
  await expect(kind(page, 0)).toHaveCount(0)
  await receive(page).click()
  await expect(receive(page)).toBeChecked()
  for (const index of kindLabels.keys()) await expect(kind(page, index)).toBeChecked()
  await page.reload()
  await expect(receive(page)).toBeChecked()
  for (const index of kindLabels.keys()) await expect(kind(page, index)).toBeChecked()
  await kind(page, 2).click()
  await page.reload()
  await expect(kind(page, 2)).not.toBeChecked()
  await expect(kind(page, 0)).toBeChecked()
})

test('a refusal leaves the switch off and explains', async ({ page }) => {
  await fakeNotifications(page, { permission: 'default', answer: 'denied' })
  await page.goto('/my/settings?tab=notifications')
  await receive(page).click()
  await expect(page.getByText(denied)).toBeVisible()
  await expect(receive(page)).not.toBeChecked()
  await expect(kind(page, 0)).toHaveCount(0)
})

test('a revoked permission explains instead of listing kinds', async ({ page, context, baseURL }) => {
  const value = encodeURIComponent(JSON.stringify({ enabled: true, kinds: { clarifications: true, contests: true, featured: true, editorials: true } }))
  await context.addCookies([{ name: cookieName, value, url: baseURL! }])
  await fakeNotifications(page, { permission: 'denied', answer: 'denied' })
  await page.goto('/my/settings?tab=notifications')
  await expect(page.getByText(denied)).toBeVisible()
  await expect(kind(page, 0)).toHaveCount(0)
})

test.describe('phones', () => {
  test.use({ userAgent: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/126.0 Mobile Safari/537.36' })
  test('point to the bell', async ({ page }) => {
    await fakeNotifications(page, { permission: 'default', answer: 'granted' })
    await page.goto('/my/settings?tab=notifications')
    await expect(page.getByText('このブラウザでは通知を出せません。ベルで確かめてください。')).toBeVisible()
    await expect(page.getByRole('switch')).toHaveCount(0)
  })
})

test('the notification tab fits a phone screen', async ({ page }) => {
  await fakeNotifications(page, { permission: 'default', answer: 'granted' })
  await page.setViewportSize({ width: 375, height: 800 })
  await page.goto('/my/settings?tab=notifications')
  await receive(page).click()
  await expect(kind(page, 3)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

const contestId = '88888888-8888-4888-8888-888888888888'
const notice = (kind: string, id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa0') => ({ id, contestId, title: '春のコンテスト', actor: 'bob', kind, createdAt: '2026-10-22T11:00:00Z' })
async function presetCookie(context: BrowserContext, baseURL: string, clarifications = true) {
  const value = encodeURIComponent(JSON.stringify({ enabled: true, kinds: { clarifications, contests: true, featured: true, editorials: true } }))
  await context.addCookies([{ name: cookieName, value, url: baseURL }])
}
// The first request answers `first`; every later one answers `later`.
async function serveNotifications(page: Page, first: unknown[], later: unknown[]) {
  const counter = { count: 0 }
  await page.route('**/api/my/notifications', route => {
    counter.count++
    return route.fulfill({ json: { notifications: counter.count === 1 ? first : later } })
  })
  return counter
}

test('a new question is shown once and opens the clarifications', async ({ page, context, baseURL }) => {
  await presetCookie(context, baseURL!)
  await fakeNotifications(page, { permission: 'granted', answer: 'granted' })
  await serveNotifications(page, [], [notice('clarification_question')])
  await page.clock.install()
  await page.goto('/')
  await page.clock.runFor(30_000)
  await expect.poll(() => shown(page)).toEqual([{ title: '質問が届きました', body: '春のコンテスト', tag: 'notification:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa0' }])
  await page.clock.runFor(30_000)
  expect(await shown(page)).toHaveLength(1)
  await page.evaluate(() => (window as unknown as { __instances: { onclick: () => void }[] }).__instances[0]!.onclick())
  await expect(page).toHaveURL(`/contests/${shortId(contestId)}?view=clarifications`)
})

test('old unread and result notifications stay quiet', async ({ page, context, baseURL }) => {
  await presetCookie(context, baseURL!)
  await fakeNotifications(page, { permission: 'granted', answer: 'granted' })
  const old = notice('clarification_question', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb0')
  await serveNotifications(page, [old], [old, notice('contest_results')])
  await page.clock.install()
  await page.goto('/')
  await page.clock.runFor(60_000)
  expect(await shown(page)).toEqual([])
})

test('a switched-off clarification kind stays quiet', async ({ page, context, baseURL }) => {
  await presetCookie(context, baseURL!, false)
  await fakeNotifications(page, { permission: 'granted', answer: 'granted' })
  await serveNotifications(page, [], [notice('clarification_answer')])
  await page.clock.install()
  await page.goto('/')
  await page.clock.runFor(60_000)
  expect(await shown(page)).toEqual([])
})

for (const [label, clarifications, atLeast] of [['keep polling when wanted', true, 2], ['stop polling when not', false, 0]] as const) {
  test(`hidden tabs ${label}`, async ({ page, context, baseURL }) => {
    await presetCookie(context, baseURL!, clarifications)
    await fakeNotifications(page, { permission: 'granted', answer: 'granted' })
    await page.addInitScript(() => {
      Object.defineProperty(document, 'hidden', { get: () => true })
      Object.defineProperty(document, 'visibilityState', { get: () => 'hidden' })
    })
    const counter = await serveNotifications(page, [], [])
    await page.clock.install()
    await page.goto('/')
    await expect.poll(() => counter.count).toBeGreaterThanOrEqual(1)
    const before = counter.count
    await page.clock.runFor(90_000)
    await expect.poll(() => counter.count - before).toBeGreaterThanOrEqual(atLeast)
    if (!atLeast) expect(counter.count - before).toBe(0)
  })
}
