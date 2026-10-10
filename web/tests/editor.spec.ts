import { expect, test } from './fixtures/account'

test('creation page has noindex and is linked from navigation', async ({ page, request }) => {
  const response = await request.get('/problems/new')
  expect(response.status()).toBe(200)
  expect(await response.text()).toContain('name="robots" content="noindex, nofollow"')
  await page.goto('/')
  await page.locator('.site-header').getByRole('button', { name: 'アカウントメニュー', exact: true }).click()
  await page.getByRole('navigation', { name: 'アカウント', exact: true }).getByRole('link', { name: 'マイページ', exact: true }).click()
  await page.getByRole('main').getByRole('link', { name: '新規問題' }).click()
  await expect(page).toHaveTitle('問題を作成 | ShareOJ')
  await expect(page.getByLabel('問題のタイトル', { exact: true })).toBeEnabled()
  await expect(page.locator('#memory-limit')).toHaveValue('512')
  await expect(page.locator('#memory-limit')).toHaveAttribute('aria-valuemax', '512')
})

test('editing, math preview, automatic save, and reload work together', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (/hydration/i.test(message.text())) errors.push(message.text()) })
  await page.goto('/problems/new')
  await expect(page.getByLabel('問題のタイトル', { exact: true })).toBeEnabled()
  // Hold the first draft URL update to reproduce a reload during navigation.
  await page.evaluate(() => {
    const app = (document.querySelector('#__nuxt') as any).__vue_app__
    app.config.globalProperties.$router.beforeEach(async (to: any) => {
      if (to.query.problem) {
        document.documentElement.dataset.draftNavigation = 'pending'
        await new Promise<void>(resolve => {
          ;(window as any).finishDraftNavigation = resolve
        })
      }
    })
  })
  await page.getByLabel('問題のタイトル', { exact: true }).fill('数列の和')
  await page.locator('#time-limit').fill('3000')
  await page.locator('#memory-limit').fill('512')
  const body = '## 好きな構成\n\n合計は $a_1+a_2$ です。\n\n```input\n$N$\n$A_1 \\quad A_N$\n```\n\n```math\n\\sum_{i=1}^{N} A_i\n```\n\n```text\n$literal$\n```'
  await page.locator('#problem-source').fill(body)
  await expect(page.getByRole('region', { name: '問題のプレビュー' }).getByRole('heading', { name: '好きな構成' })).toBeVisible()
  await expect(page.locator('.preview-pane .input-format .katex')).toHaveCount(2)
  await expect(page.locator('.preview-pane .katex-display')).toHaveCount(1)
  await page.getByRole('button', { name: '解説', exact: true }).click()
  await page.locator('#problem-source').fill('## 解法\n\n```cpp\nreturn A + B;\n```')
  await expect(page.getByRole('region', { name: '解説のプレビュー' }).getByRole('heading', { name: '解法' })).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-draft-navigation', 'pending')
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saving')
  await page.evaluate(() => (window as any).finishDraftNavigation())
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await expect(page).toHaveURL(/\/problems\/new\?problem=[^&]+$/)
  await page.reload()
  await expect(page.locator('#problem-title')).toHaveValue('数列の和')
  await expect(page.locator('#problem-source')).toHaveText(body, { useInnerText: true })
  await page.getByRole('button', { name: '解説', exact: true }).click()
  await expect(page.locator('#problem-source')).toHaveText(/## 解法/, { useInnerText: true })
  await page.getByRole('button', { name: '問題文', exact: true }).click()
  await expect(page.locator('#time-limit')).toHaveAttribute('aria-valuenow', '3000')
  await expect(page.locator('#memory-limit')).toHaveAttribute('aria-valuenow', '512')
  expect(errors).toEqual([])
})

