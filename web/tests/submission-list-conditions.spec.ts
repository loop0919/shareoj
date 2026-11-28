import type { APIRequestContext, Locator, Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { quickModes, quickly } from './fixtures/quickly'
import { shortId } from '../shared/utils/short-id'

// The fixture API echoes the requests for this contest and this problem.
const contest = '66666666-6666-4666-8666-666666666666'
const echoProblem = 'd8d8d8d8-d8d8-4d8d-8d8d-d8d8d8d8d8d8'
const problem = '11111111-1111-4111-8111-111111111111'
const session = { Cookie: 'openoj_access=valid-access' }

const echoed = async (request: APIRequestContext, path: string, headers?: Record<string, string>) => {
  const response = await request.get(path, { headers })
  expect(response.ok()).toBe(true)
  const { route, query } = await response.json() as { route: string, query: string }
  return { route, params: [...new URLSearchParams(query)] }
}

test('the contest list route passes the conditions on', async ({ request }) => {
  const list = `/api/contests/${contest}/submissions`
  const all = await echoed(request, `${list}?sort=fastest&verdict=AC&language=cpp&user=alice&problem=${problem}&limit=20`)
  expect(all.params).toEqual([['offset', '0'], ['mine', '0'], ['limit', '20'], ['problem', problem], ['sort', 'fastest'], ['verdict', 'AC'], ['language', 'cpp'], ['user', 'alice']])
  // The runtime is passed in place of the language.
  expect((await echoed(request, `${list}?runtime=cpp23-gcc`)).params).toEqual([['offset', '0'], ['mine', '0'], ['runtime', 'cpp23-gcc']])
  expect((await echoed(request, `${list}?language=cpp&runtime=cpp23-gcc`)).params).toEqual([['offset', '0'], ['mine', '0'], ['runtime', 'cpp23-gcc']])
  // Values that cannot be valid are dropped.
  for (const bad of ['sort=longest', 'verdict=%3Cscript%3E', 'verdict=ac', 'user=a%20b', 'language=C%2B%2B', 'runtime=cpp23-gcc-isolate', 'runtime=cpp23-local', 'evil=1']) {
    expect((await echoed(request, `${list}?${bad}`)).params, bad).toEqual([['offset', '0'], ['mine', '0']])
  }
  // The sort and the verdict take every listed value.
  for (const sort of ['newest', 'oldest', 'shortest', 'fastest']) expect((await echoed(request, `${list}?sort=${sort}`)).params).toContainEqual(['sort', sort])
  for (const verdict of ['AC', 'WA', 'TLE', 'MLE', 'OLE', 'RE', 'CE', 'JE']) expect((await echoed(request, `${list}?verdict=${verdict}`)).params).toContainEqual(['verdict', verdict])
  // The signed-in routes and the contest problem's list receive the conditions too.
  const own = await echoed(request, `${list}?mine=1&scope=onsite&sort=oldest&verdict=WA`, session)
  expect(own.route).toBe('private')
  expect(own.params).toEqual([['offset', '0'], ['mine', '1'], ['scope', 'onsite'], ['sort', 'oldest'], ['verdict', 'WA']])
  const byProblem = await echoed(request, `/api/contests/${contest}/problems/${problem}/submissions?sort=shortest&language=python&user=bob&evil=1`)
  expect(byProblem.route).toBe('public')
  expect(byProblem.params).toEqual([['offset', '0'], ['mine', '0'], ['sort', 'shortest'], ['language', 'python'], ['user', 'bob']])
  const byProblemOwn = await echoed(request, `/api/contests/${contest}/problems/${problem}/submissions?mine=team&runtime=cpp17`, session)
  expect(byProblemOwn.route).toBe('private')
  expect(byProblemOwn.params).toEqual([['offset', '0'], ['mine', 'team'], ['runtime', 'cpp17']])
})

test('the problem list route passes the conditions on', async ({ request }) => {
  const list = `/api/problems/${echoProblem}/submissions`
  const all = await echoed(request, `${list}?offset=20&sort=fastest&verdict=TLE&language=cpp&user=alice&evil=1`)
  expect(all.route).toBe('public')
  expect(all.params).toEqual([['offset', '20'], ['mine', '0'], ['sort', 'fastest'], ['verdict', 'TLE'], ['language', 'cpp'], ['user', 'alice']])
  expect((await echoed(request, `${list}?runtime=python314&language=python`)).params).toEqual([['offset', '0'], ['mine', '0'], ['runtime', 'python314']])
  for (const bad of ['sort=longest', 'verdict=%3Cscript%3E', 'user=a%20b', 'language=C%2B%2B', 'runtime=cpp23-gcc-local']) {
    expect((await echoed(request, `${list}?${bad}`)).params, bad).toEqual([['offset', '0'], ['mine', '0']])
  }
  // With a session the private path receives the same conditions.
  const signedIn = await echoed(request, `${list}?sort=oldest&verdict=AC&runtime=cpp23-gcc&user=bob`, session)
  expect(signedIn.route).toBe('private')
  expect(signedIn.params).toEqual([['offset', '0'], ['mine', '0'], ['sort', 'oldest'], ['verdict', 'AC'], ['runtime', 'cpp23-gcc'], ['user', 'bob']])
  const mine = await echoed(request, `${list}?mine=1&sort=shortest`, session)
  expect(mine.route).toBe('private')
  expect(mine.params).toEqual([['offset', '0'], ['mine', '1'], ['sort', 'shortest']])
})

// The problem page's list, in the browser. The mocked API filters and sorts a few runs by the query it gets, as the real
// one does, and keeps each query it got.
type Run = { id: string, author: string, runtime: string, sourceBytes: number, cpuTimeMs: number, verdict: string, status: string, createdAt: string }
const pagePath = `/problems/${shortId(problem)}`
const run = (n: number, author: string, runtime: string, sourceBytes: number, cpuTimeMs: number, verdict: string): Run => ({
  id: `33333333-3333-4333-8333-${String(n).padStart(12, '0')}`, author, runtime, sourceBytes, cpuTimeMs, verdict, status: 'DONE',
  createdAt: new Date(Date.UTC(2026, 8, 10) + n * 60_000).toISOString(),
})
// ui_test is the fixture's signed-in viewer.
const someRuns = () => [
  run(1, 'alice', 'cpp23-gcc', 300, 5, 'AC'),
  run(2, 'alice', 'cpp17', 120, 30, 'AC'),
  run(3, 'alice', 'python314', 80, 200, 'AC'),
  run(4, 'bob', 'cpp23-gcc', 90, 10, 'WA'),
  run(5, 'bob', 'rust2024', 400, 2, 'AC'),
  run(6, 'carol', 'cpp17', 150, 50, 'WA'),
  run(7, 'ui_test', 'cpp23-gcc', 200, 8, 'AC'),
  run(8, 'ui_test', 'python314', 60, 100, 'WA'),
]
const languageRuntimes: Record<string, string[]> = { cpp: ['cpp17', 'cpp23-gcc'], python: ['python314', 'pypy311'], rust: ['rust2024'] }
const newestFirst = (a: Run, b: Run) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id)
const orders: Record<string, (a: Run, b: Run) => number> = {
  newest: newestFirst,
  oldest: (a, b) => newestFirst(b, a),
  shortest: (a, b) => a.sourceBytes - b.sourceBytes || newestFirst(a, b),
  // A run being judged has no time yet and goes last.
  fastest: (a, b) => (a.status === 'DONE' ? a.cpuTimeMs : Infinity) - (b.status === 'DONE' ? b.cpuTimeMs : Infinity) || newestFirst(a, b),
}
async function mockList(page: Page, runs: Run[]) {
  const requests: Record<string, string>[] = []
  await page.route('**/api/problems/*/submissions?*', route => {
    const query = Object.fromEntries(new URL(route.request().url()).searchParams)
    requests.push(query)
    const matching = runs.filter(item => (query.mine !== '1' || item.author === 'ui_test')
      && (!query.verdict || (item.status === 'DONE' && item.verdict === query.verdict))
      && (!query.language || languageRuntimes[query.language]?.includes(item.runtime))
      && (!query.runtime || item.runtime === query.runtime)
      && (!query.user || item.author === query.user)).sort(orders[query.sort ?? 'newest'])
    const offset = Number(query.offset)
    const items = matching.slice(offset, offset + 50).map(item => ({
      id: item.id, problemId: problem, problemTitle: 'A + B', problemVersion: 1, author: item.author, runtime: item.runtime, sourceBytes: item.sourceBytes,
      status: item.status, createdAt: item.createdAt, result: item.status === 'DONE' ? { verdict: item.verdict, passed: 1, total: 1, cpuTimeMs: item.cpuTimeMs, memoryBytes: 1048576 } : null,
    }))
    return route.fulfill({ json: { items, hasMore: offset + 50 < matching.length } })
  })
  return requests
}
const conditionsButton = (page: Page) => page.getByRole('button', { name: /^絞り込み・並び替え/ })
const allList = (page: Page) => page.getByRole('region', { name: 'すべての提出' })
// Each row's code length, top to bottom.
const sizes = (list: Locator) => list.locator('tbody tr td[data-label="コード長"]')

