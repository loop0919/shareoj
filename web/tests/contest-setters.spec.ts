import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

// The fixture user is ui_test. alice owns the contests below unless a test says otherwise.
const id = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
const own = '11111111-aaaa-4aaa-8aaa-000000000001'
const bobs = '22222222-bbbb-4bbb-8bbb-000000000002'
const gone = '33333333-cccc-4ccc-8ccc-000000000003'
const updatedAt = '2026-10-01T00:00:00Z'
const ready = { publish: [], contest: [], featured: [] }
const token = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
const renewed = 'BCDEFGHIJKLMNOPQRSTUVWXYZ2345672'

function draftResult(authors: string[], owner: boolean) {
  return {
    id, version: 2, updatedAt, issues: ['start_missing'], authors, owner,
    draft: { title: '共同のコンテスト', description: '説明', startsAt: null, durationMinutes: 120, penaltyMinutes: 5, problems: [{ id: own, points: 100 }, { id: bobs, points: 200 }, { id: gone, points: 300 }] },
    // The server omits problems that no current setter manages, so their titles never reach the editor.
    problems: [
      { id: own, title: '自分の問題', author: 'ui_test', issues: [] },
      ...authors.includes('bob') ? [{ id: bobs, title: 'ボブの問題', author: 'bob', issues: ['editorial_missing'] }] : [],
    ],
  }
}

