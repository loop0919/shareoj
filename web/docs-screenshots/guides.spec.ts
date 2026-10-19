import type { Page } from '@playwright/test'
import { expect, test } from '../tests/fixtures/account'
import { shortId } from '../shared/utils/short-id'
import { annotate, shoot } from './annotate'

// Each test writes public/images/docs/<name>.png for the guides in app/content/docs.
const problemId = '11111111-1111-4111-8111-111111111111'
const draftId = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'
const updatedAt = '2026-10-10T00:00:00Z'
const source = '#include <iostream>\n\nint main() {\n  long long a, b;\n  std::cin >> a >> b;\n  std::cout << a + b << \'\\n\';\n}'
const draft = {
  title: 'A + B', markdown: '2 つの整数 $A$ と $B$ の和を求めてください。', editorial: '$A+B$ を出力します。', difficulty: 1,
  timeLimitMs: '2000', memoryLimitMb: '1024', tags: [],
  testCases: [
    { name: 'sample_1', input: '3 5\n', output: '8\n', isSample: true },
    { name: 'sample_2', input: '100 200\n', output: '300\n', isSample: true },
    { name: 'max_1', input: '1000000000 1000000000\n', output: '2000000000\n', isSample: false },
  ],
}

test.beforeEach(async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  // Registered after the policy change, so the notice about it stays hidden.
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-10-17T00:00:00Z' } } }))
  await page.route('**/api/my/favorites/*', route => route.fulfill({ json: { favorited: false, favoriteCount: 3 } }))
  await page.route('**/api/my/notifications', route => route.fulfill({ json: { notifications: [] } }))
})

async function savedProblem(page: Page, extra: Record<string, unknown> = {}) {
  const { draft: draftOverride, ...rest } = extra as { draft?: Record<string, unknown> }
  await page.route(`**/api/my/problems/${draftId}`, route => route.fulfill({ json: {
    id: draftId, version: 3, updatedAt, draft: { ...draft, ...draftOverride },
    readiness: { publish: [], contest: [], featured: [] }, ...rest,
  } }))
  await page.goto(`/problems/new?problem=${draftId}`)
  await expect(page.locator('#problem-title')).toHaveValue('A + B')
}

async function expandSidebar(page: Page) {
  await page.getByRole('button', { name: 'サイドバーを展開' }).click()
}

test('create menu', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 360 })
  await page.goto('/')
  // Only the header matters here; the page below would show through the crop.
  await page.addStyleTag({ content: 'body * { visibility: hidden } .site-header, .site-header * { visibility: visible }' })
  const menu = page.locator('details.create-menu')
  await menu.locator('summary').click()
  const nav = page.getByRole('navigation', { name: 'メインナビゲーション' })
  const links = menu.locator('.create-menu-links')
  await annotate(page, [menu.locator('summary'), links.getByRole('link', { name: '新規問題' })])
  await shoot(page, 'create-problem', [nav, links])
  await annotate(page, [menu.locator('summary'), links.getByRole('link', { name: '新規コンテスト' })])
  await shoot(page, 'create-contest', [nav, links])
})

test('problem editor', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 720 })
  await savedProblem(page)
  await expandSidebar(page)
  const nav = page.getByRole('navigation', { name: '問題作成メニュー' })
  await annotate(page, [
    ...['問題文', 'テストケース', '生成・検証・判定', '採点', '解説', '問題管理'].map(name => ({ target: nav.getByRole('button', { name, exact: true }), pad: 2, badge: 'right' as const })),
    { target: page.getByRole('button', { name: '保存', exact: true }), badge: 'bottom' },
  ])
  await shoot(page, 'problem-editor')
})

test('test cases', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 560 })
  await savedProblem(page)
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  await expect(page.getByRole('checkbox', { name: 'サンプルケースにする' })).toBeChecked()
  await annotate(page, [
    page.getByRole('button', { name: 'ファイルから一括追加' }).first(),
    page.getByText('サンプルケースにする', { exact: true }),
    page.getByRole('button', { name: 'サンプルを問題文に追加' }),
  ])
  await shoot(page, 'problem-tests')
})

