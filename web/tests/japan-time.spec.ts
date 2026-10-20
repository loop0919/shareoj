import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

// The browser runs in UTC, nine hours behind Japan, so each instant below falls on another day or month there.
test.use({ timezoneId: 'UTC' })
const at = '2026-09-30T20:00:00Z' // 2026/10/1 5:00 in Japan
const problemId = 'dddddddd-0000-4000-8000-000000000001'
const submissionId = 'dddddddd-0000-4000-8000-000000000002'

test('the my page shows its dates in Japan time in any time zone', async ({ page }) => {
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'ui_test', avatar: '', version: 1, createdAt: at } } }))
  await page.route(url => url.pathname === '/api/my/problems', route => route.fulfill({ json: { items: [{ id: problemId, title: '時刻の問題', updatedAt: at }], nextCursor: '' } }))
  await page.route('**/api/my/posts', route => route.fulfill({ json: { items: [{ id: 'dddddddd-0000-4000-8000-000000000003', title: '時刻の記事', publishedVersion: 0, publishedAt: null, updatedAt: at, isOperator: false }], nextCursor: '' } }))
  await page.route('**/api/my/notifications**', route => route.fulfill({ json: { notifications: [{ id: 'dddddddd-0000-4000-8000-000000000004', problemId, title: '時刻の問題', actor: 'bob', kind: 'favorite', createdAt: at }] } }))
  await page.goto('/my')
  await expect(page.getByText('2026年10月に登録', { exact: true })).toBeVisible()
  await expect(page.getByRole('table', { name: '自分の問題' }).locator('tbody time')).toHaveText('2026/10/01 05:00')
  await page.getByLabel('通知（未読1件）').click()
  await expect(page.locator('.notification-panel time')).toHaveText('2026/10/1 5:00:00')
  await page.keyboard.press('Escape')
  await expect(page.locator('.notification-panel')).toBeHidden()
  const tabs = page.getByRole('navigation', { name: 'マイページのコンテンツ' })
  await tabs.getByRole('button', { name: '記事', exact: true }).click()
  await expect(page.getByRole('table', { name: '作成した記事' }).locator('tbody time')).toHaveText('2026/10/01 05:00')
  await tabs.getByRole('button', { name: 'イベント', exact: true }).click()
  await expect(page.getByRole('region', { name: 'イベント', exact: true }).locator('time')).toHaveText('2026/10/1 5:00:00')
})

test('submission times read in Japan time in any time zone', async ({ page }) => {
  const submission = { id: submissionId, problemId, problemVersion: 1, problemTitle: '時刻の問題', runtime: 'cpp17', sourceBytes: 12, status: 'DONE', result: { verdict: 'AC', passed: 1, total: 1 }, createdAt: at }
  await page.route('**/api/my/submissions', route => route.fulfill({ json: { items: [submission] } }))
  await page.route(`**/api/my/submissions/${submissionId}`, route => route.fulfill({ json: { ...submission, source: 'int main(){}', rejudgedAt: '2026-10-31T16:00:00Z' } }))
  await page.goto('/my?tab=submissions')
  await expect(page.getByRole('table', { name: '提出履歴' }).locator('tbody time')).toHaveText('2026/10/1 5:00:00')
  // Rejudged at 16:00 on October 31 in UTC, already November in Japan.
  await page.goto(`/my/submissions/${shortId(submissionId)}`)
  await expect(page.getByRole('table', { name: '提出情報' }).locator('time')).toHaveText(['2026/10/1 5:00:00', '2026/11/1 1:00:00'])
})