test('the problem page lists submissions by the chosen conditions', async ({ page }) => {
  const requests = await mockList(page, someRuns())
  await page.goto(pagePath)
  await page.getByRole('navigation', { name: '問題メニュー' }).getByRole('link', { name: 'すべての提出' }).click()
  await expect(page).toHaveURL(`${pagePath}?view=submissions`)
  const list = allList(page)
  await expect(list.locator('tbody tr')).toHaveCount(8)
  const toggle = conditionsButton(page)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=shortest`)
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'AC' })
  await page.getByRole('combobox', { name: '言語' }).selectOption({ label: 'C++（すべて）' })
  await page.getByRole('textbox', { name: '提出者' }).fill('alice')
  await page.getByRole('textbox', { name: '提出者' }).press('Enter')
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=shortest&verdict=AC&language=cpp&user=alice`)
  await expect.poll(() => requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'shortest', verdict: 'AC', language: 'cpp', user: 'alice' })
  // Alice's accepted C++ runs, shortest first.
  await expect(sizes(list)).toHaveText(['120 bytes', '300 bytes'])
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  const language = page.getByRole('combobox', { name: '言語' })
  await expect(language.locator(':scope > option')).toHaveText(['すべて'])
  await expect(language.locator('optgroup')).toHaveCount(3)
  await expect(language.locator('optgroup[label="C++"] option')).toHaveText(['C++（すべて）', 'C++17 (GCC)', 'C++23 (GCC)'])
  await expect(language.locator('optgroup[label="Rust"] option')).toHaveText(['Rust (Edition 2024)'])
  await expect(language.locator('optgroup[label="Python"] option')).toHaveText(['Python（すべて）', 'Python (CPython 3.14)', 'Python (PyPy 3.11)'])
  await language.selectOption({ label: 'C++23 (GCC)' })
  await expect(page).toHaveURL(/runtime=cpp23-gcc/)
  await expect(page).not.toHaveURL(/language=/)
  await expect.poll(() => requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'shortest', verdict: 'AC', runtime: 'cpp23-gcc', user: 'alice' })
  await expect(sizes(list)).toHaveText(['300 bytes'])
  await expect(page.getByRole('combobox', { name: '並び順' })).toHaveValue('shortest')
  await expect(page.getByRole('combobox', { name: '結果' })).toHaveValue('AC')
  await expect(page.getByRole('textbox', { name: '提出者' })).toHaveValue('alice')
  // The changes replaced the list's entry in the history, so Back leaves the list.
  await page.goBack()
  await expect(page).toHaveURL(pagePath)
})

