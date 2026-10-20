import { expect, test, type Browser, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { shortId } from '../../shared/utils/short-id'

const origin = 'http://127.0.0.1:13002'
async function login(page: Page, name: string) {
  await page.getByLabel('メールアドレス', { exact: true }).fill(`${name}@example.test`)
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
}
async function user(browser: Browser, name: string) {
  const context = await browser.newContext({ baseURL: origin })
  const page = await context.newPage()
  await page.goto('/login')
  await login(page, name)
  await expect(page).toHaveURL('/my')
  return page
}
async function seed(page: Page, title: string) {
  const id = randomUUID()
  const response = await page.request.put(`/api/my/problems/${id}`, { headers: { origin }, data: { version: 0, draft: { title, difficulty: 3, markdown: '## 共同作成の本文\n整数を出力してください。', editorial: '## 共同作成の解説\n答えは2です。', timeLimitMs: '1500', memoryLimitMb: '256', testCases: [{ input: '1', output: '2', isSample: true }] } } })
  expect(response.status()).toBe(200)
  return id
}
// The date input reads Japan time.
function japanDate(value: number) {
  return new Date(value + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)
}
const noScroll = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

test('co-setters join through a link, publish together, and leave or are removed without competing', async ({ page, browser }, testInfo) => {
  test.slow()
  const alice = page
  await alice.goto('/login')
  await login(alice, 'alice')
  await expect(alice).toHaveURL('/my')
  const theirs = await seed(alice, 'アリスの共同問題')
  const id = randomUUID()
  const created = await alice.request.put(`/api/my/contests/${id}/draft`, { headers: { origin }, data: { version: 0, draft: { title: '共同作成のコンテスト', description: '', startsAt: null, durationMinutes: 60, penaltyMinutes: 5, problems: [{ id: theirs, points: 100 }] } } })
  expect(created.status()).toBe(200)
  const editor = `/my/contests/${shortId(id)}`
  await alice.goto(`${editor}?section=manage`)
  const setters = alice.getByRole('list', { name: '作成者', exact: true })
  await expect(setters.getByRole('listitem')).toHaveText([/alice\s*オーナー/])
  await alice.getByRole('button', { name: 'リンクを発行', exact: true }).click()
  const link = await alice.getByLabel('招待リンク', { exact: true }).inputValue()
  expect(link).toMatch(/\/my\/contest-invitations\/[A-Z2-7]{32}$/)
  const token = link.split('/').pop()!

  // bob follows the link through the login page and joins.
  const bobContext = await browser.newContext({ baseURL: origin })
  const bob = await bobContext.newPage()
  await bob.goto(link)
  await expect(bob).toHaveURL(/\/login\?next=/)
  await login(bob, 'bob')
  await expect(bob).toHaveURL(link)
  await expect(bob.getByRole('heading', { name: '共同作成のコンテスト', level: 2 })).toBeVisible()
  await expect(bob.getByText('オーナー alice')).toBeVisible()
  expect((await bob.request.get(`/api/my/contests/${id}/draft`)).status()).toBe(404)
  expect((await bob.request.post(`/api/my/setter-invitations/${token}`)).status()).toBe(403)
  for (const width of [320, 375, 768]) {
    await bob.setViewportSize({ width, height: 900 })
    expect(await noScroll(bob)).toBe(true)
  }
  await bob.screenshot({ path: testInfo.outputPath('setter-invitation-mobile.png'), fullPage: true })
  await bob.setViewportSize({ width: 1280, height: 900 })
  const mine = await seed(bob, 'ボブの共同問題')
  await bob.getByRole('button', { name: '共同作成者になる', exact: true }).click()
  await expect(bob).toHaveURL(editor)
  // The owner's problem keeps its title for the co-setter, who adds a problem of their own and publishes.
  await bob.getByRole('button', { name: '問題・配点', exact: true }).click()
  await expect(bob.locator('.selected strong')).toHaveText(['アリスの共同問題'])
  await expect(bob.locator('.selected li').first()).toContainText('作者 alice')
  await bob.getByRole('searchbox', { name: '問題を検索' }).fill(mine)
  await bob.getByRole('button', { name: 'ボブの共同問題を追加', exact: true }).click()
  await bob.getByRole('button', { name: '期間・ペナルティ', exact: true }).click()
  await bob.getByLabel('開始日時').fill(japanDate(Date.now() + 86400000))
  await bob.getByRole('button', { name: 'コンテスト管理', exact: true }).click()
  await expect(bob.getByRole('list', { name: '作成者', exact: true }).getByRole('listitem')).toHaveText([/alice\s*オーナー/, /bob\s*共同作成者/])
  await expect(bob.getByRole('button', { name: '下書きを削除' })).toHaveCount(0)
  await expect(bob.getByRole('button', { name: '共同作成者から抜ける', exact: true })).toBeVisible()
  bob.once('dialog', dialog => dialog.accept())
  await bob.getByRole('button', { name: 'コンテストを公開', exact: true }).click()
  await expect(bob).toHaveURL(`/contests/${shortId(id)}`)
  await expect(bob.locator('.problem-meta')).toContainText('作成者 alice、bob')
  const published = await (await bob.request.get(`/api/contests/${id}`)).json()
  expect([published.author, published.authors]).toEqual(['alice', ['alice', 'bob']])

  // The owner hears about the new co-setter and sees bob's problem in the contest.
  await alice.goto('/my')
  await alice.getByLabel(/^通知（未読\d+件）$/).click()
  await alice.getByRole('link', { name: /bobが共同作成者に加わりました/ }).click()
  await expect(alice).toHaveURL(`${editor}?section=manage`)
  await expect(setters.getByRole('listitem')).toHaveText([/alice\s*オーナー/, /bob\s*共同作成者/])
  await alice.getByRole('button', { name: '問題・配点', exact: true }).click()
  await expect(alice.locator('.selected strong')).toHaveText(['アリスの共同問題', 'ボブの共同問題'])
  await alice.goto('/my?tab=testing')
  await expect(alice.getByRole('table', { name: 'テスト中の問題', exact: true }).getByRole('rowheader', { name: /ボブの共同問題/ })).toContainText('コンテスト「共同作成のコンテスト」の作成者として')
  // The author of a problem sees who edits it through the contest, and such editors cannot invite testers.
  await bob.goto(`/problems/new?problem=${theirs}`)
  await bob.getByRole('button', { name: '問題管理', exact: true }).click()
  await expect(bob.getByRole('region', { name: 'コンテストの作成者', exact: true })).toContainText('コンテスト「共同作成のコンテスト」の作成者（alice、bob）')
  await expect(bob.getByRole('button', { name: 'リンクを発行', exact: true })).toBeDisabled()

  // A registered participant cannot become a co-setter.
  const carol = await user(browser, 'carol')
  expect((await carol.request.post(`/api/my/contests/${id}/participation`, { headers: { origin } })).status()).toBe(200)
  expect((await carol.request.post(`/api/my/contests/${id}/setter-invitation`, { headers: { origin } })).status()).toBe(404)
  await carol.goto(link)
  await carol.getByRole('button', { name: '共同作成者になる', exact: true }).click()
  await expect(carol.getByRole('alert')).toHaveText('このコンテストに参加登録しているため、共同作成者になれません。')
  await expect(carol).toHaveURL(link)

  // dave joins and leaves; having seen the problems, he may not compete.
  const dave = await user(browser, 'dave')
  await dave.goto(link)
  await dave.getByRole('button', { name: '共同作成者になる', exact: true }).click()
  await expect(dave).toHaveURL(editor)
  await dave.getByRole('button', { name: 'コンテスト管理', exact: true }).click()
  for (const width of [320, 375, 768]) {
    await dave.setViewportSize({ width, height: 900 })
    expect(await noScroll(dave)).toBe(true)
  }
  await dave.screenshot({ path: testInfo.outputPath('contest-setters-mobile.png'), fullPage: true })
  dave.once('dialog', dialog => dialog.accept())
  await dave.getByRole('button', { name: '共同作成者から抜ける', exact: true }).click()
  await expect(dave).toHaveURL('/my?tab=contests')
  await dave.goto(`/contests/${shortId(id)}`)
  await expect(dave.getByText('このコンテストのスタッフを務めたため、参加できません。', { exact: true })).toBeVisible()
  await expect(dave.getByRole('button', { name: '参加する' })).toHaveCount(0)
  expect((await dave.request.post(`/api/my/contests/${id}/participation`, { headers: { origin } })).status()).toBe(409)

  // The owner removes bob, which renews the link; bob's problem then needs another setter before the next save.
  expect((await alice.request.delete(`/api/my/contests/${id}/setters/bob`, { headers: { origin: 'https://attacker.example' } })).status()).toBe(403)
  await alice.goto(`${editor}?section=manage`)
  alice.once('dialog', dialog => dialog.accept())
  await alice.getByRole('button', { name: 'bobを共同作成者から外す', exact: true }).click()
  await expect(setters.getByRole('listitem')).toHaveText([/alice\s*オーナー/])
  await alice.getByRole('button', { name: '問題・配点', exact: true }).click()
  await expect(alice.locator('.selected li').filter({ hasText: 'ボブの共同問題' })).toContainText('作成者の誰も作者・テスターではないため、外すまで保存できません')
  await bob.goto(link)
  await expect(bob.getByRole('alert')).toHaveText('招待リンクが無効か、作り直されています。コンテストの作成者に今のリンクを確認してください。')
  // bob still authors a problem in the contest, so he stays staff but no longer sets it.
  const removed = await (await bob.request.get(`/api/my/contests/${id}`)).json()
  expect([removed.setter, removed.staff, removed.canEdit]).toEqual([false, true, false])
  await bob.goto('/my?tab=contests')
  await expect(bob.getByRole('heading', { name: '作成したコンテスト' })).toBeVisible()
  await expect(bob.getByRole('link', { name: '共同作成のコンテスト', exact: true })).toHaveCount(0)
  // dave's arrival reached the owner as well; reading it leaves alice's bell empty for the specs that follow.
  const unread = await alice.request.post('/api/my/notifications/read', { headers: { origin } })
  expect((await unread.json()).notifications).toEqual([expect.objectContaining({ contestId: id, actor: 'dave', kind: 'contest_setter', title: '共同作成のコンテスト' })])
  await Promise.all([bobContext.close(), carol.context().close(), dave.context().close()])
})

test('a newcomer opens a co-setter link, creates a profile, and returns to the invitation', async ({ page, browser }) => {
  await page.goto('/login')
  await login(page, 'alice')
  await expect(page).toHaveURL('/my')
  const id = randomUUID()
  const created = await page.request.put(`/api/my/contests/${id}/draft`, { headers: { origin }, data: { version: 0, draft: { title: '新しい人を招くコンテスト', description: '', startsAt: null, durationMinutes: 60, penaltyMinutes: 5, problems: [] } } })
  expect(created.status()).toBe(200)
  const issued = await page.request.post(`/api/my/contests/${id}/setter-invitation`, { headers: { origin } })
  expect(issued.status()).toBe(200)
  const link = `${origin}/my/contest-invitations/${(await issued.json()).token}`
  const newcomerContext = await browser.newContext({ baseURL: origin })
  try {
    const newcomer = await newcomerContext.newPage()
    const email = `setter-${Date.now()}@example.test`
    for (const [action, data] of [
      ['signup', { email, password: 'ValidPassword123!' }],
      ['confirm-signup', { email, code: '123456' }],
    ] as const) {
      expect((await newcomer.request.post(`/api/auth/${action}`, { headers: { origin }, data })).ok()).toBe(true)
    }
    await newcomer.goto(link)
    await expect(newcomer).toHaveURL(/\/login\?next=/)
    await newcomer.getByLabel('メールアドレス').fill(email)
    await newcomer.getByLabel('パスワード', { exact: true }).fill('ValidPassword123!')
    await newcomer.getByRole('button', { name: 'ログイン', exact: true }).click()
    // Without a profile the link waits behind onboarding, then opens again.
    await expect(newcomer).toHaveURL(/\/onboarding\?next=/)
    await newcomer.getByLabel('ユーザーID', { exact: true }).fill(`setter_${Date.now()}`)
    await newcomer.getByRole('button', { name: '登録してはじめる' }).click()
    await expect(newcomer).toHaveURL(link)
    await expect(newcomer.getByRole('heading', { name: '新しい人を招くコンテスト', level: 2 })).toBeVisible()
    await expect(newcomer.getByRole('button', { name: '共同作成者になる', exact: true })).toBeVisible()
  } finally {
    await newcomerContext.close()
    await page.request.delete(`/api/my/contests/${id}/draft?version=1`, { headers: { origin } })
  }
})
