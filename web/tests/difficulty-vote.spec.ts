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
  const panel = page.getByRole('region', { name: '難易度投票' })
  const select = panel.getByLabel('あなたの評価')
  await expect(panel).toContainText('未投票')
  await expect(select).toBeEnabled()
  await select.selectOption('3')
  await panel.getByRole('button', { name: '投票する', exact: true }).click()
  await expect(panel).toContainText('Lv.3.0')
  await expect(panel).toContainText('（1票）')
  await page.reload()
  await expect(select).toHaveValue('3')
  await select.selectOption('10')
  fail = true
  await panel.getByRole('button', { name: '投票を変更', exact: true }).click()
  await expect(panel.getByRole('alert')).toContainText('投票を保存できませんでした')
  await expect(panel).toContainText('Lv.3.0')
  fail = false
  await panel.getByRole('button', { name: '投票を変更', exact: true }).click()
  await expect(panel).toContainText('Lv.10.0')
  await expect(panel).toContainText('（1票）')
  await panel.getByRole('button', { name: '投票を取り消す', exact: true }).click()
  await expect(panel).toContainText('未投票')
  await expect(panel).toContainText('（0票）')
  await expect(panel.getByRole('button', { name: '投票する', exact: true })).toBeDisabled()
})

test('vote loading can be retried and signed-out users see a login link', async ({ page }) => {
  let fail = true
  await page.route('**/api/my/difficulty-votes/*', route => route.fulfill(fail
    ? { status: 503, json: {} }
    : { json: { difficulty: 5, difficultyAverage: 5, difficultyVoteCount: 1 } }))
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  const panel = page.getByRole('region', { name: '難易度投票' })
  await expect(panel.getByLabel('あなたの評価')).toBeDisabled()
  fail = false
  await panel.getByRole('button', { name: '再試行' }).click()
  await expect(panel.getByLabel('あなたの評価')).toHaveValue('5')
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.reload()
  await expect(panel.getByRole('link', { name: 'ログインして難易度を投票' })).toHaveAttribute('href', /\/login\?next=/)
  await expect(panel.getByRole('combobox')).toHaveCount(0)
})
