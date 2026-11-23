import type { BrowserContext, Page } from '@playwright/test'
import { expect, test } from '@playwright/test'
import { fakeNotifications } from './fixtures/notifications'
import { shortId } from '../shared/utils/short-id'
import { closedInStreaming } from '../shared/utils/streaming-mode'

const profile = { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' }
const contest = `/contests/${shortId('88888888-8888-4888-8888-888888888888')}`
const notify = encodeURIComponent(JSON.stringify({ enabled: true, kinds: { clarifications: true, contests: true, featured: true, editorials: true } }))

async function signIn(page: Page) {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile } }))
}
async function streamingOn(context: BrowserContext) {
  await context.addCookies([{ name: 'openoj-streaming', value: '1', url: 'http://127.0.0.1:13000' }])
}
// Scoped to the header: the settings tab of the same name is a button too.
const badge = (page: Page) => page.getByRole('banner').getByRole('button', { name: '配信モード', exact: true })
const streamingSwitch = (page: Page) => page.getByRole('switch', { name: 'このブラウザを配信モードにする' })

test('the settings switch turns streaming mode on, and off only after a confirmation', async ({ page, context }) => {
  await signIn(page)
  await page.goto('/my/settings?tab=streaming')
  await expect(badge(page)).toHaveCount(0)
  await streamingSwitch(page).check()
  await expect(badge(page)).toBeVisible()
  expect((await context.cookies()).find(cookie => cookie.name === 'openoj-streaming')?.value).toBe('1')
  await page.reload()
  await expect(badge(page)).toBeVisible()
  // click, not uncheck(): the switch stays on until the confirmation, which uncheck() would read as a failed click.
  await streamingSwitch(page).click()
  const dialog = page.getByRole('dialog', { name: '配信モードを切りますか？' })
  await expect(dialog).toContainText('マイページやスタッフ向けの画面が、このブラウザで見えるようになります。')
  // Nothing is switched off before the confirmation.
  expect((await context.cookies()).find(cookie => cookie.name === 'openoj-streaming')?.value).toBe('1')
  await dialog.getByRole('button', { name: 'キャンセル', exact: true }).click()
  await expect(dialog).toBeHidden()
  await expect(badge(page)).toBeVisible()
  await expect(streamingSwitch(page)).toBeChecked()
  await badge(page).click()
  await dialog.getByRole('button', { name: '切る', exact: true }).click()
  await expect(badge(page)).toHaveCount(0)
  expect((await context.cookies()).some(cookie => cookie.name === 'openoj-streaming')).toBe(false)
  await expect(streamingSwitch(page)).not.toBeChecked()
})

test('the streaming cookie lasts a year and is readable by the page', async ({ page, context }) => {
  await signIn(page)
  await page.goto('/my/settings?tab=streaming')
  await streamingSwitch(page).check()
  await expect(badge(page)).toBeVisible()
  const cookie = (await context.cookies()).find(entry => entry.name === 'openoj-streaming')!
  expect(cookie.sameSite).toBe('Lax')
  expect(cookie.path).toBe('/')
  expect(cookie.httpOnly).toBe(false)
  const lifetime = cookie.expires - Date.now() / 1000
  expect(lifetime).toBeGreaterThan(60 * 60 * 24 * 364)
  expect(lifetime).toBeLessThanOrEqual(60 * 60 * 24 * 365)
})

async function watchReads(page: Page) {
  const reads: string[] = []
  page.on('request', request => {
    const path = new URL(request.url()).pathname
    if (path === '/api/my/notifications' || path === '/api/my/contest-schedule') reads.push(path)
  })
  await page.route('**/api/my/notifications', route => route.fulfill({ json: { notifications: [] } }))
  await page.route('**/api/my/contest-schedule', route => route.fulfill({ json: { items: [] } }))
  await fakeNotifications(page, { permission: 'granted', answer: 'granted' })
  return reads
}

test('without streaming mode the same page reads notifications', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'openoj-browser-notifications', value: notify, url: baseURL! }])
  await signIn(page)
  const reads = await watchReads(page)
  await page.goto('/')
  await expect.poll(() => reads).toContain('/api/my/notifications')
  await expect.poll(() => reads).toContain('/api/my/contest-schedule')
  await expect(page.locator('.create-menu summary')).toHaveText('作成')
  await expect(page.getByLabel(/^通知（未読/)).toBeVisible()
  await page.getByRole('button', { name: 'アカウントメニュー' }).click()
  await expect(page.getByRole('link', { name: 'マイページ' })).toBeVisible()
})

test('streaming mode hides 作成, the bell and マイページ, and reads no notifications', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'openoj-browser-notifications', value: notify, url: baseURL! }])
  await streamingOn(context)
  await signIn(page)
  const reads = await watchReads(page)
  await page.goto('/')
  await expect(badge(page)).toBeVisible()
  await expect(page.locator('.create-menu')).toHaveCount(0)
  await expect(page.getByLabel(/^通知（未読/)).toHaveCount(0)
  await page.getByRole('button', { name: 'アカウントメニュー' }).click()
  await expect(page.getByRole('link', { name: '設定' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'ログアウト' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'マイページ' })).toHaveCount(0)
  await page.waitForTimeout(3000)
  expect(reads).toEqual([])
})