test('chips, clearing and the empty result', async ({ page }) => {
  const runs = someRuns()
  const requests = await mockList(page, runs)
  await page.goto(`${pagePath}?view=submissions&sort=fastest&verdict=AC&language=cpp&user=alice`)
  const chips = page.getByRole('list', { name: '効いている条件' })
  await expect(chips.getByRole('listitem')).toHaveText([/実行時間が短い順/, /結果: AC/, /言語: C\+\+/, /提出者: alice/])
  const toggle = conditionsButton(page)
  await expect(toggle).toContainText('4')
  await expect(sizes(allList(page))).toHaveText(['300 bytes', '120 bytes'])
  await chips.getByRole('button', { name: '結果: ACを外す' }).click()
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=fastest&language=cpp&user=alice`)
  await expect(toggle).toBeFocused()
  await expect.poll(() => requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'fastest', language: 'cpp', user: 'alice' })
  await expect(toggle).toContainText('3')
  // No run matches: the list says so and offers a way out, after which the focus is on the panel's button.
  await page.goto(`${pagePath}?view=submissions&verdict=TLE`)
  await expect(page.getByText('条件に合う提出はありません。')).toBeVisible()
  await expect(page.getByText('提出はまだありません。')).toHaveCount(0)
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL(`${pagePath}?view=submissions`)
  await expect(toggle).toBeFocused()
  await expect(allList(page).locator('tbody tr')).toHaveCount(8)
  expect(requests.at(-1)).toEqual({ mine: '0', offset: '0' })
  // No condition and no run: the list's own words, and nothing to clear.
  runs.length = 0
  await page.reload()
  await expect(page.getByText('提出はまだありません。')).toBeVisible()
  await expect(page.getByText('条件に合う提出はありません。')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '条件をすべて外す' })).toHaveCount(0)
})

test('paging starts over on a change and keeps the conditions', async ({ page }) => {
  const runs = Array.from({ length: 120 }, (_, i) => run(i, `user${i}`, 'cpp17', 1000 - i, i, 'AC'))
  const requests = await mockList(page, runs)
  await page.goto(`${pagePath}?view=submissions&verdict=AC`)
  const list = allList(page)
  await expect(list.locator('tbody tr')).toHaveCount(50)
  await list.getByRole('button', { name: '次へ' }).click()
  await expect(list.getByRole('status').filter({ hasText: '2 ページ' })).toBeVisible()
  expect(requests.at(-1)).toEqual({ mine: '0', offset: '50', verdict: 'AC' })
  // The list's page is its own: the URL keeps the conditions alone.
  await expect(page).toHaveURL(`${pagePath}?view=submissions&verdict=AC`)
  await conditionsButton(page).click()
  requests.length = 0
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=shortest&verdict=AC`)
  await expect(list.getByRole('status').filter({ hasText: '1 ページ' })).toBeVisible()
  await expect(sizes(list).first()).toHaveText('881 bytes')
  // One request, for the first page in the new order: the second page is never asked for in it.
  expect(requests).toEqual([{ mine: '0', offset: '0', sort: 'shortest', verdict: 'AC' }])
})

