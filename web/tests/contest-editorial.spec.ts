import { expect, test } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

// The fixture contest is running. The server tells each viewer whether the editorial is readable before the end,
// so co-setters get the tab as well as the problem's author and testers.
const path = `/contests/${shortId('88888888-8888-4888-8888-888888888888')}/A`

for (const visible of [true, false]) {
  test(`running contest editorial tab follows the server: ${visible}`, async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'viewer' } } }))
    await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'viewer', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
    await page.route('**/api/contests/88888888-8888-4888-8888-888888888888/problems/**', async route => {
      const response = await route.fetch()
      await route.fulfill({ response, json: { ...await response.json(), canReadEditorial: visible } })
    })
    // Logging in refetches the problem, so the menu follows this viewer's answer.
    const refreshed = page.waitForResponse('**/api/contests/88888888-8888-4888-8888-888888888888/problems/**')
    await page.goto(path)
    await refreshed
    const menu = page.getByRole('navigation', { name: '問題メニュー' })
    if (!visible) {
      await expect(menu.getByRole('link')).toHaveText(['問題文', '自分の提出', 'すべての提出'])
      return
    }
    await expect(menu.getByRole('link')).toHaveText(['問題文', '解説', '自分の提出', 'すべての提出'])
    await menu.getByRole('link', { name: '解説', exact: true }).click()
    await expect(page).toHaveURL(`${path}?view=editorial`)
    await expect(page.getByRole('heading', { name: '解説', exact: true })).toBeVisible()
  })
}

for (const status of ['running', 'ended'] as const) {
  test(`contest editorial list links readable editorials: ${status}`, async ({ page }) => {
    const id = '88888888-8888-4888-8888-888888888888'
    const [a, b] = ['11111111-1111-4111-8111-111111111111', '33333333-3333-4333-8333-333333333333']
    const contest = {
      id, title: '解説一覧テスト', author: 'setter', description: '', status,
      startsAt: '2026-09-15T12:00:00Z', endsAt: '2026-09-15T14:00:00Z', penaltyMinutes: 5,
      version: 1, official: status === 'ended', canEdit: false, canViewSubmissions: true,
      problems: [{ id: a, title: 'A + B', points: 100, hasEditorial: true }, { id: b, title: 'A - B', points: 200, ...status === 'ended' ? { hasEditorial: false } : {} }],
    }
    await page.route(`**/api/contests/${id}**`, route => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/standings')) return route.fulfill({ json: [] })
      if (path.endsWith('/editorials')) return route.fulfill({ json: { items: [{ problemId: a, id: '44444444-4444-4444-8444-444444444444', title: '別解の記事', author: 'bob', publishedAt: '2026-09-15T15:00:00Z', isOperator: false }] } })
      if (path.includes('/problems/')) return route.fulfill({ json: { id: a, title: 'A + B', author: 'setter', testers: [], markdown: '足し算', editorial: '## 解説\n\n足すだけです。', timeLimitMs: '2000', memoryLimitMb: '512', canReadEditorial: true } })
      return route.fulfill({ json: contest })
    })
    // The server render reads the fixture API, so refresh in the browser to use the routed contest.
    await page.goto(`/contests/${shortId(id)}?view=standings`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: '今すぐ更新' }).click()
    await expect(page.locator('.site-header')).toContainText(contest.title)
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '解説一覧', exact: true }).click()
    await expect(page).toHaveURL(`/contests/${shortId(id)}?view=editorials`)
    await expect(page.getByText('解説はコンテスト終了後に公開されます。', { exact: false })).toBeVisible({ visible: status === 'running' })
    const list = page.getByRole('region', { name: '解説一覧のスクロール領域' })
    // User editorials appear once the problems become public at the end.
    await expect(list.getByRole('link')).toHaveText(status === 'ended' ? ['A + B', '別解の記事'] : ['A + B'])
    await expect(list.getByRole('columnheader', { name: 'ユーザー解説' })).toHaveCount(status === 'ended' ? 1 : 0)
    await expect(list.getByRole('rowheader', { name: /A - B/ })).toContainText(status === 'ended' ? '未登録' : '終了後に公開')
    await list.getByRole('link', { name: 'A + B', exact: true }).click()
    await expect(page).toHaveURL(`/contests/${shortId(id)}/A?view=editorial`)
    await expect(page.getByText('足すだけです。')).toBeVisible()
    await expect(page.getByRole('region', { name: 'ユーザー解説' })).toHaveCount(status === 'ended' ? 1 : 0)
  })
}