test('the server renders streaming mode without a flash', async ({ request }) => {
  const html = await (await request.get('/', { headers: { cookie: 'openoj-streaming=1' } })).text()
  expect(html).toMatch(/<button[^>]*class="streaming-badge"/)
  expect(html).not.toContain('class="create-menu"')
  const plain = await (await request.get('/')).text()
  expect(plain).toContain('class="create-menu"')
  expect(plain).not.toMatch(/<button[^>]*class="streaming-badge"/)
})

for (const width of [375, 320]) {
  test(`the badge shows on a phone and on a contest page at ${width}px`, async ({ page, context }) => {
    await streamingOn(context)
    await page.setViewportSize({ width, height: 800 })
    for (const path of ['/', contest]) {
      await page.goto(path)
      await expect(badge(page)).toBeInViewport({ ratio: 1 })
      const bounds = (await badge(page).boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    }
  })
}

test('a signed-out visitor turns streaming mode off from the badge', async ({ page, context }) => {
  await streamingOn(context)
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'ログイン' })).toBeVisible()
  await expect(badge(page)).toHaveAttribute('aria-haspopup', 'dialog')
  await badge(page).click()
  await page.getByRole('dialog', { name: '配信モードを切りますか？' }).getByRole('button', { name: '切る', exact: true }).click()
  await expect(badge(page)).toHaveCount(0)
  expect((await context.cookies()).some(cookie => cookie.name === 'openoj-streaming')).toBe(false)
})

test('streaming mode survives a logout', async ({ page, context }) => {
  await streamingOn(context)
  await signIn(page)
  await page.route('**/api/auth/logout', route => route.fulfill({ json: {} }))
  await page.goto('/')
  await page.getByRole('button', { name: 'アカウントメニュー' }).click()
  await page.getByRole('button', { name: 'ログアウト' }).click()
  await expect(page).toHaveURL(/\/login$/)
  await expect(badge(page)).toBeVisible()
  expect((await context.cookies()).find(cookie => cookie.name === 'openoj-streaming')?.value).toBe('1')
})

test('the settings page shows only 配信モード in streaming mode', async ({ page, context }) => {
  await streamingOn(context)
  await signIn(page)
  for (const tab of ['identity', 'deletion']) {
    await page.goto(`/my/settings?tab=${tab}`)
    const items = page.getByRole('navigation', { name: '設定の項目' }).getByRole('button')
    await expect(items).toHaveText(['配信モード'])
    await expect(streamingSwitch(page)).toBeChecked()
    await expect(page.getByRole('textbox')).toHaveCount(0)
  }
})

test('the settings page lists every item without streaming mode', async ({ page }) => {
  await signIn(page)
  await page.goto('/my/settings?tab=identity')
  await expect(page.getByRole('navigation', { name: '設定の項目' }).getByRole('button')).toHaveText(['プロフィール', '外部サービス', '認証設定', 'プッシュ通知', '配信モード', 'アカウント削除'])
  await expect(page.getByRole('textbox').first()).toBeVisible()
})

const contestId = '88888888-8888-4888-8888-888888888888'
const closedPages = ['/my', '/my/contests', '/my/contests/new', `/my/contests/${shortId(contestId)}`, '/my/contest-invitations/token', '/my/tester-invitations/token', '/problems/new', '/problems/post', '/featured/apply', '/blog/new', '/onboarding', `/contests/${shortId(contestId)}/team`]
const closedHeading = (page: Page) => page.getByRole('heading', { name: '配信モード中は表示できません', exact: true })
// Requests to the private API other than the profile read; the closed screen must make none.
function watchPrivateReads(page: Page) {
  const reads: string[] = []
  page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/my/') && !request.url().endsWith('/api/my/profile')) reads.push(request.url()) })
  return reads
}

for (const path of closedPages) {
  test(`streaming mode closes ${path}`, async ({ page, context }) => {
    await signIn(page)
    await streamingOn(context)
    const privateReads = watchPrivateReads(page)
    const response = await page.goto(path)
    // The server-rendered HTML already holds the closed screen, so nothing of the page shows first.
    expect(await response!.text()).toContain('このページは、配信に映らないように閉じています。')
    await expect(closedHeading(page)).toBeVisible()
    await expect(page.getByText('このページは、配信に映らないように閉じています。')).toBeVisible()
    await expect(badge(page)).toBeVisible()
    await expect(page.getByRole('button', { name: '配信モードを切る', exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: 'ホームへ戻る' })).toHaveAttribute('href', '/')
    await expect(page).toHaveURL(new RegExp(`${path.replace(/[/]/g, '\\/')}$`))
    expect(privateReads).toEqual([])
  })
}

// The router matches case-insensitively and ignores a trailing slash, so the page still resolves; it must stay closed.
for (const path of ['/MY', '/My/Contests', '/PROBLEMS/NEW', '/my/', `/Contests/${shortId(contestId)}/Team`]) {
  test(`streaming mode closes the respelled ${path}`, async ({ page, context }) => {
    await signIn(page)
    await streamingOn(context)
    const privateReads = watchPrivateReads(page)
    const response = await page.goto(path)
    expect(await response!.text()).toContain('このページは、配信に映らないように閉じています。')
    await expect(closedHeading(page)).toBeVisible()
    expect(privateReads).toEqual([])
  })
}

test('a percent-encoded /%6dy never shows the my page in streaming mode', async ({ page, context }) => {
  await signIn(page)
  await streamingOn(context)
  const privateReads = watchPrivateReads(page)
  await page.goto('/%6dy')
  await expect(page.getByRole('heading', { name: 'alice', level: 1 })).toHaveCount(0)
  await expect(closedHeading(page).or(page.getByRole('heading', { name: 'ページが見つかりません' }))).toBeVisible()
  expect(privateReads).toEqual([])
})

