import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const id = '88888888-8888-4888-8888-888888888888'
const pa = '11111111-1111-4111-8111-111111111111'
const pb = '33333333-3333-4333-8333-333333333333'
const path = `/contests/${shortId(id)}`
const running = {
  id, title: '自分の提出テスト', author: 'setter', description: '', status: 'running',
  startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5, version: 1,
  official: true, participating: true, canEdit: false, canViewSubmissions: false,
  problems: [{ id: pa, title: 'A + B', points: 100 }, { id: pb, title: 'A × B', points: 200 }],
}
const whole = '提出はコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。'

function submission(n: number, problemId: string, author: string, createdAt: string) {
  return {
    id: `00000000-0000-4000-8000-00000000000${n}`, contestId: id, problemId, problemTitle: problemId === pa ? 'A + B' : 'A × B', author,
    problemVersion: 1, runtime: 'cpp17', sourceBytes: 24, createdAt, status: 'DONE', result: { verdict: 'AC', passed: 1, total: 1, cpuTimeMs: 1, memoryBytes: 1000, cases: [] },
  }
}
const ownA = submission(1, pa, 'ui_test', '2026-09-15T12:10:00Z')
const ownB = submission(2, pb, 'ui_test', '2026-09-15T12:20:00Z')
const mateB = submission(3, pb, 'mate', '2026-09-15T12:30:00Z')

// The server renders the page from the real API, so the mocked contest comes in on the client, and the views after it
// are opened from the menu. requested records the mine parameter of each list, and the problem after a colon.
async function open(page: Page, contest: typeof running & Record<string, unknown>, lists: Record<string, unknown[]>) {
  const requested: string[] = []
  await page.route(`**/api/contests/${id}**`, route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/submissions')) {
      const mine = url.searchParams.get('mine') ?? '0', problem = url.searchParams.get('problem')
      requested.push(problem ? `${mine}:${problem}` : mine)
      const items = lists[mine]
      return route.fulfill(items ? { json: { items, hasMore: false, total: items.length } } : { status: 404, json: {} })
    }
    if (url.pathname.endsWith('/standings')) return route.fulfill({ json: [] })
    if (url.pathname.includes('/problems/')) return route.fulfill({ json: { id: pa, title: 'A + B', author: 'setter', testers: [], markdown: '足し算', editorial: '', timeLimitMs: '2000', memoryLimitMb: '512' } })
    return route.fulfill({ json: contest })
  })
  await page.goto(`${path}?view=standings`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await expect(page.locator('.site-header')).toContainText(contest.title)
  return requested
}

async function fits(page: Page) {
  for (const width of [320, 375, 768]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '質問', exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `width ${width}`).toBe(true)
  }
}

async function visit(page: Page, to: string) {
  await page.evaluate(to => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(to), to)
}

