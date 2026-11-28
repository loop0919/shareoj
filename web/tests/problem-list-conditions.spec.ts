import type { APIResponse, Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { quickModes, quickly } from './fixtures/quickly'
import { outsidePanel } from './fixtures/panel'

const session = { cookie: 'openoj_access=valid-access' }
// The browser is signed in through the fixture; the cookie lets the server see the session too.
const signIn = (page: Page) => page.context().addCookies([{ name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }])
// The query of each /api/problems call from the browser, oldest first.
function listRequests(page: Page) {
  const requests: Record<string, string>[] = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (url.pathname === '/api/problems') requests.push(Object.fromEntries(url.searchParams))
  })
  return requests
}
const conditionsButton = (page: Page) => page.getByRole('button', { name: /^絞り込み・並び替え/ })
// The remembered conditions are an object in a cookie, which Nuxt's useCookie writes as URL-encoded JSON.
const encoded = (value: unknown) => encodeURIComponent(JSON.stringify(value))
const remembered = async (page: Page) => JSON.parse(decodeURIComponent((await page.context().cookies()).find(c => c.name === 'openoj-problem-conditions')?.value ?? 'null'))
const remember = (page: Page, value: string) => page.context().addCookies([{ name: 'openoj-problem-conditions', value, url: 'http://127.0.0.1:13000' }])
// The browser reports each cookie write to the page later, through cookieStore change events, and Nuxt's useCookie copies
// each report into its ref. The report of a write can arrive after the next write, handing the older value back until
// the next report. This holds back the report of the remembered conditions' deletion until releaseCookieDeletion, and
// counts the reports of its other writes, so a test can wait for the late one and look at the page in between.
const holdCookieDeletion = (page: Page) => page.addInitScript(() => {
  const name = 'openoj-problem-conditions'
  const held: (() => void)[] = []
  // Every useCookie has a listener of its own, so a report is counted once, not once a listener.
  const counted = new WeakSet<Event>()
  const reports = { written: 0, release: () => held.splice(0).forEach(deliver => deliver()) }
  Object.assign(window, { cookieReports: reports })
  const store = (globalThis as unknown as { CookieStore: { prototype: EventTarget } }).CookieStore.prototype
  const add = store.addEventListener
  store.addEventListener = function (this: EventTarget, type: string, listener: EventListenerOrEventListenerObject | null, options?: boolean | AddEventListenerOptions) {
    if (type !== 'change' || typeof listener !== 'function') return add.call(this, type, listener, options)
    return add.call(this, type, (event: Event) => {
      const { changed, deleted } = event as Event & { changed: { name: string }[], deleted: { name: string }[] }
      if (deleted.some(cookie => cookie.name === name)) {
        held.push(() => listener.call(this, event))
        return
      }
      if (changed.some(cookie => cookie.name === name) && !counted.has(event)) {
        counted.add(event)
        reports.written++
      }
      listener.call(this, event)
    }, options)
  }
})
const cookieReports = (page: Page) => page.evaluate(() => (window as unknown as { cookieReports: { written: number } }).cookieReports.written)
const releaseCookieDeletion = (page: Page) => page.evaluate(() => (window as unknown as { cookieReports: { release: () => void } }).cookieReports.release())

test('the list route passes the order and the difficulty range on', async ({ request }) => {
  const oldest = await (await request.get('/api/problems?tag=pagination&sort=oldest&limit=20')).json()
  expect(oldest.items[0].id).toBe('00000000-0000-4000-8000-000000000120')
  expect(oldest.solved).toBe('')
  expect((await (await request.get('/api/problems?tag=pagination&difficulty=1-9')).json()).total).toBe(0)
  expect((await (await request.get('/api/problems?tag=pagination&difficulty=10-10')).json()).total).toBe(120)
  expect((await request.get('/api/problems?sort=popular')).status()).toBe(400)
  expect((await request.get('/api/problems?difficulty=7-3')).status()).toBe(400)
})

