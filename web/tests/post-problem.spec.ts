import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const id = '11111111-1111-4111-8111-111111111111'
const updatedAt = '2026-09-10T00:00:00Z'
const ready = { publish: [], contest: [], featured: [] }
const draft = { title: '投稿する下書き', markdown: '問題本文', difficulty: 2, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] }

test('the editor asks whether to post or apply, and says why a destination is closed', async ({ page }, testInfo) => {
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
    id, version: 2, updatedAt, author: 'ui_test', draft, readiness: { ...ready, featured: ['editorial_missing'] },
  } }))
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items: [{ id, title: draft.title, difficulty: 2, updatedAt, readiness: ready }], nextCursor: '' } }))
  await page.goto(`/problems/new?problem=${id}`)
  const trigger = page.getByRole('button', { name: '公開・応募', exact: true })
  await trigger.click()
  const choice = page.getByRole('dialog', { name: '公開・応募' })
  await expect(choice).toContainText('投稿する下書き')
  await expect(choice.getByRole('link', { name: /定期便に応募/ })).toHaveCount(0)
  await expect(choice).toContainText('解説がありません')
  for (const width of [320, 375, 1280]) {
    await page.setViewportSize({ width, height: 800 })
    const box = (await choice.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`publish-choice-${width}.png`) })
  }
  await page.keyboard.press('Escape')
  await expect(choice).toBeHidden()
  await expect(trigger).toBeFocused()
  await trigger.click()
  await choice.getByRole('link', { name: /問題を投稿/ }).click()
  await expect(page).toHaveURL(`/problems/post?problem=${id}`)
  await expect(page.getByRole('radio', { name: /投稿する下書き/ })).toBeChecked()
})

test('testers and published problems get no posting button in the editor', async ({ page }) => {
  let author = 'someone_else'
  let publishedVersion = 0
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 2, updatedAt, author, publishedVersion, draft, readiness: ready } }))
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.locator('#problem-title')).toHaveValue('投稿する下書き')
  await expect(page.getByRole('button', { name: '公開・応募', exact: true })).toHaveCount(0)
  author = 'ui_test'
  publishedVersion = 1
  await page.reload()
  await expect(page.locator('#problem-title')).toHaveValue('投稿する下書き')
  await expect(page.getByRole('button', { name: '公開・応募', exact: true })).toHaveCount(0)
})

test('posting lists unpublished problems across pages, explains blocked ones and handles failures', async ({ page }, testInfo) => {
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: new URL(route.request().url()).searchParams.get('cursor') ? {
    items: [{ id, title: draft.title, difficulty: 2, updatedAt, readiness: ready }], nextCursor: '',
  } : {
    items: [
      { id: '22222222-2222-4222-8222-222222222222', title: '公開済み', publishedVersion: 1, difficulty: 3, updatedAt, readiness: ready },
      { id: '33333333-3333-4333-8333-333333333333', title: 'コンテスト用', contestId: 'contest', contestScheduled: true, difficulty: 5, updatedAt, readiness: { ...ready, publish: ['in_contest'] } },
    ], nextCursor: 'next',
  } }))
  let attempts = 0
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 4, updatedAt, draft, readiness: ready } }))
  await page.route(`**/api/problems/${id}`, route => route.fulfill({ json: {
    id, title: draft.title, markdown: draft.markdown, author: 'ui_test', difficulty: 2, timeLimitMs: 2000, memoryLimitMb: 512, publishedAt: updatedAt,
  } }))
  await page.route(`**/api/my/problems/${id}/publication`, route => {
    expect(route.request().postDataJSON()).toEqual({ version: 4, publish: true })
    attempts++
    if (attempts === 2) return route.fulfill({ status: 400, json: { statusCode: 400, data: { code: 'incomplete_problem' } } })
    return route.fulfill(attempts === 1 ? { status: 409, json: {} } : { json: {} })
  })
  await page.goto('/problems')
  await page.getByRole('link', { name: '問題投稿', exact: true }).click()
  await expect(page).toHaveURL('/problems/post')
  const choices = page.getByRole('group', { name: '問題' })
  await expect(choices.getByRole('radio')).toHaveCount(2)
  await expect(choices.locator('li').first()).toContainText('投稿する下書き')
  await expect(choices.getByRole('radio', { name: /コンテスト用/ })).toBeDisabled()
  await expect(choices).toContainText('コンテストに登録済みのため、コンテストが終わるまで公開できません')
  await expect(choices).not.toContainText('公開済み')
  const hide = page.getByRole('switch', { name: /投稿不可の問題を隠す/ })
  await hide.check()
  await expect(choices.getByRole('radio')).toHaveCount(1)
  await expect(choices.getByRole('status')).toHaveText('1件を隠しています')
  await hide.uncheck()
  await expect(choices.getByRole('radio')).toHaveCount(2)
  const submit = page.getByRole('button', { name: '投稿する', exact: true })
  await expect(submit).toBeDisabled()
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`post-${width}.png`), fullPage: true })
  }
  await choices.getByRole('radio', { name: /投稿する下書き/ }).check()
  await submit.click()
  await expect(page.getByRole('alert')).toContainText('別の画面で更新されています')
  await expect(page.locator('.toast').getByRole('status')).toHaveCount(0)
  // The draft lost something after the list loaded: the refusal speaks of publishing, not of saving.
  await submit.click()
  await expect(page.getByRole('alert')).toContainText('足りない項目があるため公開できません。')
  await expect(page.getByRole('alert')).not.toContainText('保存')
  await submit.click()
  await expect(page).toHaveURL(`/problems/${shortId(id)}`)
  await expect(page.locator('.toast').getByRole('status')).toHaveText('公開しました')
})

