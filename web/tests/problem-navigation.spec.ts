import { expect, test } from './fixtures/account'

const id = '11111111-1111-4111-8111-111111111111'

for (const handle of ['alice', 'bob', 'reader', null]) {
  test(`problem editing navigation for ${handle ?? 'anonymous'}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: handle === 'alice' ? 320 : 375, height: 812 })
    await page.route('**/api/my/profile', route => route.fulfill({ json: {
      profile: { handle: handle ?? 'reader', avatar: '', version: 1, createdAt: '2026-09-10T00:00:00Z' },
    } }))
    if (!handle) await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
    await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
      id, version: 1, publishedVersion: 1, updatedAt: '2026-09-10T00:00:00Z',
      draft: { title: 'A + B', markdown: '保存済みの問題文', timeLimitMs: '2000', memoryLimitMb: '512' },
    } }))
    await page.goto(`/problems/${id}`)
    const edit = page.getByRole('link', { name: '作問画面へ', exact: true })
    if (handle !== 'alice' && handle !== 'bob') {
      // Wait for account loading before asserting that the link stays hidden.
      await expect(page.getByRole('navigation', { name: 'メインナビゲーション' })).toBeVisible()
      await expect(page.getByRole('link', { name: handle ? 'マイページ' : 'ログイン', exact: true }).first()).toBeVisible()
      await expect(edit).toHaveCount(0)
      return
    }
    await expect(edit).toHaveAttribute('href', `/problems/new?problem=${id}`)
    await edit.click()
    await expect(page.locator('#problem-title')).toHaveValue('A + B')
    const view = page.getByRole('link', { name: '問題を見る', exact: true })
    await expect(view).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('editor-navigation.png') })
    await page.locator('#problem-title').fill('未保存の変更')
    await view.click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.getByRole('button', { name: '編集を続ける', exact: true }).click()
    await expect(page.locator('#problem-title')).toHaveValue('未保存の変更')
    await view.click()
    await page.getByRole('button', { name: '保存せずに移動', exact: true }).click()
    await expect(page).toHaveURL(`/problems/${id}`)
    await page.getByRole('link', { name: '解説', exact: true }).click()
    await expect(edit).toBeVisible()
    await edit.click()
    await expect(page.locator('#problem-title')).toHaveValue('A + B')
  })
}

test('new problems get a viewing link only after a successful save', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await expect(page.locator('#problem-title')).toBeEnabled()
  const view = page.getByRole('link', { name: '問題を見る', exact: true })
  await expect(view).toHaveCount(0)
  await page.route('**/api/my/problems/*', route => route.fulfill({ status: 503, json: {} }))
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'failed')
  await expect(view).toHaveCount(0)
  await page.unroute('**/api/my/problems/*')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  const savedId = new URL(page.url()).searchParams.get('problem')!
  await expect(view).toHaveAttribute('href', `/problems/${savedId}`)
})
