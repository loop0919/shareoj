import type { Page, Route } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

type Run = { id: string, status: 'QUEUED' | 'RUNNING' | 'DONE', result: Record<string, unknown> | null, createdAt: string }
const createdAt = '2026-10-25T00:00:00Z'
const queued: Run = { id: '33333333-3333-4333-8333-333333333333', status: 'QUEUED', result: null, createdAt }
const done = (result: Record<string, unknown>): Run => ({ ...queued, status: 'DONE', result })
const accepted = done({ verdict: 'AC', exitCode: 0, cpuTimeMs: 12, wallTimeMs: 20, memoryBytes: 3600000, stdout: { text: '3\n', truncated: false }, stderr: { text: '', truncated: false } })

// Every latest call answers with the next entry; the last one repeats. null is the 404 of "no run yet".
async function mockLatest(page: Page, answers: (Run | null)[]) {
  const state = { calls: 0 }
  await page.route('**/api/my/code-runs/latest', route => {
    const answer = answers[Math.min(state.calls++, answers.length - 1)]
    return answer ? route.fulfill({ json: answer }) : route.fulfill({ status: 404, json: { data: { code: 'code_run_not_found' } } })
  })
  return state
}

test('a signed-in user runs code, sees WJ with a spinner, then the output', async ({ page }) => {
  await mockLatest(page, [null, queued, accepted])
  const post = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/api/my/code-runs'))
  await page.route('**/api/my/code-runs', route => route.fulfill({ status: 202, json: queued }))
  await page.goto('/run')
  const source = 'int main(){}'
  await page.getByRole('combobox', { name: '言語' }).selectOption('cpp23-gcc')
  await page.getByLabel('ソースコード', { exact: true }).fill(source)
  await page.getByLabel('標準入力', { exact: true }).fill('1 2')
  const run = page.getByRole('button', { name: /実行$/ })
  // The output sits right under the input, and says where the output will appear before the first run.
  const output = page.getByRole('region', { name: '標準出力' })
  await expect(output).toContainText('実行すると、ここに出力が表示されます。')
  await expect(page.getByRole('heading', { name: '実行結果' })).toHaveCount(0)
  await run.click()
  expect((await post).postDataJSON()).toEqual({ runtime: 'cpp23-gcc', source, stdin: '1 2' })
  await expect(run).toHaveAttribute('aria-busy', 'true')
  await expect(run).toBeDisabled()
  await expect(run.locator('.loading-spinner')).toBeVisible()
  // The verdict and the measurements form one line in the output box's heading.
  const status = output.locator('.io-heading')
  await expect(status.locator('.verdict-badge')).toHaveText('WJ')
  await expect(status.locator('.verdict-badge')).toHaveText('AC')
  await expect(status).toContainText('終了コード 0 ・ 12 ms ・ 3.60 MB')
  await expect(output.locator('pre')).toHaveText('3')
  await expect(page.getByRole('region', { name: '標準エラー' })).toContainText('（出力なし）')
  await expect(run).toBeEnabled()
  await expect(run).not.toHaveAttribute('aria-busy', 'true')
})

test('the latest run comes back after leaving the page', async ({ page }) => {
  // Calls: mount (404), then two polls that stay queued; after returning, a queued read and the finished one.
  const latest = await mockLatest(page, [null, queued, queued, queued, accepted])
  await page.route('**/api/my/code-runs', route => route.fulfill({ status: 202, json: queued }))
  await page.goto('/run')
  await page.getByLabel('ソースコード', { exact: true }).fill('print(3)')
  await page.getByLabel('標準入力', { exact: true }).fill('1 2')
  await page.getByRole('combobox', { name: '言語' }).selectOption('python314')
  await page.getByRole('button', { name: /実行$/ }).click()
  await expect(page.locator('.verdict-badge')).toHaveText('WJ')
  await page.getByRole('link', { name: '問題', exact: true }).click()
  await expect(page).toHaveURL(/\/problems$/)
  const calls = latest.calls
  await page.waitForTimeout(3500)
  expect(latest.calls).toBe(calls)
  await page.goBack()
  await expect(page).toHaveURL(/\/run$/)
  await expect(page.getByLabel('ソースコード', { exact: true })).toHaveText('print(3)')
  await expect(page.getByLabel('標準入力', { exact: true })).toHaveValue('1 2')
  await expect(page.getByRole('combobox', { name: '言語' })).toHaveValue('python314')
  await expect(page.locator('.verdict-badge')).toHaveText('WJ')
  await expect(page.locator('.verdict-badge')).toHaveText('AC')
})

