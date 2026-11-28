import type { APIRequestContext, Locator, Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { quickModes, quickly } from './fixtures/quickly'
import { outsidePanel } from './fixtures/panel'
import { visit } from './fixtures/visit'
import { shortId } from '../shared/utils/short-id'

// The fixture API echoes the requests for this contest and this problem.
const contest = '66666666-6666-4666-8666-666666666666'
const echoProblem = 'd8d8d8d8-d8d8-4d8d-8d8d-d8d8d8d8d8d8'
const problem = '11111111-1111-4111-8111-111111111111'
// The contest's second problem, B.
const problemB = '44444444-4444-4444-8444-444444444444'
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
// one does, refuses what the real one refuses, and keeps each query it got.
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
// The fixture's catalogue.
const languageRuntimes: Record<string, string[]> = { cpp: ['cpp17', 'cpp23-gcc'], python: ['python314', 'pypy311'], rust: ['rust2024'] }
// The API answers 400 to a language or a runtime its catalogue does not list, and to both at once.
const refused = (query: Record<string, string>) => (query.language !== undefined && !Object.hasOwn(languageRuntimes, query.language))
  || (query.runtime !== undefined && !Object.values(languageRuntimes).flat().includes(query.runtime))
  || (query.language !== undefined && query.runtime !== undefined)
const invalidRequest = { status: 400, json: { code: 'invalid_request' } }
const newestFirst = (a: Run, b: Run) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id)
const orders: Record<string, (a: Run, b: Run) => number> = {
  newest: newestFirst,
  oldest: (a, b) => newestFirst(b, a),
  shortest: (a, b) => a.sourceBytes - b.sourceBytes || newestFirst(a, b),
  // A run being judged has no time yet and goes last.
  fastest: (a, b) => (a.status === 'DONE' ? a.cpuTimeMs : Infinity) - (b.status === 'DONE' ? b.cpuTimeMs : Infinity) || newestFirst(a, b),
}
// The sort's and the filters' part of the API: verdict, language, runtime and user.
const matches = (item: Run, query: Record<string, string>) => (!query.verdict || (item.status === 'DONE' && item.verdict === query.verdict))
  && (!query.language || languageRuntimes[query.language]?.includes(item.runtime))
  && (!query.runtime || item.runtime === query.runtime)
  && (!query.user || item.author === query.user)
// A run as the API lists it; a contest's run names its problem.
const listed = (item: Run & { problemId?: string }) => ({
  id: item.id, problemId: item.problemId ?? problem, problemTitle: item.problemId === problemB ? 'A × B' : 'A + B', problemVersion: 1, author: item.author, runtime: item.runtime, sourceBytes: item.sourceBytes,
  status: item.status, createdAt: item.createdAt, result: item.status === 'DONE' ? { verdict: item.verdict, passed: 1, total: 1, cpuTimeMs: item.cpuTimeMs, memoryBytes: 1048576 } : null,
})
async function mockList(page: Page, runs: Run[]) {
  const requests: Record<string, string>[] = []
  await page.route('**/api/problems/*/submissions?*', route => {
    const query = Object.fromEntries(new URL(route.request().url()).searchParams)
    requests.push(query)
    if (refused(query)) return route.fulfill(invalidRequest)
    const matching = runs.filter(item => (query.mine !== '1' || item.author === 'ui_test') && matches(item, query)).sort(orders[query.sort ?? 'newest'])
    const offset = Number(query.offset)
    return route.fulfill({ json: { items: matching.slice(offset, offset + 50).map(listed), hasMore: offset + 50 < matching.length } })
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
  expect(await outsidePanel(page)).toEqual([])
})

// The URL may name a language or a runtime the catalogue does not list: a hand-edited link, a runtime's ID given as a
// language, a language given as a runtime, or a runtime dropped since the link was made. The lists read it as すべて, as
// they read any unknown value: it is neither shown, nor counted, nor sent, the tabs and the menu leave it out, and so
// does a change in the panel.
const unknownLanguages = ['language=zzz', 'language=cpp23-gcc', 'runtime=cpp', 'runtime=cobol85']
const chipList = (page: Page) => page.getByRole('list', { name: '効いている条件' })
// A signed-in viewer's list is read again once the browser has the viewer; a viewer who is not signed in has no such
// second read, so the requests of a load can be counted.
const signedOut = (page: Page) => page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
for (const unknown of unknownLanguages) {
  test(`the problem page reads ${unknown} as every language`, async ({ page }) => {
    await signedOut(page)
    const requests = await mockList(page, someRuns())
    await page.clock.install()
    const opened = `${pagePath}?view=submissions&verdict=AC&${unknown}`
    await page.goto(opened)
    const list = allList(page)
    // The accepted runs, newest first, in one request without the language.
    await expect(sizes(list)).toHaveText(['200 bytes', '400 bytes', '80 bytes', '120 bytes', '300 bytes'])
    await expect(list.getByText(/提出一覧を取得できませんでした/)).toHaveCount(0)
    expect(requests).toEqual([{ mine: '0', offset: '0', verdict: 'AC' }])
    await expect(chipList(page).getByRole('listitem')).toHaveText([/結果: AC/])
    const toggle = conditionsButton(page)
    await expect(toggle).toContainText('1')
    await toggle.click()
    const language = page.getByRole('combobox', { name: '言語' })
    await expect(language).toHaveValue('')
    await expect(language.locator('option:checked')).toHaveText('すべて')
    // The tabs keep the other conditions alone.
    const menu = page.getByRole('navigation', { name: '問題メニュー' })
    await expect(menu.getByRole('link', { name: '自分の提出' })).toHaveAttribute('href', `${pagePath}?view=my-submissions&verdict=AC`)
    await expect(menu.getByRole('link', { name: 'すべての提出' })).toHaveAttribute('href', `${pagePath}?view=submissions&verdict=AC`)
    // The regular refresh asks with the same conditions, also after the catalogue has been read again (every 30
    // seconds).
    await page.clock.pauseAt(new Date(Date.now() + 1000))
    for (let i = 0; i < 2; i++) expect(await step(page, requests, 15)).toBe(1)
    expect(requests).toEqual(Array(3).fill({ mine: '0', offset: '0', verdict: 'AC' }))
    // The URL keeps the value until the conditions change; a change in the panel leaves it out.
    await expect(page).toHaveURL(opened)
    await page.clock.resume()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
    await expect(page).toHaveURL(`${pagePath}?view=submissions&sort=shortest&verdict=AC`)
    await expect(sizes(list)).toHaveText(['80 bytes', '120 bytes', '200 bytes', '300 bytes', '400 bytes'])
    expect(requests.at(-1)).toEqual({ mine: '0', offset: '0', sort: 'shortest', verdict: 'AC' })
    expect(requests.filter(refused)).toEqual([])
  })
}

test('the own list and the empty list read an unknown language as every language', async ({ page }) => {
  const runs = someRuns()
  const requests = await mockList(page, runs)
  await page.goto(`${pagePath}?view=my-submissions&verdict=AC&runtime=cpp`)
  // ui_test's accepted run.
  await expect(sizes(page.getByRole('region', { name: '自分の提出' }))).toHaveText(['200 bytes'])
  await expect.poll(() => requests.at(-1)).toEqual({ mine: '1', offset: '0', verdict: 'AC' })
  await expect(chipList(page).getByRole('listitem')).toHaveText([/結果: AC/])
  await expect(page.getByRole('navigation', { name: '問題メニュー' }).getByRole('link', { name: 'すべての提出' })).toHaveAttribute('href', `${pagePath}?view=submissions&verdict=AC`)
  // Without a run the list says it has none: the value is no condition that could have emptied it.
  runs.length = 0
  await page.goto(`${pagePath}?view=submissions&language=zzz`)
  await expect(allList(page).getByText('提出はまだありません。')).toBeVisible()
  await expect(page.getByText('条件に合う提出はありません。')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '条件をすべて外す' })).toHaveCount(0)
  await expect(conditionsButton(page)).not.toContainText(/\d/)
  expect(requests.filter(refused)).toEqual([])
})

