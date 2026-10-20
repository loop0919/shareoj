import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'

// Saves a new problem and reopens it as published or as a contest problem, which save only on an explicit action.
// The returned function counts the PUTs made after the reload.
async function openManualSaveOnly(page: Page, mode: string) {
  await page.goto('/problems/new')
  await page.locator('#problem-title').fill('保存確認')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/problem=/)
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  const id = new URL(page.url()).searchParams.get('problem')!
  const stored = await page.evaluate(async id => (await fetch(`/api/my/problems/${id}`)).json(), id)
  stored.publishedVersion = mode === 'published' ? 1 : 0
  stored.contestId = mode === 'contest' ? 'cccccccc-cccc-4ccc-8ccc-cccccccccccc' : ''
  let writes = 0
  await page.route(`**/api/my/problems/${id}`, route => {
    if (route.request().method() === 'PUT') {
      writes++
      stored.draft = route.request().postDataJSON().draft
      stored.version++
    }
    return route.fulfill({ json: stored })
  })
  await page.reload()
  await expect(page.locator('#problem-title')).toBeEnabled()
  return () => writes
}

for (const mode of ['published', 'contest']) {
  test(`${mode} problems save only on an explicit action`, async ({ page }) => {
    const writes = await openManualSaveOnly(page, mode)
    await page.locator('#problem-title').fill('明示的に反映')
    // Wait beyond the autosave debounce to prove no write occurs.
    await page.waitForTimeout(900)
    expect(writes()).toBe(0)
    await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'dirty')
    await page.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
    expect(writes()).toBe(1)
    await page.locator('#problem-title').fill('まだ未保存')
    await page.waitForTimeout(900)
    expect(writes()).toBe(1)
    await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.getByRole('button', { name: '保存して移動', exact: true }).click()
    await expect(page).toHaveURL('/my')
    expect(writes()).toBe(2)
  })

  test(`${mode} problems stay unsaved after an edit and after the leave and delete dialogs close`, async ({ page }) => {
    const writes = await openManualSaveOnly(page, mode)
    await page.clock.install()
    await page.clock.pauseAt(new Date(Date.now() + 1000))
    const status = page.locator('[data-save-state]')
    const reminder = page.getByText('変更を反映するには保存ボタンを押してください。')
    await page.locator('#problem-title').fill('ダイアログ後も未保存')
    await page.clock.runFor(2000)
    expect(writes()).toBe(0)
    await expect(status).toHaveAttribute('data-save-state', 'dirty')
    await expect(reminder).toHaveCount(0)
    await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.getByRole('button', { name: '編集を続ける', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeHidden()
    await page.clock.runFor(2000)
    expect(writes()).toBe(0)
    await expect(status).toHaveAttribute('data-save-state', 'dirty')
    await expect(reminder).toHaveCount(0)
    await page.getByRole('button', { name: '問題管理', exact: true }).click()
    await page.getByRole('button', { name: '問題を削除', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.getByRole('button', { name: 'キャンセル', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeHidden()
    await page.clock.runFor(2000)
    expect(writes()).toBe(0)
    await expect(status).toHaveAttribute('data-save-state', 'dirty')
    await expect(reminder).toHaveCount(0)
  })
}

test('save toasts expire, reset on repeated saves and never report a failed save as success', async ({ page }, testInfo) => {
  await page.goto('/problems/new')
  await expect(page.getByRole('button', { name: '保存', exact: true })).toBeEnabled()
  const toast = page.locator('.toast')
  await expect(toast).toHaveCount(0)
  await page.locator('#problem-title').fill('トースト確認')
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await expect(toast).toHaveCount(0)
  await page.clock.install()
  const save = page.getByRole('button', { name: '保存', exact: true })
  await save.click()
  await expect(toast.getByRole('status')).toHaveText('保存しました。')
  await page.clock.fastForward(4000)
  await save.click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.clock.fastForward(1500)
  await expect(toast).toBeVisible()
  await page.clock.fastForward(3501)
  await expect(toast).toHaveCount(0)
  await save.click()
  await expect(toast).toBeVisible()
  await toast.getByRole('button', { name: '通知を閉じる' }).focus()
  await page.clock.fastForward(6000)
  await expect(toast).toBeVisible()
  await toast.getByRole('button', { name: '通知を閉じる' }).click()
  await expect(toast).toHaveCount(0)
  await page.setViewportSize({ width: 375, height: 812 })
  await save.click()
  await expect(toast).toBeInViewport()
  await page.screenshot({ path: testInfo.outputPath('save-toast-mobile.png') })
  await page.route('**/api/my/problems/*', route => route.request().method() === 'PUT' ? route.fulfill({ status: 500, json: {} }) : route.fallback())
  await save.click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'failed')
  await expect(toast).toHaveCount(0)
  await expect(page.getByRole('alert')).toBeVisible()
})
