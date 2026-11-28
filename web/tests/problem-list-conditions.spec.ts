import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'

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
  await toggle.click()
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

test('the conditions fit a phone and work without JavaScript', async ({ page, browser }) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto('/problems?sort=solvers-desc&contest=exclude&difficulty=3-6')
  await conditionsButton(page).click()
  await expect(page.getByRole('combobox', { name: '並び順' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const context = await browser.newContext({ javaScriptEnabled: false })
  const plain = await context.newPage()
  await plain.goto('/problems?tag=pagination&sort=oldest')
  await expect(plain.getByRole('table').locator('tbody tr').first()).toContainText('問題 120')
  await expect(plain.getByRole('list', { name: '効いている条件' })).toContainText('公開日が古い順')
  await context.close()
})
