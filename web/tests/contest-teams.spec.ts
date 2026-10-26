import type { Locator, Page, Route } from '@playwright/test'
import { expect, test } from './fixtures/account'
import type { Contest, Entrant, MyTeam, Standing } from '../shared/types/contest'
import type { Submission } from '../shared/types/submission'
import { shortId } from '../shared/utils/short-id'

// The fixture user is ui_test. The fixture API serves this contest to the server-rendered page; the routes below replace
// it once the page refreshes (ADR 0024).
const id = '88888888-8888-4888-8888-888888888888'
const problemId = '11111111-1111-4111-8111-111111111111'
const title = 'チーム戦テスト'
const alpha = 'aaaaaaaa-1111-4111-8111-000000000001'
const beta = 'bbbbbbbb-2222-4222-8222-000000000002'
const base: Contest = {
  id, title, author: 'alice', authors: ['alice'], problemAuthors: [], testers: [], description: '',
  startsAt: '2099-10-01T03:00:00Z', endsAt: '2099-10-01T05:00:00Z', penaltyMinutes: 5, version: 1, status: 'scheduled',
  canEdit: false, participating: false, official: true, canViewSubmissions: false, staff: false, setter: false, excluded: false,
  teamMode: false, maxTeamSize: null, onsiteDivision: false, team: null, onsite: false,
  problems: [{ id: problemId, title: 'A + B', points: 100 }],
}
const problem = { id: problemId, title: 'A + B', author: 'alice', authors: ['alice'], testers: [], markdown: '足し算', editorial: '', timeLimitMs: '2000', memoryLimitMb: '512', hasSamples: false, canReadEditorial: false }
const submission = (id: string, author: string, createdAt = '2026-09-15T12:30:00Z'): Submission => ({
  id, author, problemId, problemTitle: 'A + B', problemVersion: 1, runtime: 'cpp17', sourceBytes: 120, status: 'DONE', createdAt,
  result: { verdict: 'WA', passed: 0, total: 1, cpuTimeMs: 2, memoryBytes: 1048576 },
})
type State = { contest: Contest, rows?: Standing[] | ((query: URLSearchParams) => Standing[]), items?: Submission[], standings: URLSearchParams[], lists: { path: string, query: URLSearchParams }[] }

async function routeContest(page: Page, contest: Contest, extra: Partial<State> = {}) {
  const state: State = { contest, standings: [], lists: [], ...extra }
  await page.route(`**/api/contests/${id}**`, route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/standings')) { state.standings.push(url.searchParams); return route.fulfill({ json: typeof state.rows === 'function' ? state.rows(url.searchParams) : state.rows ?? [] }) }
    if (url.pathname.endsWith('/submissions')) { state.lists.push({ path: url.pathname, query: url.searchParams }); return route.fulfill({ json: { items: state.items ?? [], hasMore: false } }) }
    if (url.pathname.includes('/problems/')) return route.fulfill({ json: problem })
    return route.fulfill({ json: state.contest })
  })
  return state
}
const views = { overview: '概要', standings: '順位表', submissions: 'すべての提出', 'my-submissions': 'チームの提出' }
async function menu(page: Page, view: keyof typeof views) {
  const nav = page.getByRole('navigation', { name: 'コンテストメニュー' })
  // The whole list sits under 「提出」 (ADR 0030).
  if (view === 'submissions') await nav.getByText('提出', { exact: true }).click()
  await nav.getByRole('link', { name: views[view], exact: true }).click()
}
// Opens a view no menu offers, as a link from elsewhere would.
async function visit(page: Page, path: string) {
  await page.evaluate(to => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(to), path)
}
// The server render reads the fixture API, so the page refreshes in the browser to read the routed contest.
async function open(page: Page, view: keyof typeof views = 'overview', query = '') {
  await page.goto(`/contests/${shortId(id)}?view=standings${query}`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.site-header')).toContainText(title)
  if (view !== 'standings') await menu(page, view)
}
// Records each participation request and answers with the next reply; a contest reply also becomes the page's contest.
async function routeJoin(page: Page, state: State, replies: (Contest | { status: number, code: string, retryAfter?: number })[]) {
  const bodies: unknown[] = []
  await page.route(`**/api/my/contests/${id}/participation`, (route: Route) => {
    const data = route.request().postData()
    bodies.push(data ? JSON.parse(data) : null)
    const reply = replies.shift()!
    if ('code' in reply) return route.fulfill({ status: reply.status, json: { statusCode: reply.status, data: { code: reply.code, retryAfter: reply.retryAfter } } })
    state.contest = reply
    return route.fulfill({ json: reply })
  })
  return bodies
}
// Only the standings view has a refresh button; it rereads the contest for every view.
async function reread(page: Page, view: keyof typeof views) {
  await menu(page, 'standings')
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  if (view !== 'standings') await menu(page, view)
}
async function noHorizontalScroll(page: Page) {
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `width ${width}`).toBe(true)
  }
}

// The team page reads the contest again on the way in, so it shows the routed contest.
async function openTeam(page: Page, link: string) {
  await page.locator('.contest-participation').getByRole('link', { name: link, exact: true }).click()
  await expect(page).toHaveURL(`/contests/${shortId(id)}/team`)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('チーム')
}
const ruleMessage = '会場の合言葉は、英数字とアンダースコア（_）の4〜64文字で入力してください。'
const validation = (field: Locator) => field.evaluate((input: HTMLInputElement) => input.validationMessage)

test('an onsite contest takes the venue passphrase on the overview only, keeps it on a mistake and names the retry time', async ({ page }) => {
  const contest = { ...base, onsiteDivision: true }
  const state = await routeContest(page, contest)
  const bodies = await routeJoin(page, state, [
    { status: 400, code: 'incorrect_venue_passphrase' },
    { status: 429, code: 'contest_attempts_limited', retryAfter: 125 },
    { ...contest, participating: true, onsite: true },
  ])
  await open(page, 'standings')
  // Only the overview takes the passphrase; elsewhere joining is open and says where to go instead.
  await expect(page.getByRole('button', { name: '参加する', exact: true })).toBeVisible()
  await expect(page.getByLabel('会場の合言葉（オンサイト参加者のみ）')).toHaveCount(0)
  await expect(page.locator('.contest-participation')).toContainText('オンサイト参加の人は、概要ページで会場の合言葉を入れて参加してください。')
  await menu(page, 'overview')
  await expect(page.locator('.contest-scoring')).toContainText(['オンサイト参加とオープン参加の区分があります。'])
  const field = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await expect(field).toHaveAccessibleDescription('英数字とアンダースコア（_）で4〜64文字です。入れるとオンサイト参加、空のままならオープン参加です。オンサイト参加は順位表に表示されます。')
  await field.fill('tokyo')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('会場の合言葉が違います。会場で配られた合言葉を確かめてください。')
  await expect(field).toHaveValue('tokyo')
  await expect(page.getByText('参加済み', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('参加コードか会場の合言葉の誤りが続いたため、一時的に試せません。約3分後にもう一度お試しください。')
  // Other characters can never be right, so the browser stops them before a try is counted.
  await field.fill('Venue-2026')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  expect(await validation(field)).toBe(ruleMessage)
  await field.fill(' Venue_2026 ')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.getByText('参加済み', { exact: true })).toBeVisible()
  await expect(page.locator('.participation-division')).toHaveText('オンサイト参加')
  await expect(page.getByRole('alert')).toHaveCount(0)
  // The server trims and folds the passphrase, so the page sends it as typed.
  expect(bodies).toEqual([{ onsitePassphrase: 'tokyo' }, { onsitePassphrase: 'tokyo' }, { onsitePassphrase: ' Venue_2026 ' }])
  await noHorizontalScroll(page)
})

test('venue passphrase fields guide typing and name the rule for other characters and lengths', async ({ page }) => {
  const contest = { ...base, onsiteDivision: true }
  const state = await routeContest(page, contest)
  const bodies = await routeJoin(page, state, [{ ...contest, participating: true }])
  await open(page)
  const field = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  // Case matters, so nothing capitalises, corrects or fills in what is typed.
  for (const [name, value] of [['autocomplete', 'off'], ['autocapitalize', 'off'], ['autocorrect', 'off'], ['spellcheck', 'false']] as const) await expect(field).toHaveAttribute(name, value)
  // The server folds and trims before it checks, so full-width letters, digits and underscores, and spaces around them,
  // pass. The edge values are those of TestVenuePassphrases, so the browser and the server agree on them.
  for (const value of ['abcd', 'Venue_2026', ' Ｖｅｎｕｅ＿２０２６　', 'a'.repeat(64), '\uFEFFVenue_2026', 'Venue_2026\u0085', '\u00A0Venue_2026\u2029', '　Ｖｅｎｕｅ＿２０２６', 'ﬁne_day']) {
    await field.fill(value)
    expect(await validation(field), value).toBe('')
  }
  for (const value of ['abc', 'Venue-2026', 'Venue 2026', 'かいじょう', 'ａｂｃ', 'a'.repeat(65), 'Venue_\uFEFF2026', 'a\u200Bbcd']) {
    await field.fill(value)
    expect(await validation(field), value).toBe(ruleMessage)
    await page.getByRole('button', { name: '参加する', exact: true }).click()
  }
  expect(bodies).toEqual([])
  // The field is optional, and spaces alone are empty, as on the server: they join open without a passphrase.
  await field.fill('　 ')
  expect(await validation(field)).toBe('')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.locator('.participation-division')).toHaveText('オープン参加')
  expect(bodies).toEqual([null])
})