test('the same check sees private reads on /my without streaming mode', async ({ page }) => {
  await signIn(page)
  const privateReads = watchPrivateReads(page)
  await page.goto('/my')
  await expect(closedHeading(page)).toHaveCount(0)
  await expect.poll(() => privateReads.length).toBeGreaterThan(0)
})

test('closedInStreaming keeps the own submission and the settings open', () => {
  for (const name of ['my-settings', 'my-submissions-id', 'index', 'problems', 'contests-id', 'contests-id-label', '']) expect(closedInStreaming(name), name).toBe(false)
  for (const name of ['my', 'my-contests', 'my-contests-id', 'my-new-page', 'problems-new', 'problems-post', 'featured-apply', 'blog-new', 'onboarding', 'contests-id-team']) expect(closedInStreaming(name), name).toBe(true)
})

test('a link and the back button also land on the closed screen', async ({ page, context }) => {
  await signIn(page)
  await streamingOn(context)
  await page.goto('/')
  await expect(badge(page)).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as any).__vue_app__.config.globalProperties.$router.push('/problems/new').catch(() => {}))
  await expect(closedHeading(page)).toBeVisible()
  await expect(page).toHaveURL(/\/problems\/new$/)
  await page.getByRole('link', { name: 'ホームへ戻る' }).click()
  await expect(page).toHaveURL(/\/$/)
  await expect(closedHeading(page)).toHaveCount(0)
  await page.goBack()
  await expect(closedHeading(page)).toBeVisible()
})

// Paths of the main-frame documents the page requests, so a full load can be told from an in-site move.
function watchDocuments(page: Page) {
  const documents: string[] = []
  page.on('request', request => { if (request.isNavigationRequest() && request.frame() === page.mainFrame()) documents.push(new URL(request.url()).pathname) })
  return documents
}
// Loads a closed page directly and, once it has hydrated, arms its page component: every run of its setup is reported
// to the test, also from a document that has since been replaced.
async function loadClosed(page: Page, path: string) {
  const setups: string[] = []
  await page.exposeFunction('reportClosedSetup', (name: string) => { setups.push(name) })
  await page.goto(path)
  await expect(closedHeading(page)).toBeVisible()
  await page.waitForFunction(() => (document.querySelector('#__nuxt') as any)?.__vue_app__?.config.globalProperties.$nuxt.isHydrating === false)
  const armed = await page.evaluate(async target => {
    const names: string[] = []
    for (const record of (document.querySelector('#__nuxt') as any).__vue_app__.config.globalProperties.$router.resolve(target).matched) {
      const entry = record.components.default
      const component = typeof entry === 'function' ? (await entry()).default : entry
      const setup = component.setup
      if (typeof setup !== 'function') throw new Error(`${String(record.name)} has no setup`)
      component.setup = function (this: unknown, ...args: unknown[]) {
        (window as unknown as { reportClosedSetup: (name: string) => void }).reportClosedSetup(String(record.name))
        return setup.apply(this, args)
      }
      names.push(String(record.name))
    }
    return names
  }, path)
  expect(armed).toHaveLength(1)
  return setups
}
const home = (page: Page) => page.locator('#hero-title')

for (const path of closedPages) {
  test(`the home link on the closed ${path} is a full load that never runs the page`, async ({ page, context }) => {
    await signIn(page)
    await streamingOn(context)
    const privateReads = watchPrivateReads(page)
    const documents = watchDocuments(page)
    const setups = await loadClosed(page, path)
    await page.getByRole('link', { name: 'ホームへ戻る' }).click()
    await expect(home(page)).toBeVisible()
    await expect(badge(page)).toBeVisible()
    await page.waitForLoadState('networkidle')
    expect(setups).toEqual([])
    expect(privateReads).toEqual([])
    expect(documents).toHaveLength(2)
    expect(documents[1]).toBe('/')
  })

  test(`a double click on a header link on the closed ${path} never runs the page`, async ({ page, context }) => {
    await signIn(page)
    await streamingOn(context)
    const privateReads = watchPrivateReads(page)
    const setups = await loadClosed(page, path)
    // Each move waits a moment, as it does while the next page still loads, so the second click cancels the first.
    await page.evaluate(() => (document.querySelector('#__nuxt') as any).__vue_app__.config.globalProperties.$router.beforeEach(() => new Promise(resolve => setTimeout(resolve, 300))))
    await page.getByRole('banner').getByRole('link', { name: 'ホーム', exact: true }).dblclick()
    await expect(home(page)).toBeVisible()
    await page.waitForLoadState('networkidle')
    expect(setups).toEqual([])
    expect(privateReads).toEqual([])
  })

  test(`a move to the closed ${path} itself keeps the closed screen`, async ({ page, context }) => {
    await signIn(page)
    await streamingOn(context)
    const privateReads = watchPrivateReads(page)
    const setups = await loadClosed(page, path)
    // The router already stands on the closed route, so the move is a duplicate; the error is cleared after it all the same.
    await page.evaluate(target => (document.querySelector('#__nuxt') as any).__vue_app__.config.globalProperties.$router.push(target), path)
    await page.waitForTimeout(500)
    await expect(closedHeading(page)).toBeVisible()
    await expect(page.getByRole('banner').getByRole('link', { name: 'ホーム', exact: true })).toBeVisible()
    await expect(badge(page)).toBeVisible()
    expect(setups).toEqual([])
    expect(privateReads).toEqual([])
  })
}

