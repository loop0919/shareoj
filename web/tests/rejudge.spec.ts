import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const problemId = '11111111-1111-4111-8111-111111111111'
const done = (id: string, verdict: string) => ({
  id, problemId, problemTitle: 'A + B', problemVersion: 1, author: 'carol', runtime: 'cpp17', sourceBytes: 12, status: 'DONE',
  createdAt: '2026-09-10T00:00:00Z', result: { verdict, passed: 0, total: 1, cpuTimeMs: 2, memoryBytes: 1048576 },
})
const first = '33333333-3333-4333-8333-333333333331'
const second = '33333333-3333-4333-8333-333333333332'
const judging = { ...done('33333333-3333-4333-8333-333333333333', 'WA'), status: 'RUNNING', result: null }

test('authors select finished submissions in the list and rejudge them together', async ({ page }, testInfo) => {
  let allowed = true
  const requests: unknown[] = []
  await page.route('**/api/my/problems/*/rejudge', async route => {
    if (!allowed) return route.fulfill({ status: 404, json: {} })
    if (route.request().method() === 'GET') return route.fulfill({ json: { pending: 0 } })
    const body = route.request().postDataJSON() as { submissionIds?: string[] }
    requests.push(body)
    return route.fulfill({ status: 202, json: { queued: body.submissionIds?.length ?? 0 } })
  })
  await page.route('**/api/problems/*/submissions?*', route => route.fulfill({ json: { items: [done(first, 'WA'), done(second, 'AC'), judging], hasMore: false } }))
  await page.goto(`/problems/${shortId(problemId)}?view=submissions`)
  const list = page.getByRole('region', { name: 'すべての提出' })
  const all = list.getByRole('checkbox', { name: 'このページの提出をすべて選択' })
  await expect(all).toBeVisible()
  const rows = list.getByRole('checkbox', { name: /の提出を選択$/ })
  await expect(rows).toHaveCount(3)
  await expect(rows.nth(2)).toBeDisabled()
  const action = list.getByRole('button', { name: /選択した提出をリジャッジ/ })
  await expect(action).toBeDisabled()
  await all.check()
  await expect(action).toHaveText('選択した提出をリジャッジ（2件）')
  await rows.nth(1).uncheck()
  await expect(all).not.toBeChecked()
  await all.check()
  for (const width of [320, 768]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await page.screenshot({ path: testInfo.outputPath('rejudge-list.png'), fullPage: true })
  let confirmation = ''
  page.once('dialog', dialog => { confirmation = dialog.message(); void dialog.accept() })
  await action.click()
  await expect(list.getByRole('status').filter({ hasText: '2件の提出をリジャッジしています。' })).toBeVisible()
  expect(confirmation).toContain('選択した2件の提出をリジャッジしますか？')
  expect(requests).toEqual([{ submissionIds: [first, second] }])
  await expect(action).toHaveText('選択した提出をリジャッジ（0件）')
  // Viewers who cannot rejudge see the plain list.
  allowed = false
  await page.reload()
  await expect(list.getByRole('row')).toHaveCount(4)
  await expect(list.getByRole('checkbox')).toHaveCount(0)
  await expect(list.getByRole('button', { name: /リジャッジ/ })).toHaveCount(0)
})

test('problem management rejudges every saved submission and reports progress and errors', async ({ page }) => {
  let pending = 0
  let failure = ''
  const requests: unknown[] = []
  await page.route('**/api/my/problems/*/rejudge', async route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { pending } })
    requests.push(route.request().postDataJSON())
    if (failure) return route.fulfill({ status: 409, json: { statusCode: 409, data: { code: failure } } })
    pending = 5
    return route.fulfill({ status: 202, json: { queued: 5 } })
  })
  await page.goto('/problems/new?fresh=1')
  await page.locator('#problem-title').fill('リジャッジする問題')
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  // Rejudging waits for the first save.
  await expect(page.getByRole('heading', { name: 'リジャッジ', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/problem=/)
  const row = page.getByRole('region', { name: 'リジャッジ' })
  const button = row.getByRole('button', { name: 'すべてリジャッジ', exact: true })
  await expect(button).toBeEnabled()
  page.once('dialog', dialog => void dialog.accept())
  await button.click()
  await expect(row.getByRole('status').filter({ hasText: '5件の提出をリジャッジしています。' })).toBeVisible()
  await expect(row.getByRole('status').filter({ hasText: 'リジャッジ中：残り5件' })).toBeVisible()
  expect(requests).toEqual([{}])
  pending = 0
  await expect(row.getByText('リジャッジ中', { exact: false })).toHaveCount(0, { timeout: 10_000 })
  failure = 'tests_not_ready'
  page.once('dialog', dialog => void dialog.accept())
  await button.click()
  await expect(row.getByRole('status').filter({ hasText: '今の問題の設定では採点できません。' })).toBeVisible()
  // Dismissing the confirmation sends nothing.
  page.once('dialog', dialog => void dialog.dismiss())
  await button.click()
  expect(requests).toHaveLength(2)
})

test('a rejudged submission shows when it was rejudged', async ({ page }) => {
  await page.goto(`/contests/${shortId('88888888-8888-4888-8888-888888888888')}/submissions/${shortId('22222222-2222-4222-8222-222222222222')}`)
  const info = page.getByRole('table', { name: '提出情報' })
  await expect(info.getByRole('row', { name: /リジャッジ日時/ })).toContainText('2026/9/10')
})
