import { expect, test } from './fixtures/account'

const id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const hidden = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const updatedAt = '2026-09-26T00:00:00Z'

test('featured editions show two slots, revivals, vacancies and the Ultimate label', async ({ page }, testInfo) => {
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: [] } }))
  await page.goto('/featured')
  await expect(page.getByRole('heading', { name: '定期便', exact: true })).toBeVisible()
  const notes = page.locator('.delivery-details')
  await expect(notes.locator('ul')).toBeHidden()
  await notes.locator('summary').focus()
  await page.keyboard.press('Enter')
  await expect(notes.locator('li')).toHaveCount(5)
  await expect(notes).toContainText('次回予定を含む')
  await page.keyboard.press('Enter')
  await expect(notes.locator('ul')).toBeHidden()
  await expect(page.locator('.featured-page > p')).toHaveCount(0)
  await expect(page.locator('.round')).toHaveCount(2)
  // The heading names the edition instead of a tagline.
  await expect(page.getByRole('heading', { name: '定期便 vol.13', exact: true })).toBeVisible()
  await expect(page.getByText('次の一問が、届く。')).toHaveCount(0)
  await expect(page.locator('.round h3')).toContainText(['vol.12', 'vol.11'])
  await expect(page.locator('.round').first().locator('.slot')).toHaveCount(2)
  await expect(page.getByRole('heading', { name: 'Ultimate', exact: true })).toBeVisible()
  await expect(page.getByText('欠番', { exact: true })).toBeVisible()
  await expect(page.getByText('解説公開済み', { exact: false }).first()).toBeVisible()
  await expect(page.getByRole('link', { name: '定期便の新作', exact: true })).toHaveAttribute('href', `/problems/${hidden}`)
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`featured-${width}.png`), fullPage: true })
  }
})

