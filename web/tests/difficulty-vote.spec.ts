import { expect, test } from './fixtures/account'

test('difficulty voting supports reload, changes, failed saves, and withdrawal', async ({ page }) => {
  let difficulty: number | null = null
  let fail = false
  await page.route('**/api/my/favorites/*', route => route.fulfill({ json: { favorited: false, favoriteCount: 0 } }))
  await page.route('**/api/my/difficulty-votes/*', route => {
    const method = route.request().method()
    if (method !== 'GET') {
      if (fail) return route.fulfill({ status: 503, json: {} })
      difficulty = method === 'DELETE' ? null : route.request().postDataJSON().difficulty
    }
    return route.fulfill({ json: { difficulty, difficultyAverage: difficulty, difficultyVoteCount: difficulty === null ? 0 : 1 } })
  })
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  const panel = page.getByRole('dialog', { name: '難易度評価', exact: true })
  const open = () => page.getByRole('button', { name: '難易度評価', exact: true }).click()
  await open()
  const select = panel.getByLabel('あなたの評価')
  await expect(panel).toContainText('未投票')
  await expect(select).toBeEnabled()
  await select.click()
  await page.getByRole('option', { name: 'Lv.3', exact: true }).click()
  await panel.getByRole('button', { name: '投票する', exact: true }).click()
  await expect(page.locator('.problem-meta')).toContainText('Lv.3.0')
  await expect(page.locator('.problem-meta')).toContainText('（1票）')
  await page.reload()
  await open()
  await expect(select).toContainText('Lv.3')
  await select.click()
  await page.getByRole('option', { name: 'Lv.10', exact: true }).click()
  fail = true
  await panel.getByRole('button', { name: '投票を変更', exact: true }).click()
  await expect(panel.getByRole('alert')).toContainText('投票を保存できませんでした')
  await expect(panel).toContainText('Lv.3.0')
  fail = false
  await panel.getByRole('button', { name: '投票を変更', exact: true }).click()
  await expect(panel).not.toBeVisible()
  await expect(page.locator('.problem-meta')).toContainText('Lv.10.0')
  await open()
  await expect(page.locator('.problem-meta')).toContainText('（1票）')
  await panel.getByRole('button', { name: '投票を取り消す', exact: true }).click()
  await expect(panel).not.toBeVisible()
  await expect(page.locator('.problem-meta')).toContainText('（0票）')
  await open()
  await expect(panel.getByRole('button', { name: '投票する', exact: true })).toBeDisabled()
})

test('vote loading can be retried and signed-out users see a login link', async ({ page }) => {
  let fail = true
  await page.route('**/api/my/difficulty-votes/*', route => route.fulfill(fail
    ? { status: 503, json: {} }
    : { json: { difficulty: 5, difficultyAverage: 5, difficultyVoteCount: 1 } }))
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  const panel = page.getByRole('dialog', { name: '難易度評価', exact: true })
  const open = () => page.getByRole('button', { name: '難易度評価', exact: true }).click()
  await open()
  await expect(panel.getByLabel('あなたの評価')).toBeDisabled()
  fail = false
  await panel.getByRole('button', { name: '再試行' }).click()
  await expect(panel.getByLabel('あなたの評価')).toContainText('Lv.5')
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.reload()
  await open()
  await expect(panel.getByRole('link', { name: 'ログインして難易度を投票' })).toHaveAttribute('href', /\/login\?next=/)
  await expect(panel.getByRole('combobox')).toHaveCount(0)
})

for (const width of [320, 375, 414, 768]) {
  test(`difficulty modal fits ${width}px and restores focus on Escape`, async ({ page }, testInfo) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/problems/11111111-1111-4111-8111-111111111111')
    const trigger = page.getByRole('button', { name: '難易度評価', exact: true })
    const dialog = page.getByRole('dialog', { name: '難易度評価', exact: true })
    await expect(dialog).not.toBeVisible()
    await trigger.click()
    await expect(dialog.getByLabel('あなたの評価')).toBeEnabled()
    await dialog.getByLabel('あなたの評価').click()
    await expect(dialog.locator('.difficulty-options .difficulty-dot')).toHaveCount(9)
    await expect(dialog.getByRole('option', { name: 'Lv.10', exact: true }).locator('svg')).toHaveCount(1)
    await page.screenshot({ path: testInfo.outputPath(`difficulty-options-${width}.png`) })
    await page.keyboard.press('Escape')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByRole('listbox')).toHaveCount(0)
    await dialog.getByRole('combobox').press('ArrowDown')
    await dialog.getByRole('combobox').press('End')
    await dialog.getByRole('combobox').press('Enter')
    await expect(dialog.getByRole('combobox')).toContainText('Lv.10')
    await dialog.getByRole('combobox').click()
    await page.getByRole('option', { name: 'Lv.8', exact: true }).click()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const bounds = await dialog.boundingBox()
    expect(bounds!.x).toBeGreaterThanOrEqual(0)
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`difficulty-modal-${width}.png`) })
    await page.keyboard.press('Escape')
    await expect(dialog).not.toBeVisible()
    await expect(trigger).toBeFocused()
    await trigger.click()
    await expect(dialog.getByLabel('あなたの評価')).not.toContainText('Lv.8')
    await dialog.getByRole('button', { name: '閉じる', exact: true }).click()
    await expect(dialog).not.toBeVisible()
  })
}
