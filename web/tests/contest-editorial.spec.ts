import { expect, test } from '@playwright/test'

// The fixture contest is running. Its problem is by alice and tested by bob and carol.
const path = '/contests/88888888-8888-4888-8888-888888888888/problems/11111111-1111-4111-8111-111111111111'

for (const [handle, visible] of [['alice', true], ['bob', true], ['dave', false]] as const) {
  test(`running contest editorial tab for ${handle}: ${visible}`, async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: handle } } }))
    await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle, avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
    const profile = page.waitForResponse('**/api/my/profile')
    await page.goto(path)
    await profile
    const menu = page.getByRole('navigation', { name: '問題メニュー' })
    if (!visible) {
      await expect(menu.getByRole('link')).toHaveText(['問題', '自分の提出', 'すべての提出'])
      return
    }
    await expect(menu.getByRole('link')).toHaveText(['問題', '解説', '自分の提出', 'すべての提出'])
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
      if (path.includes('/problems/')) return route.fulfill({ json: { id: a, title: 'A + B', author: 'setter', testers: [], markdown: '足し算', editorial: '## 解説\n\n足すだけです。', timeLimitMs: '2000', memoryLimitMb: '512' } })
      return route.fulfill({ json: contest })
    })
    // The server render reads the fixture API, so refresh in the browser to use the routed contest.
    await page.goto(`/contests/${id}?view=standings`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: '今すぐ更新' }).click()
    await expect(page.getByRole('heading', { name: contest.title })).toBeVisible()
    await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '解説一覧', exact: true }).click()
    await expect(page).toHaveURL(`/contests/${id}?view=editorials`)
    await expect(page.getByText('解説はコンテスト終了後に公開されます。', { exact: false })).toBeVisible({ visible: status === 'running' })
    const list = page.getByRole('region', { name: '解説一覧のスクロール領域' })
    await expect(list.getByRole('link')).toHaveText(['A + B'])
    await expect(list.getByRole('rowheader', { name: /A - B/ })).toContainText(status === 'ended' ? '未登録' : '終了後に公開')
    await list.getByRole('link', { name: 'A + B', exact: true }).click()
    await expect(page).toHaveURL(`/contests/${id}/problems/${a}?view=editorial`)
    await expect(page.getByText('足すだけです。')).toBeVisible()
  })
}