test('joining without a passphrase sends no body, and an open participant turns onsite later on the overview', async ({ page }) => {
  const contest = { ...base, status: 'running' as const, onsiteDivision: true }
  const state = await routeContest(page, contest)
  const bodies = await routeJoin(page, state, [
    { ...contest, participating: true },
    { status: 400, code: 'incorrect_venue_passphrase' },
    { ...contest, participating: true },
    { ...contest, participating: true, onsite: true },
  ])
  await open(page)
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.locator('.participation-division')).toHaveText('オープン参加')
  const later = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await expect(later).toHaveAccessibleDescription('英数字とアンダースコア（_）で4〜64文字です。会場で配られた合言葉を入れると、終了までオンサイト参加に変えられます。オンサイト参加は順位表に表示されます。')
  // Spaces alone would change nothing on the server, so the browser stops them.
  await later.fill('　 ')
  await page.getByRole('button', { name: 'オンサイト参加にする', exact: true }).click()
  expect(await validation(later)).toBe(ruleMessage)
  await expect(page.getByRole('alert')).toHaveCount(0)
  await later.fill('osaka')
  await page.getByRole('button', { name: 'オンサイト参加にする', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('会場の合言葉が違います。会場で配られた合言葉を確かめてください。')
  await expect(later).toHaveValue('osaka')
  // Only a contest that answers onsite counts as the change; anything else keeps the input and says so.
  await page.getByRole('button', { name: 'オンサイト参加にする', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('オンサイト参加に変わりませんでした。ページを再読み込みしてから、もう一度お試しください。')
  await expect(later).toHaveValue('osaka')
  await expect(page.locator('.participation-division')).toHaveText('オープン参加')
  await expect(page.locator('.toast')).toHaveCount(0)
  await later.fill('Venue_2026')
  await page.getByRole('button', { name: 'オンサイト参加にする', exact: true }).click()
  await expect(page.locator('.participation-division')).toHaveText('オンサイト参加')
  await expect(page.locator('.toast').getByRole('status')).toHaveText('オンサイト参加に変わりました')
  await expect(later).toHaveCount(0)
  expect(bodies).toEqual([null, { onsitePassphrase: 'osaka' }, { onsitePassphrase: 'osaka' }, { onsitePassphrase: 'Venue_2026' }])
  // Other views leave the registration to the overview, and after the end nobody turns onsite.
  state.contest = { ...contest, participating: true }
  await menu(page, 'standings')
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.contest-participation')).toHaveCount(0)
  state.contest = { ...contest, participating: true, status: 'ended' }
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await menu(page, 'overview')
  await expect(page.locator('.participation-division')).toHaveText('オープン参加')
  await expect(later).toHaveCount(0)
})

test('a refused participation says who cannot join', async ({ page }) => {
  const state = await routeContest(page, { ...base, status: 'running' as const })
  await routeJoin(page, state, [{ status: 409, code: 'contest_participation_unavailable' }])
  await open(page)
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('このコンテストには参加登録できません。スタッフ（作成者・作問者・テスター）と、スタッフを務めた人は参加できません。終了後は新しく参加登録できません。')
})

test('team contests offer 個人参加 and チーム参加, and joining alone works as in an individual contest', async ({ page }) => {
  const contest = { ...base, teamMode: true, maxTeamSize: 3, onsiteDivision: true }
  const state = await routeContest(page, contest)
  const bodies = await routeJoin(page, state, [
    { ...contest, participating: true },
    { status: 400, code: 'incorrect_venue_passphrase' },
    { ...contest, participating: true, onsite: true },
  ])
  await open(page, 'standings')
  // Elsewhere 個人参加 takes no passphrase and joins at once; チーム参加 leads to the team page.
  const area = page.locator('.contest-participation')
  await expect(page.getByLabel('会場の合言葉（オンサイト参加者のみ）')).toHaveCount(0)
  await expect(area.getByRole('link', { name: 'チーム参加', exact: true })).toHaveAttribute('href', `/contests/${shortId(id)}/team`)
  await expect(area).toContainText('オンサイト参加の人は、概要ページかチームのページで会場の合言葉を入れて参加してください。')
  await area.getByRole('button', { name: '個人参加', exact: true }).click()
  // Once joined, only the overview shows the registration.
  await expect(area).toHaveCount(0)
  // On the overview the passphrase goes with 個人参加, as in an individual contest.
  state.contest = contest
  await reread(page, 'overview')
  await expect(page.locator('.contest-scoring')).toContainText(['チーム戦（1チーム3人まで）'])
  await expect(page.getByRole('group', { name: '参加のしかた' })).toHaveCount(0)
  await expect(area.locator('.participation-form > p.muted')).toHaveText('参加すると順位表に表示されます。「チーム参加」では、チームのページでチームを作るか、参加コードでチームに入ります。1チーム3人までです。個人参加すると、開始後はチームを作ることもチームに入ることもできません。')
  const field = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await field.fill('tokyo')
  await area.getByRole('button', { name: '個人参加', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('会場の合言葉が違います。会場で配られた合言葉を確かめてください。')
  await expect(field).toHaveValue('tokyo')
  await field.fill('Venue_2026')
  await area.getByRole('button', { name: '個人参加', exact: true }).click()
  await expect(area.locator('.participation-state')).toHaveText('参加済みオンサイト参加個人参加')
  expect(bodies).toEqual([null, { onsitePassphrase: 'tokyo' }, { onsitePassphrase: 'Venue_2026' }])
  // Before the start someone registered alone may still move to a team, and the page says what becomes of the
  // registration. Other views leave that to the overview.
  await expect(area.getByRole('link', { name: 'チーム参加に切り替える', exact: true })).toHaveAttribute('href', `/contests/${shortId(id)}/team`)
  await expect(area).toContainText('チームを作るかチームに入ると、個人参加の登録はチームのメンバーとしての登録に置き換わります。開始後は切り替えられません。')
  await noHorizontalScroll(page)
  await page.setViewportSize({ width: 375, height: 900 })
  await area.screenshot({ path: test.info().outputPath('individual-375.png') })
  await page.setViewportSize({ width: 1280, height: 900 })
  await menu(page, 'standings')
  await expect(page.locator('#standings')).toBeVisible()
  await expect(area).toHaveCount(0)
  // After the start the switch is gone.
  state.contest = { ...state.contest, status: 'running' }
  await reread(page, 'overview')
  await expect(area.locator('.participation-state')).toHaveText('参加済みオンサイト参加個人参加')
  await expect(area.getByRole('link')).toHaveCount(0)
})

test('in a running team contest Enter in the passphrase field picks neither way in, and 個人参加 asks first', async ({ page }) => {
  const contest = { ...base, status: 'running' as const, teamMode: true, maxTeamSize: 3, onsiteDivision: true }
  const state = await routeContest(page, contest)
  const bodies = await routeJoin(page, state, [{ ...contest, participating: true, onsite: true }])
  const dialogs: string[] = []
  let accept = false
  page.on('dialog', dialog => { dialogs.push(dialog.message()); void (accept ? dialog.accept() : dialog.dismiss()) })
  await open(page)
  const area = page.locator('.contest-participation')
  await expect(area.locator('.participation-form > p.muted')).toContainText('個人参加すると、開始後はチームを作ることもチームに入ることもできません。')
  const field = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  // Enter stands for neither 個人参加 nor チーム参加: it only reports a value against the rule.
  await field.fill('Venue-2026')
  await field.press('Enter')
  expect(await validation(field)).toBe(ruleMessage)
  await field.fill('Venue_2026')
  await field.press('Enter')
  // 個人参加 checks the passphrase first, then asks, since after the start the person can never join a team.
  await field.fill('Venue-2026')
  await area.getByRole('button', { name: '個人参加', exact: true }).click()
  await field.fill('Venue_2026')
  await area.getByRole('button', { name: '個人参加', exact: true }).click()
  await expect.poll(() => dialogs.length).toBe(1)
  await expect(area.locator('.participation-state')).toHaveCount(0)
  expect(bodies).toEqual([])
  accept = true
  await area.getByRole('button', { name: '個人参加', exact: true }).click()
  await expect(area.locator('.participation-state')).toHaveText('参加済みオンサイト参加個人参加')
  expect(dialogs).toEqual(Array(2).fill('個人参加すると、このコンテストではチームを作ることもチームに入ることもできなくなります。個人参加しますか？'))
  expect(bodies).toEqual([{ onsitePassphrase: 'Venue_2026' }])
})

test('guests choose 個人参加 or チーム参加 through the login page, which keeps the page they chose', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.clock.install()
  await routeContest(page, { ...base, teamMode: true })
  await open(page)
  const area = page.locator('.contest-participation')
  await expect(area.getByRole('link', { name: '個人参加', exact: true })).toHaveAttribute('href', `/login?next=/contests/${shortId(id)}`)
  await expect(area.getByRole('link', { name: 'チーム参加', exact: true })).toHaveAttribute('href', `/login?next=/contests/${shortId(id)}/team`)
  await expect(area).toContainText('ログインしてから参加します。参加すると順位表に表示されます。')
  await noHorizontalScroll(page)
  // The team page itself gives guests the same way in; the server render reads the fixture, the next poll the route.
  await page.goto(`/contests/${shortId(id)}/team`, { waitUntil: 'networkidle' })
  await page.clock.fastForward(15000)
  await page.getByRole('link', { name: 'ログインしてチーム参加する', exact: true }).click()
  await expect(page).toHaveURL(`/login?next=/contests/${shortId(id)}/team`)
  // Logging in returns to the team page, ready to form a team.
  await page.route('**/api/auth/login', route => route.fulfill({ json: { user: { id: 'ui-test-user' } } }))
  await page.getByLabel('メールアドレス', { exact: true }).fill('ui_test@example.test')
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL(`/contests/${shortId(id)}/team`)
  await expect(page.getByRole('group', { name: '参加のしかた' })).toBeVisible()
})

test('the team page creates a team with the passphrase typed on the overview, and members find the panel there', async ({ page }) => {
  const contest = { ...base, teamMode: true, maxTeamSize: 3, onsiteDivision: true }
  const state = await routeContest(page, contest)
  const team = { id: alpha, name: 'Alpha', members: ['ui_test'], onsite: true }
  const bodies = await routeJoin(page, state, [
    { status: 409, code: 'team_name_taken' },
    { status: 400, code: 'invalid_team_name' },
    { ...contest, participating: true, team, onsite: true },
  ])
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code: 'ABCDEFGHJK' } }))
  await open(page)
  // A passphrase typed beside 個人参加 goes along to the team page, so it is not typed twice.
  await page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true }).fill('Venue_2026')
  await openTeam(page, 'チーム参加')
  await expect(page).toHaveTitle(`チーム | ${title} | ShareOJ`)
  await expect(page.getByRole('navigation', { name: 'パンくずリスト' })).toHaveText(`${title}/チーム`)
  // The page belongs to the overview, which the contest menu keeps marked.
  await expect(page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true })).toHaveAttribute('aria-current', 'true')
  await expect(page.getByRole('group', { name: '参加のしかた' }).getByRole('radio', { name: 'チームを作る' })).toBeChecked()
  const venue = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await expect(venue).toHaveValue('Venue_2026')
  await expect(venue).toHaveAccessibleDescription('英数字とアンダースコア（_）で4〜64文字です。入れるとチームがオンサイト参加になります。オンサイト参加は順位表に表示されます。')
  await expect(page.locator('.participation-form > p.muted')).toHaveText('参加すると順位表に表示されます。1チーム3人までです。開始後はチームを抜けられず、メンバーも外せません。1人で出るときは、概要ページの「個人参加」から参加できます。')
  await expect(page.locator('.participation-form').getByRole('link', { name: '概要ページ' })).toHaveAttribute('href', `/contests/${shortId(id)}`)
  await noHorizontalScroll(page)
  await page.setViewportSize({ width: 375, height: 900 })
  await page.locator('.contest-team-page').screenshot({ path: test.info().outputPath('team-form-375.png') })
  await page.setViewportSize({ width: 1280, height: 900 })
  const name = page.getByLabel('チーム名', { exact: true })
  await name.fill('alpha')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('このチーム名はほかのチームが使っています。別の名前にしてください。')
  await expect(name).toHaveValue('alpha')
  await expect(venue).toHaveValue('Venue_2026')
  await name.fill('Al​pha')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('チーム名は1〜40文字で入力してください。改行や見えない文字は使えません。')
  await name.fill('Alpha')
  await page.getByRole('button', { name: '参加する', exact: true }).click()
  const panel = page.getByRole('region', { name: '所属チーム', exact: true })
  await expect(panel.locator('.team-name strong')).toHaveText('Alpha')
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveValue('ABCDEFGHJK')
  // The panel names the team, so the state line names only the division.
  await expect(page.locator('.participation-state')).toHaveText('参加済みオンサイト参加')
  expect(bodies).toEqual([
    { team: { action: 'create', name: 'alpha' }, onsitePassphrase: 'Venue_2026' },
    { team: { action: 'create', name: 'Al​pha' }, onsitePassphrase: 'Venue_2026' },
    { team: { action: 'create', name: 'Alpha' }, onsitePassphrase: 'Venue_2026' },
  ])
  await noHorizontalScroll(page)
  // On the contest page a member sees the team and a link back here instead of the panel.
  await page.getByRole('navigation', { name: 'パンくずリスト' }).getByRole('link', { name: title }).click()
  await expect(page.locator('.participation-state')).toHaveText('参加済みオンサイト参加チーム Alpha')
  await expect(page.getByRole('region', { name: '所属チーム', exact: true })).toHaveCount(0)
  await expect(page.locator('.contest-participation').getByRole('link', { name: 'チームのページ', exact: true })).toHaveAttribute('href', `/contests/${shortId(id)}/team`)
  await expect(page.locator('.contest-participation')).toContainText('メンバーと参加コードは、チームのページで確認できます。')
  // The other views leave the registration to the overview.
  await menu(page, 'standings')
  await expect(page.locator('#standings')).toBeVisible()
  await expect(page.locator('.contest-participation')).toHaveCount(0)
})