test('incomplete drafts remain saveable', async ({ page }) => {
  await page.goto('/problems/new')
  await expect(page.getByRole('button', { name: 'Markdown を保存', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
})

test('legacy memory limits are clamped to the current maximum', async ({ page }) => {
  const id = '11111111-1111-4111-8111-111111111112'
  await page.goto('/problems/new')
  await page.evaluate(async (id) => {
    await fetch(`/api/my/problems/${id}`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ version: 0, draft: { title: '', markdown: '', timeLimitMs: '2000', memoryLimitMb: '1024', testCases: [] } }),
    })
  }, id)
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.locator('#memory-limit')).toHaveValue('512')
  await expect(page.locator('#memory-limit')).toHaveAttribute('aria-valuemax', '512')
})

test('cache failures do not prevent DB saving', async ({ page }) => {
  await page.addInitScript(() => {
    Storage.prototype.setItem = () => { throw new DOMException('Full', 'QuotaExceededError') }
  })
  await page.goto('/problems/new')
  await page.locator('#problem-title').fill('保存できない問題')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await expect(page.locator('#problem-title')).toBeEnabled()
})

for (const width of [320, 375, 414, 768, 1280]) {
  test(`editor is usable at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/problems/new')
    await expect(page.locator('#problem-source')).toBeEnabled()
    const sidebar = page.getByRole('complementary', { name: '問題作成サイドバー' })
    const toggle = sidebar.getByRole('button', { name: /サイドバーを/ })
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    const collapsedWidth = (await sidebar.boundingBox())!.width
    await toggle.focus()
    await page.keyboard.press('Enter')
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect((await sidebar.boundingBox())!.width).toBeGreaterThan(collapsedWidth)
    await expect(sidebar.locator('.editor-sidebar-label').filter({ hasText: '問題文' })).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath(`sidebar-expanded-${width}.png`), fullPage: true })
    await sidebar.getByRole('button', { name: '問題管理', exact: true }).click()
    await expect(page.getByRole('heading', { name: '問題管理', exact: true })).toBeVisible()
    await sidebar.getByRole('button', { name: '問題文', exact: true }).click()
    await expect(page.locator('#problem-source')).toBeVisible()
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    const editorial = page.getByRole('button', { name: '解説', exact: true })
    await expect(editorial).toBeEnabled()
    const bounds = await editorial.boundingBox()
    expect(bounds!.x).toBeGreaterThanOrEqual(0)
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width)
    await expect(page.getByRole('heading', { name: '問題を作成', exact: true })).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true)
    await page.locator('#problem-source').fill('## テスト\n')
    await page.getByRole('button', { name: '入力形式', exact: true }).click()
    await expect(page.locator('#problem-source')).toHaveText(/```input/, { useInnerText: true })
    if (width < 960) await page.getByRole('button', { name: 'プレビュー', exact: true }).click()
    await expect(page.locator('.preview-pane').getByRole('heading', { name: 'テスト', exact: true })).toBeVisible()
    await expect(page.locator('.preview-pane .input-format')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`editor-${width}.png`), fullPage: true })
  })
}