test('errors, limits and special results', async ({ page }) => {
  await mockLatest(page, [null])
  let posts = 0
  let answer: (route: Route) => Promise<void> = async () => {}
  await page.route('**/api/my/code-runs', route => { posts++; return answer(route) })
  await page.goto('/run')
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  const run = page.getByRole('button', { name: /実行$/ })
  const output = page.getByRole('region', { name: '標準出力' })

  answer = route => route.fulfill({ status: 429, headers: { 'Retry-After': '30' }, json: { data: { code: 'code_run_rate_limited', retryAfter: 30 } } })
  await run.click()
  await expect(page.getByRole('alert')).toHaveText('コード実行の頻度制限に到達しました。30秒後に再度試してください。')

  const before = posts
  await page.getByLabel('標準入力', { exact: true }).fill('a'.repeat(1048577))
  await run.click()
  await expect(page.getByRole('alert')).toHaveText('標準入力は1,048,576バイト以内で入力してください。')
  expect(posts).toBe(before)

  await page.getByLabel('標準入力', { exact: true }).fill('')
  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'JE', expired: true }) })
  await run.click()
  await expect(output.getByText('混雑のため実行できませんでした。時間をおいてもう一度実行してください。')).toBeVisible()
  await expect(output.locator('.verdict-badge')).toHaveCount(0)

  // A compile error runs nothing; the third box turns into the compiler's message.
  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'CE', compileLog: 'error: expected ;' }) })
  await run.click()
  await expect(output.locator('.io-heading .verdict-badge')).toHaveText('CE')
  await expect(output.getByText('コンパイルエラーのため実行していません。')).toBeVisible()
  await expect(page.getByRole('region', { name: '標準エラー' })).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'コンパイルのメッセージ' }).locator('pre')).toHaveText('error: expected ;')

  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'CE', compileLog: '' }) })
  await run.click()
  await expect(page.getByRole('region', { name: 'コンパイルのメッセージ' }).getByText('コンパイルのメッセージはありません。')).toBeVisible()

  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'JE' }) })
  await run.click()
  await expect(output.getByText('ジャッジで問題が起きたため、実行できませんでした。時間をおいてもう一度実行してください。')).toBeVisible()
  await expect(output.locator('pre')).toHaveCount(0)

  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'AC', exitCode: 0, cpuTimeMs: 1, memoryBytes: 1000000, stdout: { text: 'x'.repeat(100), truncated: true }, stderr: { text: '', truncated: false } }) })
  await run.click()
  await expect(page.getByText('長いため、先頭の64 KiBだけを表示しています。')).toHaveCount(1)
  await expect(page.getByRole('region', { name: '標準エラー' }).getByText('（出力なし）')).toBeVisible()
})

