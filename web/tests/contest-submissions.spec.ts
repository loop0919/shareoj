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
    await expect(page.getByRole('heading', { name: contest.title })).toBeVisible()
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '提出一覧', exact: true }).click()
    if (allowed) {
      await expect(page.getByRole('cell', { name: 'participant', exact: true })).toBeVisible()
      await expect(page.getByRole('cell', { name: '42 ms・ 2.50 MB', exact: true })).toBeVisible()
      await expect(page.getByRole('cell', { name: '24 bytes', exact: true })).toBeVisible()
      await page.getByRole('link', { name: 'A + B', exact: true }).click()
      await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText(item.source)
      await page.getByRole('navigation', { name: '提出メニュー' }).getByRole('link', { name: '問題', exact: true }).click()
    } else {
      await expect(page.getByText('提出一覧と提出コードはコンテスト終了後に公開されます。終了前はコンテストセッターとテスターが閲覧できます。')).toBeVisible()
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
      await expect(page.getByText('すべての提出はコンテスト終了後に公開されます。終了前はコンテストセッターとテスターが閲覧できます。')).toBeVisible()
      await expect(page.locator('tbody')).toHaveCount(0)
    }
  })
}