test.describe('small touch screens', () => {
  test.use({ hasTouch: true, isMobile: true })

  for (const viewport of [{ width: 312, height: 553 }, { width: 375, height: 667 }, { width: 667, height: 375 }, { width: 320, height: 360 }]) {
    test(`keeps a usable Markdown area at ${viewport.width}x${viewport.height}`, async ({ page }, testInfo) => {
      await page.setViewportSize(viewport)
      await page.goto('/problems/new')
      const source = page.locator('#problem-source')
      await expect(source).toBeEnabled()
      const settings = page.getByRole('button', { name: '問題設定', exact: true })
      await expect(settings).toHaveAttribute('aria-expanded', 'false')
      await expect(page.locator('#time-limit')).toBeHidden()
      await expect(page.locator('#memory-limit')).toBeHidden()
      await settings.click()
      await expect(settings).toHaveAttribute('aria-expanded', 'true')
      const difficultyHeight = (await page.locator('#problem-difficulty').boundingBox())!.height
      for (const stepper of await page.locator('#problem-settings .limit-stepper').all()) {
        expect((await stepper.boundingBox())!.height).toBe(difficultyHeight)
      }
      await page.locator('#time-limit').fill('3000')
      await page.locator('#memory-limit').fill('256')
      await page.screenshot({ path: testInfo.outputPath('mobile-settings-expanded.png'), fullPage: true })
      await settings.click()
      await expect(settings).toHaveText(/3000 ms \/ 256 MiB/)
      await expect(page.locator('#memory-limit')).toBeHidden()
      const topbar = page.locator('.editor-topbar')
      if (viewport.width < 640) expect((await topbar.boundingBox())!.height).toBeLessThan(65)
      const toolbar = page.getByLabel('記法を挿入', { exact: true })
      expect((await toolbar.boundingBox())!.height).toBeLessThan(65)
      expect((await source.boundingBox())!.height).toBeGreaterThanOrEqual(220)
      await source.scrollIntoViewIfNeeded()
      const visibleHeight = await source.evaluate(element => {
        let top = 0, bottom = innerHeight
        for (let parent: Element | null = element; parent; parent = parent.parentElement) {
          const bounds = parent.getBoundingClientRect()
          top = Math.max(top, bounds.top)
          bottom = Math.min(bottom, bounds.bottom)
        }
        return bottom - top
      })
      expect(visibleHeight).toBeGreaterThanOrEqual(200)
      await source.fill('## スマホで編集\n\n複数行を表示できます。\n')
      await expect(source).toHaveText(/複数行を表示できます/, { useInnerText: true })
      await page.screenshot({ path: testInfo.outputPath('mobile-markdown.png'), fullPage: true })
      await page.getByRole('button', { name: '折りたたみ', exact: true }).click()
      await expect(source).toHaveText(/:::details/, { useInnerText: true })
      await page.getByLabel('問題のタイトル', { exact: true }).fill('スマホの問題')
      await page.getByRole('button', { name: '保存', exact: true }).click()
      await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
      await page.getByRole('button', { name: 'プレビュー', exact: true }).click()
      await expect(page.getByRole('heading', { name: 'スマホで編集', exact: true })).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true)
    })
  }
})

test('problem settings follow viewport width without losing edited values', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/problems/new')
  const settings = page.getByRole('button', { name: '問題設定', exact: true })
  const time = page.locator('#time-limit')
  await expect(settings).toBeHidden()
  const difficultyHeight = (await page.locator('#problem-difficulty').boundingBox())!.height
  for (const stepper of await page.locator('#problem-settings .limit-stepper').all()) {
    expect((await stepper.boundingBox())!.height).toBe(difficultyHeight)
  }
  await time.fill('3000')
  await page.setViewportSize({ width: 375, height: 667 })
  await expect(settings).toHaveAttribute('aria-expanded', 'false')
  await expect(time).toBeHidden()
  await settings.focus()
  await page.keyboard.press('Enter')
  await expect(time).toBeVisible()
  await expect(time).toHaveValue('3000')
  await settings.click()
  await page.setViewportSize({ width: 1280, height: 900 })
  await expect(settings).toBeHidden()
  await expect(time).toBeVisible()
  await expect(time).toHaveValue('3000')
})

test('line numbers follow wrapped lines and scrolling', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 700 })
  await page.goto('/problems/new')
  const source = ['長い文章です。'.repeat(60), ...Array.from({ length: 70 }, (_, i) => `行 ${i}`)].join('\n')
  await page.locator('#problem-source').fill(source)
  const scroller = page.locator('.markdown-source-editor .cm-scroller')
  const firstLine = page.locator('#problem-source .cm-line').first()
  const firstNumber = page.locator('.markdown-source-editor .cm-lineNumbers .cm-gutterElement').filter({ hasText: /^1$/ })
  await expect(firstNumber).toBeVisible()
  expect((await firstLine.boundingBox())!.height).toBeGreaterThan(22)
  expect(Math.abs((await firstNumber.boundingBox())!.y - (await firstLine.boundingBox())!.y)).toBeLessThan(2)
  await scroller.evaluate(element => { element.scrollTop = element.scrollHeight })
  await expect(page.locator('.markdown-source-editor .cm-lineNumbers .cm-gutterElement').filter({ hasText: /^71$/ })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true)
})