test('two columns on wide screens and one on phones', async ({ page }) => {
  await mockLatest(page, [null])
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/run')
  const source = page.getByLabel('ソースコード', { exact: true })
  const stdin = page.getByLabel('標準入力', { exact: true })
  await expect(source).toBeVisible()
  const wideSource = (await source.boundingBox())!
  const wideStdin = (await stdin.boundingBox())!
  expect(wideSource.x + wideSource.width).toBeLessThanOrEqual(wideStdin.x)
  // The heading has the size of the other pages the header opens, and both framed boxes and their headings line up.
  await expect(page.getByRole('heading', { level: 1 })).toHaveCSS('font-size', '28px')
  const frames = await Promise.all(['.source-code-editor', '.stdin-box'].map(async selector => (await page.locator(selector).boundingBox())!))
  expect(frames[1]!.y).toBeCloseTo(frames[0]!.y, 0)
  const bodies = await Promise.all(['.code-surface', '#code-run-stdin'].map(async selector => (await page.locator(selector).boundingBox())!))
  expect(bodies[1]!.y).toBeCloseTo(bodies[0]!.y, 0)
  // The run button shares the top row with the language, at its right end; the right column stacks input, output and error.
  const button = (await page.getByRole('button', { name: /実行$/ }).boundingBox())!
  const language = (await page.getByRole('combobox', { name: '言語' }).boundingBox())!
  expect(button.y + button.height).toBeLessThanOrEqual(frames[0]!.y)
  expect(button.x).toBeGreaterThan(language.x + language.width)
  expect(button.x + button.width).toBeCloseTo(frames[1]!.x + frames[1]!.width, 0)
  const boxes = await Promise.all(['.stdin-box', '[aria-labelledby="code-run-stdout-title"]', '[aria-labelledby="code-run-stderr-title"]'].map(async selector => (await page.locator(selector).boundingBox())!))
  expect(boxes[1]!.y).toBeGreaterThan(boxes[0]!.y + boxes[0]!.height)
  expect(boxes[2]!.y).toBeGreaterThan(boxes[1]!.y + boxes[1]!.height)
  expect(boxes.every(box => Math.abs(box.x - boxes[0]!.x) < 1)).toBe(true)
  // The three boxes' titles look alike: small and muted, like the source editor's.
  for (const title of [page.getByText('標準入力', { exact: true }), page.getByRole('heading', { name: '標準出力' }), page.getByRole('heading', { name: '標準エラー' })]) {
    await expect(title).toHaveCSS('font-size', '11px')
  }
  // A source editor stretched taller than the right column still leaves room above the footer.
  await page.locator('.code-surface').evaluate(element => { (element as HTMLElement).style.height = '1600px' })
  const editorBottom = await page.locator('.source-code-editor').evaluate(element => element.getBoundingClientRect().bottom)
  const footerTop = await page.locator('.site-footer').evaluate(element => element.getBoundingClientRect().top)
  expect(footerTop - editorBottom).toBeGreaterThanOrEqual(48)
  await page.locator('.code-surface').evaluate(element => { (element as HTMLElement).style.height = '' })
  await page.setViewportSize({ width: 375, height: 800 })
  await expect.poll(async () => (await stdin.boundingBox())!.y).toBeGreaterThanOrEqual((await source.boundingBox())!.y + (await source.boundingBox())!.height)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('signed-out visitors are asked to log in', async ({ page }) => {
  const latest = await mockLatest(page, [null])
  await page.route('**/api/auth/me', route => route.fulfill({ status: 401, json: {} }))
  await page.goto('/run')
  await expect(page.getByRole('heading', { name: 'ログインしてコードを実行' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'ログインする' })).toHaveAttribute('href', /^\/login\?next=(\/|%2F)run$/)
  expect(latest.calls).toBe(0)
})

test('maintenance closes the runner', async ({ page }) => {
  await mockLatest(page, [null])
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [], maintenance: true } }))
  await page.goto('/run')
  await expect(page.getByText('現在、コード実行の受付を停止しています。', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: /実行$/ })).toBeDisabled()
})