test('the user field refuses what cannot be a user ID', async ({ page }) => {
  const requests = await mockList(page, someRuns())
  await page.goto(`${pagePath}?view=submissions&sort=oldest`)
  await expect(allList(page).locator('tbody tr')).toHaveCount(8)
  const toggle = conditionsButton(page)
  await toggle.click()
  const panel = page.locator(`[id="${await toggle.getAttribute('aria-controls')}"]`)
  const box = panel.getByRole('textbox', { name: '提出者' })
  await expect(box).toHaveAttribute('placeholder', 'ユーザーID')
  const reason = 'ユーザーIDは英小文字で始まる3〜20文字（英小文字・数字・_）です。'
  requests.length = 0
  for (const text of ['a b', 'ab', '1alice']) {
    await box.fill(text)
    await box.press('Enter')
    await expect(panel.getByRole('alert')).toHaveText(reason)
    await expect(box).toHaveAccessibleDescription(reason)
    await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=oldest`)
  }
  expect(requests).toEqual([])
  // Typing again takes the reason away; the button applies as Enter does.
  await box.fill('bob')
  await expect(panel.getByRole('alert')).toHaveCount(0)
  await panel.getByRole('button', { name: '絞り込む' }).click()
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=oldest&user=bob`)
  await expect(sizes(allList(page))).toHaveText(['90 bytes', '400 bytes'])
  // An empty box takes the submitter off.
  await box.fill('')
  await box.press('Enter')
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=oldest`)
  await expect.poll(() => requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'oldest' })
})

test('switching between all and own submissions keeps the conditions but the user', async ({ page }) => {
  const requests = await mockList(page, someRuns())
  await page.goto(`${pagePath}?view=submissions&sort=shortest&verdict=AC&user=alice`)
  const menu = page.getByRole('navigation', { name: '問題メニュー' })
  // The statement and the editorial start without the conditions.
  await expect(menu.getByRole('link', { name: '問題文' })).toHaveAttribute('href', pagePath)
  await expect(menu.getByRole('link', { name: '解説' })).toHaveAttribute('href', `${pagePath}?view=editorial`)
  await conditionsButton(page).click()
  await menu.getByRole('link', { name: '自分の提出' }).click()
  await expect(page).toHaveURL(`${pagePath}?view=my-submissions&sort=shortest&verdict=AC`)
  const own = page.getByRole('region', { name: '自分の提出' })
  await expect(sizes(own)).toHaveText(['200 bytes'])
  expect(requests.at(-1)).toEqual({ mine: '1', offset: '0', sort: 'shortest', verdict: 'AC' })
  // Another list starts with the panel closed, and the own list has no 提出者 field.
  const toggle = conditionsButton(page)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/コード長が短い順/, /結果: AC/])
  await toggle.click()
  await expect(page.getByRole('combobox', { name: '結果' })).toHaveValue('AC')
  await expect(page.getByRole('textbox', { name: '提出者' })).toHaveCount(0)
  // A submitter in the URL of the own list is neither shown nor sent.
  await page.goto(`${pagePath}?view=my-submissions&sort=shortest&verdict=AC&user=alice`)
  await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/コード長が短い順/, /結果: AC/])
  await expect(toggle).toContainText('2')
  await expect(sizes(own)).toHaveText(['200 bytes'])
  expect(requests.at(-1)).toEqual({ mine: '1', offset: '0', sort: 'shortest', verdict: 'AC' })
  await expect(menu.getByRole('link', { name: 'すべての提出' })).toHaveAttribute('href', `${pagePath}?view=submissions&sort=shortest&verdict=AC`)
  // A change there leaves it out of the URL too.
  await toggle.click()
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'WA' })
  await expect(page).toHaveURL(`${pagePath}?view=my-submissions&sort=shortest&verdict=WA`)
  await expect(sizes(own)).toHaveText(['60 bytes'])
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'AC' })
  await expect(page).toHaveURL(`${pagePath}?view=my-submissions&sort=shortest&verdict=AC`)
  await menu.getByRole('link', { name: 'すべての提出' }).click()
  await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=shortest&verdict=AC`)
  await expect(sizes(allList(page))).toHaveText(['80 bytes', '120 bytes', '200 bytes', '300 bytes', '400 bytes'])
  expect(requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'shortest', verdict: 'AC' })
})