test('posting keeps the timing beside a long list and the button on screen', async ({ page }, testInfo) => {
  const items = Array.from({ length: 30 }, (_, index) => ({
    id: `aaaaaaaa-aaaa-4aaa-8aaa-${String(index).padStart(12, '0')}`, title: `問題 ${index + 1}`, difficulty: index % 10 + 1, updatedAt,
    readiness: { ...ready, publish: index % 4 ? [] : ['in_contest'] },
  }))
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items, nextCursor: '' } }))
  for (const size of [{ width: 1280, height: 800 }, { width: 375, height: 812 }]) {
    await page.setViewportSize(size)
    await page.goto(`/problems/post?problem=${items[29]!.id}`)
    const radio = page.getByRole('radio', { name: /問題 30/ })
    await expect(radio).toBeChecked()
    await expect(radio).toBeInViewport()
    await expect(page.getByRole('button', { name: '投稿する', exact: true })).toBeInViewport()
    await expect(page.locator('.post-bar')).toContainText('「問題 30」を今すぐ公開します')
    expect(await page.evaluate(() => scrollY)).toBe(0)
    await page.screenshot({ path: testInfo.outputPath(`post-long-${size.width}.png`) })
  }
  // On a wide screen the timing sits to the right of the list.
  await page.setViewportSize({ width: 1280, height: 800 })
  const list = (await page.getByRole('group', { name: '問題' }).boundingBox())!
  const timing = (await page.getByRole('group', { name: '公開のタイミング' }).boundingBox())!
  expect(timing.x).toBeGreaterThan(list.x + list.width)
  // The button closes the page at the bottom right, under the timing.
  const button = (await page.getByRole('button', { name: '投稿する', exact: true }).boundingBox())!
  expect(button.x).toBeGreaterThan(timing.x)
  await page.getByRole('radio', { name: '日時を指定' }).check()
  await page.getByLabel('公開日時（日本時間）').fill('2099-10-20T21:00')
  await expect(page.locator('.post-bar')).toContainText('「問題 30」を10/20(火) 21:00に公開します')
  await expect(page.getByRole('button', { name: '予約する', exact: true })).toBeInViewport()
})

test('posting has an empty state', async ({ page }) => {
  await page.goto('/problems/post')
  await expect(page.getByText('投稿できる未公開の問題はありません。')).toBeVisible()
  await expect(page.getByRole('link', { name: '新規問題を作成' })).toHaveAttribute('href', '/problems/new?fresh=1')
})