test('the solved filter needs a session and says whether it was applied', async ({ request }) => {
  const mine = await (await request.get('/api/problems?tag=pagination&solved=unsolved', { headers: session })).json()
  expect([mine.total, mine.solved]).toEqual([60, 'unsolved'])
  const done = await (await request.get('/api/problems?tag=pagination&solved=solved&sort=oldest', { headers: session })).json()
  expect([done.total, done.solved, done.items[0].id]).toEqual([60, 'solved', '00000000-0000-4000-8000-000000000119'])
  const guest = await (await request.get('/api/problems?tag=pagination&solved=unsolved')).json()
  expect([guest.total, guest.solved]).toEqual([120, ''])
  // A session the API refuses falls back to the whole list.
  const expired = await (await request.get('/api/problems?tag=pagination&solved=unsolved', { headers: { cookie: 'openoj_access=expired' } })).json()
  expect([expired.total, expired.solved]).toEqual([120, ''])
  // An unknown value is not sent on.
  const unknown = await (await request.get('/api/problems?tag=pagination&solved=maybe', { headers: session })).json()
  expect([unknown.total, unknown.solved]).toEqual([120, ''])
})

test('a refused order stays a 400 on the signed-in path', async ({ request }) => {
  expect((await request.get('/api/problems?sort=popular&solved=unsolved', { headers: session })).status()).toBe(400)
})

// The list holds the viewer's own solved filter, and a bare /problems redirects by a cookie, so no cache keeps either.
test('the list and its route are not stored, and vary with the cookie', async ({ request }) => {
  const headers = (response: APIResponse) => [response.headers()['cache-control'], response.headers().vary]
  const cookieVaries = expect.stringMatching(/\bcookie\b/i)
  expect(headers(await request.get('/problems'))).toEqual(['no-store', cookieVaries])
  const mine = await request.get('/problems?solved=unsolved', { headers: session })
  expect([mine.status(), ...headers(mine)]).toEqual([200, 'no-store', cookieVaries])
  const redirect = await request.get('/problems', { headers: { cookie: `openoj-problem-conditions=${encoded({ sort: 'oldest' })}` }, maxRedirects: 0 })
  expect([redirect.status(), ...headers(redirect)]).toEqual([302, 'no-store', cookieVaries])
  expect(headers(await request.get('/api/problems?solved=unsolved', { headers: session }))).toEqual(['no-store', cookieVaries])
  // The fallbacks to the public list.
  expect((await request.get('/api/problems?solved=unsolved', { headers: { cookie: 'openoj_access=expired' } })).headers()['cache-control']).toBe('no-store')
  expect((await request.get('/api/problems?solved=unsolved')).headers()['cache-control']).toBe('no-store')
})

test('the untagged problem has no difficulty and is not solved', async ({ request }) => {
  expect((await (await request.get('/api/problems?difficulty=1-10', { headers: session })).json()).total).toBe(0)
  const unsolved = await (await request.get('/api/problems?solved=unsolved', { headers: session })).json()
  expect([unsolved.total, unsolved.solved]).toEqual([1, 'unsolved'])
  expect((await (await request.get('/api/problems?solved=solved', { headers: session })).json()).total).toBe(0)
})

test('the panel changes the conditions, the URL and the list', async ({ page }) => {
  await signIn(page)
  const requests = listRequests(page)
  await page.goto('/problems')
  const toggle = conditionsButton(page)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByRole('combobox', { name: '並び順' })).toHaveCount(0)
  // The button names the panel only while there is one.
  await expect(toggle).not.toHaveAttribute('aria-controls')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-controls', /./)
  await expect(page.locator(`[id="${await toggle.getAttribute('aria-controls')}"]`)).toContainText('並び順')
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '正解者数が多い順' })
  await expect(page).toHaveURL('/problems?sort=solvers-desc')
  // The panel stays open.
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' }).click()
  await page.getByRole('combobox', { name: 'コンテスト' }).selectOption({ label: 'コンテストの問題を除く' })
  await page.getByRole('combobox', { name: '難易度の下限' }).selectOption({ label: 'Lv.3' })
  await page.getByRole('combobox', { name: '難易度の上限' }).selectOption({ label: 'Lv.6' })
  await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved&contest=exclude&difficulty=3-6')
  await expect.poll(() => requests.at(-1)).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'exclude', difficulty: '3-6', offset: '0', limit: '50' })
  await expect(page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' })).toHaveAttribute('aria-pressed', 'true')
  // A lower bound above the upper one moves the upper one with it.
  await page.getByRole('combobox', { name: '難易度の下限' }).selectOption({ label: 'Lv.8' })
  await expect(page).toHaveURL(/difficulty=8-8/)
  await expect(page.getByRole('combobox', { name: '難易度の上限' })).toHaveValue('8')
})