// The clock stands still between steps. A poll sets its next one only once it is answered, so each step waits for the
// answer (the refresh button is busy until then) and counts the requests it brought.
async function step(page: Page, requests: unknown[], seconds = 3) {
  const before = requests.length
  await page.clock.runFor(seconds * 1000)
  await expect(allList(page).getByRole('button', { name: '更新', exact: true })).toBeEnabled()
  return requests.length - before
}

test('the list refreshes with its conditions and keeps the panel and the selection', async ({ page }) => {
  const runs = someRuns()
  // The viewer may rejudge; a rejudged run is judged until the test says otherwise.
  await page.route('**/api/my/problems/*/rejudge', route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { pending: 0 } })
    const { submissionIds } = route.request().postDataJSON() as { submissionIds: string[] }
    for (const item of runs) if (submissionIds.includes(item.id)) item.status = 'RUNNING'
    return route.fulfill({ status: 202, json: { queued: submissionIds.length } })
  })
  const requests = await mockList(page, runs)
  await page.clock.install()
  await page.goto(`${pagePath}?view=submissions`)
  const list = allList(page)
  const toggle = conditionsButton(page)
  await toggle.click()
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'WA' })
  await expect(page).toHaveURL(`${pagePath}?view=submissions&verdict=WA`)
  const rows = list.getByRole('checkbox', { name: /の提出を選択$/ })
  await expect(rows).toHaveCount(3)
  await rows.nth(1).check()
  await rows.nth(2).check()
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  // The regular refresh reads the list with its conditions, and leaves the panel and the chosen rows as they are.
  expect(await step(page, requests, 15)).toBe(1)
  expect(requests.at(-1)).toEqual({ mine: '0', offset: '0', verdict: 'WA' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(rows).toHaveCount(3)
  await expect(rows.nth(1)).toBeChecked()
  await expect(rows.nth(2)).toBeChecked()
  page.once('dialog', dialog => void dialog.accept())
  await list.getByRole('button', { name: '選択した2件の提出をリジャッジ', exact: true }).click()
  // The rejudged runs are being judged, so the list of WA no longer holds them, and the message still shows.
  await expect(list.getByRole('status').filter({ hasText: '2件の提出をリジャッジしています。' })).toBeVisible()
  await expect(rows).toHaveCount(1)
  // The list follows them every 3 seconds though none is in view...
  for (let i = 0; i < 3; i++) expect(await step(page, requests)).toBeGreaterThanOrEqual(1)
  expect(requests.at(-1)).toEqual({ mine: '0', offset: '0', verdict: 'WA' })
  // ...until they are back and judged. Then only the regular refresh is left, at most once in 9 seconds.
  for (const item of runs) if (item.status === 'RUNNING') item.status = 'DONE'
  expect(await step(page, requests)).toBe(1)
  await expect(rows).toHaveCount(3)
  let later = 0
  for (let i = 0; i < 3; i++) later += await step(page, requests)
  expect(later).toBeLessThanOrEqual(1)
  // Runs that never come back are followed for a minute after the rejudge, and no longer.
  await rows.nth(1).check()
  await rows.nth(2).check()
  page.once('dialog', dialog => void dialog.accept())
  await list.getByRole('button', { name: '選択した2件の提出をリジャッジ', exact: true }).click()
  await expect(rows).toHaveCount(1)
  for (let i = 0; i < 19; i++) expect(await step(page, requests)).toBeGreaterThanOrEqual(1)
  await step(page, requests)
  await step(page, requests)
  later = 0
  for (let i = 0; i < 3; i++) later += await step(page, requests)
  expect(later).toBeLessThanOrEqual(1)
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
})