test('authors choose a preference, see validation failures, apply and withdraw', async ({ page }) => {
  let applied = false
  let attempts = 0
  let withdrawals = 0
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: applied ? [{ problemId: id, title: '応募する問題', preference: 'later', enteredAt: updatedAt }] : [] } }))
  await page.route(/\/api\/my\/problems(?:\?.*)?$/, route => route.fulfill({ json: { items: [
    { id, title: '応募する問題', featuredPreference: applied ? 'later' : '', updatedAt },
    { id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', title: '過去に公開済み', everPublished: true, updatedAt },
    { id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', title: 'コンテスト用', contestId: 'contest', updatedAt },
  ], nextCursor: '' } }))
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
    id, version: 3, updatedAt, draft: { title: '応募する問題', markdown: '本文', editorial: '解説', difficulty: 4, timeLimitMs: '2000', memoryLimitMb: '256', testCases: [] },
  } }))
  await page.route(`**/api/my/problems/${id}/featured`, route => {
    const body = route.request().postDataJSON()
    expect(body.version).toBe(3)
    if (body.preference === '') {
      if (++withdrawals === 1) return route.fulfill({ status: 503, json: {} })
      applied = false
      return route.fulfill({ status: 204 })
    }
    expect(body.preference).toBe('later')
    if (++attempts === 1) return route.fulfill({ status: 400, json: { data: { code: 'featured_ineligible' } } })
    applied = true
    return route.fulfill({ status: 204 })
  })
  await page.goto('/featured')
  await page.getByRole('button', { name: '新作を応募' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('投稿する問題').locator('option')).toHaveText(['問題を選択してください', '応募する問題'])
  await expect(dialog).not.toContainText('Ultimate')
  const rules = dialog.locator('.application-details')
  await expect(rules.locator('ul')).toBeHidden()
  await rules.locator('summary').click()
  await expect(rules.locator('li')).toHaveCount(6)
  await expect(rules).toContainText('応募は作者ごとに3件まで')
  await rules.locator('summary').click()
  await expect(rules.locator('ul')).toBeHidden()
  await dialog.getByLabel('投稿する問題').selectOption(id)
  await dialog.getByLabel('公開の希望').selectOption('later')
  await dialog.getByRole('button', { name: '応募する', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('難易度・解説')
  await dialog.getByRole('button', { name: '応募する', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('heading', { name: '応募中の問題', exact: true })).toHaveCount(0)
  await page.goto('/my?tab=problems')
  const row = page.getByRole('row').filter({ has: page.getByRole('rowheader', { name: '応募する問題', exact: true }) })
  await expect(row).toContainText('応募中（定期便）')
  await row.getByRole('img', { name: 'ゆっくりで良い' }).focus()
  await expect(row.getByRole('tooltip')).toHaveText('ゆっくりで良い')
  const withdrawButton = row.getByRole('button', { name: '応募する問題の定期便応募を取り下げる' })
  const confirmation = page.getByRole('dialog', { name: '定期便の応募を取り下げますか？' })
  await withdrawButton.click()
  await expect(confirmation).toContainText('応募する問題')
  await expect(confirmation.getByRole('button', { name: 'キャンセル' })).toBeFocused()
  expect(withdrawals).toBe(0)
  await confirmation.getByRole('button', { name: 'キャンセル' }).click()
  await expect(confirmation).toBeHidden()
  await expect(withdrawButton).toBeFocused()
  await expect(row).toContainText('応募中（定期便）')
  await withdrawButton.click()
  await page.keyboard.press('Escape')
  await expect(confirmation).toBeHidden()
  expect(withdrawals).toBe(0)
  await page.setViewportSize({ width: 375, height: 812 })
  await withdrawButton.click()
  const bounds = (await confirmation.boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(375)
  await confirmation.getByRole('button', { name: '取り下げる', exact: true }).click()
  await expect(confirmation.getByRole('alert')).toBeVisible()
  expect(applied).toBe(true)
  await expect(row).toContainText('応募中（定期便）')
  await confirmation.getByRole('button', { name: '取り下げる', exact: true }).click()
  await expect(confirmation).toBeHidden()
  expect(withdrawals).toBe(2)
  await expect(row).toContainText('準備中')
  await expect(row.getByRole('img')).toHaveCount(0)
})

test('hidden editorial and other submissions show the unlock time while own results stay accessible', async ({ page }) => {
  await page.goto(`/problems/${hidden}?view=editorial`)
  await expect(page.getByText('解説は公開待ちです。')).toBeVisible()
  await expect(page.getByText(/解法のネタバレを含めないでください/)).toBeVisible()
  await page.getByRole('link', { name: 'すべての提出', exact: true }).click()
  await expect(page.getByText(/他者の提出は解禁後に閲覧できます/)).toBeVisible()
  await expect(page.locator('.problem-submissions')).toHaveCount(0)
  await page.getByRole('link', { name: '自分の提出', exact: true }).click()
  await expect(page.getByRole('heading', { name: '自分の提出', exact: true })).toBeVisible()
})

test('featured history has a recoverable error state', async ({ page }) => {
  await page.goto('/problems')
  await page.route('**/api/featured**', route => route.fulfill({ status: 502, json: {} }))
  await page.locator('header').getByRole('link', { name: '定期便', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: '定期便を取得できませんでした' })).toBeVisible()
  await expect(page.getByRole('button', { name: '再試行', exact: true }).first()).toBeVisible()
})


test('home and featured share upcoming credits and waiting counts without preview titles', async ({ page }, testInfo) => {
  await page.goto('/')
  const delivery = page.getByRole('region', { name: 'ShareOJ定期便' })
  await expect(delivery).toContainText('次回予告')
  await expect(delivery.locator('.delivery-problem')).toHaveCount(2)
  await expect(delivery.locator('.delivery-waiting')).toContainText('Easy 3問')
  await expect(delivery.locator('.delivery-waiting')).toContainText('Hard 2問')
  await expect(delivery.locator('.delivery-problem').first()).toContainText('writeralice')
  await expect(delivery.locator('.delivery-problem').first()).toContainText('testerbob')
  await expect(delivery.locator('a[href^="/problems/"]')).toHaveCount(0)
  await expect(delivery).not.toContainText('定期便の新作')
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`home-preview-${width}.png`), fullPage: true })
  }
  await delivery.getByRole('link', { name: '定期便・新作の応募' }).focus()
  await expect(delivery.getByRole('link', { name: '定期便・新作の応募' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/featured$/)
  await expect(delivery).toContainText('次回予告')
  await expect(delivery.locator('.delivery-waiting')).toContainText('Easy 3問')
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.screenshot({ path: testInfo.outputPath('featured-preview-dark.png'), fullPage: true })
})

