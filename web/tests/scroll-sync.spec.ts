import type { Locator, Page } from '@playwright/test'
import { expect, test } from './fixtures/account'

const source = Array.from({ length: 40 }, (_, i) => `## 節 ${i}\n\n${'長い本文です。'.repeat(i % 2 ? 3 : 30)}\n\n${'- 項目\n'.repeat(i % 3)}`).join('\n')

// Each pane follows the point that moves from the top of its viewport to the bottom as it scrolls.
const sectionAtFollowedPoint = (pane: Locator, selector: string) => pane.evaluate((element, selector) => {
  const followed = element.getBoundingClientRect().top + element.clientHeight * element.scrollTop / (element.scrollHeight - element.clientHeight)
  let section = -1
  for (const node of element.querySelectorAll(selector)) {
    const match = /^(?:## )?節 (\d+)$/.exec(node.textContent ?? '')
    if (match && node.getBoundingClientRect().top <= followed) section = Math.max(section, Number(match[1]))
  }
  return section
}, selector)
// Scroll events are dispatched before animation frames, so two frames are enough for handlers and
// CodeMirror's redraw to run.
const settle = (page: Page) => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
async function scrollTo(pane: Locator, ratio: number) {
  await pane.evaluate((element, ratio) => { element.scrollTop = (element.scrollHeight - element.clientHeight) * ratio }, ratio)
  await settle(pane.page())
}
const scrollRatio = (pane: Locator) => pane.evaluate(element => element.scrollTop / (element.scrollHeight - element.clientHeight))
async function expectAligned(editor: Locator, preview: Locator) {
  await expect.poll(async () => Math.abs(await sectionAtFollowedPoint(preview, 'h2') - await sectionAtFollowedPoint(editor, '.cm-line'))).toBeLessThanOrEqual(1)
}

for (const [path, selector] of [
  ['/problems/new', '#problem-source'],
  ['/blog/new', '#post-body'],
  ['/my/contests/new', '#contest-description'],
]) {
  test(`editor and preview scroll together in ${path}`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto(path!)
    await page.locator(selector!).fill(source)
    const editor = page.locator('.markdown-source-editor .cm-scroller')
    const preview = page.locator('.preview-document')
    await expect(preview.locator('h2')).toHaveCount(40)
    await scrollTo(editor, .5)
    expect(await sectionAtFollowedPoint(editor, '.cm-line')).toBeGreaterThan(10)
    await expectAligned(editor, preview)
    await scrollTo(editor, 1)
    await expect.poll(() => scrollRatio(preview)).toBeGreaterThan(.99)
    await scrollTo(preview, .3)
    await expectAligned(editor, preview)
    await scrollTo(preview, 0)
    await expect.poll(() => editor.evaluate(element => element.scrollTop)).toBe(0)
  })
}

test('scroll sync can be turned off, persists, and keeps the line when switching views', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/problems/new')
  await page.locator('#problem-source').fill(source)
  const editor = page.locator('.markdown-source-editor .cm-scroller')
  const preview = page.locator('.preview-document')
  await expect(preview.locator('h2')).toHaveCount(40)
  const settings = page.getByRole('button', { name: 'エディタ設定', exact: true })
  const dialog = page.getByRole('dialog', { name: 'エディタ設定（Markdown）' })
  const toggle = dialog.getByRole('switch', { name: '編集欄とプレビューのスクロールを連動' })
  await settings.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await expect(dialog.getByText('ON', { exact: true })).toBeVisible()
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await expect(dialog.getByText('OFF', { exact: true })).toBeVisible()
  await page.mouse.click(2, 2)

  const previewTop = await preview.evaluate(element => element.scrollTop)
  await scrollTo(editor, .5)
  expect(await preview.evaluate(element => element.scrollTop)).toBe(previewTop)
  const editorTop = await editor.evaluate(element => element.scrollTop)
  await scrollTo(preview, .2)
  expect(await editor.evaluate(element => element.scrollTop)).toBe(editorTop)

  await settings.click()
  await dialog.getByText('編集欄とプレビューのスクロールを連動').click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await expectAligned(editor, preview)
  await toggle.click()
  await page.reload()
  await expect(page.locator('#problem-source')).toBeEditable()
  await settings.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await toggle.click()
  await page.mouse.click(2, 2)

  await page.locator('#problem-source').fill(source)
  await page.getByRole('button', { name: '編集', exact: true }).click()
  await scrollTo(editor, .6)
  const section = await sectionAtFollowedPoint(editor, '.cm-line')
  expect(section).toBeGreaterThan(10)
  await page.getByRole('button', { name: 'プレビュー', exact: true }).click()
  await expect.poll(async () => Math.abs(await sectionAtFollowedPoint(preview, 'h2') - section)).toBeLessThanOrEqual(1)
  await scrollTo(preview, .2)
  const previewSection = await sectionAtFollowedPoint(preview, 'h2')
  await page.getByRole('button', { name: '編集', exact: true }).click()
  await expect.poll(async () => Math.abs(await sectionAtFollowedPoint(editor, '.cm-line') - previewSection)).toBeLessThanOrEqual(1)
})
