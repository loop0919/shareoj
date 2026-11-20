import { expect, test } from './fixtures/account'

for (const conflict of [false, true]) {
  test(conflict ? 'a conflicting write preserves edits, and a later edit writes nothing' : 'a committed write with a lost response is recovered without another write', async ({ page }) => {
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