test('the home link on the closed editor never shows the draft its tab has cached', async ({ page, context }) => {
  await signedInForReal(context)
  const draft = '55555555-5555-4555-8555-555555555555'
  await page.goto(`/problems/new?problem=${draft}`)
  await expect(page.locator('#problem-title')).toHaveValue('非公開の練習問題')
  const cached = () => page.evaluate(() => Object.keys(sessionStorage).filter(key => key.startsWith('openoj.problem-cache.')))
  // The editor has kept the server's copy in this tab's sessionStorage.
  expect(await cached()).toHaveLength(1)
  const shown: string[] = []
  await page.exposeFunction('reportDraftTitle', (where: string) => { shown.push(where) })
  // Every document from here on looks for the draft's title in every frame and after every change.
  await page.addInitScript(() => {
    const look = () => {
      const fields = [...document.querySelectorAll('input, textarea')] as HTMLInputElement[]
      if (fields.some(field => field.value.includes('非公開の練習問題')) || document.body?.innerText.includes('非公開の練習問題')) (window as unknown as { reportDraftTitle: (where: string) => void }).reportDraftTitle(location.pathname)
    }
    const frame = () => { look(); requestAnimationFrame(frame) }
    requestAnimationFrame(frame)
    new MutationObserver(look).observe(document, { subtree: true, childList: true, characterData: true, attributes: true })
  })
  await streamingOn(context)
  await page.reload()
  await expect(closedHeading(page)).toBeVisible()
  await page.getByRole('link', { name: 'ホームへ戻る' }).click()
  await expect(home(page)).toBeVisible()
  await page.waitForTimeout(1000)
  expect(shown).toEqual([])
  // The cache stays as it is; no page that stays open reads it.
  expect(await cached()).toHaveLength(1)
})

test('配信モードを切る on the closed screen returns the page', async ({ page, context }) => {
  await signIn(page)
  await streamingOn(context)
  await page.goto('/my')
  await expect(closedHeading(page)).toBeVisible()
  await page.getByRole('button', { name: '配信モードを切る', exact: true }).click()
  await page.getByRole('dialog', { name: '配信モードを切りますか？' }).getByRole('button', { name: '切る', exact: true }).click()
  await expect(closedHeading(page)).toHaveCount(0)
  await expect(page).toHaveURL(/\/my$/)
  await expect(page.getByRole('heading', { name: 'alice', level: 1 })).toBeVisible()
  expect((await context.cookies()).some(cookie => cookie.name === 'openoj-streaming')).toBe(false)
})

test('pages outside the list stay open in streaming mode', async ({ page, context }) => {
  await signIn(page)
  await streamingOn(context)
  for (const path of ['/my/settings', '/', '/problems']) {
    await page.goto(path)
    await expect(badge(page)).toBeVisible()
    await expect(closedHeading(page)).toHaveCount(0)
  }
})

test('the settings page links back to the home in streaming mode and to the my page otherwise', async ({ page, context }) => {
  await signIn(page)
  await page.goto('/my/settings')
  await expect(page.getByRole('link', { name: 'マイページへ戻る' })).toHaveAttribute('href', '/my')
  await expect(page.getByRole('link', { name: 'ホームへ戻る' })).toHaveCount(0)
  await streamingOn(context)
  await page.goto('/my/settings')
  await expect(page.getByRole('link', { name: 'ホームへ戻る' })).toHaveAttribute('href', '/')
  await expect(page.getByRole('link', { name: 'マイページへ戻る' })).toHaveCount(0)
})

const problemId = '11111111-1111-4111-8111-111111111111'
const scheduledContest = `/contests/${shortId('99999999-9999-4999-8999-999999999999')}`
const own = { name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }
// The first request of the browser carries the session cookie, so the server renders as a signed-in viewer sees it.
async function signedInForReal(context: BrowserContext) { await context.addCookies([own]) }
// Every 403 from /api/ and every error the page itself logs while the pages are open.
function watchRefusals(page: Page) {
  const seen = { refused: [] as string[], logged: [] as string[] }
  page.on('response', response => { if (response.status() === 403 && new URL(response.url()).pathname.startsWith('/api/')) seen.refused.push(response.url()) })
  page.on('console', message => { if (message.type() === 'error') seen.logged.push(`${message.text()} ${message.location().url}`) })
  page.on('pageerror', error => seen.logged.push(error.message))
  return seen
}

const publicPages = ['/', '/problems', '/contests', '/featured', `/problems/${shortId(problemId)}`, `/problems/${shortId(problemId)}?view=submissions`, contest, `${contest}?view=problems`, `${contest}?view=standings`, `${contest}?view=clarifications`, `${contest}/A`, scheduledContest]

test('public pages show no error in streaming mode', async ({ page, context }) => {
  await streamingOn(context)
  await signedInForReal(context)
  const seen = watchRefusals(page)
  // These reads stay open in streaming mode; the fixture API has no answer shaped for them.
  await page.route('**/api/my/favorites/*', route => route.fulfill({ json: { favorited: false, favoriteCount: 3 } }))
  await page.route('**/api/my/difficulty-votes/*', route => route.fulfill({ json: { difficulty: null, difficultyAverage: null, difficultyVoteCount: 0, difficultyDistribution: Array.from({ length: 10 }, () => 0) } }))
  for (const path of publicPages) {
    await page.goto(path)
    await expect(badge(page)).toBeVisible()
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(500)
    await expect(page.getByRole('alert'), path).toHaveCount(0)
  }
  expect(seen.refused).toEqual([])
  expect(seen.logged).toEqual([])
})

