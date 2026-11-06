import { expect, test } from './fixtures/account'
import type { Page } from '@playwright/test'

const title = (page: Page) => page.getByLabel('問題のタイトル')
const source = (page: Page) => page.locator('#problem-source')

async function open(page: Page) {
  await page.goto('/problems/new')
  await expect(source(page)).toHaveAttribute('contenteditable', 'true')
  await expect(title(page)).toBeVisible()
}
const write = (page: Page) => source(page).tap({ position: { x: 120, y: 40 } })

test.describe('on a phone', () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 375, height: 667 } })

  test('writing the statement gives the source the room of the fields above it', async ({ page }) => {
    await open(page)
    const before = await page.locator('.source-pane .cm-scroller').boundingBox()
    await write(page)
    await expect(title(page)).toBeHidden()
    await expect(page.locator('#problem-source-label')).toBeHidden()
    const after = await page.locator('.source-pane .cm-scroller').boundingBox()
    expect(after!.height - before!.height).toBeGreaterThanOrEqual(160)
    await expect(page.getByRole('button', { name: '太字', exact: true })).toBeInViewport()
    await expect(page.getByRole('button', { name: '保存', exact: true })).toBeInViewport()
    await page.screenshot({ path: test.info().outputPath('writing.png') })
  })

  test('the toolbar keeps the source in focus while writing', async ({ page }) => {
    await open(page)
    await write(page)
    await expect(title(page)).toBeHidden()
    await page.getByRole('button', { name: '太字', exact: true }).tap()
    await expect(source(page)).toContainText('**強調**')
    await expect(source(page)).toBeFocused()
    await expect(title(page)).toBeHidden()
  })

  test('the toolbar works from outside the source and starts writing', async ({ page }) => {
    await open(page)
    await page.getByRole('button', { name: '太字', exact: true }).tap()
    await expect(source(page)).toContainText('**強調**')
    await expect(title(page)).toBeHidden()
  })

  test('the editor settings open while writing', async ({ page }) => {
    await open(page)
    await write(page)
    await expect(title(page)).toBeHidden()
    await page.getByRole('button', { name: 'エディタ設定', exact: true }).tap()
    await expect(page.getByRole('dialog', { name: 'エディタ設定（Markdown）' })).toBeVisible()
  })

  test('leaving the source brings the fields back', async ({ page }) => {
    await open(page)
    await write(page)
    await expect(title(page)).toBeHidden()
    await page.locator('.editor-sidebar').tap({ position: { x: 28, y: 400 } })
    await expect(title(page)).toBeVisible()
    await title(page).tap()
    await expect(title(page)).toBeFocused()
  })
})

test.describe('with a mouse in a narrow window', () => {
  test.use({ viewport: { width: 375, height: 667 } })

  test('the fields stay while writing', async ({ page }) => {
    await open(page)
    await source(page).click({ position: { x: 120, y: 40 } })
    await expect(source(page)).toBeFocused()
    await expect(title(page)).toBeVisible()
  })
})

test.describe('on a tablet', () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 768, height: 1024 } })

  test('the fields stay while writing', async ({ page }) => {
    await open(page)
    await write(page)
    await expect(source(page)).toBeFocused()
    await expect(title(page)).toBeVisible()
  })
})