// The page is rendered with the catalogue, so a load asks for the list once, with a language as without one, and the
// browser's own read of the catalogue right after does not ask again.
for (const [query, sent] of [['language=cpp', { language: 'cpp' }], ['runtime=cpp23-gcc', { runtime: 'cpp23-gcc' }], ['language=zzz', {}], ['', {}]] as const) {
  test(`a load of the problem page asks for the list once ${query ? `with ${query}` : 'without a language'}`, async ({ page }) => {
    await signedOut(page)
    const requests = await mockList(page, someRuns())
    await page.clock.install()
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto(`${pagePath}?view=submissions${query ? `&${query}` : ''}`)])
    await expect(allList(page).locator('tbody tr').first()).toBeVisible()
    const asked = { mine: '0', offset: '0', ...sent }
    await page.clock.pauseAt(new Date(Date.now() + 1000))
    expect(await step(page, requests, 15)).toBe(1)
    expect(requests).toEqual([asked, asked])
  })
}

// Until the catalogue is read the list cannot tell a language the API takes from one it refuses, so it asks without
// any, and once more when the catalogue comes and lists it. The second server's API has the problem but no catalogue,
// so the page is rendered without one; the browser's own read of it is let through when the test says so.
test('before the catalogue is read the list asks without the language', async ({ page }) => {
  await signedOut(page)
  const requests = await mockList(page, someRuns())
  let release = () => {}
  const released = new Promise<void>(resolve => { release = resolve })
  const catalogue = { items: [], languages: [{ key: 'cpp', label: 'C++', runtimes: [{ id: 'cpp17', label: 'C++17 (GCC)' }, { id: 'cpp23-gcc', label: 'C++23 (GCC)' }] }] }
  await page.route('**/api/runtimes', async route => { await released; await route.fulfill({ json: catalogue }) })
  const path = `/problems/${shortId('22222222-2222-4222-8222-222222222222')}`
  await page.goto(`http://127.0.0.1:13001${path}?view=submissions&verdict=AC&language=cpp`)
  await expect(page.getByText(/ジャッジ機能の状態を確認できません/)).toBeVisible()
  const list = allList(page)
  const menu = page.getByRole('navigation', { name: '問題メニュー' })
  await expect(sizes(list)).toHaveText(['200 bytes', '400 bytes', '80 bytes', '120 bytes', '300 bytes'])
  expect(requests).toEqual([{ mine: '0', offset: '0', verdict: 'AC' }])
  await expect(chipList(page).getByRole('listitem')).toHaveText([/結果: AC/])
  await expect(menu.getByRole('link', { name: '自分の提出' })).toHaveAttribute('href', `${path}?view=my-submissions&verdict=AC`)
  // The catalogue comes and lists C++.
  release()
  await expect(chipList(page).getByRole('listitem')).toHaveText([/結果: AC/, /言語: C\+\+/])
  await expect(sizes(list)).toHaveText(['200 bytes', '120 bytes', '300 bytes'])
  expect(requests).toEqual([{ mine: '0', offset: '0', verdict: 'AC' }, { mine: '0', offset: '0', verdict: 'AC', language: 'cpp' }])
  await expect(menu.getByRole('link', { name: '自分の提出' })).toHaveAttribute('href', `${path}?view=my-submissions&verdict=AC&language=cpp`)
})