test('the same check sees a refused read and a logged error', async ({ page, context }) => {
  await streamingOn(context)
  await signedInForReal(context)
  const seen = watchRefusals(page)
  await page.goto('/')
  await expect(badge(page)).toBeVisible()
  await page.evaluate(() => fetch('/api/my/notifications'))
  await expect.poll(() => seen.refused.length).toBe(1)
  await expect.poll(() => seen.logged.length).toBeGreaterThan(0)
})

test('the join panel gives way to a note in streaming mode', async ({ page, context }) => {
  await streamingOn(context)
  await signedInForReal(context)
  await page.goto(scheduledContest)
  const note = '配信モード中は参加登録できません。'
  await expect(page.getByText(note, { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '参加する' })).toHaveCount(0)
})

test('the join panel is unchanged for a signed-out visitor in streaming mode, and for a signed-in one without it', async ({ page, context }) => {
  await streamingOn(context)
  await page.goto(scheduledContest)
  await expect(page.getByRole('link', { name: 'ログインして参加する' })).toBeVisible()
  await expect(page.getByText('配信モード中は参加登録できません。')).toHaveCount(0)
  await context.clearCookies()
  await signedInForReal(context)
  await page.goto(scheduledContest)
  await expect(page.getByRole('button', { name: '参加する' })).toBeVisible()
  await expect(page.getByText('配信モード中は参加登録できません。')).toHaveCount(0)
})

test('the contest menu drops 質問 in streaming mode', async ({ page, context }) => {
  await streamingOn(context)
  await signedInForReal(context)
  const response = await page.goto(contest)
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await expect(menu.getByRole('link', { name: '解説', exact: true })).toBeVisible()
  await expect(menu.getByRole('link', { name: '質問', exact: true })).toHaveCount(0)
  expect(await response!.text()).not.toContain('view=clarifications')
  const asked: string[] = []
  page.on('request', request => { if (request.url().includes('/clarifications')) asked.push(request.url()) })
  await page.goto(`${contest}?view=clarifications`)
  await expect(page.getByText('配信モード中は表示できません。', { exact: true })).toBeVisible()
  await page.waitForTimeout(500)
  expect(asked).toEqual([])
})

test('the contest menu keeps 質問 without streaming mode', async ({ page, context }) => {
  await signedInForReal(context)
  await page.route('**/api/my/contests/*/clarifications*', route => route.fulfill({ json: { items: [], hasMore: false, total: 0 } }))
  await page.goto(contest)
  await expect(page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '質問', exact: true })).toBeVisible()
  await page.goto(`${contest}?view=clarifications`)
  await expect(page.getByText('配信モード中は表示できません。', { exact: true })).toHaveCount(0)
})

test('the own list of a staff contest says it is closed', async ({ page, context }) => {
  await streamingOn(context)
  await signIn(page)
  await page.route('**/api/contests/*/submissions?*mine=1*', route => route.fulfill({ status: 403, json: { data: { code: 'streaming_mode' } } }))
  await page.goto(`${contest}?view=my-submissions`)
  await expect(page.getByText('配信モード中は表示できません。', { exact: true })).toBeVisible()
  await expect(page.getByText('提出一覧を取得できませんでした。')).toHaveCount(0)
})

test('another failure of the own list keeps its own message', async ({ page, context }) => {
  await streamingOn(context)
  await signIn(page)
  await page.route('**/api/contests/*/submissions?*mine=1*', route => route.fulfill({ status: 502, json: {} }))
  await page.goto(`${contest}?view=my-submissions`)
  await expect(page.getByText('提出一覧を取得できませんでした。')).toBeVisible()
  await expect(page.getByText('配信モード中は表示できません。', { exact: true })).toHaveCount(0)
})

// The staff answer carries the flags; the public answer, which the server gives staff in streaming mode, has none of them.
const contestAnswer = (extra: Record<string, unknown>) => ({
  id: contestId, title: '共有コンテスト', author: 'alice', description: '', startsAt: '2026-09-10T00:00:00Z', endsAt: '2099-09-10T02:00:00Z',
  freezeAt: '2026-09-10T01:00:00Z', freezeMinutes: 60, penaltyMinutes: 5, status: 'running', canEdit: false, official: true, participating: false, canViewSubmissions: false,
  problems: [{ id: problemId, title: 'A + B', points: 100 }], ...extra,
})
const standingRows = [{ rank: 1, handle: 'bob', points: 100, timeMs: 60000, problems: { [problemId]: { points: 100, score: 100, wrong: 0, pending: 0, acceptedAt: '2026-09-10T00:01:00Z', submissionId: '22222222-2222-4222-8222-222222222222' } } }]
async function answerContest(page: Page, extra: Record<string, unknown>) {
  await page.route(`**/api/contests/${contestId}`, route => route.fulfill({ json: contestAnswer(extra) }))
  await page.route('**/api/contests/*/standings*', route => route.fulfill({ json: standingRows }))
}
const staffAnswer = { staff: true, canViewSubmissions: true, canViewLiveStandings: true, official: false }