test('a saved language survives maintenance in the draft', async ({ page }) => {
  await mockLatest(page, [null])
  await page.addInitScript(() => localStorage.setItem('openoj.code-run-draft', JSON.stringify({ runtime: 'python314', source: 'x', stdin: 'y' })))
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [], maintenance: true } }))
  await page.goto('/run')
  await expect(page.getByLabel('標準入力', { exact: true })).toHaveValue('y')
  await expect(page.getByLabel('ソースコード', { exact: true })).toHaveText('x')
  const draft = await page.evaluate(() => JSON.parse(localStorage.getItem('openoj.code-run-draft') ?? '{}'))
  expect(draft).toEqual({ runtime: 'python314', source: 'x', stdin: 'y' })
})

test('an unavailable draft language falls back to the submission form language', async ({ page }) => {
  await mockLatest(page, [null])
  await page.addInitScript(() => {
    localStorage.setItem('openoj.code-run-draft', JSON.stringify({ runtime: 'gone-lang', source: 'x', stdin: '' }))
    localStorage.setItem('openoj.submission-runtime', 'python314')
  })
  await page.goto('/run')
  await expect(page.getByRole('combobox', { name: '言語' })).toHaveValue('python314')
})

const contestId = '88888888-8888-4888-8888-888888888888'
const contestPath = `/contests/${shortId(contestId)}`
const contestFixture = {
  id: contestId, title: 'コード実行テスト', author: 'setter', description: '', status: 'running',
  startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5, version: 1,
  official: true, participating: true, canEdit: false, canViewSubmissions: false, problems: [],
}

