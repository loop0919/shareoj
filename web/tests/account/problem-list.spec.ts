import { randomUUID } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { origin, user } from './helpers'
import { recordAccepted } from './schedule'

test('a solved problem leaves the unsolved list and shows in the solved one', async ({ browser }) => {
  const alice = await user(browser, 'alice')
  const unsolved = '通しテスト未ACの問題'
  const solved = '通しテストAC済みの問題'
  const ids: string[] = []
  for (const title of [unsolved, solved]) {
    const id = randomUUID()
    const draft = { title, difficulty: 3, markdown: '整数を出力してください。', editorial: '2を出力します。', timeLimitMs: '1000', memoryLimitMb: '256', testCases: [{ input: '1', output: '2' }] }
    expect((await alice.request.put(`/api/my/problems/${id}`, { headers: { origin }, data: { version: 0, draft } })).status()).toBe(200)
    expect((await alice.request.put(`/api/my/problems/${id}/publication`, { headers: { origin }, data: { version: 1, publish: true } })).status()).toBe(200)
    ids.push(id)
  }
  const page = await user(browser, 'bob')
  await recordAccepted(page.request, ids[1], 'bob')

  await page.goto('/problems')
  await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
  const status = page.getByRole('group', { name: '解答状況' })
  await status.getByRole('button', { name: '未ACのみ' }).click()
  await expect(page).toHaveURL('/problems?solved=unsolved')
  const table = page.getByRole('table')
  await expect(table.getByRole('link', { name: unsolved, exact: true })).toBeVisible()
  await expect(table.getByRole('link', { name: solved, exact: true })).toHaveCount(0)
  await status.getByRole('button', { name: 'AC済みのみ' }).click()
  await expect(page).toHaveURL('/problems?solved=solved')
  await expect(table.getByRole('link', { name: new RegExp(`^${solved}`) })).toBeVisible()
  await expect(table.getByRole('link', { name: unsolved, exact: true })).toHaveCount(0)
  // Every listed row is marked solved.
  await expect(table.locator('tr.solved')).toHaveCount(await table.locator('tbody tr').count())
  // A reload renders the same list on the server.
  await page.reload()
  await expect(table.getByRole('link', { name: new RegExp(`^${solved}`) })).toBeVisible()
  await expect(table.getByRole('link', { name: unsolved, exact: true })).toHaveCount(0)
})
