import { expect, test } from './fixtures/account'

test('multiple drafts remain independent and reopen from the library', async ({ page }) => {
  await page.goto('/my?tab=problems')
  await expect(page.getByText('保存した問題はまだありません。')).toBeVisible()
  for (const title of ['最初の問題', '次の問題']) {
    await page.getByRole('main').getByRole('link', { name: '新規問題' }).click()
    await page.locator('#problem-title').fill(title)
    await page.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page).toHaveURL(/problem=/)
    await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
    await expect(page).toHaveURL('/my')
  }
  await expect(page.locator('.content-table tbody tr')).toHaveCount(2)
  await page.getByRole('link', { name: '最初の問題を編集' }).click()
  await expect(page.locator('#problem-title')).toHaveValue('最初の問題')
  await page.locator('#problem-title').fill('最初の問題・改訂')
  await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
  await page.getByRole('button', { name: '保存して移動', exact: true }).click()
  await expect(page).toHaveURL('/my')
  await expect(page.locator('.content-table tbody tr')).toHaveCount(2)
  await expect(page.locator('.content-table tbody tr').first()).toContainText('最初の問題・改訂')
  await page.reload()
  await expect(page.locator('.content-table tbody tr')).toHaveCount(2)
})

for (const width of [320, 375, 414, 768, 1280]) {
  test(`draft library fits ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/my?tab=problems')
    await expect(page.getByText('保存した問題はまだありません。')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`library-${width}.png`), fullPage: true })
  })
}

for (const width of [375, 1280]) {
  test(`content menu and table actions work at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const id = '11111111-1111-4111-8111-111111111111'
    const draftId = '22222222-2222-4222-8222-222222222222'
    const updatedAt = '2026-09-10T00:00:00Z'
    await page.route('**/api/my/problems', route => route.fulfill({ json: { items: [
      { id, title: 'A + B', publishedVersion: 1, updatedAt },
      { id: draftId, title: '未公開の問題', publishedVersion: 0, updatedAt },
    ], nextCursor: '' } }))
    await page.route('**/api/my/posts', route => route.fulfill({ json: { items: [
      { id, title: '公開記事', publishedVersion: 1, publishedAt: updatedAt, updatedAt, isOperator: false },
      { id: draftId, title: '下書き記事', publishedVersion: 0, publishedAt: null, updatedAt, isOperator: false },
    ], nextCursor: '' } }))
    await page.route('**/api/my/submissions', route => route.fulfill({ json: { items: [] } }))
    await page.goto('/my')
    await expect(page.getByRole('table', { name: '自分の問題' })).toBeVisible()
    await expect(page.getByRole('table', { name: '作成した記事' })).toBeHidden()
    await expect(page.getByRole('link', { name: 'A + Bを編集' })).toHaveAttribute('href', `/problems/new?problem=${id}`)
    await expect(page.getByRole('link', { name: '未公開の問題を閲覧' })).toHaveAttribute('href', `/problems/${draftId}`)
    await page.getByRole('button', { name: '記事', exact: true }).click()
    await expect(page.getByRole('button', { name: '記事', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByRole('table', { name: '自分の問題' })).toBeHidden()
    await expect(page.getByRole('table', { name: '作成した記事' })).toBeVisible()
    await expect(page.getByRole('link', { name: '公開記事を編集' })).toHaveAttribute('href', `/blog/new?post=${id}`)
    await expect(page.getByRole('link', { name: '公開記事を閲覧' })).toHaveAttribute('href', `/blog/${id}`)
    await expect(page.getByRole('button', { name: '下書き記事を閲覧（公開すると閲覧できます）' })).toBeDisabled()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.getByRole('button', { name: '提出履歴', exact: true }).click()
    await expect(page).toHaveURL('/my?tab=submissions')
    await expect(page.getByRole('button', { name: '提出履歴', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByRole('heading', { name: '提出履歴', exact: true })).toBeVisible()
    await expect(page.getByText('提出はまだありません。')).toBeVisible()
    await expect(page.getByRole('table', { name: '作成した記事' })).toBeHidden()
    await expect(page.getByRole('table', { name: '自分の問題' })).toBeHidden()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.getByRole('button', { name: '自分の問題', exact: true }).click()
    await expect(page.getByRole('heading', { name: '提出履歴', exact: true })).toHaveCount(0)
    await page.getByRole('link', { name: 'A + Bを閲覧' }).click()
    await expect(page).toHaveURL(`/problems/${id}`)
    await expect(page.getByRole('heading', { name: 'A + B', exact: true })).toBeVisible()
  })
}


test('problem states and SVG delivery preferences appear in the library', async ({ page }, testInfo) => {
  const updatedAt = '2026-09-26T00:00:00Z'
  const items = [
    { id: '11111111-1111-4111-8111-111111111111', title: 'まだ書きかけの問題', publishedVersion: 0 },
    { id: '22222222-2222-4222-8222-222222222222', title: '週末コンテストの問題', contestId: 'contest', contestScheduled: true },
    { id: '33333333-3333-4333-8333-333333333333', title: '石の並べ替え', featuredPreference: 'soon' },
    { id: '44444444-4444-4444-8444-444444444444', title: '遠回りの最短路', featuredPreference: 'later' },
    { id: '55555555-5555-4555-8555-555555555555', title: 'A + B', publishedVersion: 2, contestId: 'past-contest' },
    { id: '66666666-6666-4666-8666-666666666666', title: '公開を取り消した問題', publishedVersion: 0, everPublished: true, contestId: 'past-contest' },
  ].map(entry => ({ ...entry, updatedAt }))
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items, nextCursor: '' } }))
  await page.goto('/my?tab=problems')
  const table = page.getByRole('table', { name: '自分の問題', exact: true })
  const rows = table.locator('tbody tr')
  for (const [index, state] of ['準備中', '応募中（コンテスト）', '応募中（定期便）', '応募中（定期便）', '公開中', '準備中'].entries()) {
    await expect(rows.nth(index).locator('td').first()).toContainText(state)
  }
  for (const label of ['早めに出したい', 'ゆっくりで良い']) {
    const mark = table.getByRole('img', { name: label })
    await expect(mark.locator('svg')).toHaveCount(1)
    await mark.hover()
    await expect(mark.getByRole('tooltip')).toHaveText(label)
    await page.mouse.move(0, 0)
    await mark.focus()
    await expect(mark.getByRole('tooltip')).toBeVisible()
    await page.keyboard.press('Tab')
    await expect(mark.getByRole('tooltip')).toBeHidden()
  }
  await expect(table.getByRole('button', { name: /定期便応募を取り下げる/ })).toHaveCount(2)
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await table.getByRole('img', { name: '早めに出したい' }).focus()
    await page.screenshot({ path: testInfo.outputPath(`problem-states-${width}.png`), fullPage: true })
  }
  await page.getByRole('button', { name: 'テスト中の問題', exact: true }).click()
  const testing = page.getByRole('table', { name: 'テスト中の問題', exact: true })
  await expect(testing.getByRole('img', { name: '早めに出したい' })).toBeVisible()
  await expect(testing.getByRole('button', { name: /定期便応募を取り下げる/ })).toHaveCount(0)
})