// A read of the catalogue that fails once the list has it changes nothing: the languages last read still hold.
test('a failed read of the catalogue keeps the languages last read', async ({ page }) => {
  await signedOut(page)
  const requests = await mockList(page, someRuns())
  await page.route('**/api/runtimes', route => route.fulfill({ status: 502, json: {} }))
  await page.clock.install()
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto(`${pagePath}?view=submissions&language=cpp`)])
  await expect(page.getByText(/ジャッジ機能の状態を確認できません/)).toBeVisible()
  await expect(chipList(page).getByRole('listitem')).toHaveText([/言語: C\+\+/])
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  expect(await step(page, requests, 15)).toBe(1)
  expect(requests).toEqual(Array(2).fill({ mine: '0', offset: '0', language: 'cpp' }))
  await expect(chipList(page).getByRole('listitem')).toHaveText([/言語: C\+\+/])
})

// A contest's lists, in the browser. The mocked API filters, sorts and pages the runs by the query it gets, as the real
// one does, counts what the conditions leave, refuses what the real one refuses, and keeps each query it got. ui_test
// is on the contest's staff, so the whole list is open while the contest runs.
type ContestRun = Run & { problemId: string, onsite: boolean }
type Team = { id: string, name: string, members: string[], onsite: boolean }
const contestId = '88888888-8888-4888-8888-888888888888'
const contestPath = `/contests/${shortId(contestId)}`
const staffContest = {
  id: contestId, title: '提出の絞り込みテスト', author: 'alice', authors: ['alice'], description: '', status: 'running', startsAt: '2026-09-10T00:00:00Z', endsAt: '2099-09-10T02:00:00Z',
  penaltyMinutes: 5, version: 1, official: false, staff: true, canEdit: false, participating: false, canViewSubmissions: true, onsiteDivision: true, teamMode: false, team: null as Team | null,
  problems: [{ id: problem, title: 'A + B', points: 100 }, { id: problemB, title: 'A × B', points: 200 }],
}
// someRuns over the contest's two problems, A's from the first one on and B's in between; bob takes part on site.
const contestRuns = (): ContestRun[] => someRuns().map((item, index) => ({ ...item, problemId: index % 2 ? problemB : problem, onsite: item.author === 'bob' }))
async function mockContest(page: Page, runs: ContestRun[]) {
  const state = { contest: staffContest, requests: [] as Record<string, string>[] }
  await page.route(`**/api/contests/${contestId}**`, route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/standings')) return route.fulfill({ json: [] })
    if (!url.pathname.endsWith('/submissions')) return route.fulfill({ json: state.contest })
    const query = Object.fromEntries(url.searchParams)
    state.requests.push(query)
    if (refused(query)) return route.fulfill(invalidRequest)
    const team = state.contest.team?.members ?? []
    const own = (item: ContestRun) => query.mine === 'team' ? team.includes(item.author) : item.author === 'ui_test'
    const matching = runs.filter(item => (!query.mine || own(item)) && (query.scope !== 'onsite' || item.onsite) && (!query.problem || item.problemId === query.problem) && matches(item, query))
      .sort(orders[query.sort ?? 'newest'])
    const offset = Number(query.offset), limit = Number(query.limit)
    const items = matching.slice(offset, offset + limit).map(item => ({ ...listed(item), contestId }))
    return route.fulfill({ json: { items, hasMore: offset + limit < matching.length, total: matching.length } })
  })
  return state
}
// The server renders the page from the fixture API. Once the browser has the viewer, the page reads the contest again,
// and the mocked one comes in.
async function openContest(page: Page, query: string, title = staffContest.title) {
  await page.goto(`${contestPath}?${query}`)
  await expect(page.locator('.site-header')).toContainText(title)
}
const contestList = (page: Page) => page.locator('#submissions')
const ownList = (page: Page) => page.locator('#my-submissions')
const conditionChips = (page: Page) => page.getByRole('list', { name: '効いている条件' })
const listCount = (list: Locator) => list.locator('.list-count')
const pageLink = (list: Locator, number: number) => list.getByRole('navigation', { name: 'ページ' }).last().getByRole('link', { name: String(number), exact: true })
const noOverflow = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