test('the public answer of a contest gives no staff controls', async ({ page, context }) => {
  await streamingOn(context)
  await signIn(page)
  await answerContest(page, {})
  await page.goto(`${contest}?view=standings`)
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(1)
  await expect(page.locator('.standings-table a[href*="/submissions/"]')).toHaveCount(0)
  await expect(page.getByText('凍結なしで表示')).toHaveCount(0)
  await page.goto(`${contest}/A`)
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await expect(page.getByRole('button', { name: '提出する', exact: true })).toHaveCount(0)
})

test('the same pages show the staff controls when the answer carries the staff flags', async ({ page }) => {
  await signIn(page)
  await answerContest(page, staffAnswer)
  await page.goto(`${contest}?view=standings`)
  await expect(page.locator('.standings-table a[href*="/submissions/"]')).toHaveCount(1)
  await expect(page.getByText('凍結なしで表示')).toBeVisible()
  await page.goto(`${contest}/A`)
  await expect(page.getByRole('button', { name: '提出する', exact: true })).toBeVisible()
})

test('a problem page still submits and runs code in streaming mode', async ({ page, context }) => {
  await streamingOn(context)
  await signedInForReal(context)
  const detail = 'c1c1c1c1-c1c1-4c1c-8c1c-c1c1c1c1c1c1'
  const submitted: number[] = []
  // The requests go to the real web server, which must let them through; only the fixture's empty answer is replaced.
  await page.route('**/api/my/submissions', async route => {
    const upstream = await route.fetch({ headers: { ...route.request().headers(), origin: 'https://judge.example' } })
    submitted.push(upstream.status())
    await route.fulfill({ status: 202, json: { id: detail, status: 'QUEUED' } })
  })
  await page.goto(`/problems/${shortId(problemId)}`)
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(page).toHaveURL(`/my/submissions/${shortId(detail)}`)
  expect(submitted).toEqual([202])
  await expect(closedHeading(page)).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '提出結果', exact: true })).toBeVisible()

  const ran: number[] = []
  await page.route('**/api/my/code-runs/latest', route => route.fulfill({ status: 404, json: { data: { code: 'code_run_not_found' } } }))
  await page.route('**/api/my/code-runs', async route => {
    const upstream = await route.fetch({ headers: { ...route.request().headers(), origin: 'https://judge.example' } })
    ran.push(upstream.status())
    await route.fulfill({ status: 202, json: { id: '33333333-3333-4333-8333-333333333333', status: 'QUEUED', result: null, createdAt: '2026-10-25T00:00:00Z' } })
  })
  await page.goto('/run')
  await page.getByRole('combobox', { name: '言語' }).selectOption('cpp23-gcc')
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  const run = page.getByRole('button', { name: /実行$/ })
  await expect(run).toBeEnabled()
  await run.click()
  await expect.poll(() => ran).toHaveLength(1)
  expect(ran[0]).not.toBe(403)
})

test('the reveal buttons of the 定期便 are not drawn in streaming mode, and are drawn without it', async ({ page, context }) => {
  await signedInForReal(context)
  await page.goto('/featured')
  // alice writes one announced slot and tests the other, so both show the button.
  await expect(page.getByRole('button', { name: '問題名を表示' })).toHaveCount(2)
  await streamingOn(context)
  const asked: string[] = []
  page.on('request', request => { if (request.url().includes('/api/my/featured/next')) asked.push(request.url()) })
  await page.goto('/featured')
  await expect(page.locator('.delivery-problem')).toHaveCount(2)
  await expect(page.getByRole('button', { name: '問題名を表示' })).toHaveCount(0)
  expect(asked).toEqual([])
})

test('the venue passphrase form of a participant is not drawn in streaming mode, and is drawn without it', async ({ page, context }) => {
  await signIn(page)
  await answerContest(page, { participating: true, onsiteDivision: true, onsite: false })
  const posted: string[] = []
  page.on('request', request => { if (request.method() === 'POST' && /\/api\/my\/contests\/[^/]+\/participation/.test(request.url())) posted.push(request.url()) })
  const form = page.getByRole('button', { name: 'オンサイト参加にする' })
  await page.goto(contest)
  await expect(form).toBeVisible()
  await streamingOn(context)
  await page.goto(contest)
  // The participant's state stays; only the upgrade form goes. The viewer is known on the client only, so the server HTML is the same either way.
  await expect(page.getByText('参加済み').first()).toBeVisible()
  await expect(form).toHaveCount(0)
  await expect(page.getByLabel('会場の合言葉（オンサイト参加者のみ）')).toHaveCount(0)
  await expect(page.getByText('配信モード中は参加登録できません。')).toHaveCount(0)
  expect(posted).toEqual([])
})

// Marks the document so a test can tell a reload (fresh document, marker gone) from a tab that was only covered.
async function mark(page: Page) { await page.evaluate(() => { (window as unknown as { __loaded: boolean }).__loaded = true }) }
const marked = (page: Page) => page.evaluate(() => (window as unknown as { __loaded?: boolean }).__loaded === true)
const reloadButton = (page: Page) => page.getByRole('button', { name: '読み込み直す', exact: true })

