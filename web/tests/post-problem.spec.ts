import { expect, test } from './fixtures/account'

test('posting selects only unpublished standalone problems across pages and handles failures', async ({ page }, testInfo) => {
  await page.goto('/problems/new')
  await page.locator('#problem-title').fill('投稿する下書き')
  await page.getByRole('combobox', { name: '難易度（作成者設定）' }).click()
  await page.getByRole('option', { name: 'Lv.2', exact: true }).click()
  await page.locator('#problem-source').fill('問題本文')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/problem=/)
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  const id = new URL(page.url()).searchParams.get('problem')!
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await expect(page.getByRole('button', { name: '公開する', exact: true })).toHaveCount(0)
  const updatedAt = '2026-09-10T00:00:00Z'
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: new URL(route.request().url()).searchParams.get('cursor') ? {
    items: [{ id, title: '投稿する下書き', publishedVersion: 0, updatedAt }], nextCursor: '',
  } : {
    items: [
      { id: '11111111-1111-4111-8111-111111111111', title: '公開済み', publishedVersion: 1, updatedAt },
      { id: '22222222-2222-4222-8222-222222222222', title: 'コンテスト用', contestId: 'contest', publishedVersion: 0, updatedAt },
    ], nextCursor: 'next',
  } }))
  let attempts = 0
  await page.route(`**/api/my/problems/${id}/publication`, route => {
    expect(route.request().postDataJSON()).toMatchObject({ publish: true, version: expect.any(Number) })
    attempts++
    return route.fulfill(attempts === 1 ? { status: 409, json: {} } : { json: {} })
  })
  await page.goto('/problems')
  await page.getByRole('button', { name: '問題投稿', exact: true }).click()
  const select = page.getByLabel('投稿する問題')
  await expect(select.locator('option')).toHaveText(['問題を選択してください', '投稿する下書き'])
  await expect(page.getByRole('dialog').getByRole('button', { name: '投稿', exact: true })).toBeDisabled()
  await expect(page).toHaveURL('/problems')
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(select).toHaveCSS('min-height', '44px')
    const box = (await page.getByRole('dialog').boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`post-modal-${width}.png`) })
  }
  await select.selectOption(id)
  await page.getByRole('button', { name: 'キャンセル' }).click()
  await expect(page.getByRole('dialog')).not.toBeVisible()
  expect(attempts).toBe(0)
  await page.getByRole('button', { name: '問題投稿', exact: true }).click()
  await select.selectOption(id)
  await page.getByRole('dialog').getByRole('button', { name: '投稿', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('別の画面で更新されています')
  await page.getByRole('dialog').getByRole('button', { name: '投稿', exact: true }).click()
  await expect(page).toHaveURL(`/problems/${id}`)
})

test('posting has an empty state', async ({ page }) => {
  await page.goto('/problems')
  await page.getByRole('button', { name: '問題投稿', exact: true }).click()
  await expect(page.getByText('投稿できる未公開の問題はありません。')).toBeVisible()
  await expect(page.getByRole('link', { name: '新規問題を作成' })).toBeVisible()
})

test('Escape closes the modal and returns focus to the posting button', async ({ page }) => {
  await page.goto('/problems')
  const trigger = page.getByRole('button', { name: '問題投稿', exact: true })
  await trigger.click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).not.toBeVisible()
  await expect(trigger).toBeFocused()
})

test('guests can log in and return to the posting modal', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto('/problems?post=1')
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('link', { name: 'ログインして投稿する' })).toHaveAttribute('href', '/login?next=/problems?post=1')
})

test('schedule in Japan time, show the reservation, update it and cancel it', async ({ page }, testInfo) => {
  const id = '11111111-1111-4111-8111-111111111111'
  let scheduledAt: string | undefined
  let version = 1
  const problem = () => ({ id, version, scheduledAt, publishedVersion: 0, updatedAt: '2026-09-10T00:00:00Z', draft: { title: '予約する問題', markdown: '問題文', difficulty: 3, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] } })
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items: [{ ...problem(), title: '予約する問題' }], nextCursor: '' } }))
  await page.route('**/api/my/problems', route => route.fulfill({ json: { items: [{ ...problem(), title: '予約する問題' }], nextCursor: '' } }))
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: problem() }))
  const requests: Record<string, unknown>[] = []
  await page.route(`**/api/my/problems/${id}/publication`, route => {
    const body = route.request().postDataJSON()
    expect(body.version).toBe(version)
    requests.push(body)
    scheduledAt = body.publish ? body.scheduledAt : undefined
    version++
    return route.fulfill({ json: problem() })
  })
  await page.goto('/problems?post=1')
  const dialog = page.getByRole('dialog')
  await page.getByLabel('投稿する問題').selectOption(id)
  await page.getByLabel('公開するタイミング').selectOption('schedule')
  const input = page.getByLabel('公開日時（日本時間）')
  await input.fill('2000-01-01T23:00')
  await dialog.getByRole('button', { name: '予約する', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('未来の公開日時')
  expect(requests).toHaveLength(0)
  const year = new Date().getUTCFullYear() + 1
  await input.fill(`${year}-01-02T23:00`)
  await page.setViewportSize({ width: 375, height: 900 })
  const box = (await dialog.boundingBox())!
  expect(box.x).toBeGreaterThanOrEqual(0)
  expect(box.x + box.width).toBeLessThanOrEqual(375)
  await page.screenshot({ path: testInfo.outputPath('schedule-mobile.png') })
  await dialog.getByRole('button', { name: '予約する', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=problems')
  expect(requests[0]).toEqual({ version: 1, publish: true, scheduledAt: `${year}-01-02T14:00:00.000Z` })
  const row = page.getByRole('row').filter({ hasText: '予約する問題' })
  await expect(row).toContainText('公開予約中')
  await expect(row).toContainText('23:00')
  await row.getByRole('link', { name: '予約を変更' }).click()
  await expect(input).toHaveValue(`${year}-01-02T23:00`)
  await input.fill(`${year}-01-03T09:30`)
  await dialog.getByRole('button', { name: '予約を更新', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=problems')
  expect(requests[1]).toEqual({ version: 2, publish: true, scheduledAt: `${year}-01-03T00:30:00.000Z` })
  await row.getByRole('link', { name: '予約を変更' }).click()
  // Cancellation must work even if the date input is empty or no longer valid.
  await input.fill('')
  await dialog.getByRole('button', { name: '予約を取り消す' }).click()
  await expect(page).toHaveURL('/my?tab=problems')
  expect(requests[2]).toEqual({ version: 3, publish: false })
  await expect(row).not.toContainText('公開予約中')
})