test('the closed panel shows the conditions as chips', async ({ page }) => {
  await signIn(page)
  const hydration: string[] = []
  page.on('console', message => { if (/hydration/i.test(message.text())) hydration.push(message.text()) })
  const requests = listRequests(page)
  // 60 of the fixture's 120 problems are unsolved, so page 2 exists at 50 rows a page.
  await page.goto('/problems?sort=solvers-desc&solved=unsolved&difficulty=3-10&tag=pagination&page=2')
  const toggle = conditionsButton(page)
  await expect(toggle).toContainText('4')
  const chips = page.getByRole('list', { name: '効いている条件' })
  await expect(chips.getByRole('listitem')).toHaveText([/正解者数が多い順/, /未ACのみ/, /難易度 Lv\.3〜10/, /タグ: pagination/])
  const rows = page.getByRole('table').locator('tbody tr')
  await expect(rows.first().locator('th')).toHaveText('問題 102')
  requests.length = 0
  await chips.getByRole('button', { name: '難易度 Lv.3〜10を外す' }).click()
  // Back on page 1, with one request carrying the new conditions and the first page together.
  await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved&tag=pagination')
  await expect(rows.first().locator('th')).toHaveText('問題 002')
  expect(requests).toEqual([{ sort: 'solvers-desc', solved: 'unsolved', tag: 'pagination', offset: '0', limit: '50' }])
  await expect(toggle).toContainText('3')
  await toggle.click()
  await expect(chips).toHaveCount(0)
  // The panel says the same.
  await expect(page.getByRole('combobox', { name: '並び順' })).toHaveValue('solvers-desc')
  await expect(page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('combobox', { name: '難易度の下限' })).toHaveValue('1')
  await expect(page.getByRole('combobox', { name: '難易度の上限' })).toHaveValue('10')
  await expect(page.getByRole('combobox', { name: 'タグで絞り込む' })).toHaveValue('pagination')
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL('/problems')
  await expect(toggle).not.toContainText(/\d/)
  await expect(page.getByRole('combobox', { name: '並び順' })).toHaveValue('newest')
  expect(hydration).toEqual([])
})

test('the back button returns to the earlier conditions', async ({ page }) => {
  const requests = listRequests(page)
  await page.goto('/problems')
  await conditionsButton(page).click()
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '公開日が古い順' })
  await expect(page).toHaveURL('/problems?sort=oldest')
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '難易度が高い順' })
  await expect(page).toHaveURL('/problems?sort=difficulty-desc')
  await page.goBack()
  await expect(page).toHaveURL('/problems?sort=oldest')
  await expect(page.getByRole('combobox', { name: '並び順' })).toHaveValue('oldest')
  await expect.poll(() => requests.at(-1)?.sort).toBe('oldest')
})

// A bare /problems opens the remembered conditions, so once a change is remembered, going back to it would show the same
// list again. The first change made on it takes its place in the history instead.
test('the first change on a bare /problems takes its place in the history', async ({ page }) => {
  const sort = page.getByRole('combobox', { name: '並び順' })
  const contest = page.getByRole('combobox', { name: 'コンテスト' })
  const openList = async () => {
    await page.goto('/contests')
    await page.getByRole('navigation', { name: 'メインナビゲーション' }).getByRole('link', { name: '問題', exact: true }).click()
    await expect(page).toHaveURL('/problems')
    await conditionsButton(page).click()
  }
  await openList()
  await sort.selectOption({ label: '公開日が古い順' })
  await expect(page).toHaveURL('/problems?sort=oldest')
  await page.goBack()
  await expect(page).toHaveURL('/contests')
  // With one more change, Back returns to the first change, and then leaves the list.
  await page.context().clearCookies({ name: 'openoj-problem-conditions' })
  await openList()
  await sort.selectOption({ label: '公開日が古い順' })
  await expect(page).toHaveURL('/problems?sort=oldest')
  await contest.selectOption({ label: 'コンテストの問題を除く' })
  await expect(page).toHaveURL('/problems?sort=oldest&contest=exclude')
  await page.goBack()
  await expect(page).toHaveURL('/problems?sort=oldest')
  await expect(sort).toHaveValue('oldest')
  await expect(contest).toHaveValue('')
  await page.goBack()
  await expect(page).toHaveURL('/contests')
})

