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