test('scoring', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 860 })
  // A subtask problem: the samples stay out of the groups, and the largest case counts in both (ADR 0025).
  await savedProblem(page, { draft: {
    scoring: { mode: 'groups', groups: [{ name: '小課題1', points: 30 }, { name: '小課題2', points: 70 }] },
    testCases: [...draft.testCases.slice(0, 2), { ...draft.testCases[2], groups: ['小課題2'] }, { name: 'small_1', input: '1 2\n', output: '3\n', isSample: false, groups: ['小課題1', '小課題2'] }],
  } })
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const methods = page.getByRole('group', { name: '採点方式' })
  await expect(methods.getByRole('radio', { name: '部分点採点' })).toBeChecked()
  await expect(page.getByRole('switch', { name: '集計コードで採点する' })).not.toBeChecked()
  await annotate(page, [
    { target: methods, badge: 'top-right' },
    { target: page.locator('.scoring-switch'), badge: 'right' },
    { target: page.locator('.scoring-groups'), badge: 'top-right' },
    page.getByRole('button', { name: 'グループを追加' }),
  ])
  await shoot(page, 'problem-scoring', [page.locator('.scoring-editor header'), page.locator('.scoring-editor .scoring-note').last()])
})

test('problem management', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 1100 })
  // An unpublished draft with gaps shows what is missing.
  await savedProblem(page, { draft: { difficulty: null, editorial: '' }, readiness: { publish: ['difficulty_missing'], contest: ['difficulty_missing'], featured: ['difficulty_missing', 'editorial_missing'] } })
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const status = page.getByRole('region', { name: '状態' })
  await annotate(page, [status, status.getByRole('button', { name: '問題文を開く' })])
  await shoot(page, 'problem-status', [status])
})

test('published problem management', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 1100 })
  await page.route('**/api/my/problems/*/rejudge', route => route.fulfill({ json: { pending: 0 } }))
  await page.route('**/api/my/problems/*/tester-invitation', route => route.fulfill({ json: { token: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567' } }))
  await savedProblem(page, { publishedVersion: 3, everPublished: true, readiness: { publish: [], contest: ['published'], featured: ['ever_published'] } })
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await annotate(page, [page.getByRole('button', { name: '非公開に戻す' }), page.getByRole('button', { name: '問題を削除' })])
  await shoot(page, 'problem-unpublish', [page.locator('.management-row').first(), page.locator('.management-row').last()])
  const rejudge = page.getByRole('region', { name: 'リジャッジ' })
  await annotate(page, [rejudge.getByRole('button', { name: 'すべてリジャッジ' })])
  await shoot(page, 'problem-rejudge', [rejudge])
  await page.getByRole('button', { name: 'リンクを発行' }).click()
  const tester = page.locator('.management-row').filter({ hasText: 'テスターリンク' })
  // The link uses the page origin; show the production one.
  await tester.locator('#tester-link').evaluate((input: HTMLInputElement) => { input.value = input.value.replace(location.origin, 'https://www.share-oj.net') })
  await annotate(page, [tester.getByRole('button', { name: 'リンクを発行' }), tester.getByRole('button', { name: 'リンクをコピー' })])
  await shoot(page, 'problem-testers', [tester])
})

test('post a problem', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 640 })
  await savedProblem(page, { author: 'alice' })
  await page.getByRole('button', { name: '公開・応募', exact: true }).click()
  const choice = page.getByRole('dialog', { name: '公開・応募' })
  await annotate(page, [choice.getByRole('link', { name: /問題を投稿/ }), choice.getByRole('link', { name: /定期便に応募/ })])
  await shoot(page, 'problem-publish-choice', [choice])
  const items = [{ id: draftId, title: 'A + B', difficulty: 1, publishedVersion: 0, updatedAt, readiness: { publish: [], contest: [], featured: [] } }]
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items, nextCursor: '' } }))
  await choice.getByRole('link', { name: /問題を投稿/ }).click()
  const choices = page.getByRole('group', { name: '問題' })
  await expect(choices.getByRole('radio', { name: /A \+ B/ })).toBeChecked()
  await annotate(page, [choices, page.getByRole('group', { name: '公開のタイミング' }), page.getByRole('button', { name: '投稿する', exact: true })])
  await shoot(page, 'problem-post', [page.locator('.problem-post')])
})

test('submission form', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 1800 })
  await page.goto(`/problems/${shortId(problemId)}`)
  const form = page.locator('.submission-form')
  await form.locator('#submission-language').selectOption('cpp23-gcc')
  await page.getByLabel('ソースコード', { exact: true }).fill(source)
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await annotate(page, [
    form.locator('#submission-language'),
    { target: page.getByLabel('ソースコード', { exact: true }).locator('xpath=ancestor::*[contains(@class, "source-editor") or contains(@class, "code-editor")][1]'), pad: 2 },
    form.getByRole('button', { name: 'サンプル検証', exact: true }),
    form.getByRole('button', { name: '提出する', exact: true }),
  ])
  await shoot(page, 'submit-form', [form])
})