async function routeEditor(page: Page, draft: () => unknown) {
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: new URL(route.request().url()).searchParams.get('role') === 'tester' ? [] : [{ id: own, title: '自分の問題', updatedAt, readiness: ready }], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${id}/draft`, route => route.fulfill({ json: draft() }))
}

test('the owner sees co-setters\' problems, issues and renews the link, and removes a co-setter', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  let authors = ['ui_test', 'bob']
  await routeEditor(page, () => draftResult(authors, true))
  const requests: string[] = []
  await page.route(`**/api/my/contests/${id}/setter-invitation`, route => {
    requests.push(route.request().method())
    return route.fulfill({ json: { token: route.request().method() === 'PUT' ? renewed : token } })
  })
  await page.route(`**/api/my/contests/${id}/setters/*`, route => {
    requests.push(`${route.request().method()} ${new URL(route.request().url()).pathname.split('/').pop()}`)
    authors = ['ui_test']
    return route.fulfill({ status: 204 })
  })
  await page.goto(`/my/contests/${shortId(id)}`)
  await page.getByRole('button', { name: '問題・配点', exact: true }).click()
  // Another setter's problem keeps its title and readiness; one nobody manages says why it cannot stay.
  const selected = page.locator('.selected li')
  await expect(selected.locator('strong')).toHaveText(['自分の問題', 'ボブの問題', '表示できない問題'])
  await expect(selected.nth(1)).toContainText('作者 bob')
  await expect(selected.nth(1)).toContainText('解説がありません')
  await expect(selected.nth(0).locator('.problem-role')).toHaveCount(0)
  await expect(selected.nth(2)).toContainText('削除されたか、作成者の誰も作者・テスターではないため出題できません')
  await expect(page.getByText('ほかの作成者の問題は、その作成者が追加します。', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'コンテスト管理', exact: true }).click()
  const setters = page.getByRole('list', { name: '作成者', exact: true })
  await expect(setters.getByRole('listitem')).toHaveText([/ui_test\s*オーナー/, /bob\s*共同作成者/])
  await expect(page.getByRole('button', { name: '下書きを削除', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '共同作成者から抜ける' })).toHaveCount(0)
  await page.getByRole('button', { name: 'リンクを発行', exact: true }).click()
  const link = page.getByLabel('招待リンク', { exact: true })
  await expect(link).toHaveValue(new RegExp(`/my/contest-invitations/${token}$`))
  await page.getByRole('button', { name: 'リンクをコピー', exact: true }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(await link.inputValue())
  // Renewing asks first; a dismissed dialog keeps the link.
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('button', { name: 'リンクを作り直す', exact: true }).click()
  await expect(link).toHaveValue(new RegExp(`${token}$`))
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'リンクを作り直す', exact: true }).click()
  await expect(link).toHaveValue(new RegExp(`/my/contest-invitations/${renewed}$`))
  await expect(page.getByText('招待リンクを作り直しました。前のリンクは使えません。')).toBeVisible()
  for (const width of [320, 375, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  let message = ''
  page.once('dialog', dialog => { message = dialog.message(); void dialog.accept() })
  await page.getByRole('button', { name: 'bobを共同作成者から外す', exact: true }).click()
  await expect(setters.getByRole('listitem')).toHaveText([/ui_test\s*オーナー/])
  expect(message).toContain('外した人は、このコンテストに参加できなくなります。')
  // Removal renews the link on the server, so the old one disappears from the screen.
  await expect(link).toHaveCount(0)
  await expect(page.getByText('bobを共同作成者から外しました。招待リンクは作り直されました。')).toBeVisible()
  expect(requests).toEqual(['POST', 'PUT', 'DELETE bob'])
  // Without bob, his problem reads like any other problem nobody manages.
  await page.getByRole('button', { name: '問題・配点', exact: true }).click()
  await expect(selected.nth(1)).toContainText('削除されたか、作成者の誰も作者・テスターではないため出題できません')
})

test('a co-setter cannot delete the draft and leaves after confirming', async ({ page }) => {
  await routeEditor(page, () => draftResult(['alice', 'ui_test'], false))
  let left = ''
  await page.route(`**/api/my/contests/${id}/setters/*`, route => {
    left = `${route.request().method()} ${new URL(route.request().url()).pathname.split('/').pop()}`
    return route.fulfill({ status: 204 })
  })
  await page.goto(`/my/contests/${shortId(id)}?section=manage`)
  await expect(page.getByRole('button', { name: 'コンテスト管理', exact: true })).toHaveAttribute('aria-current', 'page')
  const setters = page.getByRole('list', { name: '作成者', exact: true })
  await expect(setters.getByRole('listitem')).toHaveText([/alice\s*オーナー/, /ui_test\s*共同作成者/])
  await expect(page.getByRole('button', { name: /を共同作成者から外す$/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '下書きを削除', exact: true })).toHaveCount(0)
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('button', { name: '共同作成者から抜ける', exact: true }).click()
  expect(left).toBe('')
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '共同作成者から抜ける', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=contests')
  await expect(page.locator('.toast').getByRole('status')).toHaveText('共同作成者から抜けました')
  expect(left).toBe('DELETE ui_test')
})

test('after the start a setter still manages co-setters while the settings stay locked', async ({ page }) => {
  const contest = {
    id, title: '開催中の共同コンテスト', author: 'ui_test', authors: ['ui_test', 'bob'], problemAuthors: [], testers: [], description: '説明',
    startsAt: '2026-10-01T00:00:00Z', endsAt: '2099-10-01T02:00:00Z', penaltyMinutes: 5, version: 3, status: 'running',
    canEdit: false, participating: false, official: false, staff: true, setter: true, owner: true, canViewSubmissions: true,
    problems: [{ id: own, title: '自分の問題', points: 100 }],
  }
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${id}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${id}`, route => route.fulfill({ json: contest }))
  await page.route(`**/api/my/contests/${id}/setter-invitation`, route => route.fulfill({ json: { token } }))
  await page.goto(`/my/contests/${shortId(id)}`)
  await expect(page.getByRole('button', { name: 'コンテスト管理', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByText('公開済み（開始後）', { exact: true })).toBeVisible()
  await expect(page.getByText('開始後は設定を変更できません。共同作成者の招待と解除はできます。')).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '変更を保存' })).toBeDisabled()
  await expect(page.locator('.draft-status')).toContainText('編集できません')
  await expect(page.getByRole('button', { name: 'bobを共同作成者から外す', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'リンクを発行', exact: true }).click()
  await expect(page.getByLabel('招待リンク', { exact: true })).toHaveValue(new RegExp(`${token}$`))
  await page.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
  await expect(page.getByLabel('誤答ペナルティ（分）')).toBeDisabled()
})

test('after the end invitations close while the owner can still remove co-setters', async ({ page }) => {
  const contest = {
    id, title: '終了した共同コンテスト', author: 'ui_test', authors: ['ui_test', 'bob'], problemAuthors: [], testers: [], description: '説明',
    startsAt: '2026-10-01T00:00:00Z', endsAt: '2026-10-01T02:00:00Z', penaltyMinutes: 5, version: 3, status: 'ended',
    canEdit: false, participating: false, official: false, staff: true, setter: true, owner: true, canViewSubmissions: true,
    problems: [{ id: own, title: '自分の問題', points: 100 }],
  }
  let issued = 0
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${id}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${id}`, route => route.fulfill({ json: contest }))
  await page.route(`**/api/my/contests/${id}/setter-invitation`, route => { issued++; return route.fulfill({ status: 409, json: { statusCode: 409, data: { code: 'contest_invitation_closed' } } }) })
  await page.goto(`/my/contests/${shortId(id)}`)
  await expect(page.getByText('結果が確定したため、共同作成者の招待はできません。解除と脱退はできます。')).toBeVisible()
  await expect(page.getByRole('button', { name: /^リンクを(発行|作り直す)$/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'bobを共同作成者から外す', exact: true })).toBeEnabled()
  // The problems were published at the end; that is no issue once the problem set is fixed.
  await page.getByRole('button', { name: '問題・配点', exact: true }).click()
  await expect(page.locator('.selected li strong')).toHaveText(['自分の問題'])
  await expect(page.locator('.selected .problem-issue')).toHaveCount(0)
  expect(issued).toBe(0)
})

