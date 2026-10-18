import { expect, test } from '@playwright/test'

const id = '88888888-8888-4888-8888-888888888888'
const pid = '11111111-1111-4111-8111-111111111111'
const sid = '22222222-2222-4222-8222-222222222222'

for (const allowed of [true, false]) {
  test(`contest submission access follows the API permission: ${allowed}`, async ({ page }) => {
    const contest = {
      id, title: '閲覧権限テスト', author: 'setter', description: '', status: 'running',
      startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5,
      version: 1, official: !allowed, canEdit: false, canViewSubmissions: allowed,
      problems: [{ id: pid, title: 'A + B', points: 100 }],
    }
    const item = {
      id: sid, contestId: id, problemId: pid, problemTitle: 'A + B', author: 'participant',
      problemVersion: 1, runtime: 'cpp17', sourceBytes: 24, source: 'int main() { return 42; }',
      createdAt: '2026-09-15T12:10:00Z', status: 'DONE', result: { verdict: 'WA', passed: 0, total: 1, cpuTimeMs: 42, memoryBytes: 2500000, cases: [] },
    }
    await page.route(`**/api/contests/${id}**`, route => {
      const path = new URL(route.request().url()).pathname
      if (path.includes('/submissions')) return route.fulfill(allowed ? { json: path.endsWith(sid) ? item : { items: [item], hasMore: false } } : { status: 404, json: {} })
      if (path.endsWith('/standings')) return route.fulfill({ json: [] })
      if (path.includes('/problems/')) return route.fulfill({ json: { id: pid, title: 'A + B', author: 'setter', testers: [], markdown: '足し算', editorial: '', timeLimitMs: '2000', memoryLimitMb: '512' } })
      return route.fulfill({ json: contest })
    })
    await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: '今すぐ更新' }).click()
    await expect(page.locator('.site-header')).toContainText(contest.title)
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '提出一覧', exact: true }).click()
    if (allowed) {
      await expect(page.getByRole('cell', { name: 'participant', exact: true })).toBeVisible()
      await expect(page.getByRole('cell', { name: '42 ms・ 2.50 MB', exact: true })).toBeVisible()
      await expect(page.getByRole('cell', { name: '24 bytes', exact: true })).toBeVisible()
      await page.getByRole('link', { name: 'A + B', exact: true }).click()
      await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText(item.source)
      await page.getByRole('navigation', { name: '提出メニュー' }).getByRole('link', { name: '問題', exact: true }).click()
    } else {
      await expect(page.getByText('提出一覧と提出コードはコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。')).toBeVisible()
      await expect(page.locator('tbody')).toHaveCount(0)
      await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
      await page.getByRole('link', { name: 'A + B', exact: true }).click()
    }
    await page.getByRole('navigation', { name: '問題メニュー' }).getByRole('link', { name: 'すべての提出', exact: true }).click()
    if (allowed) {
      await expect(page.getByRole('cell', { name: 'participant', exact: true })).toBeVisible()
      await expect(page.getByRole('cell', { name: '42 ms・ 2.50 MB', exact: true })).toBeVisible()
      await expect(page.getByRole('cell', { name: '24 bytes', exact: true })).toBeVisible()
      await page.getByRole('link', { name: '詳細', exact: true }).click()
      await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText(item.source)
    } else {
      await expect(page.getByText('提出はコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。')).toBeVisible()
      await expect(page.locator('tbody')).toHaveCount(0)
    }
  })
}

// ADR 0023: others do not see the submissions made after the freeze until the results are published.
for (const phase of ['running', 'ended', 'revealed'] as const) {
  test(`both submission lists explain the hidden frozen submissions: ${phase}`, async ({ page }) => {
    // 21:00–23:00 in Japan, freezing at 22:00.
    const contest = {
      id, title: '凍結した提出のテスト', author: 'setter', description: '', status: phase === 'running' ? 'running' : 'ended',
      startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5, version: 1, official: true, canEdit: false,
      canViewSubmissions: phase !== 'running', freezeMinutes: 60, freezeAt: '2026-09-15T13:00:00Z', revealed: phase === 'revealed', resultsFinal: phase === 'revealed',
      problems: [{ id: pid, title: 'A + B', points: 100 }],
    }
    await page.route(`**/api/contests/${id}**`, route => {
      const path = new URL(route.request().url()).pathname
      if (path.includes('/submissions')) return route.fulfill(phase === 'running' ? { status: 404, json: {} } : { json: { items: [], hasMore: false } })
      if (path.endsWith('/standings')) return route.fulfill({ json: [] })
      if (path.includes('/problems/')) return route.fulfill({ json: { id: pid, title: 'A + B', author: 'setter', testers: [], markdown: '足し算', editorial: '', timeLimitMs: '2000', memoryLimitMb: '512' } })
      return route.fulfill({ json: contest })
    })
    await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: '今すぐ更新' }).click()
    await expect(page.locator('.site-header')).toContainText(contest.title)
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '提出一覧', exact: true }).click()
    const note = '22:00以降の提出は、結果の公開まで本人とスタッフだけが見られます。'
    const section = page.locator('#submissions')
    if (phase === 'running') await expect(section.locator('.notice')).toHaveText(`提出一覧と提出コードはコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。${note}`)
    else await expect(section.locator('p.muted').first()).toHaveText(`提出コードを閲覧できます。練習提出も掲載します。${phase === 'ended' ? note : ''}`)
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '問題', exact: true }).click()
    await page.getByRole('link', { name: 'A + B', exact: true }).click()
    await page.getByRole('navigation', { name: '問題メニュー' }).getByRole('link', { name: 'すべての提出', exact: true }).click()
    if (phase === 'running') await expect(page.locator('.submission-notice')).toHaveText(`提出はコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。${note}`)
    else {
      await expect(page.getByRole('heading', { name: 'すべての提出' })).toBeVisible()
      await expect(page.locator('.submission-freeze')).toHaveText(phase === 'ended' ? [note] : [])
    }
    // The submitter's own list needs no such note.
    await page.getByRole('navigation', { name: '問題メニュー' }).getByRole('link', { name: '自分の提出', exact: true }).click()
    await expect(page.locator('.submission-freeze')).toHaveCount(0)
  })
}