test('joining a team takes only the join code and explains a wrong code, a full team and the retry time', async ({ page }) => {
  const contest = { ...base, status: 'running' as const, teamMode: true }
  const state = await routeContest(page, contest)
  const team = { id: alpha, name: 'Alpha', members: ['bob', 'ui_test'], onsite: false }
  const bodies = await routeJoin(page, state, [
    { status: 400, code: 'incorrect_team_code' },
    { status: 409, code: 'team_full' },
    { status: 429, code: 'contest_attempts_limited', retryAfter: 42 },
    { ...contest, participating: true, team },
  ])
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code: 'ABCDEFGHJK' } }))
  await open(page)
  await openTeam(page, 'チーム参加')
  // Without a division there is no passphrase, and without a limit no size is named.
  await expect(page.getByLabel('会場の合言葉（オンサイト参加者のみ）')).toHaveCount(0)
  await expect(page.locator('.participation-form')).not.toContainText('1チーム')
  await page.getByRole('group', { name: '参加のしかた' }).getByRole('radio', { name: 'チームに入る' }).check()
  await expect(page.getByLabel('チーム名', { exact: true })).toHaveCount(0)
  const code = page.getByLabel('チームの参加コード', { exact: true })
  await expect(code).toHaveAccessibleDescription('チームのメンバーに参加コードを聞いて入力します。')
  await code.fill('abcde-fghjj')
  const join = page.getByRole('button', { name: '参加する', exact: true })
  await join.click()
  await expect(page.getByRole('alert')).toHaveText('参加コードが違います。チームのメンバーに確かめてください。')
  await expect(code).toHaveValue('abcde-fghjj')
  await join.click()
  await expect(page.getByRole('alert')).toHaveText('このチームは人数の上限に達しているため入れません。')
  await join.click()
  await expect(page.getByRole('alert')).toHaveText('参加コードか会場の合言葉の誤りが続いたため、一時的に試せません。42秒後にもう一度お試しください。')
  await code.fill('abcde-fghjk')
  await join.click()
  await expect(page.getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['bob', 'ui_test自分'])
  expect(bodies).toEqual([
    ...Array(3).fill({ team: { action: 'join', code: 'abcde-fghjj' } }),
    { team: { action: 'join', code: 'abcde-fghjk' } },
  ])
  // After the start nobody leaves or removes anyone.
  await expect(page.getByRole('button', { name: 'チームを抜ける' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'bobをチームから外す' })).toHaveCount(0)
})

test('someone registered alone switches to a team on the team page before the start, bringing the onsite division', async ({ page }) => {
  const contest = { ...base, teamMode: true, onsiteDivision: true, participating: true, team: null, onsite: true }
  const state = await routeContest(page, contest)
  const team = { id: alpha, name: 'Alpha', members: ['bob', 'ui_test'], onsite: true }
  const bodies = await routeJoin(page, state, [
    { status: 409, code: 'contest_individual_locked' },
    { ...contest, team },
  ])
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code: 'ABCDEFGHJK' } }))
  await open(page)
  await openTeam(page, 'チーム参加に切り替える')
  // The page says the registration moves into the team; an onsite participant's team turns onsite, so nothing is asked.
  await expect(page.locator('.participation-form .notice')).toHaveText('個人参加で登録しています。チームを作るかチームに入ると、個人参加の登録はチームのメンバーとしての登録に置き換わります。オンサイト参加なので、チームもオンサイト参加になります。開始後は切り替えられません。')
  await expect(page.getByLabel('会場の合言葉（オンサイト参加者のみ）')).toHaveCount(0)
  await expect(page.locator('.participation-form > p.muted')).toHaveText('開始後はチームを抜けられず、メンバーも外せません。')
  await page.getByRole('group', { name: '参加のしかた' }).getByRole('radio', { name: 'チームに入る' }).check()
  const code = page.getByLabel('チームの参加コード', { exact: true })
  await code.fill('abcde-fghjk')
  const submit = page.getByRole('button', { name: 'チーム参加に切り替える', exact: true })
  // The start may pass while the page is open; the server refuses then, and the page says why.
  await submit.click()
  await expect(page.getByRole('alert')).toHaveText('個人参加で登録しているため、開始後はチームを作ることもチームに入ることもできません。')
  await expect(code).toHaveValue('abcde-fghjk')
  await submit.click()
  await expect(page.getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['bob外す', 'ui_test自分'])
  await expect(page.locator('.participation-state')).toHaveText('参加済みオンサイト参加')
  expect(bodies).toEqual(Array(2).fill({ team: { action: 'join', code: 'abcde-fghjk' } }))
  await noHorizontalScroll(page)
})

test('someone registered open switches to a team with the venue passphrase on the team page, which then makes the team onsite', async ({ page }) => {
  const contest = { ...base, teamMode: true, maxTeamSize: 3, onsiteDivision: true, participating: true, team: null, onsite: false }
  const state = await routeContest(page, contest)
  const team = { id: alpha, name: 'Rocket', members: ['ui_test'], onsite: true }
  const bodies = await routeJoin(page, state, [
    { status: 409, code: 'team_name_taken' },
    { status: 400, code: 'incorrect_venue_passphrase' },
    { ...contest, team, onsite: true },
  ])
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code: 'ABCDEFGHJK' } }))
  await open(page)
  await openTeam(page, 'チーム参加に切り替える')
  await expect(page.locator('.participation-form .notice')).toHaveText('個人参加で登録しています。チームを作るかチームに入ると、個人参加の登録はチームのメンバーとしての登録に置き換わります。開始後は切り替えられません。')
  await page.getByLabel('チーム名', { exact: true }).fill('Rocket')
  const venue = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  const submit = page.getByRole('button', { name: 'チーム参加に切り替える', exact: true })
  // Spaces alone are empty, so the switch goes without a passphrase; a refusal keeps every input.
  await venue.fill('　 ')
  await submit.click()
  await expect(page.getByRole('alert')).toHaveText('このチーム名はほかのチームが使っています。別の名前にしてください。')
  await venue.fill('Venue_2025')
  await submit.click()
  await expect(page.getByRole('alert')).toHaveText('会場の合言葉が違います。会場で配られた合言葉を確かめてください。')
  await expect(venue).toHaveValue('Venue_2025')
  await expect(page.getByLabel('チーム名', { exact: true })).toHaveValue('Rocket')
  await venue.fill('Venue_2026')
  await submit.click()
  await expect(page.getByRole('region', { name: '所属チーム', exact: true })).toBeVisible()
  await expect(page.locator('.participation-division')).toHaveText('オンサイト参加')
  const create = { team: { action: 'create', name: 'Rocket' } }
  expect(bodies).toEqual([create, { ...create, onsitePassphrase: 'Venue_2025' }, { ...create, onsitePassphrase: 'Venue_2026' }])
})

test('the team page explains a contest without teams, and after the start offers no switch to someone registered alone', async ({ page }) => {
  await page.clock.install()
  const state = await routeContest(page, { ...base, status: 'running' })
  await page.goto(`/contests/${shortId(id)}/team`, { waitUntil: 'networkidle' })
  await expect(page.locator('.contest-team-page .notice')).toHaveText('このコンテストは個人戦です。概要ページから参加してください。')
  await expect(page.locator('.contest-team-page .notice').getByRole('link', { name: '概要ページ' })).toHaveAttribute('href', `/contests/${shortId(id)}`)
  // The page follows the contest as it polls.
  state.contest = { ...base, status: 'running', teamMode: true, participating: true, team: null }
  await page.clock.fastForward(15000)
  await expect(page.locator('.contest-participation')).toContainText('開始後は、個人参加からチーム参加に切り替えられません。')
  await expect(page.locator('.participation-state')).toHaveText('参加済み個人参加')
  await expect(page.getByRole('group', { name: '参加のしかた' })).toHaveCount(0)
  await noHorizontalScroll(page)
  // The contest page offers no way to switch either.
  await page.getByRole('navigation', { name: 'パンくずリスト' }).getByRole('link', { name: title }).click()
  await expect(page.locator('.participation-state')).toHaveText('参加済み個人参加')
  await expect(page.locator('.contest-participation').getByRole('link')).toHaveCount(0)
})

test('the team page gives staff, former staff and visitors after the end the contest page\'s reasons instead of the form', async ({ page }) => {
  await page.clock.install()
  const state = await routeContest(page, { ...base, teamMode: true, staff: true, setter: true, official: false })
  await page.goto(`/contests/${shortId(id)}/team`, { waitUntil: 'networkidle' })
  const area = page.locator('.contest-team-page .contest-participation')
  const form = page.getByRole('group', { name: '参加のしかた' })
  const states: [Contest, string][] = [
    [state.contest, 'スタッフ（作成者・作問者・テスター）は公式参加の対象外です。'],
    [{ ...base, teamMode: true, excluded: true }, 'このコンテストのスタッフを務めたため、参加できません。'],
    [{ ...base, teamMode: true, status: 'ended' }, '参加受付は終了しました。'],
  ]
  for (const [contest, reason] of states) {
    state.contest = contest
    await page.clock.fastForward(15000)
    await expect(area).toHaveText(reason)
    await expect(form).toHaveCount(0)
  }
})

