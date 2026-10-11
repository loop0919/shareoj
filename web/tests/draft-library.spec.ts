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

test('author problems filter by status across every page', async ({ page }) => {
  const updatedAt = '2026-09-10T00:00:00Z'
  const ready = { publish: [], contest: [], featured: [] }
  const pages: Record<string, object[]> = {
    '': [
      { id: '11111111-1111-4111-8111-111111111111', title: '書きかけ', updatedAt, readiness: { publish: [], contest: ['title_missing'], featured: ['title_missing'] } },
      { id: '22222222-2222-4222-8222-222222222222', title: '出せる問題', updatedAt, readiness: ready },
      { id: '33333333-3333-4333-8333-333333333333', title: 'コンテストの問題', updatedAt, contestId: 'contest', contestScheduled: true, readiness: ready },
    ],
    'page-2': [
      { id: '44444444-4444-4444-8444-444444444444', title: '定期便の問題', updatedAt, featuredPreference: 'soon', readiness: ready },
    ],
  }
  await page.route(url => url.pathname === '/api/my/problems', route => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor') ?? ''
    return route.fulfill({ json: { items: pages[cursor], nextCursor: cursor ? '' : 'page-2' } })
  })
  await page.goto('/my?tab=problems')
  const filters = page.getByRole('group', { name: '状態で絞り込む' })
  const filter = (label: string) => filters.getByRole('button', { name: new RegExp(`^${label}(?: \\d+)?$`) })
  const rows = page.getByRole('table', { name: '自分の問題', exact: true }).locator('tbody tr')
  await expect(rows).toHaveCount(3)
  // Counts wait for every page, since a partial count would mislead.
  await expect(filter('すべて')).toHaveAccessibleName('すべて')
  // 応募中 covers both contests and 定期便, including problems on later pages.
  await filter('応募中').click()
  await expect(page).toHaveURL(/status=applying/)
  await expect(filter('応募中')).toHaveAttribute('aria-pressed', 'true')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toContainText('コンテストの問題')
  await expect(rows.nth(1)).toContainText('定期便の問題')
  await expect(filter('すべて')).toHaveAccessibleName('すべて 4')
  await expect(filter('準備中')).toHaveAccessibleName('準備中 2')
  await expect(page.getByRole('button', { name: 'さらに読み込む' })).toHaveCount(0)
  await filter('準備中').click()
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toContainText('書きかけ')
  await expect(rows.nth(1)).toContainText('出せる問題')
  await page.reload()
  await expect(filter('準備中')).toHaveAttribute('aria-pressed', 'true')
  await expect(rows).toHaveCount(2)
  await filter('公開中').click()
  await expect(page.getByText('「公開中」の問題はありません。')).toBeVisible()
  await expect(page.getByRole('table', { name: '自分の問題', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'すべて表示' }).click()
  await expect(page).not.toHaveURL(/status=/)
  await expect(rows).toHaveCount(4)
  await filter('応募中').click()
  // Switching tabs starts unfiltered, and testers filter by the same labels.
  await page.getByRole('button', { name: 'テスト中の問題', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=testing')
  await expect(filter('すべて')).toHaveAttribute('aria-pressed', 'true')
  await expect(filter('準備中')).toBeVisible()
  await page.getByRole('button', { name: '自分の問題', exact: true }).click()
  await expect(filter('すべて')).toHaveAttribute('aria-pressed', 'true')
  await expect(rows).toHaveCount(4)
})

test('posts filter into drafts and published across every page', async ({ page }) => {
  const updatedAt = '2026-09-10T00:00:00Z'
  const post = (n: number, title: string, publishedVersion: number) => ({ id: `${n}`.repeat(8) + '-1111-4111-8111-111111111111', title, publishedVersion, publishedAt: publishedVersion ? updatedAt : null, updatedAt, isOperator: false })
  await page.route(url => url.pathname === '/api/my/posts', route => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    return route.fulfill({ json: cursor ? { items: [post(3, '後の公開記事', 1)], nextCursor: '' } : { items: [post(1, '下書き記事', 0), post(2, '公開記事', 1)], nextCursor: 'page-2' } })
  })
  await page.goto('/my?tab=posts')
  const filters = page.getByRole('group', { name: '状態で絞り込む' })
  const rows = page.getByRole('table', { name: '作成した記事' }).locator('tbody tr')
  await expect(rows).toHaveCount(2)
  await filters.getByRole('button', { name: /^公開中/ }).click()
  await expect(page).toHaveURL(/tab=posts&status=published/)
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(1)).toContainText('後の公開記事')
  await expect(filters.getByRole('button', { name: /^下書き/ })).toHaveAccessibleName('下書き 1')
  await page.reload()
  await expect(rows).toHaveCount(2)
  await filters.getByRole('button', { name: /^下書き/ }).click()
  await expect(rows).toHaveCount(1)
  await expect(rows.first()).toContainText('下書き記事')
})

