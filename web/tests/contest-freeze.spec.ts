import type { Page, Route } from '@playwright/test'
import { expect, test } from './fixtures/account'
import type { Contest, Standing } from '../shared/types/contest'
import { shortId } from '../shared/utils/short-id'

// The fixture API serves this contest to the server-rendered page; the routes below replace it once the page refreshes.
const id = '88888888-8888-4888-8888-888888888888'
const contest: Contest = {
  id, title: '凍結テスト', author: 'alice', problemAuthors: [], testers: [], description: '',
  // 21:00–23:00 in Japan, freezing at 22:00.
  startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5, version: 1,
  status: 'ended', participating: false, canEdit: false, official: true, canViewSubmissions: true,
  freezeMinutes: 60, freezeAt: '2026-09-15T13:00:00Z', revealed: false, revealedAt: null, resultsFinal: false,
  canViewLiveStandings: false, canRevealResults: false,
  problems: ['a', 'b'].map(id => ({ id, title: id, points: 100 })),
}
const frozenRows: Standing[] = [
  { rank: 1, handle: 'alice', points: 100, timeMs: 600000, problems: {
    a: { points: 100, wrong: 0, pending: 0, frozen: 0, acceptedAt: '2026-09-15T12:10:00Z' },
    b: { points: 0, wrong: 1, pending: 0, frozen: 2 },
  } },
  { rank: 2, handle: 'bob', points: 0, timeMs: 0, problems: {
    a: { points: 0, wrong: 0, pending: 1, frozen: 1 },
  } },
  { rank: 2, handle: 'carol', points: 0, timeMs: 0, problems: {
    a: { points: 0, wrong: 0, pending: 0, frozen: 3 },
  } },
]
const liveRows: Standing[] = [
  { rank: 1, handle: 'carol', points: 100, timeMs: 3900000, problems: { a: { points: 100, wrong: 2, pending: 0, frozen: 0, acceptedAt: '2026-09-15T13:05:00Z' } } },
]

