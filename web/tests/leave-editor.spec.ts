import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'

// These tests leave before the 600 ms autosave. A slow runner could reach it before the click (then
// the guard holds the link while saving, or lets it go once saved), so its timer is frozen while
// typing and leaving. Each attempt to leave clears the pending timer.
async function typeTitle(page: Page, title: string) {
  // Like fill, a click waits as long as the test may for the editor to be ready.
  await page.locator('#problem-title').click()
  await page.clock.install()
  await page.clock.pauseAt(new Date(Date.now() + 1000))
  await page.locator('#problem-title').fill(title)
}

test('unsaved navigation can be cancelled or discarded without writing a draft', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 320, height: 700 })
  await page.goto('/problems/new?fresh=1')
  await typeTitle(page, '未保存の問題')
  await page.getByRole('link', { name: 'ShareOJ マイページ' }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  const dialog = await page.getByRole('dialog').boundingBox()
  expect(dialog!.x).toBeGreaterThanOrEqual(0)
  expect(dialog!.x + dialog!.width).toBeLessThanOrEqual(320)
  await page.screenshot({ path: testInfo.outputPath('leave-dialog.png') })
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.locator('#problem-title')).toHaveValue('未保存の問題')
  await page.getByRole('link', { name: 'ShareOJ マイページ' }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.mouse.click(2, 2)
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.locator('#problem-title')).toHaveValue('未保存の問題')
  await expect(page).toHaveURL(/\/problems\/new/)
  await page.getByRole('link', { name: 'ShareOJ マイページ' }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  // Navigation needs the clock; the open dialog has cleared the autosave timer.
  await page.clock.resume()
  await page.getByRole('button', { name: '保存せずに移動', exact: true }).click()
  await expect(page).toHaveURL('/my')
  await page.goto('/my?tab=problems')
  await expect(page.getByText('保存した問題はまだありません。')).toBeVisible()
})

test('failed save keeps the confirmation open', async ({ page }) => {
  await page.route('**/api/my/problems/*', route => route.abort())
  await page.goto('/problems/new?fresh=1')
  await typeTitle(page, '保存に失敗')
  await page.getByRole('link', { name: 'ShareOJ マイページ' }).click()
  await page.getByRole('button', { name: '保存して移動', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
  await page.getByRole('button', { name: '編集を続ける', exact: true }).click()
  await expect(page.locator('#problem-title')).toHaveValue('保存に失敗')
})