for (const [mode, nextTask] of quickModes) {
  test(`two changes ${mode} both take effect`, async ({ page }) => {
    const requests = await mockList(page, someRuns())
    await page.goto(`${pagePath}?view=submissions`)
    await expect(allList(page).locator('tbody tr')).toHaveCount(8)
    const toggle = conditionsButton(page)
    await toggle.click()
    const sort = page.getByRole('combobox', { name: '並び順' })
    const verdict = page.getByRole('combobox', { name: '結果' })
    requests.length = 0
    await quickly(page, [{ target: sort, pick: 'oldest' }, { target: verdict, pick: 'WA' }], { nextTask })
    await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=oldest&verdict=WA`)
    await expect(sort).toHaveValue('oldest')
    await expect(verdict).toHaveValue('WA')
    // The list on screen is the one in the URL.
    await expect(sizes(allList(page))).toHaveText(['90 bytes', '150 bytes', '60 bytes'])
    await expect.poll(() => requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'oldest', verdict: 'WA' })
    expect(requests.length).toBeLessThanOrEqual(2)
    await toggle.click()
    const chips = page.getByRole('list', { name: '効いている条件' })
    await expect(chips.getByRole('listitem')).toHaveText([/提出日時が古い順/, /結果: WA/])
    // Two chips taken off at once are both gone.
    requests.length = 0
    await quickly(page, [{ target: chips.getByRole('button', { name: '提出日時が古い順を外す' }) }, { target: chips.getByRole('button', { name: '結果: WAを外す' }) }], { nextTask })
    await expect(page).toHaveURL(`${pagePath}?view=submissions`)
    await expect(chips).toHaveCount(0)
    await expect(toggle).not.toContainText(/\d/)
    await expect(allList(page).locator('tbody tr')).toHaveCount(8)
    await expect.poll(() => requests.at(-1)).toEqual({ mine: '0', offset: '0' })
    expect(requests.length).toBeLessThanOrEqual(2)
  })
}

test('the panel fits a phone', async ({ page }) => {
  await mockList(page, someRuns())
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto(`${pagePath}?view=submissions&sort=shortest&verdict=AC&runtime=cpp23-gcc&user=alice`)
  await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/コード長が短い順/, /結果: AC/, /言語: C\+\+23 \(GCC\)/, /提出者: alice/])
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await conditionsButton(page).click()
  await expect(page.getByRole('textbox', { name: '提出者' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