// The server renders from the real API, so the mocked contest comes in with the first refresh.
async function openContest(page: Page, view: string, contest: Record<string, unknown> = contestFixture) {
  await mockLatest(page, [null])
  await page.route(`**/api/contests/${contestId}**`, route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/standings')) return route.fulfill({ json: [] })
    if (url.pathname.endsWith('/submissions')) return route.fulfill({ json: { items: [], hasMore: false, total: 0 } })
    return route.fulfill({ json: contest })
  })
  await page.goto(`${contestPath}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.site-header')).toContainText(String(contest.title))
  if (view === 'standings') return
  await page.evaluate(to => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(to), `${contestPath}?view=${view}`)
  await expect(page).toHaveURL(`${contestPath}?view=${view}`)
}

test('the site menu offers code runs between posts and creation', async ({ page }) => {
  await mockLatest(page, [null])
  await page.goto('/')
  const nav = page.getByRole('navigation', { name: 'メインナビゲーション' })
  const order = await nav.evaluate(el => [...el.querySelectorAll(':scope > a, :scope > div > a, :scope > details > summary')].map(node => node.textContent?.trim()))
  expect(order).toEqual(['ホーム', '問題', '定期便', 'コンテスト', '記事', 'コード実行', '作成'])
  await nav.getByRole('link', { name: 'コード実行', exact: true }).click()
  await expect(page).toHaveURL(/\/run$/)
})

test('narrow screens scroll the menus instead of the page', async ({ page }) => {
  await mockLatest(page, [null])
  for (const width of [320, 375]) {
    await page.setViewportSize({ width, height: 800 })
    await page.goto('/')
    const nav = page.getByRole('navigation', { name: 'メインナビゲーション' })
    await nav.getByRole('link', { name: 'コード実行', exact: true }).scrollIntoViewIfNeeded()
    await expect(nav.getByRole('link', { name: 'コード実行', exact: true })).toBeInViewport()
    await expect(nav.locator('summary')).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await openContest(page, 'standings')
    const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
    await menu.getByRole('link', { name: 'コード実行', exact: true }).scrollIntoViewIfNeeded()
    await expect(menu.getByRole('link', { name: 'コード実行', exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.unrouteAll()
  }
})

test('dropdowns stay whole on a narrow screen', async ({ page }) => {
  await mockLatest(page, [null])
  await page.setViewportSize({ width: 320, height: 800 })
  await page.goto('/')
  const nav = page.getByRole('navigation', { name: 'メインナビゲーション' })
  await nav.locator('summary').click()
  const links = nav.locator('.create-menu-links a')
  await expect(links).toHaveCount(3)
  for (const link of await links.all()) await expect(link).toBeInViewport({ ratio: 1 })
  await page.unrouteAll()
  await openContest(page, 'standings', { ...contestFixture, status: 'ended', canViewSubmissions: true })
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await menu.evaluate(el => { el.scrollLeft = el.scrollWidth })
  await menu.locator('summary').click()
  const own = menu.getByRole('link', { name: '自分の提出', exact: true })
  const all = menu.getByRole('link', { name: 'すべての提出', exact: true })
  await expect(own).toBeInViewport({ ratio: 1 })
  await expect(all).toBeInViewport({ ratio: 1 })
  await all.click({ trial: true })
  await own.click({ trial: true })
  await menu.evaluate(el => { el.scrollLeft = 0 })
  await expect(own).toBeHidden()
})

test('the current menu item keeps its underline without a vertical scroll', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 })
  await openContest(page, 'run')
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  expect(await menu.evaluate(el => el.scrollHeight <= el.clientHeight)).toBe(true)
  const current = menu.getByRole('link', { name: 'コード実行', exact: true })
  await expect(current).toHaveCSS('border-bottom-width', '2px')
  await expect(current).toHaveAttribute('aria-current', 'page')
  await expect(current).not.toHaveCSS('border-bottom-color', 'rgba(0, 0, 0, 0)')
  const [link, box] = [(await current.boundingBox())!, (await menu.boundingBox())!]
  expect(link.y + link.height).toBeLessThanOrEqual(box.y + box.height)
})

test('the contest menu opens code runs in the contest', async ({ page }) => {
  await openContest(page, 'standings')
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await expect(menu.getByRole('link')).toHaveText(['概要', '問題', '順位表', '自分の提出', '解説一覧', '質問', 'コード実行'])
  await menu.getByRole('link', { name: 'コード実行', exact: true }).click()
  await expect(page).toHaveURL(`${contestPath}?view=run`)
  await expect(menu.getByRole('link', { name: 'コード実行', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('コード実行')
  await expect(page.getByRole('navigation', { name: 'パンくずリスト' })).toHaveText(`${contestFixture.title}/コード実行`)
  await expect(page.getByRole('button', { name: /実行$/ })).toBeVisible()
  await expect(page.locator('.site-header')).toContainText(contestFixture.title)
})

test('the header nav stays on one row from 60rem', async ({ page }) => {
  await mockLatest(page, [null])
  for (const width of [960, 1024]) {
    await page.setViewportSize({ width, height: 800 })
    await page.goto('/')
    const nav = page.getByRole('navigation', { name: 'メインナビゲーション' })
    await expect(nav.getByRole('link', { name: 'コード実行', exact: true })).toBeVisible()
    const tops = await nav.evaluate(el => [...el.querySelectorAll(':scope > .nav-links > a, :scope > details > summary')].map(node => Math.round(node.getBoundingClientRect().top)))
    expect(new Set(tops).size).toBe(1)
  }
})

for (const width of [320, 1280]) {
  test(`focus rings stay inside the contest menu at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 800 })
    await openContest(page, 'standings')
    const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
    const link = menu.getByRole('link', { name: '概要', exact: true })
    await link.focus()
    await page.keyboard.press('Shift+Tab')
    await page.keyboard.press('Tab')
    await expect(link).toBeFocused()
    const ring = await link.evaluate(el => {
      const style = getComputedStyle(el)
      const box = el.getBoundingClientRect()
      const scroller = el.parentElement!.getBoundingClientRect()
      const grow = Math.max(0, parseFloat(style.outlineOffset) + parseFloat(style.outlineWidth))
      return { top: box.top - grow - scroller.top, bottom: scroller.bottom - (box.bottom + grow), left: box.left - grow - scroller.left }
    })
    expect(ring.top).toBeGreaterThanOrEqual(0)
    expect(ring.bottom).toBeGreaterThanOrEqual(0)
    expect(ring.left).toBeGreaterThanOrEqual(0)
  })
}
