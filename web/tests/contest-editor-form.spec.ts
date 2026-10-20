import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import type { Contest, ContestDraft } from '../shared/types/contest'
import { shortId } from '../shared/utils/short-id'

// The date input reads local time, so a fixed zone keeps the instants in the bodies literal.
test.use({ timezoneId: 'Asia/Tokyo' })

const updatedAt = '2026-10-01T00:00:00Z'
const draftId = 'eeeeeeee-0000-4000-8000-000000000001'
const contestId = 'eeeeeeee-0000-4000-8000-000000000002'
const problem = { id: '11111111-aaaa-4aaa-8aaa-0000000000e1', title: 'フォームの問題', updatedAt, readiness: { publish: [], contest: [], featured: [] } }
const draftResult = (id: string, version: number, draft: ContestDraft) => ({ id, version, updatedAt, issues: ['start_missing'], authors: ['ui_test'], owner: true, problems: [], draft })
// Serves the setter's problem and one draft (404 while there is none), and records each saved body.
async function routeDraft(page: Page, draft?: ContestDraft) {
  const bodies: { version: number, draft: ContestDraft }[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: new URL(route.request().url()).searchParams.get('role') === 'tester' ? [] : [problem], nextCursor: '' } }))
  await page.route('**/api/my/contests/*/draft', (route) => {
    const request = route.request()
    if (request.method() !== 'PUT') return draft ? route.fulfill({ json: draftResult(draftId, 1, draft) }) : route.fulfill({ status: 404, json: {} })
    const body = request.postDataJSON()
    bodies.push(body)
    return route.fulfill({ json: draftResult(new URL(request.url()).pathname.split('/')[4]!, body.version + 1, body.draft) })
  })
  return bodies
}
const contest: Contest = {
  id: contestId, title: '公開済みのフォーム', author: 'ui_test', authors: ['ui_test'], problemAuthors: [], testers: [], description: '概要',
  startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T02:00:00Z', penaltyMinutes: 5, version: 3, status: 'scheduled',
  canEdit: true, participating: false, official: false, staff: true, setter: true, owner: true, canViewSubmissions: true,
  freezeMinutes: 30, freezeAt: '2099-10-01T01:30:00Z', revealed: false, revealedAt: null, resultsFinal: false,
  teamMode: true, maxTeamSize: 4, onsiteDivision: true,
  problems: [{ id: problem.id, title: problem.title, points: 100 }],
}
// Serves the published contest with the given answer from the setters' venue route, and records each saved body.
async function routeContest(page: Page, venue: { status?: number, json: unknown }) {
  const bodies: Record<string, unknown>[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${contestId}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${contestId}/onsite`, route => route.fulfill(venue))
  await page.route(`**/api/my/contests/${contestId}`, (route) => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: contest })
    bodies.push(route.request().postDataJSON())
    return route.fulfill({ json: { ...contest, version: contest.version + 1 } })
  })
  return bodies
}
const openSection = (page: Page, name: string) => page.getByRole('button', { name, exact: true }).click()

test('a new draft sends all eleven fields, each a whole number or null', async ({ page }) => {
  const bodies = await routeDraft(page)
  await page.goto('/my/contests/new')
  await page.getByLabel('コンテストタイトル', { exact: true }).fill('フォームの往復')
  await openSection(page, '問題・配点')
  await page.getByRole('button', { name: 'フォームの問題を追加', exact: true }).click()
  const points = page.locator(`#points-${problem.id}`)
  await points.fill('250')
  await openSection(page, '期間・ペナルティ')
  const start = page.getByLabel('開始日時', { exact: true })
  const duration = page.getByLabel('コンテスト時間（分）', { exact: true })
  const penalty = page.getByLabel('誤答ペナルティ（分）', { exact: true })
  await start.fill('2099-10-01T09:00')
  await duration.fill('90')
  await penalty.fill('0')
  await page.getByRole('switch', { name: '順位表の凍結を行う', exact: true }).check()
  await page.getByLabel('凍結する時間（終了前の分）', { exact: true }).fill('15')
  await openSection(page, '参加形式')
  await page.getByRole('group', { name: '個人戦・チーム戦' }).getByRole('radio', { name: 'チーム戦' }).check()
  await page.getByRole('switch', { name: '人数の上限を設ける', exact: true }).check()
  await page.getByLabel('1チームの人数の上限（人）', { exact: true }).fill('4')
  await page.getByRole('switch', { name: 'オンサイト区分を設ける', exact: true }).check()
  await page.getByLabel('会場の合言葉', { exact: true }).fill('Venue_2026')
  const save = page.getByRole('button', { name: '下書きを保存' })
  await save.click()
  await expect(page.locator('.draft-status')).toContainText('公開まであと')
  const first = { title: 'フォームの往復', description: '<!-- ここにコンテストの概要を記載 -->\n', startsAt: '2099-10-01T00:00:00.000Z', durationMinutes: 90, penaltyMinutes: 0, freezeMinutes: 15, teamMode: true, maxTeamSize: 4, onsiteDivision: true, onsitePassphrase: 'Venue_2026', problems: [{ id: problem.id, points: 250 }] }
  expect(bodies).toEqual([{ version: 0, draft: first }])
  // Empty inputs and a fraction go as null, never as '' or 1.5.
  await openSection(page, '期間・ペナルティ')
  await start.fill('')
  await duration.fill('')
  await penalty.fill('1.5')
  await openSection(page, '問題・配点')
  await points.fill('')
  await save.click()
  await expect.poll(() => bodies.length).toBe(2)
  expect(bodies[1]).toEqual({ version: 1, draft: { ...first, startsAt: null, durationMinutes: null, penaltyMinutes: null, problems: [{ id: problem.id, points: null }] } })
})