test('the contest list narrows and orders by the panel', async ({ page }) => {
  // 130 runs, each of its own length: one in three is B's, and every other one WA.
  const runs: ContestRun[] = Array.from({ length: 130 }, (_, i) => ({ ...run(i, `user${i % 7}`, 'cpp17', 100 + i, (i * 37) % 101, i % 2 ? 'WA' : 'AC'), problemId: i % 3 ? problem : problemB, onsite: false }))
  const { requests } = await mockContest(page, runs)
  await openContest(page, 'view=submissions&page=3')
  const list = contestList(page)
  await expect(listCount(list)).toHaveText('全 130 件中 101〜130 件目')
  const toggle = conditionsButton(page)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()
  const problemField = page.getByRole('combobox', { name: '問題', exact: true })
  // The problems are named as the contest names them.
  await expect(problemField.locator('option')).toHaveText(['すべて', 'A - A + B', 'B - A × B'])
  requests.length = 0
  await problemField.selectOption({ label: 'A - A + B' })
  // Chosen on the third page, a condition lists from the first, in one request that asks for the problem by its ID. The
  // count and the page numbers follow what the conditions leave.
  await expect(page).toHaveURL(`${contestPath}?view=submissions&problem=A`)
  await expect(listCount(list)).toHaveText('全 86 件中 1〜50 件目')
  expect(requests).toEqual([{ offset: '0', limit: '50', problem }])
  await expect(pageLink(list, 2)).toBeVisible()
  await expect(pageLink(list, 3)).toHaveCount(0)
  await pageLink(list, 2).click()
  await expect(page).toHaveURL(`${contestPath}?view=submissions&problem=A&page=2`)
  await expect(listCount(list)).toHaveText('全 86 件中 51〜86 件目')
  requests.length = 0
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'WA' })
  await expect(page).toHaveURL(`${contestPath}?view=submissions&verdict=WA&problem=A`)
  await expect(listCount(list)).toHaveText('全 43 件中 1〜43 件目')
  expect(requests).toEqual([{ offset: '0', limit: '50', problem, verdict: 'WA' }])
  // One page holds what is left, so there are no page numbers.
  await expect(list.getByRole('navigation', { name: 'ページ' })).toHaveCount(0)
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '実行時間が短い順' })
  await expect(page).toHaveURL(`${contestPath}?view=submissions&sort=fastest&verdict=WA&problem=A`)
  await expect.poll(() => requests.at(-1)).toEqual({ offset: '0', limit: '50', problem, verdict: 'WA', sort: 'fastest' })
  const left = runs.filter(item => item.problemId === problem && item.verdict === 'WA').sort(orders.fastest)
  await expect(sizes(list)).toHaveText(left.map(item => `${item.sourceBytes} bytes`))
  await expect(list.locator('tbody .submission-problem')).toHaveText(left.map(() => 'A - A + B'))
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  // Closed, the panel names the conditions beside its button; the problem goes by its letter.
  await toggle.click()
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['実行時間が短い順', '結果: WA', '問題: A'])
  await expect(toggle).toContainText('3')
  // The changes replaced the list's entry in the history; only the page number added one.
  await page.goBack()
  await expect(page).toHaveURL(`${contestPath}?view=submissions&problem=A`)
})

test('an old link with a problem still opens narrowed', async ({ page }) => {
  const { requests } = await mockContest(page, contestRuns())
  await openContest(page, 'view=submissions&problem=A')
  const list = contestList(page)
  // A's runs, newest first.
  await expect(sizes(list)).toHaveText(['200 bytes', '400 bytes', '80 bytes', '300 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', problem })
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['問題: A'])
  const toggle = conditionsButton(page)
  await expect(toggle).toContainText('1')
  await toggle.click()
  const problemField = page.getByRole('combobox', { name: '問題', exact: true })
  await expect(problemField).toHaveValue('A')
  await expect(problemField.locator('option:checked')).toHaveText('A - A + B')
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL(`${contestPath}?view=submissions`)
  await expect(sizes(list)).toHaveCount(8)
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50' })
  await expect(problemField).toHaveValue('')
  // The older link of the problem's own list lands on the same list (ADR 0034).
  await visit(page, `${contestPath}/A?view=submissions`)
  await expect(page).toHaveURL(`${contestPath}?view=submissions&problem=A`)
  await expect(sizes(list)).toHaveCount(4)
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', problem })
  await expect(problemField).toHaveValue('A')
  // A letter the contest does not name narrows nothing, so it is neither shown nor sent.
  await toggle.click()
  for (const letter of ['C', 'a']) {
    await visit(page, `${contestPath}?view=submissions&problem=${letter}`)
    await expect(conditionChips(page)).toHaveCount(0)
    await expect(sizes(list)).toHaveCount(8)
    expect(requests.at(-1)).toEqual({ offset: '0', limit: '50' })
    await expect(toggle).not.toContainText(/\d/)
  }
})

test('the onsite switch stays outside the panel', async ({ page }) => {
  const runs = contestRuns()
  const { requests } = await mockContest(page, runs)
  await openContest(page, 'view=submissions')
  const list = contestList(page)
  await expect(sizes(list)).toHaveCount(8)
  const toggle = conditionsButton(page)
  const onsite = page.getByRole('switch', { name: 'オンサイトのみ表示' })
  // The switch sits beside the closed panel's button. On, it is no chip and adds nothing to the count.
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(onsite).toBeVisible()
  await onsite.check()
  await expect(page).toHaveURL(`${contestPath}?view=submissions&scope=onsite`)
  // Bob's runs, newest first.
  await expect(sizes(list)).toHaveText(['400 bytes', '90 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', scope: 'onsite' })
  await expect(conditionChips(page)).toHaveCount(0)
  await expect(toggle).not.toContainText(/\d/)
  await toggle.click()
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'WA' })
  await expect(page).toHaveURL(`${contestPath}?view=submissions&scope=onsite&verdict=WA`)
  await expect(sizes(list)).toHaveText(['90 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', scope: 'onsite', verdict: 'WA' })
  await expect(toggle).toContainText('1')
  // Taking every condition off leaves the switch on.
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL(`${contestPath}?view=submissions&scope=onsite`)
  await expect(sizes(list)).toHaveText(['400 bytes', '90 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', scope: 'onsite' })
  await expect(onsite).toBeChecked()
  // No run matches the condition: the list says so and offers a way out, which leaves the switch on too and puts the
  // focus on the panel's button.
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'TLE' })
  await toggle.click()
  await expect(list.getByText('条件に合う提出はありません。')).toBeVisible()
  await expect(list.getByText('提出はまだありません。')).toHaveCount(0)
  await list.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL(`${contestPath}?view=submissions&scope=onsite`)
  await expect(toggle).toBeFocused()
  await expect(sizes(list)).toHaveCount(2)
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', scope: 'onsite' })
  // Nobody on site has submitted: with the switch alone, the list says what it always has.
  for (const item of runs) item.onsite = false
  await onsite.uncheck()
  await expect(sizes(list)).toHaveCount(8)
  await onsite.check()
  await expect(list.getByText('提出はまだありません。')).toBeVisible()
  await expect(list.getByText('条件に合う提出はありません。')).toHaveCount(0)
  await expect(list.getByRole('button', { name: '条件をすべて外す' })).toHaveCount(0)
})