test('published edition shows both titles and credits, then switches back to preview', async ({ page }, testInfo) => {
  await page.clock.install()
  await page.goto('/problems')
  const payload = await (await page.request.get('/api/featured')).json()
  payload.current = payload.items[0]
  await page.route('**/api/featured**', route => route.fulfill({ json: payload }))
  await page.locator('header').getByRole('link', { name: /ShareOJ/ }).click()
  const delivery = page.getByRole('region', { name: 'ShareOJ定期便' })
  await expect(delivery).toContainText('公開中の問題')
  await expect(delivery.getByRole('link', { name: 'A + B' })).toBeVisible()
  await expect(delivery.getByRole('link', { name: '定期便の新作' })).toHaveAttribute('href', `/problems/${hidden}`)
  await expect(delivery).toContainText('Ultimate')
  await expect(delivery).toContainText('解説・他者の提出')
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`home-current-${width}.png`), fullPage: true })
  }
  await delivery.getByRole('link', { name: '定期便・新作の応募' }).click()
  await expect(delivery).toContainText('公開中の問題')
  await page.screenshot({ path: testInfo.outputPath('featured-current.png'), fullPage: true })
  payload.current = null
  await page.clock.fastForward(61000)
  await expect(delivery).toContainText('次回予告')
  await expect(delivery.locator('a[href^="/problems/"]')).toHaveCount(0)
  await expect(page.locator('.round').first()).toContainText('定期便の新作')
})

test('empty inventory and long credits fit a narrow screen', async ({ page }) => {
  await page.goto('/problems')
  const payload = await (await page.request.get('/api/featured')).json()
  payload.waiting = { easy: 0, hard: 0 }
  payload.nextSlots[0] = { slot: 'easy', kind: 'missing', writer: '', testers: [], difficulty: null }
  payload.nextSlots[1] = { slot: 'hard', kind: 'revival', writer: 'a'.repeat(32), testers: ['b'.repeat(32), 'c'.repeat(32)], difficulty: 10 }
  await page.route('**/api/featured**', route => route.fulfill({ json: payload }))
  await page.setViewportSize({ width: 320, height: 1000 })
  await page.locator('header').getByRole('link', { name: '定期便', exact: true }).click()
  const delivery = page.getByRole('region', { name: 'ShareOJ定期便' })
  await expect(delivery).toContainText('出題する問題を募集中です。')
  await expect(delivery).toContainText('復刻')
  await expect(delivery.locator('.delivery-waiting')).toContainText('Easy 0問')
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320)
})

test('home keeps its navigation and retries when the delivery API fails', async ({ page }) => {
  await page.goto('/problems')
  let failing = true
  const payload = await (await page.request.get('/api/featured')).json()
  await page.route('**/api/featured**', route => failing ? route.fulfill({ status: 502, json: {} }) : route.fulfill({ json: payload }))
  await page.locator('header').getByRole('link', { name: /ShareOJ/ }).click()
  await expect(page.getByRole('heading', { level: 1 })).toContainText('考える楽しさを、')
  await expect(page.getByRole('alert')).toContainText('定期便を取得できませんでした')
  failing = false
  await page.getByRole('button', { name: '再試行' }).click()
  await expect(page.getByRole('region', { name: 'ShareOJ定期便' })).toContainText('次回予告')
})