test('turning a switch off is an unsaved change even when its value was already empty', async ({ page }) => {
  const bodies = await routeDraft(page)
  await page.goto('/my/contests/new')
  const status = page.locator('.draft-status')
  // The value is cleared before saving, so turning the switch off afterwards changes the switch alone.
  async function saveThenTurnOff(name: string, label: string) {
    const toggle = page.getByRole('switch', { name, exact: true })
    await toggle.check()
    await page.getByLabel(label, { exact: true }).fill('')
    await page.getByRole('button', { name: '下書きを保存' }).click()
    await expect(status).toContainText('公開まであと')
    await toggle.uncheck()
    await expect(status).toContainText('未保存の変更があります')
  }
  await openSection(page, '期間・ペナルティ')
  await saveThenTurnOff('順位表の凍結を行う', '凍結する時間（終了前の分）')
  await openSection(page, '参加形式')
  await page.getByRole('group', { name: '個人戦・チーム戦' }).getByRole('radio', { name: 'チーム戦' }).check()
  await saveThenTurnOff('人数の上限を設ける', '1チームの人数の上限（人）')
  // Turning the division on fills in a passphrase, which is cleared like the other values.
  await saveThenTurnOff('オンサイト区分を設ける', '会場の合言葉')
  expect(bodies.map(b => [b.draft.freezeMinutes, b.draft.maxTeamSize, b.draft.onsitePassphrase])).toEqual([[0, null, null], [null, 0, null], [null, null, '']])
})