test('a published contest names the problems no setter manages before saving', async ({ page }) => {
  const contest = {
    id, title: '予定の共同コンテスト', author: 'alice', authors: ['alice', 'ui_test'], problemAuthors: [], testers: [], description: '',
    startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T02:00:00Z', penaltyMinutes: 5, version: 3, status: 'scheduled',
    canEdit: true, participating: false, official: false, staff: true, setter: true, owner: false, canViewSubmissions: true,
    problems: [{ id: own, title: '自分の問題', points: 100 }, { id: gone, title: '抜けた人の問題', points: 200, unmanaged: true }],
  }
  // Problems of a published contest reach co-setters through the contest, marked with it.
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: new URL(route.request().url()).searchParams.get('role') === 'tester'
    ? [{ id: gone, title: '抜けた人の問題', updatedAt, contestId: id, viaContest: { id, title: contest.title }, readiness: ready }]
    : [{ id: own, title: '自分の問題', updatedAt, contestId: id, readiness: ready }], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${id}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${id}`, route => route.fulfill({ json: contest }))
  await page.goto(`/my/contests/${shortId(id)}`)
  await page.getByRole('button', { name: '問題・配点', exact: true }).click()
  const selected = page.locator('.selected li')
  await expect(selected.locator('strong')).toHaveText(['自分の問題', '抜けた人の問題'])
  await expect(selected.nth(1)).toContainText('作成者の誰も作者・テスターではないため、外すまで保存できません')
  await expect(selected.nth(0).locator('.problem-issue')).toHaveCount(0)
  await page.getByRole('checkbox', { name: '追加できる問題だけ表示', exact: true }).uncheck()
  // A problem reached through the contest is not one this setter tests.
  await expect(page.locator('#problem-candidates li').filter({ hasText: '抜けた人の問題' }).locator('.problem-role')).toHaveCount(0)
})

test('the invitation page previews the contest, accepts, and explains refusals', async ({ page }) => {
  const invitation = { id, title: '共同のコンテスト', author: 'alice', published: false, joined: false }
  let refusal: string | undefined
  await page.route(`**/api/my/setter-invitations/${token}`, route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: invitation })
    if (refusal) return route.fulfill({ status: 409, json: { statusCode: 409, data: { code: refusal } } })
    return route.fulfill({ json: { ...invitation, joined: true } })
  })
  await page.route(`**/api/my/setter-invitations/${renewed}`, route => route.fulfill({ status: 404, json: { statusCode: 404 } }))
  await routeEditor(page, () => draftResult(['alice', 'ui_test'], false))
  const response = await page.goto(`/my/contest-invitations/${token}`)
  expect(response?.headers()['cache-control']).toBe('no-store')
  expect(response?.headers()['referrer-policy']).toBe('no-referrer')
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content', 'noindex, nofollow')
  await expect(page.getByRole('heading', { name: '共同作成者への招待', level: 1 })).toBeVisible()
  await expect(page.getByRole('heading', { name: '共同のコンテスト', level: 2 })).toBeVisible()
  await expect(page.getByText('下書き（未公開）のコンテスト')).toBeVisible()
  await expect(page.getByText('オーナー alice')).toBeVisible()
  for (const width of [320, 375, 768]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  for (const [code, text] of [
    ['contest_participant_locked', 'このコンテストに参加登録しているため、共同作成者になれません。'],
    ['contest_invitation_closed', 'コンテストの結果が確定したため、作成者に加われません。'],
  ] as const) {
    refusal = code
    await page.getByRole('button', { name: '共同作成者になる', exact: true }).click()
    await expect(page.getByRole('alert')).toHaveText(text)
    await expect(page).toHaveURL(`/my/contest-invitations/${token}`)
  }
  refusal = undefined
  await page.getByRole('button', { name: '共同作成者になる', exact: true }).click()
  await expect(page).toHaveURL(`/my/contests/${shortId(id)}`)
  await expect(page.getByRole('heading', { name: 'コンテスト編集' })).toBeAttached()
  // A renewed or unknown link says so instead of showing an empty invitation.
  await page.goto(`/my/contest-invitations/${renewed}`)
  await expect(page.getByRole('alert')).toHaveText('招待リンクが無効か、作り直されています。コンテストの作成者に今のリンクを確認してください。')
  await expect(page.getByRole('button', { name: '共同作成者になる' })).toHaveCount(0)
})

test('a closed invitation says so instead of offering to join', async ({ page }) => {
  await page.route(`**/api/my/setter-invitations/${token}`, route => route.fulfill({ json: { id, title: '終わったコンテスト', author: 'alice', published: true, joined: false, closed: true } }))
  await page.goto(`/my/contest-invitations/${token}`)
  await expect(page.getByRole('heading', { name: '終わったコンテスト', level: 2 })).toBeVisible()
  await expect(page.getByText('コンテストの結果が確定したため、この招待は締め切られました。')).toBeVisible()
  await expect(page.getByRole('button', { name: '共同作成者になる' })).toHaveCount(0)
})

test('a setter who already joined opens the contest from the invitation', async ({ page }) => {
  await page.route(`**/api/my/setter-invitations/${token}`, route => route.fulfill({ json: { id, title: '', author: 'alice', published: true, joined: true } }))
  await page.goto(`/my/contest-invitations/${token}`)
  await expect(page.getByRole('heading', { name: '無題のコンテスト', level: 2 })).toBeVisible()
  await expect(page.getByText('公開済みのコンテスト')).toBeVisible()
  await expect(page.getByRole('button', { name: '共同作成者になる' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'コンテストを開く', exact: true })).toHaveAttribute('href', `/my/contests/${shortId(id)}`)
})

const runningContest = {
  id: '88888888-8888-4888-8888-888888888888', title: '共同開催コンテスト', author: 'alice', authors: ['alice', 'bob'], problemAuthors: ['carol'], testers: [], description: '',
  startsAt: '2026-09-15T12:00:00Z', endsAt: '2099-09-15T14:00:00Z', penaltyMinutes: 5, version: 1, status: 'running',
  canEdit: false, participating: false, canViewSubmissions: false, problems: [{ id: '11111111-1111-4111-8111-111111111111', title: 'A + B', points: 100 }],
}

for (const [role, flags, notice, participation] of [
  ['setter', { official: false, staff: true, setter: true }, 'スタッフ（作成者・作問者・テスター）としての提出は公式順位の対象外です。', 'スタッフ（作成者・作問者・テスター）は公式参加の対象外です。'],
  ['former setter', { official: false, staff: false, setter: false }, 'このコンテストのスタッフを務めたため、提出は公式順位の対象外です。', 'このコンテストのスタッフを務めたため、参加できません。'],
] as const) {
  test(`contest pages list every setter and explain the ${role}'s place`, async ({ page }) => {
    const contest = { ...runningContest, ...flags }
    await page.route(`**/api/contests/${contest.id}**`, route => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/standings')) return route.fulfill({ json: [] })
      if (path.includes('/problems/')) return route.fulfill({ json: { id: contest.problems[0]!.id, title: 'A + B', author: 'alice', authors: ['alice', 'bob'], testers: [], markdown: '足し算', editorial: '', timeLimitMs: '2000', memoryLimitMb: '512', hasSamples: false, canReadEditorial: flags.staff } })
      return route.fulfill({ json: contest })
    })
    // The server render reads the fixture API, so refresh in the browser to use the routed contest.
    await page.goto(`/contests/${shortId(contest.id)}?view=standings`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: '今すぐ更新' }).click()
    await expect(page.getByText(notice, { exact: true })).toBeVisible()
    await expect(page.getByText(participation, { exact: true })).toBeVisible()
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true }).click()
    await expect(page.locator('.problem-meta')).toContainText('作成者 alice、bob')
    await expect(page.locator('.problem-meta')).toContainText('作問者 carol')
    // Setters reach co-setter management after the start, when the settings no longer change.
    await expect(page.getByRole('link', { name: 'コンテストを編集' })).toHaveCount(0)
    const manage = page.getByRole('link', { name: '共同作成者を管理', exact: true })
    if (flags.setter) await expect(manage).toHaveAttribute('href', `/my/contests/${shortId(contest.id)}?section=manage`)
    else await expect(manage).toHaveCount(0)
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
    await page.getByRole('region', { name: 'コンテストの問題' }).getByRole('link', { name: 'A + B', exact: true }).click()
    await expect(page.locator('.problem-meta')).toContainText('作成者 alice、bob')
    // Staff test at any time; a former setter waits for the end like everyone outside the contest.
    await expect(page.getByLabel('ソースコード', { exact: true })).toHaveCount(flags.staff ? 1 : 0)
  })
}