// Serves the contest and its standings; live is set when the page asked for the table without the freeze. While hold
// is set, the standings wait for it.
async function routeContest(page: Page, state: { contest: Contest, rows: (live: boolean) => Standing[], hold?: Promise<void> }) {
  const standings: (string | null)[] = []
  await page.route(`**/api/contests/${id}**`, async route => {
    const url = new URL(route.request().url())
    if (!url.pathname.endsWith('/standings')) return route.fulfill({ json: url.pathname.endsWith(id) ? state.contest : { items: [], hasMore: false } })
    standings.push(url.searchParams.get('live'))
    const live = url.searchParams.get('live') === '1'
    await state.hold
    return route.fulfill({ json: state.rows(live) })
  })
  return standings
}
async function openStandings(page: Page) {
  await page.goto(`/contests/${shortId(id)}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.site-header')).toContainText('凍結テスト')
}
const notice = (page: Page) => page.locator('#standings .notice:not(.notice-error)')

test('the overview announces the freeze and the standings say since when results are hidden', async ({ page }) => {
  const state = { contest: { ...contest, status: 'scheduled' as const, freezeMinutes: 30, freezeAt: '2099-10-20T13:30:00Z', startsAt: '2099-10-20T12:00:00Z', endsAt: '2099-10-20T14:00:00Z' }, rows: () => [] as Standing[] }
  await routeContest(page, state)
  await page.goto(`/contests/${shortId(id)}`, { waitUntil: 'networkidle' })
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '順位表', exact: true }).click()
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.site-header')).toContainText('凍結テスト')
  // Before the freeze starts nothing is hidden yet.
  await expect(notice(page)).toHaveCount(0)
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true }).click()
  await expect(page.getByText('終了30分前から順位表を凍結します。', { exact: true })).toBeVisible()
  // A contest within one day in Japan names the time alone.
  state.contest = { ...contest, status: 'running' }
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '順位表', exact: true }).click()
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(notice(page)).toHaveText('22:00以降の提出の結果は、結果発表まで伏せています。')
  // Across midnight in Japan (21:00–01:00) the time alone could mean either day.
  state.contest = { ...contest, endsAt: '2026-09-15T16:00:00Z', freezeMinutes: 90, freezeAt: '2026-09-15T14:30:00Z' }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(notice(page)).toHaveText('9月15日 23:30以降の提出の結果は、結果発表まで伏せています。')
  // Once the results are published nothing is hidden, and a contest without a freeze never mentions one.
  state.contest = { ...contest, revealed: true, revealedAt: '2026-09-15T15:00:00Z', resultsFinal: true }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(notice(page)).toHaveCount(0)
  state.contest = { ...contest, freezeMinutes: null, freezeAt: null, resultsFinal: true }
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true }).click()
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '順位表', exact: true }).click()
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(notice(page)).toHaveCount(0)
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true }).click()
  await expect(page.locator('.contest-scoring')).toHaveText(['誤答ペナルティ 5 分 · 部分点なし'])
})

test('frozen submissions show as ?n beside, not instead of, the pending hourglass', async ({ page }) => {
  await routeContest(page, { contest, rows: () => frozenRows })
  await openStandings(page)
  const table = page.locator('.standings-table')
  const cell = (handle: string, index: number) => table.locator('tbody tr').filter({ hasText: handle }).locator('td').nth(3 + index)
  // An accepted problem stays as it was; the frozen count sits after the wrong answers.
  await expect(cell('alice', 0)).toHaveText('10010:00')
  // Screen readers hear the counts as words; the marks themselves are hidden from them.
  await expect(cell('alice', 1).locator('[aria-hidden="true"]')).toHaveText(['(1)', '?2'])
  await expect(cell('alice', 1).locator('.visually-hidden')).toHaveText(['誤答 1 回', '凍結後の提出 2 件'])
  // A pending run before the freeze keeps its hourglass; the frozen one is counted apart.
  await expect(cell('bob', 0).getByRole('img', { name: '判定待ち' })).toBeVisible()
  await expect(cell('bob', 0).locator('.standing-frozen')).toHaveText('?1凍結後の提出 1 件')
  // Frozen submissions alone replace the dash.
  await expect(cell('carol', 0)).toHaveText('?3凍結後の提出 3 件')
  await expect(table.getByRole('img', { name: '判定待ち' })).toHaveCount(1)
  await expect(table.locator('.standing-frozen')).toHaveCount(3)
  await expect(notice(page)).toHaveText('22:00以降の提出の結果は、結果発表まで伏せています。')
})

test('staff switch to the standings without the freeze only when they choose it', async ({ page }) => {
  const state: { contest: Contest, rows: (live: boolean) => Standing[], hold?: Promise<void> } = { contest: { ...contest, status: 'running' as const, staff: true, setter: true }, rows: live => live ? liveRows : frozenRows }
  const requests = await routeContest(page, state)
  await openStandings(page)
  // Participants and strangers get no choice, and staff get none while the freeze hides nothing.
  await expect(page.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  state.contest = { ...state.contest, canViewLiveStandings: true, freezeAt: '2099-09-15T13:00:00Z' }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(3)
  await expect(page.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  state.contest = { ...state.contest, freezeAt: contest.freezeAt }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  const modes = page.getByRole('group', { name: '順位表の表示' })
  await expect(modes.getByRole('radio', { name: '凍結した順位表' })).toBeChecked()
  await expect(modes.getByRole('radio', { name: '凍結なし' })).not.toBeChecked()
  await expect(notice(page).last()).toHaveText('22:00以降の提出の結果は、結果発表まで伏せています。')
  expect(requests.length).toBeGreaterThan(0)
  expect(requests.every(live => live === null)).toBe(true)
  await modes.getByRole('radio', { name: '凍結なし' }).check()
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(1)
  await expect(page.locator('.standings-table tbody tr')).toContainText('carol')
  expect(requests.at(-1)).toBe('1')
  await expect(notice(page).last()).toHaveText('凍結なしの順位表を表示しています。スタッフだけが見られます。')
  // Refreshing keeps the chosen table.
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(1)
  expect(requests.at(-1)).toBe('1')
  // Until the frozen table arrives the live rows give way to a spinner, so they never sit under the frozen notice.
  let release = () => {}
  state.hold = new Promise(resolve => { release = resolve })
  await modes.getByRole('radio', { name: '凍結した順位表' }).check()
  await expect(page.locator('#standings .loading-spinner')).toHaveText('順位表を読み込み中')
  await expect(page.locator('.standings-table')).toHaveCount(0)
  await expect(page.getByText('参加者はまだいません。')).toHaveCount(0)
  await expect(notice(page).last()).toHaveText('22:00以降の提出の結果は、結果発表まで伏せています。')
  state.hold = undefined
  release()
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(3)
  await expect(page.locator('#standings .loading-spinner')).toHaveCount(0)
  expect(requests.at(-1)).toBeNull()
  await expect(notice(page).last()).toHaveText('22:00以降の提出の結果は、結果発表まで伏せています。')
  // Once the results are published both tables match, so the choice goes away even if live was chosen.
  await modes.getByRole('radio', { name: '凍結なし' }).check()
  await expect.poll(() => requests.at(-1)).toBe('1')
  state.contest = { ...state.contest, status: 'ended', revealed: true, revealedAt: '2026-09-15T15:00:00Z', resultsFinal: true }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  await expect(notice(page)).toHaveText(['スタッフ（作成者・作問者・テスター）としての提出は公式順位の対象外です。'])
  await expect.poll(() => requests.at(-1)).toBeNull()
})

test('setters publish the results after confirming and learn why pending runs block it', async ({ page }) => {
  const state = { contest: { ...contest, setter: true, staff: true, canViewLiveStandings: true, canRevealResults: true }, rows: () => frozenRows }
  await routeContest(page, state)
  const reveals: string[] = []
  let refusal: string | undefined = 'contest_results_pending'
  let hold: Promise<void> | undefined
  await page.route(`**/api/my/contests/${id}/reveal`, async (route: Route) => {
    reveals.push(route.request().method())
    await hold
    if (refusal) return route.fulfill({ status: 409, json: { statusCode: 409, data: { code: refusal } } })
    state.contest = { ...state.contest, revealed: true, revealedAt: '2026-09-15T15:00:00Z', resultsFinal: true, canRevealResults: false }
    state.rows = () => liveRows
    return route.fulfill({ json: state.contest })
  })
  await openStandings(page)
  const reveal = page.getByRole('button', { name: '結果を公開', exact: true })
  let question = ''
  page.once('dialog', dialog => { question = dialog.message(); void dialog.dismiss() })
  await reveal.click()
  expect(question).toBe('結果を公開します。凍結後の提出の結果が順位表に反映され、元に戻せません。よろしいですか？')
  expect(reveals).toEqual([])
  page.once('dialog', dialog => void dialog.accept())
  await reveal.click()
  await expect(page.getByRole('alert')).toHaveText('判定待ちの提出が残っているため、まだ結果を公開できません。判定が終わってから、もう一度お試しください。')
  await expect(reveal).toBeEnabled()
  await expect(page.locator('.standing-frozen')).toHaveCount(3)
  refusal = 'contest_conflict'
  page.once('dialog', dialog => void dialog.accept())
  await reveal.click()
  await expect(page.getByRole('alert')).toHaveText('終了前のコンテストと、順位表を凍結しないコンテストでは結果を公開できません。')
  refusal = undefined
  let release = () => {}
  hold = new Promise(resolve => { release = resolve })
  page.once('dialog', dialog => void dialog.accept())
  await reveal.click()
  const busy = page.getByRole('button', { name: '公開中…', exact: true })
  await expect(busy).toBeDisabled()
  await expect(busy).toHaveAttribute('aria-busy', 'true')
  release()
  // The page rereads the contest and the standings: the button, the notice and the frozen counts are gone.
  // It says so, and focus moves from the vanished button to the heading of the table.
  await expect(page.getByRole('status').filter({ hasText: '結果を公開しました' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '公式順位表' })).toBeFocused()
  await expect(reveal).toHaveCount(0)
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.locator('.standing-frozen')).toHaveCount(0)
  await expect(page.locator('.standings-table tbody tr')).toHaveCount(1)
  await expect(page.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  await expect(notice(page)).toHaveText(['スタッフ（作成者・作問者・テスター）としての提出は公式順位の対象外です。'])
  expect(reveals).toEqual(['POST', 'POST', 'POST'])
})

test('a refusal goes away once someone else publishes the results', async ({ page }) => {
  const state = { contest: { ...contest, setter: true, staff: true, canViewLiveStandings: true, canRevealResults: true }, rows: (_live: boolean) => frozenRows }
  await routeContest(page, state)
  await page.route(`**/api/my/contests/${id}/reveal`, route => route.fulfill({ status: 409, json: { statusCode: 409, data: { code: 'contest_results_pending' } } }))
  await openStandings(page)
  page.once('dialog', dialog => void dialog.accept())
  await page.getByRole('button', { name: '結果を公開', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('判定待ちの提出が残っているため、まだ結果を公開できません。判定が終わってから、もう一度お試しください。')
  // A co-setter or an operator publishes meanwhile; the next read drops the button and the stale refusal with it.
  state.contest = { ...state.contest, revealed: true, revealedAt: '2026-09-15T15:00:00Z', resultsFinal: true, canRevealResults: false }
  state.rows = () => liveRows
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.getByRole('button', { name: '結果を公開', exact: true })).toHaveCount(0)
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('the standings heading keeps every control on screen at each width', async ({ page }, testInfo) => {
  await routeContest(page, { contest: { ...contest, setter: true, staff: true, canViewLiveStandings: true, canRevealResults: true }, rows: () => frozenRows })
  await openStandings(page)
  const heading = page.getByRole('heading', { name: '公式順位表' })
  const controls = [heading, page.getByRole('button', { name: '結果を公開', exact: true }), page.getByRole('button', { name: '今すぐ更新' }), page.getByRole('group', { name: '順位表の表示' })]
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const boxes = await Promise.all(controls.map(control => control.boundingBox()))
    for (const box of boxes) expect(box!.x >= 0 && box!.x + box!.width <= width).toBe(true)
    // The title stays on one line, and no control covers another.
    const lineHeight = await heading.evaluate(el => parseFloat(getComputedStyle(el).lineHeight))
    expect(boxes[0]!.height).toBeLessThan(lineHeight * 1.5)
    for (const [i, a] of boxes.entries()) for (const b of boxes.slice(i + 1)) {
      expect(a!.x + a!.width <= b!.x || b!.x + b!.width <= a!.x || a!.y + a!.height <= b!.y || b!.y + b!.height <= a!.y).toBe(true)
    }
    await page.locator('#standings').screenshot({ path: testInfo.outputPath(`standings-heading-${width}.png`) })
  }
})

test('the web server asks for the live standings only for a session that chose them, and passes on the pending refusal', async ({ request }) => {
  const session = { Cookie: 'openoj_access=valid-access' }
  const handles = async (query: string, headers?: Record<string, string>) => (await (await request.get(`/api/contests/${id}/standings${query}`, { headers })).json() as Standing[]).map(row => row.handle)
  // The public route answers without a session and for the default view, and it never takes live.
  expect(await handles('?live=1')).toEqual([])
  expect(await handles('', session)).toEqual([])
  expect(await handles('?live=true', session)).toEqual([])
  // Only a session that chose the live table reaches the private route.
  expect(await handles('?live=1', session)).toEqual(['live'])
  const reveal = (path: string, headers: Record<string, string>) => request.post(`/api/my/contests/${path}/reveal`, { headers })
  expect((await reveal(id, { Origin: 'https://judge.example' })).status()).toBe(401)
  expect((await reveal(id, { ...session, Origin: 'https://attacker.example' })).status()).toBe(403)
  expect((await reveal('not-a-contest', { ...session, Origin: 'https://judge.example' })).status()).toBe(404)
  const pending = await reveal(id, { ...session, Origin: 'https://judge.example' })
  expect(pending.status()).toBe(409)
  expect((await pending.json()).data).toEqual({ code: 'contest_results_pending' })
})

const editorId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
const draftResult = (draft: Record<string, unknown>, issues: string[] = []) => ({
  id: editorId, version: 2, updatedAt: '2026-10-01T00:00:00Z', issues, authors: ['ui_test'], owner: true, problems: [],
  draft: { title: '凍結する下書き', description: '', startsAt: null, durationMinutes: 120, penaltyMinutes: 5, problems: [], ...draft },
})

test('the editor freeze switch starts off, suggests a length, and the draft saves it as a number or null', async ({ page }, testInfo) => {
  const saved: unknown[] = []
  await page.route('**/api/my/contests/*/draft', route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ status: 404, json: {} })
    const body = route.request().postDataJSON()
    saved.push(body.draft.freezeMinutes)
    return route.fulfill({ json: { ...draftResult(body.draft, ['start_missing', 'problems_missing']), version: body.version + 1 } })
  })
  await page.goto('/my/contests/new')
  await page.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
  const freeze = page.getByRole('switch', { name: '順位表の凍結を行う', exact: true })
  const minutes = page.getByLabel('凍結する時間（終了前の分）', { exact: true })
  await expect(freeze).not.toBeChecked()
  await expect(freeze).toHaveAccessibleDescription('終了前の指定した時間の提出は、終了後に「結果を公開」を押すまで順位表で結果を伏せます。')
  await expect(minutes).toHaveCount(0)
  const section = page.locator('.settings-section').filter({ has: freeze })
  await section.screenshot({ path: testInfo.outputPath('freeze-switch-off.png') })
  await freeze.check()
  await expect(minutes).toHaveValue('60')
  // The switch is a track and thumb beside its label, not a block label over a bare checkbox.
  const track = await section.locator('.switch-track').boundingBox()
  const label = await section.locator('.freeze-switch').boundingBox()
  expect(track!.width).toBe(34)
  expect(label!.height).toBeGreaterThanOrEqual(44)
  expect(Math.abs(track!.y + track!.height / 2 - (label!.y + label!.height / 2))).toBeLessThan(2)
  expect(label!.width).toBeLessThan((await section.boundingBox())!.width / 2)
  await section.screenshot({ path: testInfo.outputPath('freeze-switch-on.png') })
  // Turning it off clears the length; a contest shorter than an hour suggests its whole length next time.
  await freeze.uncheck()
  await expect(minutes).toHaveCount(0)
  await page.getByLabel('コンテスト時間（分）', { exact: true }).fill('45')
  await freeze.check()
  await expect(minutes).toHaveValue('45')
  await minutes.fill('15')
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect.poll(() => saved).toEqual([15])
  await freeze.uncheck()
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect.poll(() => saved).toEqual([15, null])
  await freeze.check()
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await expect(minutes).toBeVisible()
  }
  await page.setViewportSize({ width: 375, height: 900 })
  await section.screenshot({ path: testInfo.outputPath('freeze-switch-375.png') })
})

test('a draft with a freeze opens with the switch on and points to an invalid length', async ({ page }) => {
  let draft = draftResult({ freezeMinutes: 200 }, ['start_missing', 'freeze_invalid'])
  const saved: unknown[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${editorId}/draft`, route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: draft })
    const body = route.request().postDataJSON()
    saved.push(body.draft.freezeMinutes)
    draft = { ...draftResult(body.draft, ['start_missing']), version: body.version + 1 }
    return route.fulfill({ json: draft })
  })
  await page.goto(`/my/contests/${shortId(editorId)}?section=manage`)
  await expect(page.getByRole('list', { name: '足りない項目' })).toContainText('順位表を凍結する時間は、コンテスト時間以内の1分以上の整数で入力するか、凍結をオフにしてください')
  await page.getByRole('list', { name: '足りない項目' }).getByRole('listitem').filter({ hasText: '順位表を凍結する時間' }).getByRole('button', { name: '期間・ペナルティを開く' }).click()
  await expect(page.getByRole('button', { name: '期間・ペナルティ', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('switch', { name: '順位表の凍結を行う', exact: true })).toBeChecked()
  const minutes = page.getByLabel('凍結する時間（終了前の分）', { exact: true })
  await expect(minutes).toHaveValue('200')
  // Opening the draft is not an edit.
  await expect(page.locator('.draft-status')).not.toContainText('未保存の変更があります')
  await minutes.fill('30')
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect.poll(() => saved).toEqual([30])
  await expect(page.locator('.readiness-badge')).toHaveText('×公開できません（1 項目）')
})

