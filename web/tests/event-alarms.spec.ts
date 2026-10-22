import type { BrowserContext, Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { fakeNotifications, shown } from './fixtures/notifications'
import type { Contest, ScheduledContest } from '../shared/types/contest'
import type { FeaturedPage } from '../shared/types/featured'
import { shortId } from '../shared/utils/short-id'

// The fake clock starts at T; the times below count minutes from it.
const T = Date.parse('2026-10-23T03:00:00Z')
const at = (minutes: number) => new Date(T + minutes * 60_000).toISOString()
const contestId = '88888888-8888-4888-8888-888888888888'
const otherId = '99999999-9999-4999-8999-999999999999'
const thirdId = '77777777-7777-4777-8777-777777777777'
const title = '秋のコンテスト'
const scheduled = (start: number, end = start + 60, id = contestId): ScheduledContest => ({ id, title, startsAt: at(start), endsAt: at(end) })
const started = (start: number, id = contestId) => ({ title: 'コンテストが始まりました', body: title, tag: `contest-start:${id}:${at(start)}` })
const missing = (slot: 'easy' | 'hard') => ({ slot, kind: 'missing' as const, difficulty: null, writer: '', testers: [] })
// The next round a week away with both slots missing, and no round waiting for its editorial: 定期便 stays quiet.
const quiet: FeaturedPage = { nextNumber: 12, nextAt: at(7 * 24 * 60), hasMore: false, waiting: { easy: 0, hard: 0 }, nextSlots: [missing('easy'), missing('hard')], current: null, items: [] }

async function prepare(page: Page, context: BrowserContext, baseURL: string, kinds: { contests?: boolean } = {}) {
  const value = encodeURIComponent(JSON.stringify({ enabled: true, kinds: { clarifications: true, contests: true, featured: true, editorials: true, ...kinds } }))
  await context.addCookies([{ name: 'openoj-browser-notifications', value, url: baseURL }])
  await fakeNotifications(page, { permission: 'granted', answer: 'granted' })
  await page.clock.install({ time: T })
}
// Answers the schedule with state.items and 定期便 with state.featured; reads counts the schedule requests.
async function serve(page: Page, items: ScheduledContest[], featured = quiet) {
  const state = { items, featured, reads: 0 }
  await page.route('**/api/my/contest-schedule', route => { state.reads++; return route.fulfill({ json: { items: state.items } }) })
  await page.route('**/api/featured', route => route.fulfill({ json: state.featured }))
  return state
}
// The terms page has no timers of its own. Waiting for the reads sets the alarms before the clock moves.
async function open(page: Page, path = '/terms', reads = ['**/api/my/contest-schedule', '**/api/featured']) {
  const responses = reads.map(url => page.waitForResponse(url))
  await page.goto(path)
  for (const response of responses) await (await response).finished()
}
const click = (page: Page, index: number) => page.evaluate(index => (window as unknown as { __instances: { onclick: () => void }[] }).__instances[index]!.onclick(), index)

test('a joined contest notifies at its start and end', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(2, 62)])
  await open(page)
  await page.clock.runFor('02:00')
  await expect.poll(() => shown(page)).toEqual([started(2)])
  await page.clock.runFor('01:00:00')
  await expect.poll(() => shown(page)).toEqual([started(2), { title: 'コンテストが終了しました', body: title, tag: `contest-end:${contestId}:${at(62)}` }])
  // The start opens the problems, and the end the standings.
  await click(page, 0)
  await expect(page).toHaveURL(`/contests/${shortId(contestId)}?view=problems`)
  await click(page, 1)
  await expect(page).toHaveURL(`/contests/${shortId(contestId)}?view=standings`)
})

const fresh = { slot: 'easy' as const, kind: 'new' as const, difficulty: 3, writer: 'bob', testers: [] }
// Released 23 hours before its editorials: a new problem hidden until T+3 and a revival shown from the release.
const waiting: NonNullable<FeaturedPage['current']> = { number: 11, scheduledAt: at(3 - 23 * 60), slots: [
  { ...fresh, problemId: 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee1', title: '新作', revealAt: at(3), editorialHidden: true },
  { ...fresh, slot: 'hard', kind: 'revival', problemId: 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee2', title: '復刻', revealAt: at(3 - 23 * 60), editorialHidden: false },
] }
const released = { title: '定期便を出題しました', body: '定期便 vol.12', tag: `featured-release:${at(1)}` }
const revealed = { title: '定期便の解説を公開しました', body: '定期便 vol.11', tag: `featured-editorial:${waiting.scheduledAt}` }
for (const [label, nextSlots, want] of [
  ['a round with a problem notifies at its release and its editorials', [fresh, missing('hard')], [released, revealed]],
  ['an all-missing round has no release', [missing('easy'), missing('hard')], [revealed]],
] as const) {
  test(label, async ({ page, context, baseURL }) => {
    await prepare(page, context, baseURL!)
    await serve(page, [], { ...quiet, nextAt: at(1), nextSlots: [...nextSlots], current: waiting })
    await open(page)
    await page.clock.runFor('03:00')
    await expect.poll(() => shown(page)).toEqual(want)
    await click(page, 0)
    await expect(page).toHaveURL('/featured')
  })
}

test('a timer that wakes more than five minutes late stays quiet', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(2, 120), scheduled(9, 120, otherId)])
  await open(page)
  // The computer sleeps through both starts and wakes at T+10: eight minutes late is too late, one minute is not.
  await page.clock.fastForward('10:00')
  await expect.poll(() => shown(page)).toEqual([started(9, otherId)])
})