test('a signed-out viewer cannot filter by solved', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  // No session: the route answers solved: ''.
  await page.goto('/problems?solved=unsolved&contest=exclude')
  await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/コンテストの問題を除く/])
  await expect(conditionsButton(page)).toContainText('1')
  await conditionsButton(page).click()
  const solved = page.getByRole('group', { name: '解答状況' })
  await expect(solved.getByRole('button', { name: '未ACのみ' })).toBeDisabled()
  await expect(solved.getByRole('button', { name: 'すべて' })).toHaveAttribute('aria-pressed', 'true')
  await expect(solved).toHaveAccessibleDescription('ログインすると使えます')
  const login = page.getByRole('link', { name: 'ログイン' }).last()
  await expect(login).toHaveAttribute('href', /^\/login\?next=/)
  expect(new URL(await login.getAttribute('href') ?? '', 'https://judge.example').searchParams.get('next')).toBe('/problems?solved=unsolved&contest=exclude')
  await expect(page.getByText('ログインすると使えます')).toBeVisible()
})

test('an empty result offers to clear the conditions, and rows keep them on a tag', async ({ page }) => {
  // The fixture problem has no difficulty.
  await page.goto('/problems?difficulty=1-9')
  await expect(page.getByText('条件に合う問題はありません。')).toBeVisible()
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL('/problems')
  await expect(page.getByRole('table').getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await page.goto('/problems?sort=oldest')
  await page.getByRole('switch', { name: 'タグを表示' }).check()
  await page.getByRole('table').getByRole('link', { name: 'DP', exact: true }).click()
  await expect(page).toHaveURL('/problems?sort=oldest&tag=DP')
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://judge.example/problems?tag=DP')
})

// The × and the empty list's button leave the page when they take effect, so the focus goes to the panel's button.
test('taking conditions off leaves the focus on the panel button', async ({ page }) => {
  // The fixture problem is no contest's.
  await page.goto('/problems?sort=oldest&contest=only')
  const toggle = conditionsButton(page)
  await page.getByRole('list', { name: '効いている条件' }).getByRole('button', { name: '公開日が古い順を外す' }).click()
  await expect(page).toHaveURL('/problems?contest=only')
  await expect(toggle).toBeFocused()
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL('/problems')
  await expect(page.getByRole('table').getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(toggle).toBeFocused()
})

// The API refuses such a tag, so the list would only fail; the box says why instead, as the tag editor does.
test('a tag the API would refuse is not searched for', async ({ page }) => {
  const requests = listRequests(page)
  await page.goto('/problems?sort=oldest')
  await page.getByRole('switch', { name: 'タグを表示' }).check()
  await conditionsButton(page).click()
  const search = page.getByRole('combobox', { name: 'タグで絞り込む' })
  requests.length = 0
  for (const [text, reason] of [['a,b', 'タグにカンマや改行は使えません。'], ['x'.repeat(31), 'タグは30文字以内にしてください。']]) {
    await search.fill(text)
    await search.press('Enter')
    await expect(page.getByRole('alert')).toHaveText(reason)
    await expect(search).toHaveAccessibleDescription(reason)
    await expect(page).toHaveURL('/problems?sort=oldest')
  }
  expect(requests).toEqual([])
  // Typing again takes the reason away, and a tag that may be used is searched for.
  await search.fill('DP')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await search.press('Enter')
  await expect(page).toHaveURL('/problems?sort=oldest&tag=DP')
  // The post list has the same box.
  await page.goto('/blog')
  const posts = page.getByRole('search', { name: '記事をタグで絞り込む' }).getByRole('combobox', { name: 'タグで絞り込む' })
  await posts.fill('a,b')
  await posts.press('Enter')
  await expect(page.getByRole('alert')).toHaveText('タグにカンマや改行は使えません。')
  await expect(page).toHaveURL('/blog')
})

test('the conditions fit a phone and work without JavaScript', async ({ page, browser }) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto('/problems?sort=solvers-desc&contest=exclude&difficulty=3-6')
  await conditionsButton(page).click()
  await expect(page.getByRole('combobox', { name: '並び順' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(await outsidePanel(page)).toEqual([])
  const context = await browser.newContext({ javaScriptEnabled: false })
  const plain = await context.newPage()
  await plain.goto('/problems?tag=pagination&sort=oldest')
  await expect(plain.getByRole('table').locator('tbody tr').first()).toContainText('問題 120')
  await expect(plain.getByRole('list', { name: '効いている条件' })).toContainText('公開日が古い順')
  await context.close()
})

test('the list remembers the conditions the viewer chose', async ({ page }) => {
  await signIn(page)
  await page.goto('/problems')
  await conditionsButton(page).click()
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '正解者数が多い順' })
  await expect(page).toHaveURL('/problems?sort=solvers-desc')
  await page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' }).click()
  await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved')
  await expect.poll(() => remembered(page)).toEqual({ sort: 'solvers-desc', solved: 'unsolved' })
  // The server applies them to a bare /problems with a redirect, which adds nothing to the history.
  const response = await page.request.get('/problems', { maxRedirects: 0 })
  expect([response.status(), response.headers().location]).toEqual([302, '/problems?sort=solvers-desc&solved=unsolved'])
  // So does a move inside the app: the bare /problems is replaced before the list loads, and the list loads once.
  await page.goto('/contests')
  const requests = listRequests(page)
  await page.getByRole('navigation', { name: 'メインナビゲーション' }).getByRole('link', { name: '問題', exact: true }).click()
  await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved')
  await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/正解者数が多い順/, /未ACのみ/])
  expect(requests).toEqual([{ sort: 'solvers-desc', solved: 'unsolved', offset: '0', limit: '50' }])
  await page.goBack()
  await expect(page).toHaveURL('/contests')
})