test('a member of an open team turns the whole team onsite on the team page, beside the team panel', async ({ page }) => {
  const team = { id: alpha, name: 'Alpha', members: ['ui_test', 'bob'], onsite: false }
  const contest = { ...base, status: 'running' as const, teamMode: true, onsiteDivision: true, participating: true, team, onsite: false }
  const state = await routeContest(page, contest)
  const bodies = await routeJoin(page, state, [{ ...contest, onsite: true, team: { ...team, onsite: true } }])
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code: 'ABCDEFGHJK' } }))
  await open(page)
  await openTeam(page, 'チームのページ')
  await expect(page.getByRole('region', { name: '所属チーム', exact: true })).toBeVisible()
  await expect(page.locator('.participation-division')).toHaveText('オープン参加')
  const field = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await expect(field).toHaveAccessibleDescription('英数字とアンダースコア（_）で4〜64文字です。会場で配られた合言葉を入れると、終了までオンサイト参加に変えられます。メンバーの1人が入れると、チーム全体がオンサイト参加になります。オンサイト参加は順位表に表示されます。')
  await field.fill('Venue_2026')
  await page.getByRole('button', { name: 'オンサイト参加にする', exact: true }).click()
  await expect(page.locator('.participation-division')).toHaveText('オンサイト参加')
  await expect(page.locator('.toast').getByRole('status')).toHaveText('オンサイト参加に変わりました')
  await expect(field).toHaveCount(0)
  await expect(page.getByRole('region', { name: '所属チーム', exact: true })).toBeVisible()
  expect(bodies).toEqual([{ onsitePassphrase: 'Venue_2026' }])
})

test('the team panel copies and renews the code, renames the team, and before the start removes a member or leaves', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const team = { id: alpha, name: 'Alpha', members: ['ui_test', 'bob', 'carol'], onsite: false }
  const contest = { ...base, teamMode: true, participating: true, team }
  const state = await routeContest(page, contest)
  let mine: MyTeam = { ...team, code: 'ABCDEFGHJK' }
  const requests: string[] = []
  let refusal = ''
  await page.route(`**/api/my/contests/${id}/team**`, route => {
    const request = route.request(), path = new URL(request.url()).pathname.replace(`/api/my/contests/${id}/team`, '')
    requests.push(`${request.method()} ${path}${request.postData() ? ` ${request.postData()}` : ''}`)
    if (refusal) { const code = refusal; refusal = ''; return route.fulfill({ status: 409, json: { statusCode: 409, data: { code } } }) }
    if (request.method() === 'PUT' && path === '/code') mine = { ...mine, code: 'MNPQRSTVWX' }
    if (request.method() === 'PUT' && path === '/name') mine = { ...mine, name: request.postDataJSON().name }
    if (request.method() === 'DELETE') {
      const handle = path.split('/').pop()!
      mine = { ...mine, members: mine.members.filter(member => member !== handle), code: handle === 'ui_test' ? mine.code : 'RENEWED234' }
      state.contest = handle === 'ui_test' ? { ...contest, participating: false, team: null } : { ...state.contest, team: { ...team, name: mine.name, members: mine.members } }
      return route.fulfill({ status: 204 })
    }
    if (request.method() === 'PUT') state.contest = { ...state.contest, team: { ...team, name: mine.name, members: mine.members } }
    return route.fulfill({ json: mine })
  })
  await open(page)
  await openTeam(page, 'チームのページ')
  const panel = page.getByRole('region', { name: '所属チーム', exact: true })
  await expect(panel.locator('.team-name strong')).toHaveText('Alpha')
  await expect(panel.getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['ui_test自分', 'bob外す', 'carol外す'])
  const code = panel.getByLabel('参加コード', { exact: true })
  await expect(code).toHaveValue('ABCDEFGHJK')
  await expect(code).toHaveAccessibleDescription('参加コードを伝えた人は、「チーム参加」の「チームに入る」から加われます。開始後はメンバーを外せませんが、参加コードを作り直すと、それ以降の加入を止められます。')
  await panel.getByRole('button', { name: '参加コードをコピー' }).click()
  await expect(panel.getByRole('button', { name: 'コピーしました' })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('ABCDEFGHJK')
  // Renewing asks first.
  let question = ''
  page.once('dialog', dialog => { question = dialog.message(); void dialog.dismiss() })
  await panel.getByRole('button', { name: '作り直す', exact: true }).click()
  expect(question).toBe('参加コードを作り直しますか？今のコードではチームに入れなくなります。')
  page.once('dialog', dialog => void dialog.accept())
  await panel.getByRole('button', { name: '作り直す', exact: true }).click()
  await expect(code).toHaveValue('MNPQRSTVWX')
  await expect(panel.getByRole('status')).toHaveText('参加コードを作り直しました。前のコードでは入れません。')
  // Renaming keeps the input on a refusal. Focus moves into the form and back to the button, never to the page.
  await panel.getByRole('button', { name: 'チーム名を変更' }).click()
  const rename = panel.getByLabel('新しいチーム名', { exact: true })
  await expect(rename).toBeFocused()
  await panel.getByRole('button', { name: 'キャンセル', exact: true }).click()
  await expect(panel.getByRole('button', { name: 'チーム名を変更' })).toBeFocused()
  await panel.getByRole('button', { name: 'チーム名を変更' }).press('Enter')
  await expect(rename).toBeFocused()
  await expect(rename).toHaveValue('Alpha')
  await rename.fill('Beta')
  refusal = 'team_name_taken'
  await panel.getByRole('button', { name: '保存', exact: true }).click()
  await expect(panel.getByRole('alert')).toHaveText('このチーム名はほかのチームが使っています。別の名前にしてください。')
  await expect(rename).toHaveValue('Beta')
  await rename.fill('Gamma')
  await panel.getByRole('button', { name: '保存', exact: true }).click()
  await expect(panel.locator('.team-name strong')).toHaveText('Gamma')
  await expect(panel.getByRole('status')).toHaveText('チーム名を変更しました。')
  await expect(panel.getByRole('button', { name: 'チーム名を変更' })).toBeFocused()
  // Removing a member renews the code, which the panel reads again.
  page.once('dialog', dialog => { question = dialog.message(); void dialog.accept() })
  await panel.getByRole('button', { name: 'bobをチームから外す' }).click()
  expect(question).toBe('bobをチームから外しますか？外した人の参加登録は取り消され、参加コードは作り直されます。')
  await expect(panel.getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['ui_test自分', 'carol外す'])
  await expect(panel.getByRole('list', { name: 'メンバー' })).toBeFocused()
  await expect(code).toHaveValue('RENEWED234')
  await expect(panel.getByRole('status')).toHaveText('bobをチームから外しました。参加コードを作り直しました。')
  await noHorizontalScroll(page)
  await page.setViewportSize({ width: 375, height: 900 })
  await panel.screenshot({ path: test.info().outputPath('team-panel-375.png') })
  await page.setViewportSize({ width: 1280, height: 900 })
  // The start locks the members on the server; a page that has not noticed yet learns why.
  refusal = 'team_locked'
  page.once('dialog', dialog => void dialog.accept())
  await panel.getByRole('button', { name: 'チームを抜ける' }).click()
  await expect(panel.getByRole('alert')).toHaveText('開始後はメンバーを外せず、チームを抜けることもできません。')
  page.once('dialog', dialog => { question = dialog.message(); void dialog.accept() })
  await panel.getByRole('button', { name: 'チームを抜ける' }).click()
  expect(question).toBe('チームを抜けますか？参加登録も取り消されます。')
  await expect(page.locator('.toast').getByRole('status')).toHaveText('チームを抜けました')
  // Leaving also ends the registration, so the page offers the forms again.
  await expect(panel).toHaveCount(0)
  await expect(page.getByRole('group', { name: '参加のしかた' })).toBeVisible()
  await expect(page.getByRole('button', { name: '参加する', exact: true })).toBeVisible()
  // The panel also rereads the code whenever the member list changes; only the changes are counted here.
  expect(requests[0]).toBe('GET ')
  expect(requests.filter(request => !request.startsWith('GET'))).toEqual([
    'PUT /code', 'PUT /name {"name":"Beta"}', 'PUT /name {"name":"Gamma"}', 'DELETE /members/bob', 'DELETE /members/ui_test', 'DELETE /members/ui_test',
  ])
})

test('after the start members only rename and renew, and after the end nothing changes', async ({ page }) => {
  await page.clock.install()
  const team = { id: alpha, name: 'Alpha', members: ['ui_test', 'bob'], onsite: false }
  const state = await routeContest(page, { ...base, status: 'running', teamMode: true, participating: true, team })
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code: 'ABCDEFGHJK' } }))
  await open(page)
  await openTeam(page, 'チームのページ')
  const panel = page.getByRole('region', { name: '所属チーム', exact: true })
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveValue('ABCDEFGHJK')
  await expect(panel.getByRole('button', { name: 'チーム名を変更' })).toBeVisible()
  await expect(panel.getByRole('button', { name: '作り直す', exact: true })).toBeVisible()
  await expect(panel.getByRole('button', { name: /外す|抜ける/ })).toHaveCount(0)
  // The team page follows the contest as it polls.
  state.contest = { ...state.contest, status: 'ended' }
  await page.clock.fastForward(15000)
  // Nobody joins after the end, so the code and the invitation to share it are gone.
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveCount(0)
  await expect(panel.getByRole('button', { name: '参加コードをコピー' })).toHaveCount(0)
  await expect(panel).toContainText('終了したため、チームには入れません。')
  await expect(panel).not.toContainText('「チームに入る」から加われます')
  await expect(panel.getByRole('button', { name: /チーム名を変更|作り直す|外す|抜ける/ })).toHaveCount(0)
})

test('the team panel follows a join code that another member renewed', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.clock.install()
  const team = { id: alpha, name: 'Alpha', members: ['ui_test', 'bob'], onsite: false }
  await routeContest(page, { ...base, status: 'running', teamMode: true, participating: true, team })
  let code = 'ABCDEFGHJK'
  await page.route(`**/api/my/contests/${id}/team`, route => route.fulfill({ json: { ...team, code } }))
  await open(page)
  await openTeam(page, 'チームのページ')
  const panel = page.getByRole('region', { name: '所属チーム', exact: true })
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveValue('ABCDEFGHJK')
  // Bob renews the code; the members stay the same, and the panel still reads the new one.
  code = 'MNPQRSTVWX'
  await page.clock.fastForward(15000)
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveValue('MNPQRSTVWX')
  await panel.getByRole('button', { name: '参加コードをコピー' }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('MNPQRSTVWX')
})

const entrants = (): Entrant[] => [
  { handle: '', team: { id: alpha, name: 'Alpha', members: ['bob', 'carol'], onsite: true }, onsite: true, onsiteAt: '2099-10-01T02:30:00Z', onsiteVia: 'passphrase', createdAt: '2099-09-30T12:00:00Z' },
  { handle: 'erin', team: null, onsite: false, onsiteAt: null, onsiteVia: '', createdAt: '2099-09-30T12:30:00Z' },
  { handle: '', team: { id: beta, name: 'Beta', members: ['dave'], onsite: false }, onsite: false, onsiteAt: null, onsiteVia: '', createdAt: '2099-09-30T13:00:00Z' },
]