test('problem tabs', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto(`/problems/${shortId(problemId)}`)
  const tabs = page.getByRole('link', { name: '自分の提出', exact: true }).locator('..')
  await annotate(page, [page.getByRole('link', { name: '自分の提出', exact: true }), page.getByRole('link', { name: 'すべての提出', exact: true })])
  await shoot(page, 'problem-tabs', [tabs])
})

test('contest editor', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 720 })
  await page.goto('/my/contests/new')
  await expandSidebar(page)
  const nav = page.getByRole('navigation', { name: 'コンテスト作成メニュー' })
  await annotate(page, [
    ...['タイトル・説明', '問題・配点', '期間・ペナルティ', '参加形式', 'コンテスト管理'].map(name => ({ target: nav.getByRole('button', { name, exact: true }), pad: 2, badge: 'right' as const })),
    { target: page.getByRole('button', { name: /下書きを保存|変更を保存/ }), badge: 'bottom' },
    { target: page.getByRole('button', { name: 'コンテストを公開' }), badge: 'bottom' },
  ])
  await shoot(page, 'contest-editor')
})

test('contest setters', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 })
  const contestId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
  await page.route(`**/api/my/contests/${contestId}/draft`, route => route.fulfill({ json: {
    id: contestId, version: 2, updatedAt, issues: [], authors: ['alice', 'bob'], owner: true, problems: [],
    draft: { title: 'ShareOJ Beginner Contest 001', description: '', startsAt: '2026-11-01T12:00:00Z', durationMinutes: 100, penaltyMinutes: 5, problems: [] },
  } }))
  await page.route(`**/api/my/contests/${contestId}/setter-invitation`, route => route.fulfill({ json: { token: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567' } }))
  await page.goto(`/my/contests/${shortId(contestId)}?section=manage`)
  await page.getByRole('button', { name: 'リンクを発行', exact: true }).click()
  const row = page.locator('.setters-row')
  // The link uses the page origin; show the production one.
  await row.locator('#setter-link').evaluate((input: HTMLInputElement) => { input.value = input.value.replace(location.origin, 'https://www.share-oj.net') })
  await annotate(page, [
    { target: row.getByRole('button', { name: 'リンクをコピー', exact: true }), badge: 'top-right' },
    { target: row.getByRole('button', { name: 'リンクを作り直す', exact: true }), badge: 'top-right' },
    { target: row.getByRole('button', { name: 'bobを共同作成者から外す', exact: true }), badge: 'top-right' },
  ])
  await shoot(page, 'contest-setters', [row])
})

test('featured application', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: [] } }))
  await page.route(/\/api\/my\/problems(?:\?.*)?$/, route => route.fulfill({ json: { items: [{ id: draftId, title: 'A + B', difficulty: 1, featuredPreference: '', updatedAt, readiness: { publish: [], contest: [], featured: [] } }], nextCursor: '' } }))
  await page.route('**/api/my/featured/schedule**', async route => {
    const plan = await (await page.request.get('/api/featured/schedule')).json()
    plan.rounds[1].slots[1].difficulty = 8
    plan.rounds[3].slots[0] = { slot: 'easy', kind: 'new', difficulty: 1, mine: true, title: 'A + B', trial: true }
    return route.fulfill({ json: plan })
  })
  await page.goto(`/featured/apply?problem=${draftId}`)
  await expect(page.locator('.apply-bar .bar-message')).toBeVisible()
  await annotate(page, [page.getByRole('group', { name: '問題' }), page.getByRole('group', { name: '希望' }), page.locator('.apply-schedule td[data-trial]'), page.getByRole('button', { name: '応募する', exact: true })])
  await shoot(page, 'featured-apply', [page.locator('.featured-apply')])
})