test('a participant lists their own submissions to every problem while the whole list stays closed', async ({ page }) => {
  const requested = await open(page, running, { 1: [ownB, ownA] })
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await expect(menu.getByRole('link')).toHaveText(['概要', '問題', '順位表', '自分の提出', '解説', '質問', 'コード実行'])
  await expect(menu.getByText('提出', { exact: true })).toHaveCount(0)
  await menu.getByRole('link', { name: '自分の提出', exact: true }).click()
  await expect(page).toHaveURL(/view=my-submissions$/)
  await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('自分の提出')
  await expect(page.getByRole('navigation', { name: 'パンくずリスト' })).toHaveText(`${running.title}/自分の提出`)
  const rows = page.locator('#my-submissions tbody tr')
  // The columns are the problem page's list with the problem added after the time.
  await expect(page.locator('#my-submissions thead th')).toHaveText(['提出日時 (JST)', '問題', 'ユーザー', '言語', 'コード長', '実行時間・メモリ', '結果', '詳細'])
  await expect(rows.locator('.submission-problem')).toHaveText(['B - A × B', 'A - A + B'])
  // The problem's name opens the problem, and 「詳細」 the submission.
  await expect(rows.first().getByRole('link', { name: 'B - A × B' })).toHaveAttribute('href', `${path}/B`)
  await expect(rows.first().getByRole('link', { name: '詳細', exact: true })).toHaveAttribute('href', `${path}/my-submissions/${shortId(ownB.id)}?from=contest`)
  expect(requested).not.toContain('0')
  // The list narrows to one problem, named as the contest names it (ADR 0034).
  const choice = page.getByLabel('問題', { exact: true })
  await expect(choice.locator('option')).toHaveText(['すべて', 'A - A + B', 'B - A × B'])
  await choice.selectOption({ label: 'B - A × B' })
  await expect(page).toHaveURL(/view=my-submissions&problem=B$/)
  await expect.poll(() => requested.at(-1)).toBe(`1:${pb}`)
  await expect(page.getByText('この問題への、自分の提出です。練習提出も掲載します。')).toBeVisible()
  await fits(page)
  await page.setViewportSize({ width: 1280, height: 900 })
  // The problem page has no tabs; the menu holds the lists.
  await menu.getByRole('link', { name: '問題', exact: true }).click()
  await page.getByRole('region', { name: 'コンテストの問題' }).getByRole('link', { name: 'A + B', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('A - A + B')
  await expect(page.getByRole('navigation', { name: '問題メニュー' })).toHaveCount(0)
  // A direct link to the whole list explains when it opens, and the menu still marks the submissions.
  await visit(page, `${path}?view=submissions`)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('すべての提出')
  await expect(page.locator('#submissions .notice')).toHaveText(whole)
  await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toHaveAttribute('aria-current', 'true')
  expect(requested).not.toContain('0')
})

test('after the end the menu folds both lists into 「提出」', async ({ page }) => {
  const practice = submission(4, pa, 'ui_test', '2026-09-15T15:00:00Z')
  await open(page, { ...running, status: 'ended', canViewSubmissions: true }, { 1: [practice, ownA], 0: [mateB, ownA] })
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  const toggle = menu.getByText('提出', { exact: true })
  await expect(menu.getByRole('link')).toHaveText(['概要', '問題', '順位表', '解説', '質問', 'コード実行'])
  await toggle.click()
  await expect(menu.getByRole('link')).toHaveText(['概要', '問題', '順位表', '自分の提出', 'すべての提出', '解説', '質問', 'コード実行'])
  await menu.getByRole('link', { name: 'すべての提出', exact: true }).click()
  await expect(page).toHaveURL(/view=submissions$/)
  await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toBeHidden()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('すべての提出')
  await expect(page.locator('#submissions tbody tr')).toHaveCount(2)
  // Escape and a click elsewhere close the menu.
  await toggle.click()
  await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toHaveAttribute('aria-current', 'page')
  await page.keyboard.press('Escape')
  await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toBeHidden()
  await toggle.click()
  await page.getByRole('heading', { level: 1 }).click()
  await expect(menu.getByRole('link', { name: '自分の提出', exact: true })).toBeHidden()
  await toggle.click()
  await fits(page)
  // A resize closes the panel, since it is placed from the summary.
  await toggle.click()
  await expect(menu.getByRole('link', { name: 'すべての提出', exact: true })).toBeInViewport()
  await page.setViewportSize({ width: 1280, height: 900 })
  await toggle.click()
  await menu.getByRole('link', { name: '自分の提出', exact: true }).click()
  await expect(page).toHaveURL(/view=my-submissions$/)
  await expect(page.locator('#my-submissions tbody tr').first().locator('.practice-badge')).toHaveText('練習')
  await expect(page.locator('#my-submissions tbody tr').last().locator('.practice-badge')).toHaveCount(0)
  await expect(page.locator('#my-submissions tbody tr')).toHaveCount(2)
  await menu.getByRole('link', { name: '問題', exact: true }).click()
  await page.getByRole('region', { name: 'コンテストの問題' }).getByRole('link', { name: 'A + B', exact: true }).click()
  await expect(page.getByRole('navigation', { name: '問題メニュー' })).toHaveCount(0)
})

test('a team member lists the team\'s submissions from the menu while the contest runs', async ({ page }) => {
  const team = { id: 'tttttttt-tttt-4ttt-8ttt-tttttttttttt', name: 'Team', members: ['ui_test', 'mate'], onsite: false }
  const requested = await open(page, { ...running, teamMode: true, team, canViewSubmissions: true }, { team: [mateB, ownA], 0: [mateB, ownA] })
  const menu = page.getByRole('navigation', { name: 'コンテストメニュー' })
  await expect(menu.getByRole('link')).toHaveText(['概要', '問題', '順位表', 'チームの提出', '解説', '質問', 'コード実行'])
  await menu.getByRole('link', { name: 'チームの提出', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('チームの提出')
  await expect(page.getByRole('navigation', { name: 'パンくずリスト' })).toHaveText(`${running.title}/チームの提出`)
  const rows = page.locator('#my-submissions tbody tr')
  await expect(rows.first().getByRole('link', { name: '詳細', exact: true })).toHaveAttribute('href', `${path}/submissions/${shortId(mateB.id)}?from=contest-team`)
  await expect(rows.last().getByRole('link', { name: '詳細', exact: true })).toHaveAttribute('href', `${path}/my-submissions/${shortId(ownA.id)}?from=contest-team`)
  await fits(page)
  await visit(page, `${path}?view=submissions`)
  await expect(page.locator('#submissions .notice')).toHaveText(`${whole}開催中のチームの提出は「チームの提出」で確認できます。`)
  expect(requested).toContain('team')
  expect(requested).not.toContain('0')
})