test('a draft with every setting on, at 0 where it takes a number, opens unchanged', async ({ page }) => {
  await routeDraft(page, { title: '設定をすべて使う下書き', description: '', startsAt: '2099-10-01T00:00:00Z', durationMinutes: 90, penaltyMinutes: 0, freezeMinutes: 0, teamMode: true, maxTeamSize: 0, onsiteDivision: true, onsitePassphrase: 'Venue_2026', problems: [] })
  const dialogs: string[] = []
  page.on('dialog', dialog => { dialogs.push(dialog.message()); return dialog.dismiss() })
  await page.goto(`/my/contests/${shortId(draftId)}`)
  await expect(page.getByRole('button', { name: '下書きを保存' })).toBeEnabled()
  // Switches turned on by loading keep the 0 they came with, so opening the draft is not an edit.
  await expect(page.locator('.draft-status')).not.toContainText('未保存の変更があります')
  await openSection(page, '期間・ペナルティ')
  await expect(page.getByLabel('開始日時', { exact: true })).toHaveValue('2099-10-01T09:00')
  await expect(page.getByLabel('コンテスト時間（分）', { exact: true })).toHaveValue('90')
  await expect(page.getByLabel('誤答ペナルティ（分）', { exact: true })).toHaveValue('0')
  await expect(page.getByRole('switch', { name: '順位表の凍結を行う', exact: true })).toBeChecked()
  await expect(page.getByLabel('凍結する時間（終了前の分）', { exact: true })).toHaveValue('0')
  await openSection(page, '参加形式')
  await expect(page.getByRole('group', { name: '個人戦・チーム戦' }).getByRole('radio', { name: 'チーム戦' })).toBeChecked()
  await expect(page.getByRole('switch', { name: '人数の上限を設ける', exact: true })).toBeChecked()
  await expect(page.getByLabel('1チームの人数の上限（人）', { exact: true })).toHaveValue('0')
  await expect(page.getByRole('switch', { name: 'オンサイト区分を設ける', exact: true })).toBeChecked()
  await expect(page.getByLabel('会場の合言葉', { exact: true })).toHaveValue('Venue_2026')
  await page.getByRole('link', { name: '自分のコンテスト', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=contests')
  expect(dialogs).toEqual([])
})

test('a published contest opens unchanged and saves every setting as loaded', async ({ page }) => {
  const bodies = await routeContest(page, { json: { onsiteDivision: true, passphrase: 'Venue_2026' } })
  const dialogs: string[] = []
  page.on('dialog', dialog => { dialogs.push(dialog.message()); return dialog.dismiss() })
  const editor = `/my/contests/${shortId(contestId)}`
  const save = page.getByRole('button', { name: '変更を保存' })
  const title = page.getByLabel('コンテストタイトル', { exact: true })
  const leave = page.getByRole('link', { name: '自分のコンテスト', exact: true })
  await page.goto(editor)
  await expect(save).toBeEnabled()
  await leave.click()
  await expect(page).toHaveURL('/my?tab=contests')
  expect(dialogs).toEqual([])
  await page.goto(editor)
  await title.fill('公開済みのフォーム（変更）')
  await leave.click()
  await expect.poll(() => dialogs).toEqual(['未保存の変更を破棄して移動しますか？'])
  await expect(page).toHaveURL(editor)
  await title.fill('公開済みのフォーム')
  await save.click()
  await expect(page).toHaveURL(`/contests/${shortId(contestId)}`)
  expect(bodies).toEqual([{ title: '公開済みのフォーム', description: '概要', startsAt: '2099-10-01T00:00:00.000Z', endsAt: '2099-10-01T02:00:00.000Z', penaltyMinutes: 5, freezeMinutes: 30, teamMode: true, maxTeamSize: 4, onsiteDivision: true, onsitePassphrase: 'Venue_2026', version: 3, problems: [{ id: problem.id, points: 100 }] }])
})

test('a published contest whose passphrase cannot be read keeps its title and schedule beside the error', async ({ page }) => {
  await routeContest(page, { status: 500, json: {} })
  await page.goto(`/my/contests/${shortId(contestId)}`)
  await expect(page.getByRole('alert')).toHaveText('作成情報を読み込めませんでした。ログイン状態を確認して再読み込みしてください。')
  // Everything but the participation is read before the passphrase, so it stays on screen.
  await expect(page.getByLabel('コンテストタイトル', { exact: true })).toHaveValue('公開済みのフォーム')
  await openSection(page, '期間・ペナルティ')
  await expect(page.getByLabel('開始日時', { exact: true })).toHaveValue('2099-10-01T09:00')
  await expect(page.getByLabel('コンテスト時間（分）', { exact: true })).toHaveValue('120')
  await expect(page.getByRole('switch', { name: '順位表の凍結を行う', exact: true })).toBeChecked()
  await expect(page.getByLabel('凍結する時間（終了前の分）', { exact: true })).toHaveValue('30')
})

test('an invalid contest is named on the draft save, the publication and the published save', async ({ page }) => {
  const invalid = { status: 400, json: { statusCode: 400, data: { code: 'invalid_contest' } } }
  const named = '保存できない値があります。「タイトル・説明」「問題・配点」「期間・ペナルティ」「参加形式」の内容を確認してください。'
  await routeDraft(page)
  let refuseDraft = true
  await page.route('**/api/my/contests/*/draft', (route) => {
    const request = route.request()
    if (request.method() !== 'PUT') return route.fallback()
    if (refuseDraft) return route.fulfill(invalid)
    const body = request.postDataJSON()
    return route.fulfill({ json: { ...draftResult(new URL(request.url()).pathname.split('/')[4]!, body.version + 1, body.draft), issues: [] } })
  })
  let publications = 0
  await page.route('**/api/my/contests/*/publication', (route) => { publications++; return route.fulfill(invalid) })
  await page.goto('/my/contests/new')
  await page.getByLabel('コンテストタイトル', { exact: true }).fill('保存できない値')
  await openSection(page, '期間・ペナルティ')
  await page.getByLabel('開始日時', { exact: true }).fill('2099-10-01T09:00')
  await page.getByLabel('コンテスト時間（分）', { exact: true }).fill('90')
  await page.getByRole('button', { name: '下書きを保存', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText(named)
  // The publication checks the saved draft again, such as a problem without points.
  refuseDraft = false
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'コンテストを公開', exact: true }).click()
  await expect.poll(() => publications).toBe(1)
  await expect(page.getByRole('alert')).toHaveText(named)
  // A published contest names the same refusal, and a 400 without a code keeps the general message.
  await routeContest(page, { json: { onsiteDivision: true, passphrase: 'Venue_2026' } })
  let reply: { status: number, json: unknown } = invalid
  await page.route(`**/api/my/contests/${contestId}`, route => route.request().method() === 'PUT' ? route.fulfill(reply) : route.fallback())
  await page.goto(`/my/contests/${shortId(contestId)}`)
  const save = page.getByRole('button', { name: '変更を保存', exact: true })
  await save.click()
  await expect(page.getByRole('alert')).toHaveText(named)
  reply = { status: 400, json: {} }
  await save.click()
  await expect(page.getByRole('alert')).toHaveText('入力内容を確認してください。')
})

test('Ctrl+S saves the contest draft', async ({ page }) => {
  const bodies = await routeDraft(page)
  await page.goto('/my/contests/new')
  await page.getByLabel('コンテストタイトル', { exact: true }).fill('ショートカットで保存')
  await page.keyboard.press('Control+s')
  await expect(page.locator('.draft-status')).toContainText('公開まであと')
  expect(bodies).toHaveLength(1)
  expect(bodies[0]!.draft.title).toBe('ショートカットで保存')
})