test('own and team lists have no user field', async ({ page }) => {
  const state = await mockContest(page, contestRuns())
  const { requests } = state
  await openContest(page, 'view=my-submissions&user=alice&verdict=AC')
  // ui_test's accepted run; the submitter in the URL is neither shown nor sent.
  await expect(sizes(ownList(page))).toHaveText(['200 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', mine: '1', verdict: 'AC' })
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['結果: AC'])
  const toggle = conditionsButton(page)
  await expect(toggle).toContainText('1')
  await toggle.click()
  await expect(page.getByRole('combobox', { name: '結果' })).toHaveValue('AC')
  await expect(page.getByRole('textbox', { name: '提出者' })).toHaveCount(0)
  // A change there leaves it out of the URL.
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
  await expect(page).toHaveURL(`${contestPath}?view=my-submissions&sort=shortest&verdict=AC`)
  // The menu's two lists keep the conditions, the submitter only on the whole list; the other views start without them.
  const kept = 'sort=shortest&verdict=AC&runtime=cpp23-gcc'
  await visit(page, `${contestPath}?view=submissions&${kept}&user=alice&problem=A`)
  await expect(sizes(contestList(page))).toHaveText(['300 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', problem, sort: 'shortest', verdict: 'AC', runtime: 'cpp23-gcc', user: 'alice' })
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await menu.getByText('提出', { exact: true }).click()
  await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=my-submissions&${kept}&problem=A`)
  await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=submissions&${kept}&user=alice&problem=A`)
  for (const [name, href] of [['概要', contestPath], ['問題', `${contestPath}?view=problems`], ['順位表', `${contestPath}?view=standings`], ['解説', `${contestPath}?view=editorials`], ['質問', `${contestPath}?view=clarifications`], ['コード実行', `${contestPath}?view=run`]] as const) {
    await expect(menu.getByRole('link', { name, exact: true })).toHaveAttribute('href', href)
  }
  await menu.getByRole('link', { name: '自分の提出', exact: true }).click()
  await expect(page).toHaveURL(`${contestPath}?view=my-submissions&${kept}&problem=A`)
  await expect(sizes(ownList(page))).toHaveText(['200 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', mine: '1', problem, sort: 'shortest', verdict: 'AC', runtime: 'cpp23-gcc' })
  // Another list starts with the panel closed.
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['コード長が短い順', '結果: AC', '言語: C++23 (GCC)', '問題: A'])
  // From the own list the whole list has no submitter to keep.
  await menu.getByText('提出', { exact: true }).click()
  await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=submissions&${kept}&problem=A`)
  await menu.getByRole('link', { name: 'すべての提出', exact: true }).click()
  await expect(page).toHaveURL(`${contestPath}?view=submissions&${kept}&problem=A`)
  await expect(sizes(contestList(page))).toHaveText(['200 bytes', '300 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', problem, sort: 'shortest', verdict: 'AC', runtime: 'cpp23-gcc' })
  // A whole language is kept as well.
  await visit(page, `${contestPath}?view=submissions&language=cpp`)
  await menu.getByText('提出', { exact: true }).click()
  await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=my-submissions&language=cpp`)
  // While a team contest runs, a member's own list is the team's, and it has no submitter to choose either.
  state.contest = { ...staffContest, title: 'チーム戦の絞り込みテスト', staff: false, participating: true, teamMode: true, team: { id: 'aaaaaaaa-1111-4111-8111-000000000001', name: 'Alpha', members: ['ui_test', 'bob'], onsite: false } }
  await openContest(page, 'view=my-submissions&user=alice&verdict=WA&language=cpp', state.contest.title)
  await expect(sizes(ownList(page))).toHaveText(['90 bytes'])
  expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', mine: 'team', verdict: 'WA', language: 'cpp' })
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['結果: WA', '言語: C++'])
  await toggle.click()
  await expect(page.getByRole('combobox', { name: '言語' })).toHaveValue('language:cpp')
  await expect(page.getByRole('textbox', { name: '提出者' })).toHaveCount(0)
  // The menu offers the team's list alone, with the conditions and without the submitter.
  await expect(menu.getByRole('link', { name: 'チームの提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=my-submissions&verdict=WA&language=cpp`)
})

// The table is busy while the page reads the contest or the list again. The clock stands still between steps, and a poll
// sets its next turn only once it is answered, so each step waits for the table to settle and counts the list's requests
// it brought.
async function contestStep(page: Page, requests: unknown[], seconds = 3) {
  const table = contestList(page).getByRole('region', { name: '提出一覧のスクロール領域' })
  await expect(table).toHaveAttribute('aria-busy', 'false')
  const before = requests.length
  await page.clock.runFor(seconds * 1000)
  await expect(table).toHaveAttribute('aria-busy', 'false')
  return requests.length - before
}

test('refreshing keeps the panel, the page and the selection', async ({ page }) => {
  // 60 of A's runs are WA, two pages of them; B's runs and A's accepted ones are left out.
  const runs: ContestRun[] = Array.from({ length: 120 }, (_, i) => ({ ...run(i, `user${i % 5}`, 'cpp17', 100 + i, i, i < 90 ? 'WA' : 'AC'), problemId: i < 60 || i >= 90 ? problem : problemB, onsite: false }))
  // ui_test may rejudge; a rejudged run is judged until the test says otherwise.
  await page.route('**/api/my/problems/*/rejudge', route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { pending: 0 } })
    const { submissionIds } = route.request().postDataJSON() as { submissionIds: string[] }
    for (const item of runs) if (submissionIds.includes(item.id)) item.status = 'RUNNING'
    return route.fulfill({ status: 202, json: { queued: submissionIds.length } })
  })
  const { requests } = await mockContest(page, runs)
  await page.clock.install()
  await openContest(page, 'view=submissions')
  const list = contestList(page)
  const toggle = conditionsButton(page)
  await toggle.click()
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'WA' })
  await page.getByRole('combobox', { name: '問題', exact: true }).selectOption({ label: 'A - A + B' })
  await expect(page).toHaveURL(`${contestPath}?view=submissions&verdict=WA&problem=A`)
  await pageLink(list, 2).click()
  const second = `${contestPath}?view=submissions&verdict=WA&problem=A&page=2`
  await expect(page).toHaveURL(second)
  // With a problem chosen, the rows can be chosen for a rejudge.
  const rows = list.getByRole('checkbox', { name: /の提出を選択$/ })
  await expect(rows).toHaveCount(10)
  await rows.nth(1).check()
  await rows.nth(2).check()
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  // The regular refresh reads the list with its conditions and its page, and leaves the panel, the page and the chosen
  // rows as they are.
  expect(await contestStep(page, requests, 15)).toBe(1)
  expect(requests.at(-1)).toEqual({ offset: '50', limit: '50', problem, verdict: 'WA' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByRole('combobox', { name: '結果' })).toHaveValue('WA')
  await expect(page).toHaveURL(second)
  await expect(listCount(list)).toHaveText('全 60 件中 51〜60 件目')
  await expect(rows).toHaveCount(10)
  await expect(rows.nth(1)).toBeChecked()
  await expect(rows.nth(2)).toBeChecked()
  page.once('dialog', dialog => void dialog.accept())
  await list.getByRole('button', { name: '選択した2件の提出をリジャッジ', exact: true }).click()
  // The rejudged runs are being judged, so the list of WA no longer holds them, and the message still shows.
  await expect(list.getByRole('status').filter({ hasText: '2件の提出をリジャッジしています。' })).toBeVisible()
  await expect(rows).toHaveCount(8)
  // The list follows them every 3 seconds though none is in view...
  for (let i = 0; i < 3; i++) expect(await contestStep(page, requests)).toBeGreaterThanOrEqual(1)
  expect(requests.at(-1)).toEqual({ offset: '50', limit: '50', problem, verdict: 'WA' })
  // ...until they are back and judged. Then only the regular refresh is left, at most once in 9 seconds.
  for (const item of runs) if (item.status === 'RUNNING') item.status = 'DONE'
  expect(await contestStep(page, requests)).toBeGreaterThanOrEqual(1)
  await expect(rows).toHaveCount(10)
  let later = 0
  for (let i = 0; i < 3; i++) later += await contestStep(page, requests)
  expect(later).toBeLessThanOrEqual(1)
  // Runs that never come back are followed for a minute after the rejudge, and no longer.
  await rows.nth(1).check()
  await rows.nth(2).check()
  page.once('dialog', dialog => void dialog.accept())
  await list.getByRole('button', { name: '選択した2件の提出をリジャッジ', exact: true }).click()
  await expect(rows).toHaveCount(8)
  for (let i = 0; i < 19; i++) expect(await contestStep(page, requests)).toBeGreaterThanOrEqual(1)
  await contestStep(page, requests)
  await contestStep(page, requests)
  later = 0
  for (let i = 0; i < 3; i++) later += await contestStep(page, requests)
  expect(later).toBeLessThanOrEqual(1)
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(page).toHaveURL(second)
})