test('another tab is covered and reloads when streaming mode turns on', async ({ context }) => {
  const staff = await context.newPage()
  await signIn(staff)
  await staff.goto('/my')
  await expect(staff.getByRole('heading', { name: 'alice' })).toBeVisible()
  await mark(staff)
  const settings = await context.newPage()
  await signIn(settings)
  await settings.goto('/my/settings?tab=streaming')
  await streamingSwitch(settings).check()
  await expect(closedHeading(staff)).toBeVisible()
  // A fresh server render of the closed screen, not the old page merely covered.
  expect(await marked(staff)).toBe(false)
  await expect(reloadButton(staff)).toHaveCount(0)
  expect(new URL(staff.url()).pathname).toBe('/my')
  await expect(badge(staff)).toBeVisible()
  expect(await staff.evaluate(() => document.documentElement.hasAttribute('data-streaming-covered'))).toBe(false)
})

test('a tab that refuses the reload stays covered, and uncovers when the mode returns', async ({ context }) => {
  const staff = await context.newPage()
  staff.on('dialog', dialog => dialog.dismiss())
  await signIn(staff)
  await staff.goto('/my')
  const heading = staff.getByRole('heading', { name: 'alice' })
  await expect(heading).toBeVisible()
  // The leave prompt may open inside reload(), before the cover renders; the content must already be hidden then.
  await staff.evaluate(() => addEventListener('beforeunload', event => {
    (window as unknown as { coveredAtLeave: boolean }).coveredAtLeave = document.documentElement.hasAttribute('data-streaming-covered')
    event.preventDefault()
  }))
  // Chromium only asks after the user has interacted with the page.
  await heading.click()
  await mark(staff)
  const settings = await context.newPage()
  await signIn(settings)
  await settings.goto('/my/settings?tab=streaming')
  await streamingSwitch(settings).check()
  await expect(badge(settings)).toBeVisible()
  await expect(reloadButton(staff)).toBeVisible()
  await expect(reloadButton(staff)).toBeFocused()
  expect(await staff.evaluate(() => (window as unknown as { coveredAtLeave?: boolean }).coveredAtLeave)).toBe(true)
  // The content underneath is out of reach: what sits at the heading's own place is the cover, and Tab never leaves it.
  expect(await staff.evaluate(() => {
    const element = document.querySelector('h1')!
    const box = element.getBoundingClientRect()
    return document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2)?.closest('dialog.streaming-cover') !== null
  })).toBe(true)
  for (let press = 0; press < 12; press++) await staff.keyboard.press('Tab')
  expect(await staff.evaluate(() => document.activeElement?.closest('dialog.streaming-cover') !== null)).toBe(true)
  // Nothing the user does may dismiss the cover: repeated Escape (Chromium stops honouring a cancelled Escape after the
  // first one without activation), a click on the backdrop, and Shift+Tab.
  const stillCovered = async (step: string) => {
    expect(await staff.evaluate(() => (document.querySelector('dialog.streaming-cover') as HTMLDialogElement | null)?.matches(':modal') === true), step).toBe(true)
    expect(await staff.evaluate(() => {
      const element = document.querySelector('h1')!
      const box = element.getBoundingClientRect()
      return document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2)?.closest('dialog.streaming-cover') !== null
    }), step).toBe(true)
    expect(await staff.evaluate(() => document.activeElement?.closest('dialog.streaming-cover') !== null), step).toBe(true)
  }
  for (let press = 0; press < 4; press++) await staff.keyboard.press('Escape')
  await stillCovered('after Escape x4')
  await staff.mouse.click(2, 2)
  await stillCovered('after backdrop click')
  await staff.keyboard.press('Shift+Tab')
  await stillCovered('after Shift+Tab')
  // Even if the dialog is closed by some means nothing cancels, the content under it is already hidden, with no gap to act in.
  const whileClosed = await staff.evaluate(() => {
    document.querySelector<HTMLDialogElement>('dialog.streaming-cover')!.close()
    const title = document.querySelector('h1')!
    return { root: getComputedStyle(document.getElementById('__nuxt')!).visibility, heading: getComputedStyle(title).visibility }
  })
  expect(whileClosed).toEqual({ root: 'hidden', heading: 'hidden' })
  // The close event fires a task later; the dialog is shown again then.
  await expect.poll(() => staff.evaluate(() => document.querySelector('dialog.streaming-cover')?.matches(':modal') === true)).toBe(true)
  await stillCovered('after a programmatic close')
  expect(await marked(staff)).toBe(true)
  await badge(settings).click()
  await settings.getByRole('dialog', { name: '配信モードを切りますか？' }).getByRole('button', { name: '切る', exact: true }).click()
  await expect(badge(settings)).toHaveCount(0)
  await expect(reloadButton(staff)).toHaveCount(0)
  await expect(heading).toBeVisible()
  expect(await staff.evaluate(() => getComputedStyle(document.getElementById('__nuxt')!).visibility)).toBe('visible')
  expect(await marked(staff)).toBe(true)
})

test('a covered tab stays hidden when its app and error views swap', async ({ context }) => {
  const staff = await context.newPage()
  staff.on('dialog', dialog => dialog.dismiss())
  await signIn(staff)
  await staff.goto('/my')
  const heading = staff.getByRole('heading', { name: 'alice' })
  await expect(heading).toBeVisible()
  await staff.evaluate(() => addEventListener('beforeunload', event => event.preventDefault()))
  await heading.click()
  await streamingOn(context)
  await staff.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(reloadButton(staff)).toBeVisible()
  // An unknown page shows the error view, which mounts its own layout and cover in place of the app's.
  await staff.evaluate(() => (document.querySelector('#__nuxt') as any).__vue_app__.config.globalProperties.$router.push('/no-such-page'))
  // Located by CSS: the heading is hidden, so it is not in the accessibility tree.
  await expect(staff.locator('h1', { hasText: 'ページが見つかりません' })).toBeAttached()
  await expect(reloadButton(staff)).toBeVisible()
  expect(await staff.evaluate(() => getComputedStyle(document.getElementById('__nuxt')!).visibility)).toBe('hidden')
})