test('a start after a sleep notifies on time', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(50, 120)])
  await open(page)
  await page.clock.runFor('05:00')
  // The computer sleeps from T+5 to T+45: the wall clock jumps, while the timers wait as if no time had passed.
  await page.clock.setSystemTime(T + 45 * 60_000)
  await page.clock.runFor('07:00')
  await expect.poll(() => shown(page)).toEqual([started(50)])
  // The timer set before the sleep does not ring again.
  await page.clock.runFor('01:00:00')
  expect(await shown(page)).toEqual([started(50)])
})

test('waking is noticed before the next read', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(48, 120)])
  await open(page)
  await page.clock.runFor('02:00')
  // Asleep from T+2 to T+45. By the wall clock the next read is not due before T+53, so only the beat can ring the
  // start by T+50.
  await page.clock.setSystemTime(T + 45 * 60_000)
  await page.clock.runFor('05:00')
  await expect.poll(() => shown(page)).toEqual([started(48)])
})

test('a page shown after a sleep rings a start that just passed', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(44, 120)])
  await open(page)
  await page.clock.runFor('02:00')
  // Asleep from T+2 to T+45, a minute past the start; the page is shown again before the next beat.
  await page.clock.setSystemTime(T + 45 * 60_000)
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect.poll(() => shown(page)).toEqual([started(44)])
})

test('a start rings once across reloads and pages', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  const state = await serve(page, [scheduled(-2)])
  await open(page)
  // Opened two minutes after the start, the page still announces it.
  await expect.poll(() => shown(page)).toEqual([started(-2)])
  // After a reload, a start not announced yet still rings, which shows the page has caught up without the first one.
  state.items = [scheduled(-2), scheduled(-1, 60, otherId)]
  await page.reload()
  await expect.poll(() => shown(page)).toEqual([started(-1, otherId)])
  // So does a second page of the same browser.
  const second = await context.newPage()
  await fakeNotifications(second, { permission: 'granted', answer: 'granted' })
  await second.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'ui-test-user' } } }))
  await serve(second, [scheduled(-2), scheduled(-1, 60, otherId), scheduled(-1, 60, thirdId)])
  await second.goto('/terms')
  await expect.poll(() => shown(second)).toEqual([started(-1, thirdId)])
})

test('without browser storage a start still rings once', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await page.addInitScript(() => Object.defineProperty(window, 'localStorage', { get() { throw new DOMException('blocked', 'SecurityError') } }))
  await serve(page, [scheduled(2)])
  await open(page)
  const reread = page.waitForResponse('**/api/my/contest-schedule')
  await page.clock.runFor('02:00')
  await expect.poll(() => shown(page)).toEqual([started(2)])
  // The read after the start finds it rung in memory.
  await (await reread).finished()
  await page.clock.runFor('03:00')
  expect(await shown(page)).toEqual([started(2)])
})

test('a moved start notifies at the new time', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  const state = await serve(page, [scheduled(20)])
  await open(page)
  state.items = [scheduled(40)]
  const reread = page.waitForResponse('**/api/my/contest-schedule')
  await page.clock.runFor('11:00')
  await (await reread).finished()
  await page.clock.runFor('10:00')
  expect(await shown(page)).toEqual([])
  await page.clock.runFor('20:00')
  await expect.poll(() => shown(page)).toEqual([started(40)])
})

test('a contest joined after the page opened notifies too', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  const state = await serve(page, [])
  const joined: Contest = {
    id: contestId, title, author: 'alice', problemAuthors: [], testers: [], description: '', startsAt: at(3), endsAt: at(63), penaltyMinutes: 5,
    version: 1, status: 'scheduled', canEdit: false, participating: true, official: true, canViewSubmissions: false, problems: [],
  }
  await page.route(`**/api/my/contests/${contestId}/participation`, route => {
    state.items = [scheduled(3)]
    return route.fulfill({ json: joined })
  })
  await open(page, `/contests/${shortId(contestId)}`)
  const reread = page.waitForResponse('**/api/my/contest-schedule')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await (await reread).finished()
  await expect(page.getByText('参加済み', { exact: true })).toBeVisible()
  await page.clock.runFor('03:00')
  await expect.poll(() => shown(page)).toEqual([started(3)])
})

test('a switched-off kind sets no timer and skips its read', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!, { contests: false })
  const state = await serve(page, [scheduled(2)])
  await open(page, '/terms', ['**/api/featured'])
  await page.clock.runFor('03:00')
  expect(await shown(page)).toEqual([])
  expect(state.reads).toBe(0)
})

test('guests read no schedule', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  const state = await serve(page, [scheduled(2)])
  await open(page, '/terms', ['**/api/auth/me'])
  await page.clock.runFor('11:00')
  expect(state.reads).toBe(0)
  expect(await shown(page)).toEqual([])
})

test('switching the kind off cancels its timers', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(2)])
  await open(page, '/my/settings?tab=notifications')
  const contests = page.getByRole('switch', { name: 'コンテストの開始と終了', exact: true })
  await contests.click()
  await expect(contests).not.toBeChecked()
  await page.clock.runFor('03:00')
  expect(await shown(page)).toEqual([])
})

test('signing out cancels the timers', async ({ page, context, baseURL }) => {
  await prepare(page, context, baseURL!)
  await serve(page, [scheduled(2)])
  await open(page)
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.route('**/api/auth/logout', route => route.fulfill({ json: {} }))
  await page.getByRole('button', { name: 'アカウントメニュー' }).click()
  await page.getByRole('navigation', { name: 'アカウント' }).getByRole('button', { name: 'ログアウト', exact: true }).click()
  await expect(page).toHaveURL('/login')
  await page.clock.runFor('03:00')
  expect(await shown(page)).toEqual([])
})