test('a URL with a query opens as it is', async ({ page }) => {
  await remember(page, encoded({ sort: 'oldest', contest: 'only' }))
  // A page number alone is a query too. The fixture's list has one page, and a page past the end is replaced by the last
  // one, so the test asks for page 1.
  for (const path of ['/problems?tag=DP', '/problems?contest=exclude', '/problems?page=1']) {
    await page.goto(path)
    await expect(page).toHaveURL(path)
  }
  // Opening them did not rewrite what is remembered.
  expect(await remembered(page)).toEqual({ sort: 'oldest', contest: 'only' })
  // Taking the tag away leaves no query, so the remembered conditions come back, and the list loads once.
  await page.goto('/problems?tag=DP')
  const requests = listRequests(page)
  await page.getByRole('list', { name: '効いている条件' }).getByRole('button', { name: 'タグ: DPを外す' }).click()
  await expect(page).toHaveURL('/problems?sort=oldest&contest=only')
  // The fixture problem is no contest's.
  await expect(page.getByText('条件に合う問題はありません。')).toBeVisible()
  expect(requests).toEqual([{ sort: 'oldest', contest: 'only', offset: '0', limit: '50' }])
  // A condition chosen on a tag's list replaces what was remembered.
  await page.goto('/problems?tag=DP')
  await conditionsButton(page).click()
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '難易度が低い順' })
  await expect(page).toHaveURL('/problems?sort=difficulty-asc&tag=DP')
  await expect.poll(() => remembered(page)).toEqual({ sort: 'difficulty-asc' })
})

test('clearing forgets, and a broken cookie reads as nothing', async ({ page }) => {
  await remember(page, encoded({ sort: 'oldest' }))
  await page.goto('/problems')
  await expect(page).toHaveURL('/problems?sort=oldest')
  await conditionsButton(page).click()
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL('/problems')
  await expect.poll(() => remembered(page)).toBeNull()
  await expect(page.getByRole('combobox', { name: '並び順' })).toHaveValue('newest')
  // The last two are broken percent escapes, which Nuxt's own cookie decoding throws on.
  for (const value of ['%7Bbroken', encoded('oldest'), encoded({ sort: 'popular', difficulty: '9-2' }), '%', '%E0%A4%A']) {
    await remember(page, value)
    const response = await page.request.get('/problems', { maxRedirects: 0 })
    expect(response.status()).toBe(200)
  }
})

// A tag, a page number or anything else the cookie holds stays out of the redirect.
test('a bare /problems takes only the remembered conditions from the cookie', async ({ request }) => {
  const cookie = `openoj-problem-conditions=${encoded({ sort: 'oldest', tag: 'DP', evil: 'https://example.com', page: '3' })}`
  const response = await request.get('/problems', { headers: { cookie }, maxRedirects: 0 })
  expect([response.status(), response.headers().location]).toEqual([302, '/problems?sort=oldest'])
})

test('a signed-out viewer keeps a remembered solved filter without it taking effect', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await remember(page, encoded({ solved: 'unsolved', sort: 'oldest' }))
  await page.goto('/problems')
  // The URL keeps solved; the list does not apply it.
  await expect(page).toHaveURL('/problems?sort=oldest&solved=unsolved')
  await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/公開日が古い順/])
  await conditionsButton(page).click()
  await page.getByRole('combobox', { name: 'コンテスト' }).selectOption({ label: 'コンテストの問題のみ' })
  await expect(page).toHaveURL('/problems?sort=oldest&solved=unsolved&contest=only')
  await expect.poll(() => remembered(page)).toEqual({ sort: 'oldest', solved: 'unsolved', contest: 'only' })
})

