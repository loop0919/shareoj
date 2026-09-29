import { expect, test } from './fixtures/account'
test('failed deletion keeps the problem and the dialog open', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.locator('#problem-title').fill('残る下書き')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/problem=/)
  await page.route('**/api/my/problems/*', route => route.request().method() === 'DELETE' ? route.abort() : route.fallback())
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await page.getByRole('button', { name: '問題を削除', exact: true }).click()
  await page.getByRole('button', { name: '削除する', exact: true }).click()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText('処理に失敗しました')
  await page.getByRole('button', { name: 'キャンセル', exact: true }).click()
  await expect(page.getByRole('heading', { name: '問題管理', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '問題文', exact: true }).click()
  await expect(page.locator('#problem-title')).toHaveValue('残る下書き')
})

for (const width of [320, 375, 414, 768, 1280]) {
  test(`management replaces the workspace at ${width}px and preserves editing`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 700 })
    await page.goto('/problems/new?fresh=1')
    await page.locator('#problem-title').fill('管理画面へ切り替え')
    await page.locator('#problem-source').fill('## 書きかけの本文\n\n$A+B$')
    await page.getByRole('button', { name: '問題管理', exact: true }).click()
    await expect(page.getByRole('heading', { name: '問題管理', exact: true })).toBeVisible()
    await expect(page.locator('#problem-source')).toBeHidden()
    await expect(page.locator('dialog[open]')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '問題管理', exact: true })).toHaveAttribute('aria-current', 'page')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`management-${width}.png`) })
    await page.getByRole('button', { name: '問題文', exact: true }).click()
    await expect(page.locator('#problem-title')).toHaveValue('管理画面へ切り替え')
    await expect(page.locator('#problem-source')).toHaveText('## 書きかけの本文\n\n$A+B$', { useInnerText: true })
  })
}

test('management shows the saved status and opens the section that needs work', async ({ page }) => {
  const id = '77777777-7777-4777-8777-777777777777'
  const draft = { title: '難易度なし', markdown: '本文', editorial: '', difficulty: null, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] }
  const readiness = { publish: ['difficulty_missing'], contest: ['difficulty_missing', 'test_cases_missing'], featured: ['difficulty_missing', 'test_cases_missing', 'editorial_missing'] }
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 1, updatedAt: '2026-09-10T00:00:00Z', draft, readiness } }))
  await page.goto(`/problems/new?problem=${id}`)
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const status = page.getByRole('region', { name: '状態' })
  await expect(status).toContainText('保存済みの内容で判定しています')
  const issues = status.getByRole('list', { name: '足りない項目' })
  await expect(issues.getByRole('listitem')).toHaveText([/難易度が設定されていません/, /テストケースが1件もありません/, /解説がありません/])
  await issues.getByRole('button', { name: 'テストケースを開く' }).click()
  await expect(page.getByRole('button', { name: 'テストケース', exact: true })).toHaveAttribute('aria-current', 'page')
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await issues.getByRole('button', { name: '問題文を開く' }).click()
  await expect(page.getByRole('combobox', { name: '難易度（作成者設定）' })).toBeVisible()
})

test('published problems explain where else they cannot go', async ({ page }) => {
  const id = '88888888-8888-4888-8888-888888888888'
  const draft = { title: '公開済み', markdown: '本文', editorial: '解説', difficulty: 3, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [{ input: '1', output: '1' }] }
  const readiness = { publish: [], contest: ['published'], featured: ['ever_published'] }
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 2, publishedVersion: 2, everPublished: true, updatedAt: '2026-09-10T00:00:00Z', draft, readiness } }))
  await page.goto(`/problems/new?problem=${id}`)
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const status = page.getByRole('region', { name: '状態' })
  await expect(status.locator('[data-kind="published"]')).toContainText('公開中')
  await expect(status.getByRole('list', { name: '足りない項目' })).toHaveCount(0)
  await expect(status.getByRole('list', { name: '提出先の制限' }).getByRole('listitem')).toHaveText(['コンテスト：公開中の問題はコンテストに登録できません', '定期便：一度公開した問題は定期便に応募できません'])
})