for (const width of [320, 1280]) {
  test(`status shows five states and why a draft is not ready at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const updatedAt = '2026-09-10T00:00:00Z'
    const ready = { publish: [], contest: [], featured: [] }
    await page.route('**/api/my/problems', route => route.fulfill({ json: { items: [
      { id: '11111111-1111-4111-8111-111111111111', title: '準備完了', updatedAt, readiness: ready },
      { id: '22222222-2222-4222-8222-222222222222', title: '書きかけ', updatedAt, readiness: { publish: [], contest: ['test_cases_missing'], featured: ['test_cases_missing', 'editorial_missing'] } },
      { id: '33333333-3333-4333-8333-333333333333', title: '応募中', updatedAt, featuredPreference: 'soon', readiness: { publish: [], contest: [], featured: ['difficulty_missing'] } },
      { id: '44444444-4444-4444-8444-444444444444', title: '公開した問題', publishedVersion: 1, updatedAt, readiness: { publish: [], contest: ['published'], featured: ['ever_published'] } },
    ], nextCursor: '' } }))
    await page.route('**/api/my/posts', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
    await page.route('**/api/my/submissions', route => route.fulfill({ json: { items: [] } }))
    await page.goto('/my?tab=problems')
    const row = (title: string) => page.getByRole('row').filter({ has: page.getByRole('rowheader', { name: title, exact: true }) })
    await expect(row('準備完了').locator('[data-kind="ready"]')).toContainText('準備中')
    await expect(row('公開した問題').locator('[data-kind="published"]')).toContainText('公開中')
    // Where a published problem already is, is not a missing item.
    await expect(row('公開した問題').locator('summary')).toHaveCount(0)
    const draft = row('書きかけ')
    await expect(draft.getByText('テストケースが1件もありません')).toBeHidden()
    await draft.locator('summary', { hasText: '準備中' }).click()
    await expect(draft.getByRole('listitem')).toHaveText(['テストケースが1件もありません', '解説がありません'])
    // An application that no longer qualifies is flagged instead of silently waiting.
    const applied = row('応募中').locator('summary')
    await expect(applied).toHaveText('応募中（定期便）')
    await expect(applied).toHaveAttribute('aria-label', /このままでは選出されません/)
    await applied.click()
    await expect(row('応募中').getByText('難易度が設定されていません')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}