test('setters open the participant tools, replace the venue passphrase, move open entrants onsite and rename teams', async ({ page }) => {
  const contest = { ...base, status: 'running' as const, setter: true, staff: true, official: false, teamMode: true, onsiteDivision: true, resultsFinal: false }
  const state = await routeContest(page, contest)
  let rows = entrants()
  let passphrase = 'Venue_2026'
  const requests: string[] = []
  let refusal = ''
  await page.route(`**/api/my/contests/${id}/onsite`, route => {
    const request = route.request()
    requests.push(`${request.method()} onsite${request.postData() ? ` ${request.postData()}` : ''}`)
    if (refusal) { const code = refusal; refusal = ''; return route.fulfill({ status: 400, json: { statusCode: 400, data: { code } } }) }
    if (request.method() === 'PUT') passphrase = request.postDataJSON().passphrase
    return route.fulfill({ json: { onsiteDivision: true, passphrase } })
  })
  await page.route(`**/api/my/contests/${id}/participants`, route => { requests.push('GET participants'); return route.fulfill({ json: { items: rows } }) })
  const moveOnsite = (key: string) => {
    rows = rows.map(row => (row.team?.id ?? row.handle) !== key ? row : { ...row, onsite: true, onsiteAt: '2099-10-01T03:40:00Z', onsiteVia: 'setter', team: row.team && { ...row.team, onsite: true } })
    return rows.find(row => (row.team?.id ?? row.handle) === key)
  }
  await page.route(`**/api/my/contests/${id}/participants/*/onsite`, route => {
    const request = route.request(), handle = new URL(request.url()).pathname.split('/').at(-2)!
    requests.push(`PUT ${handle} ${request.postData()}`)
    return route.fulfill({ json: moveOnsite(handle) })
  })
  await page.route(`**/api/my/contests/${id}/teams/*/*`, route => {
    const request = route.request(), [team, field] = new URL(request.url()).pathname.split('/').slice(-2)
    requests.push(`PUT ${field} ${request.postData()}`)
    if (refusal) { const code = refusal; refusal = ''; return route.fulfill({ status: 409, json: { statusCode: 409, data: { code } } }) }
    if (field === 'onsite') return route.fulfill({ json: moveOnsite(team!) })
    rows = rows.map(row => row.team?.id !== team ? row : { ...row, team: { ...row.team!, name: request.postDataJSON().name } })
    return route.fulfill({ json: rows.find(row => row.team?.id === team) })
  })
  await open(page)
  const tools = page.locator('details.setter-tools')
  await expect(tools.locator('summary')).toHaveText('参加者の管理作成者だけに表示')
  // Closed, the panel reads nothing, so a projected overview never shows the passphrase.
  await expect(tools).not.toHaveAttribute('open')
  expect(requests).toEqual([])
  await tools.locator('summary').click()
  const venue = tools.getByRole('region', { name: '会場の合言葉' })
  await expect(venue.getByRole('textbox', { name: '会場の合言葉' })).toHaveValue('Venue_2026')
  await expect(tools.locator('.setter-tools-body > p').first()).toHaveText('会場の合言葉は終了まで差し替えられます。オープン参加の人とチームは、結果が確定するまでオンサイト参加にできます。オープン参加には戻せません。')
  // Teams and those who joined alone share one table.
  await expect(tools.getByRole('heading', { name: 'チーム・個人 チーム 2・個人 1' })).toBeVisible()
  const table = tools.getByRole('region', { name: '参加者の一覧' })
  await expect(table.locator('thead th')).toHaveText(['チーム・個人', '区分', 'オンサイトになった時刻 (JST)', '登録日時 (JST)'])
  await expect(table.locator('tbody th')).toHaveText([/^Alpha名前を変更bob、carol$/, 'erin個人参加', /^Beta名前を変更dave$/])
  await expect(table.locator('tbody tr').first().locator('td').nth(1)).toHaveText('2099/10/01 11:30（合言葉）')
  // Replacing the passphrase checks the rule first and explains a refusal, keeping the input. Focus follows the form
  // and the button it replaces.
  await venue.getByRole('button', { name: '差し替える', exact: true }).click()
  const fresh = venue.getByLabel('新しい会場の合言葉', { exact: true })
  await expect(fresh).toBeFocused()
  await expect(fresh).toHaveAccessibleDescription('英数字とアンダースコア（_）で4〜64文字です。大文字と小文字は区別します。')
  await venue.getByRole('button', { name: 'キャンセル', exact: true }).click()
  await expect(venue.getByRole('button', { name: '差し替える', exact: true })).toBeFocused()
  await venue.getByRole('button', { name: '差し替える', exact: true }).press('Enter')
  await expect(fresh).toBeFocused()
  await fresh.fill('abc d')
  await venue.getByRole('button', { name: '保存', exact: true }).click()
  expect(await validation(fresh)).toBe(ruleMessage)
  await fresh.fill('abcd')
  refusal = 'invalid_venue_passphrase'
  await venue.getByRole('button', { name: '保存', exact: true }).click()
  await expect(tools.getByRole('alert')).toHaveText(ruleMessage)
  await expect(fresh).toHaveValue('abcd')
  await fresh.fill('Second_Venue')
  await venue.getByRole('button', { name: '保存', exact: true }).click()
  await expect(venue.getByRole('textbox', { name: '会場の合言葉' })).toHaveValue('Second_Venue')
  await expect(tools.getByRole('status')).toHaveText('会場の合言葉を差し替えました。前の合言葉は使えません。')
  await expect(venue.getByRole('button', { name: '差し替える', exact: true })).toBeFocused()
  // Open rows move onsite after a question, since nothing moves back; onsite rows have no control at all.
  const divisions = table.locator('tbody tr').locator('td:first-of-type')
  await expect(divisions).toHaveText(['オンサイト', 'オープンオンサイトにする', 'オープンオンサイトにする'])
  await expect(table.getByRole('button', { name: 'Alphaをオンサイト参加にする' })).toHaveCount(0)
  const betaButton = table.getByRole('button', { name: 'Betaをオンサイト参加にする', exact: true })
  let question = ''
  page.once('dialog', dialog => { question = dialog.message(); void dialog.dismiss() })
  await betaButton.click()
  expect(question).toBe('Betaをオンサイト参加にしますか？オープン参加には戻せません。')
  refusal = 'contest_conflict'
  page.once('dialog', dialog => void dialog.accept())
  await betaButton.click()
  await expect(tools.getByRole('alert')).toHaveText('結果が確定したため、区分は変更できません。')
  await expect(betaButton).toBeVisible()
  page.once('dialog', dialog => void dialog.accept())
  await betaButton.click()
  await expect(betaButton).toHaveCount(0)
  await expect(divisions.nth(2)).toHaveText('オンサイト')
  await expect(table.locator('tbody tr').nth(2).locator('td').nth(1)).toHaveText('2099/10/01 12:40（作成者）')
  await expect(tools.getByRole('status')).toHaveText('Betaをオンサイト参加にしました。')
  // The button goes with the move, so focus stays on the row's division.
  await expect(divisions.nth(2).locator('.setter-division')).toBeFocused()
  // Someone who joined a team contest alone moves by handle.
  page.once('dialog', dialog => void dialog.accept())
  await table.getByRole('button', { name: 'erinをオンサイト参加にする', exact: true }).click()
  await expect(divisions.nth(1)).toHaveText('オンサイト')
  // Renaming happens in the row, and focus stays there.
  await table.getByRole('button', { name: 'Alphaの名前を変更' }).click()
  await expect(table.getByRole('textbox', { name: 'Alphaの新しいチーム名' })).toBeFocused()
  await table.getByRole('button', { name: 'キャンセル', exact: true }).click()
  await expect(table.getByRole('button', { name: 'Alphaの名前を変更' })).toBeFocused()
  await table.getByRole('button', { name: 'Alphaの名前を変更' }).press('Enter')
  await table.getByRole('textbox', { name: 'Alphaの新しいチーム名' }).fill('Alpha Prime')
  await table.getByRole('button', { name: '保存', exact: true }).click()
  await expect(table.locator('tbody tr').first().locator('strong')).toHaveText('Alpha Prime')
  await expect(tools.getByRole('status')).toHaveText('チーム名を「Alpha Prime」に変更しました。')
  await expect(table.getByRole('button', { name: 'Alpha Primeの名前を変更' })).toBeFocused()
  expect(requests).toEqual([
    'GET onsite', 'GET participants', 'PUT onsite {"passphrase":"abcd"}', 'PUT onsite {"passphrase":"Second_Venue"}',
    'PUT onsite {"onsite":true}', 'PUT onsite {"onsite":true}', 'PUT erin {"onsite":true}', 'PUT name {"name":"Alpha Prime"}',
  ])
  await noHorizontalScroll(page)
  await page.setViewportSize({ width: 375, height: 900 })
  await tools.screenshot({ path: test.info().outputPath('setter-tools-375.png') })
  await page.setViewportSize({ width: 1280, height: 900 })
  // After the end of a frozen contest the passphrase stays as it is, but setters move open entrants and fix names until
  // the reveal.
  rows = entrants().map(row => row.team?.id === alpha ? { ...row, team: { ...row.team, name: 'Alpha Prime' } } : row)
  state.contest = { ...contest, status: 'ended', resultsFinal: false, freezeMinutes: 30 }
  await menu(page, 'standings')
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await menu(page, 'overview')
  await tools.locator('summary').click()
  await expect(venue.getByRole('button', { name: '差し替える', exact: true })).toHaveCount(0)
  await expect(table.getByRole('button', { name: 'Alpha Primeの名前を変更' })).toBeVisible()
  await expect(betaButton).toBeEnabled()
  // Once the results are final nothing moves and nothing is renamed.
  state.contest = { ...contest, status: 'ended', resultsFinal: true }
  await menu(page, 'standings')
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await menu(page, 'overview')
  await tools.locator('summary').click()
  await expect(table.getByRole('button', { name: /をオンサイト参加にする$/ })).toHaveCount(0)
  await expect(divisions).toHaveText(['オンサイト', 'オープン', 'オープン'])
  await expect(table.getByRole('button', { name: /の名前を変更/ })).toHaveCount(0)
  await expect(venue.getByRole('button', { name: '差し替える', exact: true })).toHaveCount(0)
})