for (const [mode, nextTask] of quickModes) {
  test(`two changes on the contest page ${mode} both take effect`, async ({ page }) => {
    const { requests } = await mockContest(page, contestRuns())
    await openContest(page, 'view=submissions')
    const list = contestList(page)
    await expect(sizes(list)).toHaveCount(8)
    const toggle = conditionsButton(page)
    await toggle.click()
    const problemField = page.getByRole('combobox', { name: '問題', exact: true })
    const verdict = page.getByRole('combobox', { name: '結果' })
    requests.length = 0
    await quickly(page, [{ target: problemField, pick: 'B' }, { target: verdict, pick: 'WA' }], { nextTask })
    await expect(page).toHaveURL(`${contestPath}?view=submissions&verdict=WA&problem=B`)
    await expect(problemField).toHaveValue('B')
    await expect(verdict).toHaveValue('WA')
    // The list on screen is the one in the URL: B's wrong answers.
    await expect(sizes(list)).toHaveText(['60 bytes', '150 bytes', '90 bytes'])
    await expect.poll(() => requests.at(-1)).toEqual({ offset: '0', limit: '50', problem: problemB, verdict: 'WA' })
    expect(requests.length).toBeLessThanOrEqual(2)
    await toggle.click()
    await expect(conditionChips(page).getByRole('listitem')).toHaveText(['結果: WA', '問題: B'])
  })

  test(`a condition and a page number taken ${mode} leave the URL and the list in step`, async ({ page }) => {
    // Every other run is WA: 60 of them take two pages, and all 120 three.
    const runs: ContestRun[] = Array.from({ length: 120 }, (_, i) => ({ ...run(i, 'alice', 'cpp17', 100 + i, i, i % 2 ? 'WA' : 'AC'), problemId: problem, onsite: false }))
    const { requests } = await mockContest(page, runs)
    const narrowed = `${contestPath}?view=submissions&verdict=WA`
    await openContest(page, 'view=submissions&verdict=WA')
    const list = contestList(page)
    await expect(listCount(list)).toHaveText('全 60 件中 1〜50 件目')
    const chip = conditionChips(page).getByRole('button', { name: '結果: WAを外す' })
    // The chip, then the page: the page's link came with the condition, and its move is the later one. In the next task
    // the chip's move may have ended first, and the link without the condition is followed instead.
    await quickly(page, [{ target: chip }, { target: pageLink(list, 2) }], { nextTask })
    await expect(page).toHaveURL(/[?&]page=2$/)
    const kept = new URL(page.url()).searchParams.has('verdict')
    if (!nextTask) expect(kept).toBe(true)
    await expect(page).toHaveURL(kept ? `${narrowed}&page=2` : `${contestPath}?view=submissions&page=2`)
    await expect(listCount(list)).toHaveText(kept ? '全 60 件中 51〜60 件目' : '全 120 件中 51〜100 件目')
    expect(requests.at(-1)).toEqual({ offset: '50', limit: '50', ...kept ? { verdict: 'WA' } : {} })
    await expect(conditionChips(page).getByRole('listitem')).toHaveText(kept ? ['結果: WA'] : [])
    await expect(list.getByRole('navigation', { name: 'ページ' }).last().locator('[aria-current="page"]')).toHaveText('2')
    // The page, then the chip: the list starts over without the condition.
    await visit(page, narrowed)
    await expect(listCount(list)).toHaveText('全 60 件中 1〜50 件目')
    await quickly(page, [{ target: pageLink(list, 2) }, { target: chip }], { nextTask })
    await expect(page).toHaveURL(`${contestPath}?view=submissions`)
    await expect(listCount(list)).toHaveText('全 120 件中 1〜50 件目')
    expect(requests.at(-1)).toEqual({ offset: '0', limit: '50' })
    await expect(conditionChips(page)).toHaveCount(0)
    await expect(list.getByRole('navigation', { name: 'ページ' }).last().locator('[aria-current="page"]')).toHaveText('1')
  })
}