test('own contests filter into drafts and published, keeping each list\'s paging', async ({ page }) => {
  const contest = (n: number, title: string, status: string) => ({ id: `${n}`.repeat(8) + '-1111-4111-8111-111111111111', author: 'ui_test', problemAuthors: [], testers: [], title, description: '', startsAt: '2026-09-01T00:00:00Z', endsAt: '2026-09-01T02:00:00Z', penaltyMinutes: 5, version: 1, status, canEdit: status === 'scheduled', participating: false, official: false, canViewSubmissions: true, problems: [] })
  let drafts = [{ id: '99999999-1111-4111-8111-111111111111', title: '下書きコンテスト', startsAt: null, updatedAt: '2026-09-10T00:00:00Z' }]
  let paged = true
  await page.route(url => url.pathname === '/api/my/contests/drafts', route => route.fulfill({ json: { items: drafts, hasMore: false } }))
  await page.route(url => url.pathname === '/api/my/contests', route => {
    const offset = Number(new URL(route.request().url()).searchParams.get('offset') ?? 0)
    return route.fulfill({ json: offset ? { items: [contest(3, '後の終了コンテスト', 'ended')], hasMore: false } : { items: [contest(1, '予定のコンテスト', 'scheduled'), contest(2, '開催中のコンテスト', 'running')], hasMore: paged } })
  })
  await page.goto('/my?tab=contests')
  const filters = page.getByRole('group', { name: '状態で絞り込む' })
  const published = page.getByRole('region', { name: 'コンテスト一覧' })
  const draftList = page.getByRole('region', { name: 'コンテストの下書き' })
  await expect(published.locator('tbody tr')).toHaveCount(2)
  await expect(draftList.locator('tbody tr')).toHaveCount(1)
  // Another page of contests means the totals are unknown.
  await expect(filters.getByRole('button', { name: /^すべて/ })).toHaveAccessibleName('すべて')
  await filters.getByRole('button', { name: /^公開中/ }).click()
  await expect(page).toHaveURL(/tab=contests&status=published/)
  await expect(draftList).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '公開したコンテスト' })).toHaveCount(0)
  await expect(published.locator('tbody tr')).toHaveCount(2)
  await page.getByRole('navigation', { name: 'ページネーション' }).getByRole('button', { name: '次へ' }).click()
  await expect(published.locator('tbody tr')).toHaveCount(1)
  await expect(published).toContainText('後の終了コンテスト')
  await filters.getByRole('button', { name: /^下書き/ }).click()
  await expect(draftList.locator('tbody tr')).toHaveCount(1)
  await expect(published).toHaveCount(0)
  drafts = []
  paged = false
  await page.reload()
  await expect(page.getByText('「下書き」のコンテストはありません。')).toBeVisible()
  await expect(filters.getByRole('button', { name: /^すべて/ })).toHaveAccessibleName('すべて 2')
  await page.getByRole('button', { name: 'すべて表示' }).click()
  await expect(page).toHaveURL('/my?tab=contests')
  await expect(published.locator('tbody tr')).toHaveCount(2)
})