test('my contests list every setter of a contest and the owner of each draft', async ({ page }) => {
  await page.route(url => url.pathname === '/api/my/contests/drafts', route => route.fulfill({ json: { items: [{ id, title: '共同の下書き', author: 'alice', startsAt: null, updatedAt }], hasMore: false } }))
  await page.route(url => url.pathname === '/api/my/contests', route => route.fulfill({ json: { items: [{ ...runningContest, status: 'scheduled', canEdit: true, setter: true, authors: ['alice', 'ui_test'] }], hasMore: false } }))
  await page.goto('/my?tab=contests')
  const drafts = page.getByRole('region', { name: 'コンテストの下書き' })
  await expect(drafts.getByRole('columnheader')).toHaveText(['下書き', '開始予定（日本時間）', '更新（日本時間）', 'オーナー', '操作'])
  await expect(drafts.locator('tbody tr')).toContainText('alice')
  const published = page.getByRole('region', { name: 'コンテスト一覧' })
  await expect(published.locator('tbody td').nth(3)).toHaveText('alice、ui_test')
  await expect(published.getByRole('link', { name: '共同開催コンテストを編集' })).toBeVisible()
  for (const width of [320, 375, 768]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('a co-setter notification opens the contest management', async ({ page }) => {
  const notifications = { notifications: [{ id: '55555555-5555-4555-8555-555555555555', contestId: id, title: '', actor: 'bob', kind: 'contest_setter', createdAt: '2026-10-11T00:00:00Z' }] }
  await page.route('**/api/my/notifications**', route => route.fulfill({ json: notifications }))
  await routeEditor(page, () => draftResult(['ui_test', 'bob'], true))
  await page.goto('/my?tab=events')
  const events = page.getByRole('region', { name: 'イベント', exact: true })
  await expect(events.getByText('bobが共同作成者に加わりました', { exact: true })).toBeVisible()
  await events.getByRole('link', { name: '無題のコンテスト', exact: true }).click()
  await expect(page).toHaveURL(`/my/contests/${shortId(id)}?section=manage`)
  await expect(page.getByRole('list', { name: '作成者', exact: true }).getByRole('listitem')).toHaveText([/ui_test\s*オーナー/, /bob\s*共同作成者/])
})

test('problems reached through a contest say so in the testing tab and the problem management', async ({ page }) => {
  const pid = '44444444-dddd-4ddd-8ddd-000000000004'
  await page.route('**/api/my/problems**', route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith(pid)) return route.fulfill({ json: { id: pid, version: 2, publishedVersion: 0, updatedAt, author: 'alice', testers: ['carol'], contestId: id, contestScheduled: true, contestEditors: { contestId: id, title: '共同のコンテスト', setters: ['alice', 'ui_test'] }, draft: { title: 'アリスの問題', markdown: '本文', editorial: '解説', timeLimitMs: '2000', memoryLimitMb: '512', testCases: [{ input: '1', output: '1' }] } } })
    return route.fulfill({ json: { items: url.searchParams.get('role') === 'tester' ? [{ id: pid, title: 'アリスの問題', updatedAt, contestId: id, contestScheduled: true, viaContest: { id, title: '共同のコンテスト' } }] : [], nextCursor: '' } })
  })
  await page.goto('/my?tab=testing')
  const table = page.getByRole('table', { name: 'テスト中の問題', exact: true })
  await expect(table.getByRole('rowheader')).toHaveText('アリスの問題コンテスト「共同のコンテスト」の作成者として')
  await table.getByRole('link', { name: 'アリスの問題を編集', exact: true }).click()
  await expect(page.locator('#problem-title')).toHaveValue('アリスの問題')
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const editors = page.getByRole('region', { name: 'コンテストの作成者', exact: true })
  await expect(editors).toContainText('コンテスト「共同のコンテスト」の作成者（alice、ui_test）は、コンテストの結果が確定するまで')
  await expect(editors.getByRole('link', { name: '共同のコンテスト', exact: true })).toHaveAttribute('href', `/contests/${shortId(id)}`)
  // Rights through a contest never hand out tester links.
  await expect(page.getByText('コンテストの作成者として編集している問題では、テスターリンクを発行できません。')).toBeVisible()
  await expect(page.getByRole('button', { name: 'リンクを発行', exact: true })).toBeDisabled()
})