test('old posting links open the posting page, and guests can log in and return to it', async ({ page }) => {
  let user: { id: string } | null = null
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user } }))
  await page.goto('/problems?post=1')
  await expect(page).toHaveURL('/problems/post')
  await expect(page.locator('.problem-post').getByRole('link', { name: 'ログイン' })).toHaveAttribute('href', '/login?next=/problems/post')
  await page.route('**/api/auth/login', route => route.fulfill({ json: { user: { id: 'ui-test-user' } } }))
  await page.locator('.problem-post').getByRole('link', { name: 'ログイン' }).click()
  await expect(page).toHaveURL('/login?next=/problems/post')
  user = { id: 'ui-test-user' }
  await page.getByLabel('メールアドレス', { exact: true }).fill('ui_test@example.test')
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/problems/post')
})

test('schedule in Japan time, show the reservation, update it and cancel it', async ({ page }, testInfo) => {
  let scheduledAt: string | undefined
  let version = 1
  const problem = () => ({ id, version, scheduledAt, publishedVersion: 0, updatedAt, draft: { ...draft, title: '予約する問題', difficulty: 3 } })
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items: [{ ...problem(), title: '予約する問題', difficulty: 3, readiness: ready }], nextCursor: '' } }))
  await page.route('**/api/my/problems', route => route.fulfill({ json: { items: [{ ...problem(), title: '予約する問題', difficulty: 3, readiness: ready }], nextCursor: '' } }))
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
  // The problem in an old link is carried over to the posting page.
  await page.goto(`/problems?post=1&problem=${id}`)
  await expect(page).toHaveURL(`/problems/post?problem=${id}`)
  await expect(page.getByRole('radio', { name: /予約する問題/ })).toBeChecked()
  await page.getByRole('radio', { name: '日時を指定' }).check()
  const input = page.getByLabel('公開日時（日本時間）')
  await input.fill('2000-01-01T23:00')
  await page.getByRole('button', { name: '予約する', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('未来の公開日時')
  await expect(input).toHaveAttribute('aria-invalid', 'true')
  expect(requests).toHaveLength(0)
  const year = new Date().getUTCFullYear() + 1
  await input.fill(`${year}-01-02T23:00`)
  // Correcting the date clears the stale error.
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(input).not.toHaveAttribute('aria-invalid')
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375)
  await page.screenshot({ path: testInfo.outputPath('schedule-mobile.png'), fullPage: true })
  await page.getByRole('button', { name: '予約する', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=problems')
  expect(requests[0]).toEqual({ version: 1, publish: true, scheduledAt: `${year}-01-02T14:00:00.000Z` })
  await expect(page.locator('.toast')).toHaveCount(0)
  const row = page.getByRole('row').filter({ hasText: '予約する問題' })
  await expect(row).toContainText('公開予約中')
  await expect(row).toContainText('23:00')
  await row.getByRole('link', { name: '予約を変更' }).click()
  await expect(page).toHaveURL(`/problems/post?problem=${id}`)
  await expect(page.getByRole('group', { name: '問題' })).toContainText('公開予約中')
  await expect(input).toHaveValue(`${year}-01-02T23:00`)
  await input.fill(`${year}-01-03T09:30`)
  await page.getByRole('button', { name: '予約を更新', exact: true }).click()
  await expect(page).toHaveURL('/my?tab=problems')
  expect(requests[1]).toEqual({ version: 2, publish: true, scheduledAt: `${year}-01-03T00:30:00.000Z` })
  await row.getByRole('link', { name: '予約を変更' }).click()
  // Cancellation must work even if the date input is empty or no longer valid.
  await input.fill('')
  await page.getByRole('button', { name: '予約を取り消す' }).click()
  await expect(page).toHaveURL('/my?tab=problems')
  expect(requests[2]).toEqual({ version: 3, publish: false })
  await expect(row).not.toContainText('公開予約中')
})

test('a problem waiting for 定期便 can be posted now but not scheduled', async ({ page }) => {
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items: [{ id, title: '応募中の問題', featuredPreference: 'soon', difficulty: 4, updatedAt, readiness: ready }], nextCursor: '' } }))
  await page.goto(`/problems/post?problem=${id}`)
  await expect(page.getByRole('group', { name: '問題' })).toContainText('定期便に応募中')
  await expect(page.getByRole('radio', { name: '今すぐ' })).toBeChecked()
  await expect(page.getByRole('radio', { name: '日時を指定' })).toBeDisabled()
  await expect(page.getByText('定期便に応募中のため予約できません。')).toBeVisible()
  await expect(page.getByRole('button', { name: '投稿する', exact: true })).toBeEnabled()
})