test('setters of an individual contest move participants onsite by handle, and others see no tools', async ({ page }) => {
  const contest = { ...base, status: 'running' as const, setter: true, staff: true, official: false, onsiteDivision: true, resultsFinal: false }
  const state = await routeContest(page, contest)
  const switched: string[] = []
  await page.route(`**/api/my/contests/${id}/onsite`, route => route.fulfill({ json: { onsiteDivision: true, passphrase: 'Venue_2026' } }))
  await page.route(`**/api/my/contests/${id}/participants**`, route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { items: [{ handle: 'bob', team: null, onsite: false, onsiteAt: null, onsiteVia: '', createdAt: '2099-09-30T12:00:00Z' }] } })
    switched.push(`${new URL(route.request().url()).pathname.split('/').slice(-2).join('/')} ${route.request().postData()}`)
    return route.fulfill({ json: { handle: 'bob', team: null, onsite: true, onsiteAt: '2099-10-01T03:40:00Z', onsiteVia: 'setter', createdAt: '2099-09-30T12:00:00Z' } })
  })
  await open(page)
  await page.locator('details.setter-tools summary').click()
  // An individual contest has no teams to mention.
  await expect(page.locator('.setter-tools-body > p').first()).toHaveText('会場の合言葉は終了まで差し替えられます。オープン参加の人は、結果が確定するまでオンサイト参加にできます。オープン参加には戻せません。')
  await expect(page.getByRole('heading', { name: '参加者 1 人' })).toBeVisible()
  const table = page.getByRole('region', { name: '参加者の一覧' })
  await expect(table.locator('thead th').first()).toHaveText('参加者')
  await expect(table.locator('tbody th')).toHaveText('bob')
  page.once('dialog', dialog => void dialog.accept())
  await table.getByRole('button', { name: 'bobをオンサイト参加にする' }).click()
  await expect(table.locator('tbody td').first()).toHaveText('オンサイト')
  await expect(table.getByRole('button', { name: 'bobをオンサイト参加にする' })).toHaveCount(0)
  expect(switched).toEqual(['bob/onsite {"onsite":true}'])
  state.contest = { ...contest, setter: false, staff: false, official: true }
  await menu(page, 'standings')
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await menu(page, 'overview')
  await expect(page.locator('details.setter-tools')).toHaveCount(0)
})

const teamRows: Standing[] = [
  { rank: 1, overallRank: 1, handle: '', team: { id: alpha, name: 'Alpha', members: ['bob', 'carol'], onsite: false }, onsite: false, points: 100, timeMs: 600000, problems: { [problemId]: { points: 100, wrong: 0, pending: 0, frozen: 0, acceptedAt: '2026-09-15T12:10:00Z' } } },
  { rank: 2, overallRank: 2, handle: '', team: { id: beta, name: 'Beta', members: ['dave'], onsite: true }, onsite: true, points: 100, timeMs: 900000, problems: { [problemId]: { points: 100, wrong: 1, pending: 0, frozen: 0, acceptedAt: '2026-09-15T12:15:00Z' } } },
  { rank: 3, overallRank: 3, handle: 'erin', team: null, onsite: true, points: 0, timeMs: 0, problems: {} },
]
const onsiteRows = teamRows.filter(row => row.onsite).map((row, index) => ({ ...row, rank: index + 1 }))

