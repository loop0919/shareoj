import { randomUUID } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { shortId } from '../../shared/utils/short-id'
import { origin, user } from './helpers'
import { recordSubmission } from './schedule'

test('submissions on a real problem filter and sort', async ({ browser }) => {
  const alice = await user(browser, 'alice')
  const id = randomUUID()
  const draft = { title: '提出一覧の通しテスト問題', difficulty: 3, markdown: '整数を出力してください。', editorial: '2を出力します。', timeLimitMs: '1000', memoryLimitMb: '256', testCases: [{ input: '1', output: '2' }] }
  expect((await alice.request.put(`/api/my/problems/${id}`, { headers: { origin }, data: { version: 0, draft } })).status()).toBe(200)
  expect((await alice.request.put(`/api/my/problems/${id}/publication`, { headers: { origin }, data: { version: 1, publish: true } })).status()).toBe(200)
  // Oldest first, so the judging run is the newest. Each source is as many bytes as its code length.
  const runs = [
    { owner: 'bob', runtime: 'cpp23-gcc', source: 'x'.repeat(60), verdict: 'AC', cpuTimeMs: 30 },
    { owner: 'bob', runtime: 'python314', source: 'x'.repeat(20), verdict: 'WA', cpuTimeMs: 90 },
    { owner: 'carol', runtime: 'pypy311', source: 'x'.repeat(200), verdict: 'AC', cpuTimeMs: 8 },
    { owner: 'carol', runtime: 'cpp17', source: 'x'.repeat(35), verdict: 'AC', cpuTimeMs: 15 },
    { owner: 'bob', runtime: 'rust2024', source: 'x'.repeat(50) },
  ]
  const reader = await user(browser, 'bob')
  for (const run of runs) await recordSubmission(reader.request, id, run)
  // The first accepted run by another user notifies the author. Read it here so the specs that count alice's unread notifications still see theirs alone.
  expect((await alice.request.post('/api/my/notifications/read', { headers: { origin } })).status()).toBe(200)

  const page = reader
  await page.goto(`/problems/${shortId(id)}?view=submissions`)
  const list = page.getByRole('region', { name: 'すべての提出' })
  // The cells of each row, top to bottom: the submitter, the language, the code length and the time.
  const column = (label: string) => list.locator(`tbody tr td[data-label="${label}"]`)
  const runtimes = () => list.locator('tbody tr td.inline-cell:nth-child(3)').allTextContents()
  const users = () => list.locator('tbody tr td.inline-cell:nth-child(2)').allTextContents()
  await expect(list.locator('tbody tr')).toHaveCount(5)
  await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()

  // AC, shortest code first.
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'AC' })
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
  await expect(page).toHaveURL(`/problems/${shortId(id)}?view=submissions&sort=shortest&verdict=AC`)
  await expect(column('コード長')).toHaveText(['35 bytes', '60 bytes', '200 bytes'])
  expect(await runtimes()).toEqual(['C++17 (GCC)', 'C++23 (GCC)', 'Python (PyPy 3.11)'])
  expect(await users()).toEqual(['carol', 'bob', 'carol'])

  // AC, fastest first.
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '実行時間が短い順' })
  await expect(column('実行時間・メモリ')).toHaveText([/^8 ms/, /^15 ms/, /^30 ms/])

  // A whole language: the Python runs of any verdict, newest first.
  await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'すべて' })
  await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '提出日時が新しい順' })
  await page.getByRole('combobox', { name: '言語' }).selectOption({ label: 'Python（すべて）' })
  await expect(page).toHaveURL(`/problems/${shortId(id)}?view=submissions&language=python`)
  await expect(list.locator('tbody tr')).toHaveCount(2)
  expect(await runtimes()).toEqual(['Python (PyPy 3.11)', 'Python (CPython 3.14)'])
  await expect(column('コード長')).toHaveText(['200 bytes', '20 bytes'])

  // C++ covers both of its runtimes, and C++ alone, not the Python ones.
  await page.getByRole('combobox', { name: '言語' }).selectOption({ label: 'C++（すべて）' })
  await expect(list.locator('tbody tr')).toHaveCount(2)
  expect(await runtimes()).toEqual(['C++17 (GCC)', 'C++23 (GCC)'])
  // One runtime narrows it further.
  await page.getByRole('combobox', { name: '言語' }).selectOption({ label: 'C++23 (GCC)' })
  await expect(list.locator('tbody tr')).toHaveCount(1)
  await expect(column('コード長')).toHaveText(['60 bytes'])

  // One submitter, everything else cleared: bob's three runs, the judging one first.
  await page.getByRole('button', { name: '条件をすべて外す' }).click()
  await expect(page).toHaveURL(`/problems/${shortId(id)}?view=submissions`)
  await page.getByRole('textbox', { name: '提出者' }).fill('bob')
  await page.getByRole('textbox', { name: '提出者' }).press('Enter')
  await expect(page).toHaveURL(`/problems/${shortId(id)}?view=submissions&user=bob`)
  await expect(list.locator('tbody tr')).toHaveCount(3)
  expect(await runtimes()).toEqual(['Rust (Edition 2024)', 'Python (CPython 3.14)', 'C++23 (GCC)'])
  await expect(column('実行時間・メモリ')).toHaveText([/^—/, /^90 ms/, /^30 ms/])

  // A reload renders the same rows.
  await page.reload()
  await expect(list.locator('tbody tr')).toHaveCount(3)
  expect(await runtimes()).toEqual(['Rust (Edition 2024)', 'Python (CPython 3.14)', 'C++23 (GCC)'])
})
