import { expect, test } from './fixtures/account'
import type { Locator, Page } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

const id = '11111111-1111-4111-8111-111111111111'
const updatedAt = '2026-09-10T00:00:00Z'
const title = (page: Page) => page.getByLabel('問題のタイトル')
const source = (page: Page) => page.locator('#problem-source')

async function open(page: Page) {
  await page.goto('/problems/new')
  await expect(source(page)).toHaveAttribute('contenteditable', 'true')
  await expect(title(page)).toBeVisible()
}
const write = (page: Page) => source(page).tap({ position: { x: 120, y: 40 } })

// A saved draft shows every action in the bar: back, publish and save.
async function openSaved(page: Page) {
  const readiness = { publish: [], contest: [], featured: [] }
  const draft = { title: '保存した下書き', markdown: '問題本文', difficulty: 2, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] }
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 2, updatedAt, author: 'ui_test', draft, readiness } }))
  await page.route('**/api/my/problems?*', route => route.fulfill({ json: { items: [{ id, title: draft.title, difficulty: 2, updatedAt, readiness }], nextCursor: '' } }))
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.getByRole('button', { name: '公開・応募', exact: true })).toBeVisible()
}

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

async function iconIsCentered(action: Locator) {
  const box = await action.boundingBox()
  const icon = await action.locator('svg').boundingBox()
  expect(Math.abs(icon!.x + icon!.width / 2 - (box!.x + box!.width / 2))).toBeLessThanOrEqual(1)
  expect(Math.abs(icon!.y + icon!.height / 2 - (box!.y + box!.height / 2))).toBeLessThanOrEqual(1)
}

test.describe('the top bar on a phone', () => {
  test.use({ hasTouch: true, isMobile: true })

  for (const width of [312, 320, 375]) {
    test(`stays one line at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 667 })
      await openSaved(page)
      const back = page.getByRole('link', { name: '問題を見る', exact: true })
      const publish = page.getByRole('button', { name: '公開・応募', exact: true })
      const save = page.getByRole('button', { name: '保存', exact: true })
      for (const action of [back, publish]) {
        await expect(action).toHaveText('', { useInnerText: true })
        await expect(action.locator('svg:visible')).toHaveCount(1)
      }
      await expect(save).toHaveText('保存', { useInnerText: true })
      const tops = new Set<number>()
      for (const action of [back, publish, save]) {
        await expect(action).toBeInViewport({ ratio: 1 })
        tops.add((await action.boundingBox())!.y)
      }
      expect(tops.size).toBe(1)
      expect((await page.locator('.editor-topbar').boundingBox())!.height).toBeLessThan(60)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.locator('.editor-topbar').screenshot({ path: test.info().outputPath(`topbar-${width}.png`) })
    })
  }

  test('one button switches between editing and the preview', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await openSaved(page)
    const edit = page.getByRole('button', { name: '編集', exact: true })
    const preview = page.getByRole('button', { name: 'プレビュー', exact: true })
    await expect(edit).toBeHidden()
    await preview.tap()
    await expect(page.locator('.preview-document')).toBeVisible()
    await expect(preview).toBeHidden()
    await edit.tap()
    await expect(source(page)).toBeVisible()
    await expect(edit).toBeHidden()
    await expect(preview).toBeVisible()
  })

  test('the back icon appears once there is a saved problem to go back to', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await open(page)
    await expect(page.locator('.back-link')).toHaveCount(0)
    await openSaved(page)
    const problem = page.getByRole('link', { name: '問題を見る', exact: true })
    await expect(problem).toHaveAttribute('href', `/problems/${shortId(id)}`)
    await iconIsCentered(problem)
  })

  test('the view button sits on the left with the actions on the right', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await openSaved(page)
    const theme = await page.locator('.editor-topbar .theme-toggle').boundingBox()
    const view = await page.getByRole('button', { name: 'プレビュー', exact: true }).boundingBox()
    const back = await page.getByRole('link', { name: '問題を見る', exact: true }).boundingBox()
    const save = await page.getByRole('button', { name: '保存', exact: true }).boundingBox()
    expect(view!.x - (theme!.x + theme!.width)).toBeLessThanOrEqual(8)
    expect(back!.x - (view!.x + view!.width)).toBeGreaterThan(8)
    expect(375 - (save!.x + save!.width)).toBeLessThanOrEqual(8)
  })
})

test('a wide screen keeps the words on the actions and every view button', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await openSaved(page)
  await expect(page.getByRole('link', { name: '問題を見る', exact: true })).toHaveText('問題を見る', { useInnerText: true })
  await expect(page.getByRole('button', { name: '公開・応募', exact: true })).toHaveText('公開・応募', { useInnerText: true })
  await expect(page.getByRole('button', { name: '保存', exact: true })).toHaveText('保存', { useInnerText: true })
  for (const name of ['編集', '分割', 'プレビュー']) await expect(page.getByRole('button', { name, exact: true })).toBeVisible()
})

test('a narrow preview leaves out the fields its own heading repeats', async ({ page }) => {
  await page.setViewportSize({ width: 639, height: 800 })
  await openSaved(page)
  await page.getByRole('button', { name: 'プレビュー', exact: true }).click()
  await expect(page.locator('.preview-title')).toHaveText('保存した下書き')
  await expect(title(page)).toBeHidden()
  await expect(page.locator('.problem-settings-toggle')).toBeHidden()
  await page.getByRole('button', { name: '編集', exact: true }).click()
  await expect(title(page)).toBeVisible()
  await expect(title(page)).toHaveValue('保存した下書き')
})

for (const width of [640, 1280]) {
  test(`the preview at ${width}px keeps the fields so the page does not jump`, async ({ page }) => {
    await page.setViewportSize({ width, height: 800 })
    await openSaved(page)
    const before = await title(page).boundingBox()
    await page.getByRole('button', { name: 'プレビュー', exact: true }).click()
    await expect(page.locator('.preview-title')).toBeVisible()
    await expect(title(page)).toBeVisible()
    expect(await title(page).boundingBox()).toEqual(before)
  })
}
