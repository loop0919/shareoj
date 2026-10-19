import type { Page } from '@playwright/test'
import type { Contest, Standing } from '../shared/types/contest'
import type { Submission } from '../shared/types/submission'
import { shortId } from '../shared/utils/short-id'
import { expect, test } from './fixtures/account'

// Judging does not run in tests, so these results stand in for what the bridge stores (ADR 0025).
const contestId = '88888888-8888-4888-8888-888888888888'
const partialId = 'abababab-abab-4bab-8bab-abababababab'
const partialContest = 'acacacac-acac-4cac-8cac-acacacacacac'
const submission = (id: string, author: string, result: Submission['result']): Submission => ({
  id, problemId: partialId, problemTitle: '部分点の問題', problemVersion: 1, author, runtime: 'cpp23-gcc', sourceBytes: 120, source: 'int main() {}',
  status: 'DONE', createdAt: '2026-09-15T12:10:00Z', result,
})
const scored = (score: number, verdict = 'WA') => ({ verdict, passed: 1, total: 3, cpuTimeMs: 4, memoryBytes: 2_000_000, scoring: { score, maxScore: 10000 } })

test('a submission shows its score, the score of each group with its cases, and the cases\' own scores', async ({ page }, testInfo) => {
  const sid = '44444444-4444-4444-8444-444444444444'
  const result = {
    verdict: 'WA', passed: 2, total: 5, cpuTimeMs: 12, memoryBytes: 2_000_000,
    cases: [
      { name: 'sample', verdict: 'WA', cpuTimeMs: 1, wallTimeMs: 2, memoryBytes: 1_000_000 },
      { name: '01', verdict: 'AC', score: 37, cpuTimeMs: 3, wallTimeMs: 4, memoryBytes: 1_000_000 },
      { name: '02', verdict: 'AC', score: 1, cpuTimeMs: 3, wallTimeMs: 4, memoryBytes: 1_000_000 },
      { name: '03', verdict: 'TLE', cpuTimeMs: 2000, wallTimeMs: 2100, memoryBytes: 1_000_000 },
      { name: '04', verdict: 'SKIPPED' },
    ],
    scoring: { score: 3750, maxScore: 10000, groups: [{ name: '小課題1', score: 3000, maxScore: 3000, cases: [1, 2] }, { name: '小課題2', score: 750, maxScore: 7000, cases: [2, 3, 4] }] },
  }
  // A case score past 2^53 reaches the browser rounded, so the page says it is approximate.
  const body = JSON.stringify(submission(sid, 'ui_test', result)).replace('"score":1,', '"score":999999999999999999,')
  await page.route(`**/api/my/submissions/${sid}`, route => route.fulfill({ contentType: 'application/json', body }))
  await page.goto(`/my/submissions/${shortId(sid)}`)
  const info = page.getByRole('table', { name: '提出情報' })
  await expect(info.getByRole('row', { name: /^得点/ })).toHaveText('得点37.50 / 100')
  const groups = page.getByRole('table', { name: 'グループごとの得点' })
  await expect(groups.locator('tbody tr')).toHaveText(['小課題130 / 3001、02', '小課題27.50 / 7002、03、04'])
  await expect(groups.locator('tbody tr').nth(0).locator('.submission-score')).toHaveAttribute('data-full', 'true')
  await expect(groups.locator('tbody tr').nth(1).locator('.submission-score')).not.toHaveAttribute('data-full', /.*/)
  const cases = page.getByRole('table', { name: 'テストケースごとの結果' })
  await expect(cases.locator('thead th')).toHaveText(['テストケース名', '結果', '点', 'CPU時間', '経過時間', '最大メモリ'])
  await expect(cases.locator('tbody tr td:nth-child(3)')).toHaveText(['—', '37', '約1000000000000000000', '—', '—'])
  await expect(cases.locator('tbody tr').nth(4).locator('td').nth(1)).toHaveText('未実行')
  await page.screenshot({ path: testInfo.outputPath('submission-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)

  // A compile error carries zero scores and no cases, so the groups name their cases by number.
  const compileError = { verdict: 'CE', passed: 0, total: 0, compileLog: 'error', scoring: { score: 0, maxScore: 10000, groups: [{ name: '小課題1', score: 0, maxScore: 3000, cases: [1, 2] }] } }
  await page.route(`**/api/my/submissions/${sid}`, route => route.fulfill({ json: submission(sid, 'ui_test', compileError) }))
  await page.reload()
  await expect(info.getByRole('row', { name: /^得点/ })).toHaveText('得点0 / 100')
  await expect(groups.locator('tbody tr')).toHaveText(['小課題10 / 30ケース2、ケース3'])
})

test('the submission history adds the score of partially scored results', async ({ page }) => {
  const items = [
    submission('55555555-5555-4555-8555-000000000001', 'ui_test', scored(3750)),
    submission('55555555-5555-4555-8555-000000000002', 'ui_test', scored(10000, 'AC')),
    submission('55555555-5555-4555-8555-000000000003', 'ui_test', { verdict: 'AC', passed: 1, total: 1 }),
  ]
  await page.route('**/api/my/submissions', route => route.fulfill({ json: { items } }))
  await page.goto('/my/submissions')
  await expect(page.locator('.submission-result')).toHaveText(['WA37.50点', 'AC100点', 'AC'])
  await expect(page.locator('.judge-score').nth(1)).toHaveAttribute('data-full', 'true')
})

test('a partially scored problem shows its full score and groups, and its submissions a score column', async ({ page }, testInfo) => {
  const items = [
    submission('66666666-6666-4666-8666-000000000001', 'alice', scored(3750)),
    submission('66666666-6666-4666-8666-000000000002', 'bob', scored(10000, 'AC')),
    // Judged before the problem became partially scored.
    submission('66666666-6666-4666-8666-000000000003', 'carol', { verdict: 'AC', passed: 1, total: 1 }),
    { ...submission('66666666-6666-4666-8666-000000000004', 'dave', null), status: 'QUEUED' as const },
  ]
  // The all-or-nothing problem lists only the result from before.
  await page.route('**/api/problems/*/submissions?*', route => route.fulfill({ json: { items: route.request().url().includes(partialId) ? items : [items[2]], hasMore: false } }))
  await page.goto(`/problems/${shortId(partialId)}`)
  const scoring = page.locator('.problem-header .problem-scoring')
  await expect(scoring.locator('.problem-scoring-total')).toHaveText('部分点あり（満点100点）')
  await expect(scoring.getByRole('list', { name: 'グループの配点' }).getByRole('listitem')).toHaveText(['小課題130点', '小課題270点'])
  await page.screenshot({ path: testInfo.outputPath('problem-desktop.png') })
  await page.getByRole('navigation', { name: '問題メニュー' }).getByRole('link', { name: 'すべての提出', exact: true }).click()
  const table = page.getByRole('region', { name: 'すべての提出' }).getByRole('table')
  await expect(table.locator('thead th')).toContainText(['得点'])
  await expect(table.locator('td.submission-score')).toHaveText(['37.50', '100', '—', '—'])
  await expect(table.locator('td.submission-score').nth(1)).toHaveAttribute('data-full', 'true')
  // An all-or-nothing problem keeps its columns.
  await page.goto('/problems/11111111-1111-4111-8111-111111111111?view=submissions')
  await expect(page.getByRole('region', { name: 'すべての提出' }).locator('tbody tr')).toHaveCount(1)
  await expect(page.getByRole('region', { name: 'すべての提出' }).locator('thead th')).not.toContainText(['得点'])
  await expect(page.locator('.problem-header .problem-scoring')).toHaveCount(0)
})

test('a contest with partial scores drops 「部分点なし」 and shows the full score on its problem pages', async ({ page }) => {
  await page.goto(`/contests/${shortId(partialContest)}`)
  await expect(page.locator('.contest-scoring').first()).toHaveText('誤答ペナルティ 5 分')
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
  await expect(page.locator('.contest-problems tbody td:last-child')).toHaveText(['100 点部分点あり', '200 点'])
  await page.getByRole('link', { name: '部分点の問題', exact: true }).click()
  await expect(page.locator('.problem-header .problem-scoring-total')).toHaveText('部分点あり（満点100点）')
  await expect(page.locator('.problem-header')).not.toContainText('配点 100 点')
  await page.goto(`/contests/${shortId(partialContest)}/B`)
  await expect(page.locator('.problem-header')).toContainText('配点 200 点')
  await expect(page.locator('.problem-header .problem-scoring')).toHaveCount(0)
})

async function openStandings(page: Page, contest: Contest, rows: Standing[]) {
  await page.route(`**/api/contests/${contestId}**`, route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/standings')) return route.fulfill({ json: rows })
    if (path.endsWith('/submissions')) return route.fulfill({ json: { items: [
      { ...submission('77777777-7777-4777-8777-000000000001', 'alice', scored(3750)), contestId },
      { ...submission('77777777-7777-4777-8777-000000000002', 'bob', { verdict: 'WA', passed: 0, total: 1 }), problemId: 'b', problemTitle: '一括の問題', contestId },
    ], hasMore: false } })
    return route.fulfill({ json: contest })
  })
  await page.goto(`/contests/${contestId}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
}

test('standings show partial scores in the ordinary colour, full scores in the accent, and FA and counts by the full score', async ({ page }, testInfo) => {
  const contest: Contest = {
    id: contestId, title: '部分点の順位表', author: 'setter', problemAuthors: [], testers: [], description: '', startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z',
    penaltyMinutes: 5, version: 1, status: 'ended', participating: false, canEdit: false, official: true, canViewSubmissions: true, partialScoring: true,
    teamMode: true, freezeMinutes: 30, freezeAt: '2026-09-15T13:30:00Z', resultsFinal: false, revealed: false,
    problems: [{ id: 'a', title: '部分点の問題', points: 100, partial: true }, { id: 'b', title: '一括の問題', points: 100 }],
  }
  const team = { id: 'tttttttt-tttt-4ttt-8ttt-tttttttttttt', name: 'チーム部分点', members: ['carol', 'dave'], onsite: false }
  const rows: Standing[] = [
    { rank: 1, handle: 'alice', team: null, points: 137, score: 13750, scoreText: '137.50', timeMs: 1500000, problems: {
      a: { points: 37, score: 3750, scoreText: '37.50', wrong: 1, pending: 0, scoredAt: '2026-09-15T12:20:00Z' },
      b: { points: 100, score: 10000, scoreText: '100', wrong: 0, pending: 0, acceptedAt: '2026-09-15T12:10:00Z', scoredAt: '2026-09-15T12:10:00Z' },
    } },
    { rank: 2, handle: 'bob', team: null, points: 100, score: 10000, scoreText: '100', timeMs: 2400000, problems: {
      a: { points: 100, score: 10000, scoreText: '100', wrong: 2, pending: 0, acceptedAt: '2026-09-15T12:30:00Z', scoredAt: '2026-09-15T12:30:00Z' },
      b: { points: 0, score: 0, scoreText: '0', wrong: 1, pending: 1 },
    } },
    { rank: 3, handle: '', team, points: 0, score: 5, scoreText: '0.05', timeMs: 600000, problems: {
      a: { points: 0, score: 5, scoreText: '0.05', wrong: 0, pending: 0, frozen: 2, scoredAt: '2026-09-15T12:10:00Z' },
    } },
  ]
  await openStandings(page, contest, rows)
  const table = page.locator('.standings-table')
  await expect(table.locator('tbody .standing-total')).toHaveText(['137.50', '100', '0.05'])
  const cell = (row: number, column: number) => table.locator('tbody tr').nth(row).locator('td').nth(column)
  await expect(cell(0, 3)).toHaveText('37.50（部分点）(1)誤答 1 回20:00')
  await expect(cell(0, 4)).toHaveText('10010:00')
  await expect(cell(1, 3)).toHaveText('100(2)誤答 2 回30:00')
  await expect(cell(2, 3)).toHaveText('0.05（部分点）?2凍結後の提出 2 件10:00')
  const partial = cell(0, 3).locator('strong'), full = cell(0, 4).locator('strong')
  await expect(partial).toHaveClass('standing-partial')
  await expect(full).toHaveClass('standing-accepted')
  const colour = (locator: typeof partial) => locator.evaluate(element => getComputedStyle(element).color)
  expect(await colour(partial)).not.toBe(await colour(full))
  expect(await colour(partial)).toBe(await table.locator('tbody th').first().evaluate(element => getComputedStyle(element).color))
  // The first full score is the FA, and the counts are of full scores.
  const footer = table.locator('tfoot tr')
  await expect(footer.nth(0).locator('th')).toHaveText('FA（初の満点）')
  await expect(footer.nth(0).locator('td')).toHaveText(['bob30:00', 'alice10:00'])
  await expect(footer.nth(1).locator('th')).toHaveText('満点数 / 提出数（チーム・個人）')
  await expect(footer.nth(1).locator('td')).toHaveText(['1 / 3', '1 / 2'])
  await page.screenshot({ path: testInfo.outputPath('standings-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)

  // The contest's submissions show a score column; results without a score read 「—」.
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '提出一覧', exact: true }).click()
  const submissions = page.getByRole('region', { name: '提出一覧のスクロール領域' }).getByRole('table')
  await expect(submissions.locator('thead th')).toHaveText(['問題', 'ユーザー', 'コード長', '実行時間・メモリ', '得点', '結果', '提出日時（日本時間）'])
  await expect(submissions.locator('td.submission-score')).toHaveText(['37.50', '—'])
})

test('standings from a server before partial scores keep their look', async ({ page }) => {
  const contest: Contest = {
    id: contestId, title: '一括の順位表', author: 'setter', problemAuthors: [], testers: [], description: '', startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z',
    penaltyMinutes: 5, version: 1, status: 'running', participating: false, canEdit: false, official: true, canViewSubmissions: false,
    problems: [{ id: 'a', title: 'A', points: 100 }],
  }
  await openStandings(page, contest, [{ rank: 1, handle: 'alice', points: 100, timeMs: 600000, problems: { a: { points: 100, wrong: 0, pending: 0, acceptedAt: '2026-09-15T12:10:00Z' } } }])
  const table = page.locator('.standings-table')
  await expect(table.locator('tbody td').nth(1)).toHaveText('100')
  await expect(table.locator('tbody td').nth(3)).toHaveText('10010:00')
  await expect(table.locator('tbody td').nth(3).locator('strong')).toHaveClass('standing-accepted')
  await expect(table.locator('tfoot th')).toHaveText(['FA（初正解）', '正解者数 / 提出者数'])
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '概要', exact: true }).click()
  await expect(page.locator('.contest-scoring').first()).toHaveText('誤答ペナルティ 5 分 · 部分点なし')
})

test('the contest editor shows a partially scored problem\'s points as its full score and sends them', async ({ page }) => {
  const updatedAt = '2026-10-01T00:00:00Z'
  const ready = { publish: [], contest: [], featured: [] }
  const items = [
    { id: '11111111-aaaa-4aaa-8aaa-000000000001', title: '部分点の問題', updatedAt, readiness: ready, partial: true, fullScore: 120 },
    { id: '22222222-bbbb-4bbb-8bbb-000000000002', title: '一括の問題', updatedAt, readiness: ready },
  ]
  const saved: { problems: { id: string, points: number | null }[] }[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: new URL(route.request().url()).searchParams.get('role') === 'tester' ? [] : items, nextCursor: '' } }))
  await page.route('**/api/my/contests/*/draft', route => {
    if (route.request().method() !== 'PUT') return route.fulfill({ status: 404, json: {} })
    const body = route.request().postDataJSON()
    saved.push(body.draft)
    return route.fulfill({ json: { id: new URL(route.request().url()).pathname.split('/')[4], version: body.version + 1, updatedAt, issues: ['start_missing'], authors: ['ui_test'], owner: true,
      problems: [{ id: items[0]!.id, title: items[0]!.title, author: 'ui_test', issues: [], partial: true, fullScore: 120 }, { id: items[1]!.id, title: items[1]!.title, author: 'ui_test', issues: [] }], draft: body.draft } })
  })
  await page.goto('/my/contests/new')
  await page.getByRole('button', { name: '問題・配点', exact: true }).click()
  for (const item of items) await page.getByRole('button', { name: `${item.title}を追加`, exact: true }).click()
  const rows = page.locator('.selected li')
  await expect(rows.nth(0).locator('.points-fixed')).toHaveText('配点 120 点（満点）')
  await expect(rows.nth(0).getByRole('spinbutton')).toHaveCount(0)
  await expect(rows.nth(1).getByRole('spinbutton', { name: '配点' })).toHaveValue('100')
  await expect(page.getByRole('heading', { name: /出題順・配点/ })).toContainText('2 問 · 合計 220 点')
  await expect(page.locator('[data-section="problems"] header')).toContainText('部分点の問題は、問題の満点が配点になります。')
  await rows.nth(1).getByRole('spinbutton', { name: '配点' }).fill('300')
  await page.getByRole('button', { name: '下書きを保存' }).click()
  await expect.poll(() => saved.length).toBe(1)
  expect(saved[0]!.problems).toEqual([{ id: items[0]!.id, points: 120 }, { id: items[1]!.id, points: 300 }])
  await expect(page.getByRole('heading', { name: /出題順・配点/ })).toContainText('合計 420 点')
})