test('team standings name each team with its members beside those who joined alone, mark onsite rows, and keep the onsite table in the URL', async ({ page }) => {
  const contest = { ...base, status: 'ended' as const, startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', teamMode: true, onsiteDivision: true, canViewSubmissions: true, resultsFinal: true }
  const state = await routeContest(page, contest, { rows: query => query.get('scope') === 'onsite' ? onsiteRows : teamRows })
  await open(page, 'standings')
  const table = page.locator('.standings-table')
  await expect(table.locator('thead th').nth(1)).toHaveText('チーム・個人')
  await expect(table.locator('tbody tr')).toHaveCount(3)
  // Someone who joined alone has a row of their own, named by the handle.
  await expect(table.locator('tbody tr').nth(2).locator('th')).toHaveText('erinオンサイト')
  const alphaRow = table.locator('tbody tr').filter({ hasText: 'Alpha' })
  await expect(alphaRow.locator('.standing-team')).toHaveText('Alpha')
  await expect(alphaRow.locator('.standing-members')).toHaveText('bob、carol')
  await expect(alphaRow.locator('.standing-onsite')).toHaveCount(0)
  await expect(table.locator('tbody tr').filter({ hasText: 'Beta' }).locator('.standing-onsite')).toHaveText('オンサイト')
  await expect(table.locator('tfoot tr').first().locator('td').first()).toContainText('Alpha')
  await expect(table.locator('tfoot th')).toHaveText(['FA（初正解）', '正解数 / 提出数（チーム・個人）'])
  await expect(table.locator('tfoot tr').nth(1).locator('td')).toHaveText(['2 / 2'])
  const onsite = page.getByRole('switch', { name: 'オンサイトのみ表示' })
  await expect(onsite).not.toBeChecked()
  await onsite.check()
  await expect(page).toHaveURL(/[?&]view=standings&scope=onsite$/)
  await expect(table.locator('tbody tr')).toHaveCount(2)
  // The rank is among the onsite rows; the whole table's rank stands beside it, and FA and counts follow the rows shown.
  await expect(table.locator('tbody tr').first().locator('td').first()).toHaveText('1全体 2 位')
  await expect(table.locator('.standing-onsite')).toHaveCount(0)
  await expect(table.locator('tfoot th')).toHaveText(['FA（初正解・オンサイト）', '正解数 / 提出数（チーム・個人、オンサイト）'])
  await expect(table.locator('tfoot tr').first().locator('td').first()).toContainText('Beta')
  await expect(table.locator('tfoot tr').nth(1).locator('td')).toHaveText(['1 / 1'])
  await expect(page.getByText('オンサイト参加のチームと個人だけを、振り直した順位で表示しています。小さい数字は全体の順位です。')).toBeVisible()
  expect(state.standings.at(-1)!.get('scope')).toBe('onsite')
  // A link keeps the filter, and the filter keeps the server's choice of the frozen or live table separate.
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(onsite).toBeChecked()
  await expect(table.locator('tbody tr')).toHaveCount(2)
  await noHorizontalScroll(page)
  await page.setViewportSize({ width: 375, height: 900 })
  await page.locator('#standings').screenshot({ path: test.info().outputPath('onsite-standings-375.png') })
  await page.setViewportSize({ width: 1280, height: 900 })
  await onsite.uncheck()
  await expect(page).toHaveURL(/[?&]view=standings$/)
  await expect(table.locator('tbody tr')).toHaveCount(3)
  await page.locator('#standings').screenshot({ path: test.info().outputPath('team-standings-1280.png') })
})

test('individual standings keep handles and mark onsite rows; contests without a division offer no filter', async ({ page }) => {
  const rows: Standing[] = [
    { rank: 1, overallRank: 1, handle: 'bob', team: null, onsite: true, points: 100, timeMs: 600000, problems: {} },
    { rank: 2, overallRank: 2, handle: 'carol', team: null, onsite: false, points: 0, timeMs: 0, problems: {} },
  ]
  const state = await routeContest(page, { ...base, status: 'running', onsiteDivision: true }, { rows })
  await open(page, 'standings')
  const table = page.locator('.standings-table')
  await expect(table.locator('thead th').nth(1)).toHaveText('ユーザー')
  await expect(table.locator('tbody th')).toHaveText(['bobオンサイト', 'carol'])
  await expect(table.locator('tfoot th').nth(1)).toHaveText('正解者数 / 提出者数')
  state.contest = { ...base, status: 'running' }
  await page.goto(`/contests/${shortId(id)}?view=standings&scope=onsite`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.getByRole('switch', { name: 'オンサイトのみ表示' })).toHaveCount(0)
  // Without a division the filter in the URL is ignored.
  expect(state.standings.at(-1)!.get('scope')).toBeNull()
})

test('the contest submission list filters onsite participants in the URL, and team members see their team\'s while it runs', async ({ page }) => {
  const contest = { ...base, status: 'ended' as const, startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', onsiteDivision: true, canViewSubmissions: true, resultsFinal: true }
  const state = await routeContest(page, contest, { items: [submission('aaaaaaaa-0000-4000-8000-000000000001', 'bob')] })
  await open(page, 'submissions')
  const onsite = page.getByRole('switch', { name: 'オンサイトのみ表示' })
  await expect(onsite).not.toBeChecked()
  await expect(page.locator('#submissions tbody tr')).toHaveCount(1)
  expect(state.lists.at(-1)!.query.get('scope')).toBeNull()
  await onsite.check()
  await expect(page).toHaveURL(/[?&]view=submissions&scope=onsite$/)
  await expect.poll(() => state.lists.at(-1)!.query.get('scope')).toBe('onsite')
  expect(state.lists.at(-1)!.query.get('offset')).toBe('0')
  await noHorizontalScroll(page)
  // While the contest runs a team member reads the team's list, which has nothing to filter, and the whole list stays
  // closed (ADR 0030).
  const team = { id: alpha, name: 'Alpha', members: ['ui_test', 'bob'], onsite: false }
  state.contest = { ...contest, status: 'running', resultsFinal: false, endsAt: '2099-09-15T14:00:00Z', teamMode: true, participating: true, team }
  await reread(page, 'my-submissions')
  await expect(page.getByText('このコンテストの全問題への、チームの提出です。練習提出も掲載します。')).toBeVisible()
  await expect(page.getByRole('switch', { name: 'オンサイトのみ表示' })).toHaveCount(0)
  await expect.poll(() => state.lists.at(-1)!.query.get('mine')).toBe('team')
  expect(state.lists.at(-1)!.query.get('scope')).toBeNull()
  await expect(page.locator('#my-submissions tbody tr a').first()).toHaveAttribute('href', `/contests/${shortId(id)}/submissions/${shortId('aaaaaaaa-0000-4000-8000-000000000001')}?from=contest-team`)
  await visit(page, `/contests/${shortId(id)}?view=submissions`)
  await expect(page.locator('#submissions .notice')).toHaveText('提出はコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。開催中のチームの提出は「チームの提出」で確認できます。')
  // In a frozen team contest the note names teammates among those who see the frozen submissions.
  state.contest = { ...state.contest, status: 'ended', endsAt: '2026-09-15T14:00:00Z', freezeMinutes: 30, freezeAt: '2026-09-15T13:30:00Z' }
  await reread(page, 'submissions')
  await expect(page.locator('#submissions > p.muted').first()).toContainText('以降の提出は、結果の公開まで本人とチームメイト、スタッフだけが見られます。')
  // Outside the team the list stays closed during the contest.
  state.contest = { ...state.contest, status: 'running', endsAt: '2099-09-15T14:00:00Z', participating: false, team: null, canViewSubmissions: false, freezeMinutes: null, freezeAt: null }
  await reread(page, 'standings')
  await visit(page, `/contests/${shortId(id)}?view=submissions`)
  await expect(page.locator('#submissions .notice')).toHaveText('提出はコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。')
})

test('the problem page lists the team\'s submissions with their submitters and filters all submissions by onsite', async ({ page }) => {
  const team = { id: alpha, name: 'Alpha', members: ['ui_test', 'bob'], onsite: false }
  const contest = { ...base, status: 'running' as const, startsAt: '2026-09-15T12:00:00Z', endsAt: '2099-09-15T14:00:00Z', teamMode: true, onsiteDivision: true, participating: true, canViewSubmissions: true, team }
  const own = 'aaaaaaaa-0000-4000-8000-000000000001', bobs = 'bbbbbbbb-0000-4000-8000-000000000002'
  const state = await routeContest(page, contest, { items: [submission(bobs, 'bob', '2026-09-15T12:40:00Z'), submission(own, 'ui_test')] })
  await page.goto(`/contests/${shortId(id)}/A?view=my-submissions`, { waitUntil: 'networkidle' })
  const menuBar = page.getByRole('navigation', { name: '問題メニュー' })
  await expect(menuBar.getByRole('link')).toHaveText(['問題文', 'チームの提出'])
  const list = page.getByRole('region', { name: 'チームの提出' })
  await expect(list.getByRole('heading', { name: 'チームの提出' })).toBeVisible()
  await expect(list.locator('tbody tr')).toHaveCount(2)
  await expect.poll(() => state.lists.at(-1)!.query.get('mine')).toBe('team')
  expect(state.lists.at(-1)!.path).toBe(`/api/contests/${id}/problems/${problemId}/submissions`)
  // A teammate's submission opens on the shared page; one's own keeps the personal page.
  await expect(list.locator('tbody tr').nth(0).getByRole('cell').nth(1)).toHaveText('bob')
  await expect(list.locator('tbody tr').nth(0).getByRole('link', { name: '詳細' })).toHaveAttribute('href', `/contests/${shortId(id)}/submissions/${shortId(bobs)}?from=team`)
  await expect(list.locator('tbody tr').nth(1).getByRole('link', { name: '詳細' })).toHaveAttribute('href', `/contests/${shortId(id)}/my-submissions/${shortId(own)}?from=team`)
  // Either way the breadcrumb names the list it came from.
  await page.route(`**/api/my/submissions/${own}`, route => route.fulfill({ json: { ...submission(own, 'ui_test'), contestId: id, source: 'int main() {}' } }))
  await list.locator('tbody tr').nth(1).getByRole('link', { name: '詳細' }).click()
  await expect(page).toHaveURL(`/contests/${shortId(id)}/my-submissions/${shortId(own)}?from=team`)
  const crumb = page.getByRole('navigation', { name: 'パンくずリスト' }).getByRole('link', { name: /^この問題の/ })
  await expect(crumb).toHaveText('この問題のチームの提出')
  await expect(crumb).toHaveAttribute('href', `/contests/${shortId(id)}/problems/${shortId(problemId)}?view=my-submissions`)
  await page.goBack()
  // While the contest runs, everyone else's submissions stay closed to a member, and a link to them says so.
  await visit(page, `/contests/${shortId(id)}/A?view=submissions`)
  await expect(page.locator('.submission-notice')).toHaveText('提出はコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。開催中のチームの提出は「チームの提出」で確認できます。')
  // After the end the list opens with the onsite filter in the URL.
  state.contest = { ...contest, status: 'ended', endsAt: '2026-09-15T14:00:00Z', resultsFinal: true }
  await page.goto(`/contests/${shortId(id)}/A?view=submissions&scope=onsite`, { waitUntil: 'networkidle' })
  const onsite = page.getByRole('switch', { name: 'オンサイトのみ表示' })
  await expect(onsite).toBeChecked({ timeout: 20000 })
  await expect.poll(() => state.lists.at(-1)!.query.get('scope')).toBe('onsite')
  expect(state.lists.at(-1)!.query.get('mine')).toBe('0')
  await onsite.uncheck()
  await expect(page).toHaveURL(/[?&]view=submissions$/)
  await expect.poll(() => state.lists.at(-1)!.query.get('scope')).toBeNull()
  await noHorizontalScroll(page)
})

test('the web server forwards the participation body and the team and setter routes with their checks', async ({ request }) => {
  const contest = '66666666-6666-4666-8666-666666666666'
  const session = { Cookie: 'openoj_access=valid-access', Origin: 'https://judge.example' }
  const echo = async (response: Awaited<ReturnType<typeof request.post>>) => { expect(response.status()).toBe(200); return await response.json() as { path: string, method: string, type: string, body: string } }
  // No body joins as before; a JSON object is passed on as it is.
  expect(await echo(await request.post(`/api/my/contests/${contest}/participation`, { headers: session }))).toMatchObject({ path: `/my/contests/${contest}/participation`, method: 'POST', body: '' })
  const joined = await echo(await request.post(`/api/my/contests/${contest}/participation`, { headers: session, data: { team: { action: 'join', code: 'ABCDEFGHJK' }, onsitePassphrase: 'Venue' } }))
  expect(JSON.parse(joined.body)).toEqual({ team: { action: 'join', code: 'ABCDEFGHJK' }, onsitePassphrase: 'Venue' })
  expect(joined.type).toContain('application/json')
  expect((await request.post(`/api/my/contests/${contest}/participation`, { headers: { ...session, 'Content-Type': 'text/plain' }, data: 'team' })).status()).toBe(415)
  expect((await request.post(`/api/my/contests/${contest}/participation`, { headers: session, data: [1] })).status()).toBe(400)
  expect((await request.post(`/api/my/contests/${contest}/participation`, { headers: session, data: { onsitePassphrase: 'x'.repeat(5000) } })).status()).toBe(413)
  expect((await request.post(`/api/my/contests/${contest}/participation`, { headers: { ...session, Origin: 'https://attacker.example' }, data: {} })).status()).toBe(403)
  expect((await request.post(`/api/my/contests/${contest}/participation`, { headers: { Origin: 'https://judge.example' }, data: {} })).status()).toBe(401)
  // Member routes.
  expect(await echo(await request.get(`/api/my/contests/${contest}/team`, { headers: session }))).toMatchObject({ path: `/my/contests/${contest}/team`, method: 'GET' })
  expect(await echo(await request.put(`/api/my/contests/${contest}/team/code`, { headers: session }))).toMatchObject({ path: `/my/contests/${contest}/team/code`, method: 'PUT' })
  expect(JSON.parse((await echo(await request.put(`/api/my/contests/${contest}/team/name`, { headers: session, data: { name: 'Alpha' } }))).body)).toEqual({ name: 'Alpha' })
  expect((await request.put(`/api/my/contests/${contest}/team/name`, { headers: session, data: { name: 'x'.repeat(3000) } })).status()).toBe(413)
  expect((await request.put(`/api/my/contests/${contest}/team/name`, { headers: { ...session, Origin: 'https://attacker.example' }, data: { name: 'Alpha' } })).status()).toBe(403)
  expect((await request.delete(`/api/my/contests/${contest}/team/members/bob`, { headers: session })).status()).toBe(204)
  expect((await request.delete(`/api/my/contests/${contest}/team/members/Bob!`, { headers: session })).status()).toBe(404)
  expect((await request.get(`/api/my/contests/not-a-contest/team`, { headers: session })).status()).toBe(404)
  expect((await request.get(`/api/my/contests/${contest}/team`)).status()).toBe(401)
  // Setter routes.
  expect(await echo(await request.get(`/api/my/contests/${contest}/onsite`, { headers: session }))).toMatchObject({ path: `/my/contests/${contest}/onsite`, method: 'GET' })
  expect(JSON.parse((await echo(await request.put(`/api/my/contests/${contest}/onsite`, { headers: session, data: { passphrase: 'Second' } }))).body)).toEqual({ passphrase: 'Second' })
  expect((await request.post(`/api/my/contests/${contest}/onsite`, { headers: session, data: {} })).status()).toBe(405)
  expect((await request.put(`/api/my/contests/${contest}/onsite`, { headers: { ...session, 'Content-Type': 'text/plain' }, data: 'Second' })).status()).toBe(415)
  expect(await echo(await request.get(`/api/my/contests/${contest}/participants`, { headers: session }))).toMatchObject({ path: `/my/contests/${contest}/participants`, method: 'GET' })
  expect(JSON.parse((await echo(await request.put(`/api/my/contests/${contest}/participants/bob/onsite`, { headers: session, data: { onsite: true } }))).body)).toEqual({ onsite: true })
  expect((await request.put(`/api/my/contests/${contest}/participants/B/onsite`, { headers: session, data: { onsite: true } })).status()).toBe(404)
  expect(JSON.parse((await echo(await request.put(`/api/my/contests/${contest}/teams/${alpha}/onsite`, { headers: session, data: { onsite: true } }))).body)).toEqual({ onsite: true })
  expect(JSON.parse((await echo(await request.put(`/api/my/contests/${contest}/teams/${alpha}/name`, { headers: session, data: { name: 'Beta' } }))).body)).toEqual({ name: 'Beta' })
  expect((await request.put(`/api/my/contests/${contest}/teams/not-a-team/name`, { headers: session, data: { name: 'Beta' } })).status()).toBe(404)
  expect((await request.put(`/api/my/contests/${contest}/teams/${alpha}/onsite`, { headers: { Origin: 'https://judge.example' }, data: { onsite: true } })).status()).toBe(401)
})

const editorId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
const draftResult = (draft: Record<string, unknown>, issues: string[] = []) => ({
  id: editorId, version: 2, updatedAt: '2026-10-01T00:00:00Z', issues, authors: ['ui_test'], owner: true, problems: [],
  draft: { title: 'チーム戦の下書き', description: '', startsAt: null, durationMinutes: 120, penaltyMinutes: 5, problems: [], ...draft },
})

test('the editor sets team mode, a team size and the onsite division with switches that reveal their inputs', async ({ page }, testInfo) => {
  const saved: Record<string, unknown>[] = []
  await page.route('**/api/my/contests/*/draft', route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ status: 404, json: {} })
    const body = route.request().postDataJSON()
    saved.push(body.draft)
    return route.fulfill({ json: { ...draftResult(body.draft, ['start_missing']), version: body.version + 1 } })
  })
  await page.goto('/my/contests/new')
  await page.getByRole('navigation', { name: 'コンテスト作成メニュー' }).getByRole('button', { name: '参加形式', exact: true }).click()
  const mode = page.getByRole('group', { name: '個人戦・チーム戦' })
  await expect(mode.getByRole('radio', { name: '個人戦' })).toBeChecked()
  const size = page.getByRole('switch', { name: '人数の上限を設ける', exact: true })
  const sizeInput = page.getByLabel('1チームの人数の上限（人）', { exact: true })
  const onsite = page.getByRole('switch', { name: 'オンサイト区分を設ける', exact: true })
  const passphrase = page.getByLabel('会場の合言葉', { exact: true })
  await expect(size).toHaveCount(0)
  await expect(onsite).not.toBeChecked()
  await expect(onsite).toHaveAccessibleDescription('会場の合言葉を入れて参加した人をオンサイト参加とし、順位表と「すべての提出」で絞り込めるようにします。')
  await expect(passphrase).toHaveCount(0)
  const section = page.locator('[data-section="participation"]')
  await section.screenshot({ path: testInfo.outputPath('participation-off.png') })
  await mode.getByRole('radio', { name: 'チーム戦' }).check()
  await expect(size).not.toBeChecked()
  await expect(size).toHaveAccessibleDescription('オフのときも、1チーム50人までです。')
  await expect(sizeInput).toHaveCount(0)
  await size.check()
  await expect(sizeInput).toHaveValue('3')
  await onsite.check()
  // Turning the switch on fills in a passphrase to read out at the venue, which meets the rule; the setter may change it.
  await expect(passphrase).toHaveValue(/^[a-hjkmnp-tv-z2-9]{8}$/)
  expect(await validation(passphrase)).toBe('')
  await expect(passphrase).toHaveAccessibleDescription('英数字とアンダースコア（_）で4〜64文字で、大文字と小文字は区別します。作成者だけが見られます。会場の参加者に配ってください。')
  for (const [name, value] of [['autocomplete', 'off'], ['autocapitalize', 'off'], ['spellcheck', 'false']] as const) await expect(passphrase).toHaveAttribute(name, value)
  await passphrase.fill('Venue_2026')
  // Each switch is a track and thumb beside its label, not a block label over a bare checkbox.
  for (const name of ['人数の上限を設ける', 'オンサイト区分を設ける']) {
    const label = page.locator('.settings-switch').filter({ hasText: name })
    const track = await label.locator('.switch-track').boundingBox(), box = await label.boundingBox()
    expect(track!.width).toBe(34)
    expect(box!.height).toBeGreaterThanOrEqual(44)
    expect(Math.abs(track!.y + track!.height / 2 - (box!.y + box!.height / 2))).toBeLessThan(2)
    expect(box!.width).toBeLessThan((await section.boundingBox())!.width / 2)
  }
  await section.screenshot({ path: testInfo.outputPath('participation-on.png') })
  const save = page.getByRole('button', { name: '下書きを保存' })
  await save.click()
  await expect.poll(() => saved.length).toBe(1)
  await sizeInput.fill('')
  await save.click()
  await expect.poll(() => saved.length).toBe(2)
  await size.uncheck()
  await onsite.uncheck()
  await save.click()
  await expect.poll(() => saved.length).toBe(3)
  await mode.getByRole('radio', { name: '個人戦' }).check()
  await save.click()
  await expect.poll(() => saved.length).toBe(4)
  const fields = (draft: Record<string, unknown>) => [draft.teamMode, draft.maxTeamSize, draft.onsiteDivision, draft.onsitePassphrase]
  // A switch left on without a whole number keeps the setting as 0, so the draft lists the issue instead of dropping it.
  expect(saved.map(fields)).toEqual([[true, 3, true, 'Venue_2026'], [true, 0, true, 'Venue_2026'], [true, null, false, null], [false, null, false, null]])
  await mode.getByRole('radio', { name: 'チーム戦' }).check()
  await size.check()
  await onsite.check()
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await expect(passphrase).toBeVisible()
  }
  await page.setViewportSize({ width: 375, height: 900 })
  await section.screenshot({ path: testInfo.outputPath('participation-375.png') })
})