test('a draft saved with the switch on and no whole length keeps the freeze and reports it', async ({ page }) => {
  let draft = draftResult({ freezeMinutes: null })
  const saved: unknown[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${editorId}/draft`, route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: draft })
    const body = route.request().postDataJSON()
    saved.push(body.draft.freezeMinutes)
    // The server reports a length outside 1 to the contest's length, as it does for any freeze that is not null.
    const length = body.draft.freezeMinutes
    draft = { ...draftResult(body.draft, ['start_missing', ...length != null && length < 1 ? ['freeze_invalid'] : []]), version: body.version + 1 }
    return route.fulfill({ json: draft })
  })
  await page.goto(`/my/contests/${shortId(editorId)}`)
  await page.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
  const freeze = page.getByRole('switch', { name: '順位表の凍結を行う', exact: true })
  const minutes = page.getByLabel('凍結する時間（終了前の分）', { exact: true })
  await freeze.check()
  // Neither an empty box nor a fraction is saved as no freeze.
  await minutes.fill('1.5')
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect.poll(() => saved).toEqual([0])
  await minutes.fill('')
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect.poll(() => saved).toEqual([0, 0])
  await expect(page.locator('.readiness-badge')).toHaveText('×公開できません（2 項目）')
  // Reopened later, the draft still has the switch on and names the length to fix.
  await page.goto(`/my/contests/${shortId(editorId)}?section=manage`)
  await expect(page.getByRole('list', { name: '足りない項目' })).toContainText('順位表を凍結する時間は、コンテスト時間以内の1分以上の整数で入力するか、凍結をオフにしてください')
  await page.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
  await expect(freeze).toBeChecked()
  await expect(minutes).toHaveValue('0')
})

test('a published contest sends freezeMinutes as a number or null and refuses a freeze longer than the contest', async ({ page }) => {
  const contestId = id
  let contest: Contest = {
    id: contestId, title: '公開済みの凍結コンテスト', author: 'ui_test', authors: ['ui_test'], problemAuthors: [], testers: [], description: '',
    startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T02:00:00Z', penaltyMinutes: 5, version: 3, status: 'scheduled',
    canEdit: true, participating: false, official: false, staff: true, setter: true, owner: true, canViewSubmissions: true,
    freezeMinutes: 30, freezeAt: '2099-10-01T01:30:00Z', revealed: false, revealedAt: null, resultsFinal: false,
    problems: [{ id: '11111111-aaaa-4aaa-8aaa-000000000001', title: '自分の問題', points: 100 }],
  }
  const bodies: Record<string, unknown>[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${contestId}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${contestId}`, route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: contest })
    const body = route.request().postDataJSON()
    bodies.push(body)
    contest = { ...contest, freezeMinutes: body.freezeMinutes, version: contest.version + 1 }
    return route.fulfill({ json: contest })
  })
  const open = async () => {
    await page.goto(`/my/contests/${shortId(contestId)}`)
    await page.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
  }
  const freeze = page.getByRole('switch', { name: '順位表の凍結を行う', exact: true })
  const minutes = page.getByLabel('凍結する時間（終了前の分）', { exact: true })
  const save = page.getByRole('button', { name: '変更を保存' })
  await open()
  await expect(freeze).toBeChecked()
  await expect(minutes).toHaveValue('30')
  // The browser stops a freeze longer than the contest before anything is sent.
  await minutes.fill('121')
  await save.click()
  expect(await minutes.evaluate((input: HTMLInputElement) => input.validity.rangeOverflow)).toBe(true)
  await minutes.fill('45')
  await save.click()
  await expect(page).toHaveURL(`/contests/${shortId(contestId)}`)
  expect(bodies.map(body => body.freezeMinutes)).toEqual([45])
  await open()
  await expect(minutes).toHaveValue('45')
  await freeze.uncheck()
  await save.click()
  await expect(page).toHaveURL(`/contests/${shortId(contestId)}`)
  // Always sent: the API keeps the stored freeze when the field is missing.
  expect(bodies.map(body => body.freezeMinutes)).toEqual([45, null])
  expect(bodies.every(body => 'freezeMinutes' in body)).toBe(true)
  await open()
  await expect(freeze).not.toBeChecked()
  await expect(minutes).toHaveCount(0)
})