for (const width of [320, 375, 1280]) {
  test(`a long problem title stays inside the panel at ${width}px`, async ({ page }) => {
    const title = 'Massive Excessive Impressive Cat Food and Other Very Long Words in One Problem Title 長い問題タイトル'
    const state = await mockContest(page, contestRuns())
    state.contest = { ...staffContest, title: '長い問題名の絞り込みテスト', problems: [{ id: problem, title, points: 100 }, { id: problemB, title: 'A × B', points: 200 }] }
    await page.setViewportSize({ width, height: 812 })
    await openContest(page, 'view=submissions&problem=A', '長い問題名の絞り込みテスト')
    await conditionsButton(page).click()
    const select = page.getByRole('combobox', { name: '問題', exact: true })
    await expect(select).toHaveValue('A')
    await expect(select.locator('option:checked')).toHaveText(`A - ${title}`)
    expect(await outsidePanel(page)).toEqual([])
    // The fields keep to their own columns as well: no control reaches over the next one.
    const boxes = await page.locator('.conditions-panel').locator('select, input').evaluateAll(controls => controls.map(control => { const box = control.getBoundingClientRect(); return { left: box.left, right: box.right, top: box.top, bottom: box.bottom } }))
    for (const [index, one] of boxes.entries()) for (const other of boxes.slice(index + 1)) {
      const apart = one.right <= other.left + 0.5 || other.right <= one.left + 0.5 || one.bottom <= other.top + 0.5 || other.bottom <= one.top + 0.5
      expect(apart, JSON.stringify([one, other])).toBe(true)
    }
  })
}

test('the conditions fit a phone', async ({ page }) => {
  await mockContest(page, contestRuns())
  await page.setViewportSize({ width: 375, height: 812 })
  await openContest(page, 'view=submissions&scope=onsite&sort=shortest&verdict=AC&runtime=rust2024&problem=A')
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['コード長が短い順', '結果: AC', '言語: Rust (Edition 2024)', '問題: A'])
  await expect(page.getByRole('switch', { name: 'オンサイトのみ表示' })).toBeChecked()
  await expect(sizes(contestList(page))).toHaveText(['400 bytes'])
  expect(await noOverflow(page)).toBe(true)
  await conditionsButton(page).click()
  await expect(page.getByRole('combobox', { name: '問題', exact: true })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '提出者' })).toBeVisible()
  expect(await outsidePanel(page)).toEqual([])
  expect(await noOverflow(page)).toBe(true)
})

