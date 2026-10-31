import { expect, test } from './fixtures/account'

test('editor explains new, unsaved, saving and saved states', async ({ page }) => {
  await page.goto('/problems/new')
  const status = page.locator('[data-save-state]')
  await expect(status).toHaveText('サンプルから書き始められます')
  await page.clock.install()
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  await page.locator('#problem-title').fill('文言確認')
  await expect(status).toHaveText('未保存の変更があります')
  let release!: () => void
  const hold = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/my/problems/*', async route => { await hold; await route.fallback() })
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(status).toHaveText('保存しています…')
  release()
  await page.clock.resume()
  await expect(status).toHaveText('保存済み')
})

test('failed leave-save explains how to keep editing or discard', async ({ page }) => {
  await page.route('**/api/my/problems/*', route => route.abort())
  await page.goto('/problems/new')
  await page.locator('#problem-title').fill('文言確認')
  await page.getByRole('link', { name: 'ShareOJ マイページ' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading')).toHaveText('未保存の変更があります')
  await page.getByRole('button', { name: '保存して移動', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveText('保存に失敗しました')
  await expect(dialog.getByRole('alert')).toHaveText('保存できませんでした。編集を続けるか、保存せずに移動してください。')
})

const manualSaveCopy = {
  published: '「保存」を押すと公開内容に反映されます。',
  scheduled: '「保存」を押すと予約の公開内容に反映されます。',
  contest: '「保存」を押すとコンテストの出題・採点内容に反映されます。',
}

for (const mode of ['published', 'scheduled', 'contest'] as const) {
  test(`${mode} drafts explain what saving updates`, async ({ page }) => {
    const id = '11111111-1111-4111-8111-111111111113'
    await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
      id, version: 1, updatedAt: new Date().toISOString(),
      publishedVersion: mode === 'published' ? 1 : 0,
      scheduledAt: mode === 'scheduled' ? '2099-10-01T14:00:00Z' : undefined,
      contestId: mode === 'contest' ? 'cccccccc-cccc-4ccc-8ccc-cccccccccccc' : '',
      draft: { title: '文言確認', markdown: '', timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] },
    } }))
    await page.goto(`/problems/new?problem=${id}`)
    await expect(page.locator('.draft-status').getByText(manualSaveCopy[mode], { exact: true })).toBeVisible()
    await expect(page.locator('.editor-notices')).not.toContainText('反映されます')
  })
}
