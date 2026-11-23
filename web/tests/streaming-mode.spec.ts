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
  const response = await page.goto('/%6dy')
  const html = await response!.text()
  console.log('PERCENT-ENCODED', response!.status(), html.includes('配信に映らないように閉じています') ? 'closed screen' : html.includes('ページが見つかりません') ? '404 page' : 'other')
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