// As on the problem page, a language or a runtime the catalogue does not list reads as すべて.
for (const unknown of unknownLanguages) {
  test(`the contest page reads ${unknown} as every language`, async ({ page }) => {
    const { requests } = await mockContest(page, contestRuns())
    await page.clock.install()
    const opened = `view=submissions&verdict=AC&${unknown}`
    await openContest(page, opened)
    const list = contestList(page)
    // The accepted runs, newest first, asked for without the language.
    await expect(sizes(list)).toHaveText(['200 bytes', '400 bytes', '80 bytes', '120 bytes', '300 bytes'])
    await expect(list.getByText('提出一覧を取得できませんでした。')).toHaveCount(0)
    await expect(conditionChips(page).getByRole('listitem')).toHaveText(['結果: AC'])
    const toggle = conditionsButton(page)
    await expect(toggle).toContainText('1')
    // The menu's two lists keep the other conditions alone.
    const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
    await menu.getByText('提出', { exact: true }).click()
    await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=my-submissions&verdict=AC`)
    await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=submissions&verdict=AC`)
    await toggle.click()
    const language = page.getByRole('combobox', { name: '言語' })
    await expect(language).toHaveValue('')
    await expect(language.locator('option:checked')).toHaveText('すべて')
    // The regular refresh asks with the same conditions, also after the catalogue has been read again (every 30
    // seconds).
    await page.clock.pauseAt(new Date(Date.now() + 1000))
    for (let i = 0; i < 2; i++) expect(await contestStep(page, requests, 15)).toBe(1)
    expect(requests).toEqual(Array(requests.length).fill({ offset: '0', limit: '50', verdict: 'AC' }))
    // The URL keeps the value until the conditions change; a change in the panel leaves it out.
    await expect(page).toHaveURL(`${contestPath}?${opened}`)
    await page.clock.resume()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
    await expect(page).toHaveURL(`${contestPath}?view=submissions&sort=shortest&verdict=AC`)
    await expect(sizes(list)).toHaveText(['80 bytes', '120 bytes', '200 bytes', '300 bytes', '400 bytes'])
    expect(requests.at(-1)).toEqual({ offset: '0', limit: '50', sort: 'shortest', verdict: 'AC' })
    expect(requests.filter(refused)).toEqual([])
  })
}

test('the contest own and team lists and the empty list read an unknown language as every language', async ({ page }) => {
  const runs = contestRuns()
  const state = await mockContest(page, runs)
  await openContest(page, 'view=my-submissions&verdict=AC&language=zzz')
  // ui_test's accepted run.
  await expect(sizes(ownList(page))).toHaveText(['200 bytes'])
  await expect.poll(() => state.requests.at(-1)).toEqual({ offset: '0', limit: '50', mine: '1', verdict: 'AC' })
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['結果: AC'])
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await menu.getByText('提出', { exact: true }).click()
  await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=submissions&verdict=AC`)
  // While a team contest runs, a member's own list is the team's.
  state.contest = { ...staffContest, title: 'チーム戦の絞り込みテスト', staff: false, participating: true, teamMode: true, team: { id: 'aaaaaaaa-1111-4111-8111-000000000001', name: 'Alpha', members: ['ui_test', 'bob'], onsite: false } }
  await openContest(page, 'view=my-submissions&verdict=WA&runtime=cpp', state.contest.title)
  // The team's wrong answers, newest first.
  await expect(sizes(ownList(page))).toHaveText(['60 bytes', '90 bytes'])
  await expect.poll(() => state.requests.at(-1)).toEqual({ offset: '0', limit: '50', mine: 'team', verdict: 'WA' })
  await expect(conditionChips(page).getByRole('listitem')).toHaveText(['結果: WA'])
  await expect(menu.getByRole('link', { name: 'チームの提出', exact: true })).toHaveAttribute('href', `${contestPath}?view=my-submissions&verdict=WA`)
  // Without a run the list says it has none: the value is no condition that could have emptied it.
  runs.length = 0
  state.contest = staffContest
  await openContest(page, 'view=submissions&runtime=cobol85')
  await expect(contestList(page).getByText('提出はまだありません。')).toBeVisible()
  await expect(contestList(page).getByText('条件に合う提出はありません。')).toHaveCount(0)
  await expect(conditionsButton(page)).not.toContainText(/\d/)
  expect(state.requests.filter(refused)).toEqual([])
})

// The page is rendered with the catalogue, so its first request for the list already has the language it lists, or
// leaves out one it does not, and the browser's own read of the catalogue right after does not ask again.
for (const [query, sent] of [['runtime=cpp17', { runtime: 'cpp17' }], ['language=zzz', {}], ['', {}]] as const) {
  test(`a load of the contest page asks for the list one way ${query ? `with ${query}` : 'without a language'}`, async ({ page }) => {
    const { requests } = await mockContest(page, contestRuns())
    await page.clock.install()
    await Promise.all([page.waitForResponse('**/api/runtimes'), openContest(page, `view=submissions${query ? `&${query}` : ''}`)])
    await expect(sizes(contestList(page))).toHaveCount(query === 'runtime=cpp17' ? 2 : 8)
    await page.clock.pauseAt(new Date(Date.now() + 1000))
    const loaded = requests.length
    expect(await contestStep(page, requests, 15)).toBe(1)
    expect(requests).toEqual(Array(loaded + 1).fill({ offset: '0', limit: '50', ...sent }))
  })
}
