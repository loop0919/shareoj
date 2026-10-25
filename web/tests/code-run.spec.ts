import type { Page, Route } from '@playwright/test'
import { expect, test } from './fixtures/account'

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
  await run.click()
  expect((await post).postDataJSON()).toEqual({ runtime: 'cpp23-gcc', source, stdin: '1 2' })
  const result = page.getByRole('region', { name: '実行結果' })
  await expect(run).toHaveAttribute('aria-busy', 'true')
  await expect(run).toBeDisabled()
  await expect(run.locator('.loading-spinner')).toBeVisible()
  await expect(result.locator('.verdict-badge')).toHaveText('WJ')
  await expect(result.locator('.verdict-badge')).toHaveText('AC')
  await expect(result.getByText('終了コード 0')).toBeVisible()
  await expect(result.getByText('12 ms')).toBeVisible()
  await expect(result.getByText('3.60 MB')).toBeVisible()
  await expect(result.getByRole('region', { name: '標準出力' }).locator('pre')).toHaveText('3')
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
  const result = page.getByRole('region', { name: '実行結果' })

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
  await expect(result.getByText('混雑のため実行できませんでした。時間をおいてもう一度実行してください。')).toBeVisible()
  await expect(result.locator('.verdict-badge')).toHaveCount(0)

  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'CE', compileLog: 'error: expected ;' }) })
  await run.click()
  await expect(result.locator('.verdict-badge')).toHaveText('CE')
  await expect(result.getByRole('region', { name: 'コンパイルのメッセージ' }).locator('pre')).toHaveText('error: expected ;')

  answer = route => route.fulfill({ status: 202, json: done({ verdict: 'AC', exitCode: 0, cpuTimeMs: 1, memoryBytes: 1000000, stdout: { text: 'x'.repeat(100), truncated: true }, stderr: { text: '', truncated: false } }) })
  await run.click()
  await expect(result.getByText('長いため、先頭の64 KiBだけを表示しています。')).toHaveCount(1)
  await expect(result.getByRole('region', { name: '標準エラー' }).getByText('（出力なし）')).toBeVisible()
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