test('generator and checker', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 })
  await savedProblem(page)
  await expandSidebar(page)
  await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
  const kinds = page.getByRole('group', { name: '生成・検証・判定の種類' })
  await page.getByLabel('言語').selectOption('cpp23-gcc')
  await page.getByLabel('入力生成のコード', { exact: true }).fill('#include <iostream>\n\nint main() {\n    long long case_number;\n    std::cin >> case_number;\n    std::cout << case_number << " " << case_number + 1 << "\\n";\n}')
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await annotate(page, [
    { target: page.getByRole('navigation', { name: '問題作成メニュー' }).getByRole('button', { name: '生成・検証・判定', exact: true }), pad: 2, badge: 'top-right' },
    ...['入力生成', '出力生成', '入力検証'].map(name => kinds.getByRole('button', { name, exact: true })),
    { target: ['言語', '開始ケース番号', '生成件数'].map(name => page.locator('.generator label').filter({ hasText: name })), badge: 'top-right' },
    page.getByRole('button', { name: '生成する', exact: true }),
    page.getByRole('button', { name: 'テストケースを確認' }),
  ])
  await shoot(page, 'generator')
  await kinds.getByRole('button', { name: '判定方法', exact: true }).click()
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
  await page.locator('#checker-language').selectOption('cpp23-gcc')
  await page.locator('#checker-protocol').selectOption({ label: 'testlib形式（Codeforces互換）' })
  await page.getByLabel('検証コード', { exact: true }).fill('#include "testlib.h"\n\nint main(int argc, char** argv) {\n  registerTestlibCmd(argc, argv);\n  int actual = ouf.readInt();\n  int expected = ans.readInt();\n  if (actual != expected) quitf(_wa, "different answer");\n  quitf(_ok, "accepted");\n}')
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await annotate(page, [
    kinds.getByRole('button', { name: '判定方法', exact: true }),
    { target: page.getByRole('combobox', { name: '判定方法', exact: true }), badge: 'bottom-right' },
    { target: page.locator('#checker-language'), badge: 'bottom-right' },
    { target: page.locator('#checker-protocol'), badge: 'bottom-right' },
  ])
  await shoot(page, 'checker')
})

test('contest participation', async ({ page }) => {
  // Tall enough that the fixed contest clock stays below the form.
  await page.setViewportSize({ width: 1280, height: 1400 })
  // The guide shows a team contest with an onsite division, which offers every choice (ADR 0024).
  const contestId = '99999999-9999-4999-8999-999999999999'
  await page.route(`**/api/contests/${contestId}`, async route => {
    const response = await route.fetch()
    return route.fulfill({ response, json: { ...await response.json(), teamMode: true, maxTeamSize: 3, onsiteDivision: true } })
  })
  // The server render reads the fixture as it is; the refresh reads the routed contest.
  await page.goto(`/contests/${shortId(contestId)}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true }).click()
  const passphrase = page.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await passphrase.fill('ShareOJ_2026')
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  const area = page.locator('.contest-participation')
  await annotate(page, [
    { target: passphrase, badge: 'right' },
    area.getByRole('button', { name: '個人参加', exact: true }),
    area.getByRole('link', { name: 'チーム参加', exact: true }),
  ])
  await shoot(page, 'contest-join', [page.getByRole('heading', { level: 1 }), area])
})

test('difficulty vote', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto(`/problems/${shortId(problemId)}`)
  await page.getByRole('button', { name: '難易度評価', exact: true }).click()
  const panel = page.getByRole('dialog', { name: '難易度評価', exact: true })
  await expect(panel).toBeVisible()
  await annotate(page, [
    page.getByRole('button', { name: '難易度評価', exact: true }),
    panel.getByRole('combobox'),
    panel.getByRole('button', { name: '投票する' }),
  ])
  await shoot(page, 'difficulty-vote', [page.getByRole('button', { name: '難易度評価', exact: true }), panel])
})

test('rejudge selected submissions', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  const done = (id: string, author: string, verdict: string) => ({
    id, problemId, problemTitle: 'A + B', problemVersion: 1, author, runtime: 'cpp23-gcc', sourceBytes: 120, status: 'DONE',
    createdAt: '2026-10-12T03:15:00Z', result: { verdict, passed: verdict === 'AC' ? 3 : 2, total: 3, cpuTimeMs: 2, memoryBytes: 3_600_000 },
  })
  await page.route('**/api/my/problems/*/rejudge', route => route.fulfill({ json: { pending: 0 } }))
  await page.route('**/api/problems/*/submissions?*', route => route.fulfill({ json: { items: [
    done('33333333-3333-4333-8333-333333333331', 'carol', 'WA'),
    done('33333333-3333-4333-8333-333333333332', 'bob', 'AC'),
    done('33333333-3333-4333-8333-333333333333', 'yuki', 'AC'),
  ], hasMore: false } }))
  await page.goto(`/problems/${shortId(problemId)}?view=submissions`)
  const list = page.getByRole('region', { name: 'すべての提出' })
  const rows = list.getByRole('checkbox', { name: /の提出を選択$/ })
  await rows.first().check()
  const action = list.getByRole('button', { name: /^選択した\d+件の提出をリジャッジ$/ })
  await expect(action).toBeVisible()
  await annotate(page, [{ target: rows.first(), pad: 6 }, action])
  await shoot(page, 'rejudge-list', [list.getByRole('heading', { name: 'すべての提出' }), list.getByRole('table'), action])
})