test('a draft reopens with its participation settings and points to an invalid size or passphrase', async ({ page }) => {
  let draft = draftResult({ teamMode: true, maxTeamSize: 80, onsiteDivision: true, onsitePassphrase: 'ab' }, ['start_missing', 'team_size_invalid', 'venue_passphrase_invalid'])
  const saved: Record<string, unknown>[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${editorId}/draft`, route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: draft })
    const body = route.request().postDataJSON()
    saved.push(body.draft)
    draft = { ...draftResult(body.draft, ['start_missing']), version: body.version + 1 }
    return route.fulfill({ json: draft })
  })
  await page.goto(`/my/contests/${shortId(editorId)}?section=manage`)
  const issues = page.getByRole('list', { name: '足りない項目' })
  await expect(issues.getByRole('listitem')).toHaveText([
    /開始日時を入力してください/,
    'チームの人数の上限は2〜50人の整数で入力するか、上限をオフにしてください参加形式を開く',
    '会場の合言葉は英数字とアンダースコア（_）の4〜64文字で入力するか、オンサイト区分をオフにしてください参加形式を開く',
  ])
  await issues.getByRole('listitem').filter({ hasText: '会場の合言葉' }).getByRole('button', { name: '参加形式を開く' }).click()
  await expect(page.getByRole('button', { name: '参加形式', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('group', { name: '個人戦・チーム戦' }).getByRole('radio', { name: 'チーム戦' })).toBeChecked()
  await expect(page.getByRole('switch', { name: '人数の上限を設ける', exact: true })).toBeChecked()
  await expect(page.getByLabel('1チームの人数の上限（人）', { exact: true })).toHaveValue('80')
  await expect(page.getByRole('switch', { name: 'オンサイト区分を設ける', exact: true })).toBeChecked()
  await expect(page.getByLabel('会場の合言葉', { exact: true })).toHaveValue('ab')
  // Opening the draft is not an edit.
  await expect(page.locator('.draft-status')).not.toContainText('未保存の変更があります')
  await page.getByLabel('1チームの人数の上限（人）', { exact: true }).fill('4')
  await page.getByLabel('会場の合言葉', { exact: true }).fill('Venue_2026')
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect(page.locator('.readiness-badge')).toHaveText('×公開できません（1 項目）')
  expect(saved.map(d => [d.teamMode, d.maxTeamSize, d.onsiteDivision, d.onsitePassphrase])).toEqual([[true, 4, true, 'Venue_2026']])
})

test('a published contest loads the passphrase for setters, sends every setting, and explains the team locks', async ({ page }) => {
  let contest: Contest = {
    ...base, title: '公開済みのチーム戦', author: 'ui_test', authors: ['ui_test'], startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T02:00:00Z', version: 3,
    canEdit: true, staff: true, setter: true, owner: true, official: false, canViewSubmissions: true, teamMode: true, maxTeamSize: 4, onsiteDivision: true,
    problems: [{ id: '11111111-aaaa-4aaa-8aaa-000000000001', title: '自分の問題', points: 100 }],
  }
  const bodies: Record<string, unknown>[] = []
  let refusal = ''
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${id}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${id}/onsite`, route => route.fulfill({ json: { onsiteDivision: true, passphrase: 'Venue_2026' } }))
  await page.route(`**/api/my/contests/${id}`, route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: contest })
    bodies.push(route.request().postDataJSON())
    if (refusal) { const code = refusal; refusal = ''; return route.fulfill({ status: 409, json: { statusCode: 409, data: { code } } }) }
    contest = { ...contest, version: contest.version + 1 }
    return route.fulfill({ json: contest })
  })
  await page.goto(`/my/contests/${shortId(id)}`)
  await page.getByRole('button', { name: '参加形式', exact: true }).click()
  await expect(page.getByLabel('1チームの人数の上限（人）', { exact: true })).toHaveValue('4')
  await expect(page.getByLabel('会場の合言葉', { exact: true })).toHaveValue('Venue_2026')
  const save = page.getByRole('button', { name: '変更を保存' })
  refusal = 'contest_team_mode_locked'
  await page.getByRole('group', { name: '個人戦・チーム戦' }).getByRole('radio', { name: '個人戦' }).check()
  await save.click()
  await expect(page.getByRole('alert')).toHaveText('参加登録があるため、個人戦とチーム戦は切り替えられません。')
  await page.getByRole('group', { name: '個人戦・チーム戦' }).getByRole('radio', { name: 'チーム戦' }).check()
  refusal = 'contest_team_size_locked'
  await page.getByLabel('1チームの人数の上限（人）', { exact: true }).fill('2')
  await save.click()
  await expect(page.getByRole('alert')).toHaveText('人数の上限は、今いちばん人数の多いチームより少なくできません。')
  // The browser stops a size outside 2 to 50 before anything is sent.
  await page.getByLabel('1チームの人数の上限（人）', { exact: true }).fill('51')
  await save.click()
  expect(await page.getByLabel('1チームの人数の上限（人）', { exact: true }).evaluate((input: HTMLInputElement) => input.validity.rangeOverflow)).toBe(true)
  await page.getByLabel('1チームの人数の上限（人）', { exact: true }).fill('5')
  // The server checks the passphrase after trimming and folding it, and so does the page, before anything is sent: a
  // passphrase from another section opens 参加形式 and names the rule.
  const passphrase = page.getByLabel('会場の合言葉', { exact: true })
  for (const value of ['ab  ', 'Venue-2026']) {
    await passphrase.fill(value)
    await page.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
    await save.click()
    await expect(page.getByRole('button', { name: '参加形式', exact: true })).toHaveAttribute('aria-current', 'page')
    expect(await validation(passphrase)).toBe(ruleMessage)
  }
  expect(bodies).toHaveLength(2)
  // Full-width characters and spaces around them are what the server folds, so they are sent as typed.
  await passphrase.fill(' Ｖｅｎｕｅ＿２０２６ ')
  await save.click()
  await expect(page).toHaveURL(`/contests/${shortId(id)}`)
  const fields = (body: Record<string, unknown>) => [body.teamMode, body.maxTeamSize, body.onsiteDivision, body.onsitePassphrase]
  expect(bodies.map(fields)).toEqual([[false, null, true, 'Venue_2026'], [true, 2, true, 'Venue_2026'], [true, 5, true, ' Ｖｅｎｕｅ＿２０２６ ']])
})

test('the editor of a started contest leaves the passphrase to the setter tools instead of showing an empty field', async ({ page }) => {
  const contest: Contest = {
    ...base, title: '開催中のチーム戦', author: 'ui_test', authors: ['ui_test'], status: 'running', startsAt: '2026-10-01T00:00:00Z',
    canEdit: false, staff: true, setter: true, owner: true, official: false, canViewSubmissions: true, teamMode: true, onsiteDivision: true, resultsFinal: false,
    problems: [{ id: '11111111-aaaa-4aaa-8aaa-000000000001', title: '自分の問題', points: 100 }],
  }
  const venue: string[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${id}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${id}/onsite`, route => { venue.push(route.request().method()); return route.fulfill({ json: { onsiteDivision: true, passphrase: 'Venue_2026' } }) })
  await page.route(`**/api/my/contests/${id}`, route => route.fulfill({ json: contest }))
  await page.goto(`/my/contests/${shortId(id)}`)
  await page.getByRole('button', { name: '参加形式', exact: true }).click()
  const onsite = page.getByRole('switch', { name: 'オンサイト区分を設ける', exact: true })
  await expect(onsite).toBeChecked()
  await expect(onsite).toBeDisabled()
  await expect(page.getByLabel('会場の合言葉', { exact: true })).toHaveCount(0)
  await expect(page.locator('[data-section="participation"]')).toContainText('会場の合言葉は、コンテストのページの「参加者の管理」で確認できます。')
  expect(venue).toEqual([])
})