test('a tab notices the change when it becomes visible', async ({ context }) => {
  await context.addInitScript(() => { (window as unknown as { BroadcastChannel?: unknown }).BroadcastChannel = undefined })
  const staff = await context.newPage()
  await signIn(staff)
  await staff.goto('/my')
  await expect(staff.getByRole('heading', { name: 'alice' })).toBeVisible()
  await mark(staff)
  await streamingOn(context)
  // Nothing has told the tab yet.
  expect(await marked(staff)).toBe(true)
  await staff.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(closedHeading(staff)).toBeVisible()
  expect(await marked(staff)).toBe(false)
})

test('a tab restored from the back/forward cache notices the change', async ({ context }) => {
  await context.addInitScript(() => { (window as unknown as { BroadcastChannel?: unknown }).BroadcastChannel = undefined })
  const staff = await context.newPage()
  await signIn(staff)
  await staff.goto('/my')
  await expect(staff.getByRole('heading', { name: 'alice' })).toBeVisible()
  await mark(staff)
  await streamingOn(context)
  expect(await marked(staff)).toBe(true)
  await staff.evaluate(() => dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })))
  await expect(closedHeading(staff)).toBeVisible()
  expect(await marked(staff)).toBe(false)
})

test('a tab notices the change when it moves to another page', async ({ context }) => {
  await context.addInitScript(() => { (window as unknown as { BroadcastChannel?: unknown }).BroadcastChannel = undefined })
  const tab = await context.newPage()
  await signIn(tab)
  await tab.goto('/')
  await expect(home(tab)).toBeVisible()
  await mark(tab)
  await streamingOn(context)
  expect(await marked(tab)).toBe(true)
  // The move itself is in-site; only the reload it sets off loads a document.
  const reloaded = tab.waitForEvent('load')
  await tab.getByRole('banner').getByRole('link', { name: '問題', exact: true }).click()
  await reloaded
  expect(await marked(tab)).toBe(false)
  expect(new URL(tab.url()).pathname).toBe('/problems')
  await expect(badge(tab)).toBeVisible()
})

test('a page rendered before the switch and hydrated after it reloads into streaming mode', async ({ page, context, baseURL }) => {
  await signIn(page)
  const documents = watchDocuments(page)
  // The server renders the first document without the cookie, which is set before the browser reads that document.
  await page.route(`${baseURL}/`, async route => {
    const response = await route.fetch()
    await streamingOn(context)
    await route.fulfill({ response })
  }, { times: 1 })
  await page.goto('/')
  await expect.poll(() => documents).toEqual(['/', '/'])
  await expect(badge(page)).toBeVisible()
  await expect(reloadButton(page)).toHaveCount(0)
})

test('moving between pages with streaming mode off writes no streaming cookie and keeps no listener behind', async ({ page, context }) => {
  await signIn(page)
  await page.addInitScript(() => {
    const seen = { writes: [] as string[], listeners: new Set<unknown>() }
    ;(window as unknown as { streamingSeen: typeof seen }).streamingSeen = seen
    const cookie = Object.getOwnPropertyDescriptor(Document.prototype, 'cookie')!
    Object.defineProperty(Document.prototype, 'cookie', {
      configurable: true,
      get() { return cookie.get!.call(this) },
      set(value: string) {
        if (String(value).startsWith('openoj-streaming=')) seen.writes.push(String(value))
        cookie.set!.call(this, value)
      },
    })
    // The change listeners alive on cookieStore: a cookie ref made at every move adds one that is never removed.
    const store = (window as unknown as { cookieStore?: EventTarget }).cookieStore
    if (!store) return
    const add = store.addEventListener.bind(store)
    const remove = store.removeEventListener.bind(store)
    store.addEventListener = (type: string, listener: EventListenerOrEventListenerObject | null, options?: AddEventListenerOptions | boolean) => { if (type === 'change') seen.listeners.add(listener); add(type, listener, options) }
    store.removeEventListener = (type: string, listener: EventListenerOrEventListenerObject | null, options?: EventListenerOptions | boolean) => { if (type === 'change') seen.listeners.delete(listener); remove(type, listener, options) }
  })
  const seen = () => page.evaluate(() => { const { writes, listeners } = (window as unknown as { streamingSeen: { writes: string[], listeners: Set<unknown> } }).streamingSeen; return { writes, listeners: listeners.size } })
  await page.goto('/')
  await expect(home(page)).toBeVisible()
  await page.waitForLoadState('networkidle')
  const before = await seen()
  const header = page.getByRole('banner')
  for (const [name, path] of [['問題', '/problems'], ['コンテスト', '/contests'], ['ホーム', '/'], ['問題', '/problems'], ['ホーム', '/']] as const) {
    await header.getByRole('link', { name, exact: true }).click()
    await expect(page).toHaveURL(path === '/' ? /\/$/ : new RegExp(`${path}$`))
  }
  await expect(home(page)).toBeVisible()
  await page.waitForLoadState('networkidle')
  expect(before.writes).toEqual([])
  expect(await seen()).toEqual({ writes: [], listeners: before.listeners })
  expect((await context.cookies()).some(cookie => cookie.name === 'openoj-streaming')).toBe(false)
})
