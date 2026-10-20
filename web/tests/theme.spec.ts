import { expect, test } from './fixtures/account'
const problem = '/problems/11111111-1111-4111-8111-111111111111'

test('system tracking, persistent overrides, SSR and editor input survive theme changes', async ({ page }) => {
  const errors: string[] = []
  page.on('console', message => { if (/hydration/i.test(message.text())) errors.push(message.text()) })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(problem)
  const toggle = page.getByRole('button', { name: /モードに切り替え$/ })
  const body = page.locator('body')
  const editor = page.getByLabel('ソースコード', { exact: true })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'system')
  await expect(body).toHaveCSS('background-color', 'rgb(21, 27, 35)')
  await expect(toggle).toHaveAccessibleName('ライトモードに切り替え')
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(body).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(toggle).toHaveAccessibleName('ダークモードに切り替え')
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(body).toHaveCSS('background-color', 'rgb(21, 27, 35)')
  await editor.fill('int main() { return 42; }')
  await expect(page.locator('.cm-editor')).toHaveCSS('background-color', 'rgb(21, 27, 35)')
  await toggle.click()
  await expect(body).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(page.locator('.cm-editor')).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(editor).toHaveText('int main() { return 42; }')
  await page.reload()
  await expect(toggle).toHaveAccessibleName('ダークモードに切り替え')
  await expect(body).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await toggle.click()
  await page.emulateMedia({ colorScheme: 'light' })
  await page.reload()
  await expect(toggle).toHaveAccessibleName('ライトモードに切り替え')
  await expect(body).toHaveCSS('background-color', 'rgb(21, 27, 35)')
  expect(await (await page.request.get(problem)).text()).toContain('data-theme="dark"')
  expect(errors).toEqual([])
})
for (const path of ['/', '/problems/new', '/blog/new', '/my/contests/new']) {
  test(`mobile selector and navigation persistence: ${path}`, async ({ page }, testInfo) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await page.setViewportSize({ width: 320, height: 740 })
    await page.goto(path)
    const toggle = page.getByRole('button', { name: /モードに切り替え$/ })
    await toggle.click()
    const bounds = (await toggle.boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(320)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320)
    await page.screenshot({ path: testInfo.outputPath('dark-mobile.png'), fullPage: true })
    await page.goto('/login')
    await expect(toggle).toHaveAccessibleName('ライトモードに切り替え')
    await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(21, 27, 35)')
  })
}
test('invalid preference falls back to system', async ({ page, context }) => {
  await context.addCookies([{ name: 'openoj-theme', value: 'invalid', url: 'http://127.0.0.1:13000' }])
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'system')
  await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(21, 27, 35)')
})

for (const reducedMotion of ['no-preference', 'reduce'] as const) {
  test(`home animation colors follow theme changes: ${reducedMotion}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light', reducedMotion })
    await page.goto('/')
    const toggle = page.getByRole('button', { name: /モードに切り替え$/ })
    const navigation = page.getByRole('navigation', { name: 'メインナビゲーション' })
    const themeBounds = (await toggle.boundingBox())!
    const navBounds = (await navigation.boundingBox())!
    expect(navBounds.x + navBounds.width).toBeLessThanOrEqual(themeBounds.x)
    if (reducedMotion === 'no-preference') {
      await page.locator('.grid-dot').evaluateAll(dots => dots.forEach(dot => dot.getAnimations().forEach(animation => {
        animation.pause()
        animation.currentTime = Number(animation.effect!.getTiming().delay) + 330
      })))
      await toggle.click()
      await expect.poll(() => page.locator('.grid-dot').evaluateAll(dots => [...new Set(dots.map(dot => getComputedStyle(dot).fill))])).toEqual(['rgb(112, 219, 162)'])
      await toggle.click()
      await expect.poll(() => page.locator('.grid-dot').evaluateAll(dots => [...new Set(dots.map(dot => getComputedStyle(dot).fill))])).toEqual(['rgb(24, 121, 78)'])
    }
    // Finish the initial reveal, including the staggered ripple on all 255 dots.
    await page.locator('.path-art').evaluate(svg => svg.getAnimations({ subtree: true }).forEach(animation => animation.finish()))
    for (const [label, line, accent] of [['ダーク', 'rgb(58, 72, 88)', 'rgb(112, 219, 162)'], ['ライト', 'rgb(220, 226, 232)', 'rgb(24, 121, 78)']]) {
      await toggle.focus()
      await page.keyboard.press('Enter')
      await expect(toggle).toHaveAccessibleName(label === 'ダーク' ? 'ライトモードに切り替え' : 'ダークモードに切り替え')
      await expect.poll(() => page.locator('.grid-dot').evaluateAll(dots => [...new Set(dots.map(dot => getComputedStyle(dot).fill))])).toEqual([line])
      await expect(page.locator('.route-node')).toHaveCSS('fill', accent!)
      await expect(page.locator('.start-node')).toHaveCSS('stroke', accent!)
    }
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect.poll(() => page.locator('.grid-dot').evaluateAll(dots => [...new Set(dots.map(dot => getComputedStyle(dot).fill))])).toEqual(['rgb(220, 226, 232)'])
  })
}

test('first click inverts the light system default and saves it', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'system')
  await page.getByRole('button', { name: 'ダークモードに切り替え', exact: true }).click()
  await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(21, 27, 35)')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
})