for (const [mode, nextTask] of quickModes) {
  test(`two changes ${mode} both take effect`, async ({ page }) => {
    await signIn(page)
    const requests = listRequests(page)
    await page.goto('/problems?tag=pagination')
    await conditionsButton(page).click()
    const sort = page.getByRole('combobox', { name: '並び順' })
    const contest = page.getByRole('combobox', { name: 'コンテスト' })
    await quickly(page, [{ target: sort, pick: 'oldest' }, { target: contest, pick: 'exclude' }], { nextTask })
    await expect(page).toHaveURL('/problems?sort=oldest&contest=exclude&tag=pagination')
    await expect(sort).toHaveValue('oldest')
    await expect(contest).toHaveValue('exclude')
    // The list on screen is the one in the URL: the fixture lists the newest first, and oldest reverses it.
    await expect(page.getByRole('table').locator('tbody tr').first().locator('th')).toHaveText('問題 120')
    await expect.poll(() => requests.at(-1)).toEqual({ sort: 'oldest', contest: 'exclude', tag: 'pagination', offset: '0', limit: '50' })
    expect(requests.length).toBeLessThanOrEqual(2)
    await expect.poll(() => remembered(page)).toEqual({ sort: 'oldest', contest: 'exclude' })
    // The back button returns to the list before both. In one task the first move never finishes, so both changes are
    // one entry. In the next task, a busy machine may finish the first move before the second starts, which gives the
    // first change an entry of its own on the way back.
    await page.goBack()
    if (nextTask && page.url().endsWith('/problems?sort=oldest&tag=pagination')) await page.goBack()
    await expect(page).toHaveURL('/problems?tag=pagination')
    await expect(sort).toHaveValue('newest')
    await expect(contest).toHaveValue('')
  })

  test(`two chips taken off ${mode} are both gone`, async ({ page }) => {
    const requests = listRequests(page)
    await page.goto('/problems?sort=oldest&contest=exclude&difficulty=3-10&tag=pagination')
    const chips = page.getByRole('list', { name: '効いている条件' })
    await expect(chips.getByRole('listitem')).toHaveText([/公開日が古い順/, /コンテストの問題を除く/, /難易度 Lv\.3〜10/, /タグ: pagination/])
    await quickly(page, [{ target: chips.getByRole('button', { name: 'コンテストの問題を除くを外す' }) }, { target: chips.getByRole('button', { name: '難易度 Lv.3〜10を外す' }) }], { nextTask })
    await expect(page).toHaveURL('/problems?sort=oldest&tag=pagination')
    await expect(chips.getByRole('listitem')).toHaveText([/公開日が古い順/, /タグ: pagination/])
    await expect.poll(() => requests.at(-1)).toEqual({ sort: 'oldest', tag: 'pagination', offset: '0', limit: '50' })
    expect(requests.length).toBeLessThanOrEqual(2)
    await expect.poll(() => remembered(page)).toEqual({ sort: 'oldest' })
  })
}

// The clear comes while the pick's move runs the route middleware. The select itself may still show the pick: the route
// never had it, so its model never left 公開日が新しい順 and Vue has nothing to redraw.
// The pick's write is reported to the page after the clear. With the deletion's report held back, the report of the
// pick is the last one the page has, whether it came before the list's move to /problems or after it: the move that once
// failed under load came just between the two.
for (const late of [false, true]) {
  test(`clearing while a change is on its way wins over it${late ? ', with the deletion reported late' : ''}`, async ({ page }) => {
    if (late) await holdCookieDeletion(page)
    await remember(page, encoded({ contest: 'only' }))
    const requests = listRequests(page)
    await page.goto('/problems?tag=pagination')
    await conditionsButton(page).click()
    const sort = page.getByRole('combobox', { name: '並び順' })
    await quickly(page, [{ target: sort, pick: 'oldest' }, { target: page.getByRole('button', { name: '条件をすべて外す' }) }], { nextTask: true })
    if (late) await expect.poll(() => cookieReports(page)).toBe(1)
    // Nothing is left in force, and nothing remembered comes back.
    const cleared = async () => {
      await expect(page.getByRole('table').getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
      await expect(conditionsButton(page)).not.toContainText(/\d/)
      await expect(page).toHaveURL('/problems')
      await expect.poll(() => requests.at(-1)).toEqual({ offset: '0', limit: '50' })
      await expect.poll(() => remembered(page)).toBeNull()
    }
    await cleared()
    if (late) {
      await releaseCookieDeletion(page)
      await cleared()
    }
  })
}
