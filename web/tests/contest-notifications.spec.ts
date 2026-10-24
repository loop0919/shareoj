import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const contestId = '88888888-8888-4888-8888-888888888888'
const kinds = [
  ['clarification_question', 'bobが質問しました', 'clarifications'],
  ['clarification_answer', 'bobが質問に回答しました', 'clarifications'],
  ['clarification_published', 'bobが質問・回答を全体公開しました', 'clarifications'],
  ['contest_results', 'bobが結果を公開しました', 'standings'],
] as const
const notifications = kinds.map(([kind], index) => ({
  id: `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa${index}`, contestId, title: '春のコンテスト', actor: 'bob', kind, createdAt: '2026-10-22T11:00:00Z',
}))

test.beforeEach(async ({ page }) => {
  await page.route('**/api/my/notifications**', route => route.fulfill({ json: { notifications } }))
})

test('the bell lists contest question and result notifications with links', async ({ page }) => {
  await page.goto('/', { waitUntil: 'networkidle' })
  await page.locator('summary[title="通知"]').click()
  const links = page.locator('.notification-panel li a')
  await expect(links).toHaveCount(4)
  for (const [index, [, text, view]] of kinds.entries()) {
    await expect(links.nth(index)).toContainText('春のコンテスト')
    await expect(links.nth(index)).toContainText(text)
    await expect(links.nth(index)).toHaveAttribute('href', `/contests/${shortId(contestId)}?view=${view}`)
  }
})

test('the event list shows the same messages', async ({ page }) => {
  await page.goto('/my?tab=events', { waitUntil: 'networkidle' })
  const list = page.locator('.event-list')
  for (const [, text] of kinds) await expect(list).toContainText(text)
})

test('a rejudge opens the own submissions of the problem, in its contest when it has one', async ({ page }) => {
  const problemId = '99999999-9999-4999-8999-999999999999'
  const rejudges = [{ title: '足し算' }, { title: 'コンテストの題', contestId, problemIndex: 1 }].map((extra, index) => ({
    id: `bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb${index}`, problemId, actor: 'alice', kind: 'rejudge', createdAt: '2026-10-22T11:00:00Z', ...extra,
  }))
  await page.route('**/api/my/notifications**', route => route.fulfill({ json: { notifications: rejudges } }))
  await page.goto('/', { waitUntil: 'networkidle' })
  await page.locator('summary[title="通知"]').click()
  const links = page.locator('.notification-panel li a')
  await expect(links).toHaveCount(2)
  await expect(links.nth(0)).toContainText('aliceがリジャッジしました')
  await expect(links.nth(0)).toHaveAttribute('href', `/problems/${shortId(problemId)}?view=my-submissions`)
  await expect(links.nth(1)).toContainText('コンテストの題')
  await expect(links.nth(1)).toHaveAttribute('href', `/contests/${shortId(contestId)}/B?view=my-submissions`)
})
