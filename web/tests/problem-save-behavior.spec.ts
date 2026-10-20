import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'

for (const conflict of [false, true]) {
  test(conflict ? 'a conflicting write preserves edits and stops automatic retries' : 'a committed write with a lost response is recovered without another write', async ({ page }) => {
    let stored: unknown
    let writes = 0
    await page.route('**/api/my/problems/*', async route => {
      if (route.request().method() === 'PUT') {
        writes++
        const body = route.request().postDataJSON()
        stored = {
          id: new URL(route.request().url()).pathname.split('/').at(-1),
          version: body.version + 1, updatedAt: new Date().toISOString(),
          draft: { ...body.draft, title: conflict ? '別の画面の変更' : body.draft.title },
        }
        return route.fulfill({ status: conflict ? 409 : 502, json: {} })
      }
      return route.fulfill({ json: stored })
    })
    await page.goto('/problems/new')
    await page.locator('#problem-title').fill('保持する変更')
    await page.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', conflict ? 'failed' : 'saved')
    await expect(page.locator('#problem-title')).toHaveValue('保持する変更')
    if (conflict) {
      await page.locator('#problem-title').fill('さらに編集')
      await page.waitForTimeout(900)
      expect(writes).toBe(1)
      await expect(page.locator('#problem-title')).toHaveValue('さらに編集')
    } else {
      expect(writes).toBe(1)
      await page.reload()
      await expect(page.locator('#problem-title')).toHaveValue('保持する変更')
    }
  })
}

test('edits during an in-flight save are serialized with the next server version', async ({ page }) => {
  const versions: number[] = []
  let release!: () => void
  const hold = new Promise<void>(resolve => { release = resolve })
  await page.goto('/problems/new')
  await page.locator('#problem-title').fill('初回')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  const problemId = new URL(page.url()).searchParams.get('problem')!
  await page.route(`**/api/my/problems/${problemId}`, async route => {
    if (route.request().method() === 'PUT') {
      versions.push(route.request().postDataJSON().version)
      if (versions.length === 1) await hold
    }
    return route.fallback()
  })
  await page.locator('#problem-title').fill('保存中の内容')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect.poll(() => versions.length).toBe(1)
  await page.locator('#problem-title').fill('後から編集した内容')
  await page.keyboard.press('Control+s')
  await page.waitForTimeout(700)
  expect(versions).toEqual([1])
  release()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  expect(versions).toEqual([1, 2])
  await page.reload()
  await expect(page.locator('#problem-title')).toHaveValue('後から編集した内容')
})

// Saves a new problem, after prepare edits it further if given, then freezes the clock and records the title of every
// later write. expectOneAutosave checks that the pending edit is written exactly once, when the 600 ms debounce ends.
async function saveWithFrozenClock(page: Page, prepare?: () => Promise<void>) {
  await page.goto('/problems/new?fresh=1')
  await page.locator('#problem-title').fill('保存済みの題名')
  await prepare?.()
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/problem=/)
  const status = page.locator('[data-save-state]')
  await expect(status).toHaveAttribute('data-save-state', 'saved')
  await page.clock.install()
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  const titles: string[] = []
  page.on('request', request => { if (request.method() === 'PUT' && request.url().includes('/api/my/problems/')) titles.push(request.postDataJSON().draft.title) })
  async function expectOneAutosave(title: string) {
    await page.clock.runFor(599)
    await expect(status).toHaveAttribute('data-save-state', 'dirty')
    expect(titles).toEqual([])
    await page.clock.runFor(1)
    await expect.poll(() => titles).toEqual([title])
    await expect(status).toHaveAttribute('data-save-state', 'saved')
    await page.clock.runFor(2000)
    expect(titles).toEqual([title])
  }
  return { status, titles, expectOneAutosave }
}

test('keeping on editing from the leave dialog autosaves the edit once after the debounce', async ({ page }) => {
  const { expectOneAutosave } = await saveWithFrozenClock(page)
  await page.locator('#problem-title').fill('続きの編集')
  await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('button', { name: '編集を続ける', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeHidden()
  await expectOneAutosave('続きの編集')
})

test('cancelling the delete confirmation autosaves only an unsaved edit, once after the debounce', async ({ page }) => {
  const { status, titles, expectOneAutosave } = await saveWithFrozenClock(page)
  async function cancelDelete() {
    await page.getByRole('button', { name: '問題管理', exact: true }).click()
    await page.getByRole('button', { name: '問題を削除', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.getByRole('button', { name: 'キャンセル', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeHidden()
  }
  await cancelDelete()
  await page.clock.runFor(2000)
  expect(titles).toEqual([])
  await expect(status).toHaveAttribute('data-save-state', 'saved')
  await page.getByRole('button', { name: '問題文', exact: true }).click()
  await page.locator('#problem-title').fill('削除せずに編集')
  await cancelDelete()
  await expectOneAutosave('削除せずに編集')
})

test('a generation that finishes behind the leave dialog leaves a single autosave timer', async ({ page }) => {
  const { expectOneAutosave } = await saveWithFrozenClock(page, async () => {
    await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
    await page.getByLabel('入力生成のコード', { exact: true }).fill('print(1)')
  })
  let finish!: () => void
  const finished = new Promise<void>(resolve => { finish = resolve })
  await page.route('**/api/my/submissions', async (route) => {
    await finished
    await route.fulfill({ json: { id: 'generation-job', status: 'DONE', result: { verdict: 'AC', cases: [{ output: '1\n' }] } } })
  })
  const generation = page.waitForRequest('**/api/my/submissions')
  await page.getByRole('button', { name: '生成する', exact: true }).click()
  await generation
  // An edit during the generation makes leaving ask first, so the generation finishes behind the dialog.
  await page.getByRole('button', { name: '問題文', exact: true }).click()
  await page.locator('#problem-title').fill('生成中の編集')
  await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  finish()
  await expect(page.getByText('1件の入力を追加しました。', { exact: true })).toBeAttached()
  await page.getByRole('button', { name: '編集を続ける', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeHidden()
  // A timer left from the generation would write 600 ms after closing, before this edit's debounce ends.
  await page.clock.runFor(300)
  await page.locator('#problem-title').fill('閉じた後の編集')
  await expectOneAutosave('閉じた後の編集')
})
