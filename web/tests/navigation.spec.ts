import { expect, test } from '@playwright/test'

for (const width of [320, 375, 414, 768, 1280]) {
  test(`creation dropdown fits ${width}px when open`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/')
    await page.locator('.create-menu summary').click()
    const menu = page.locator('.create-menu-links')
    await expect(menu).toBeVisible()
    const bounds = (await menu.boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
  })
}

test('creation dropdown supports keyboard, dismissal, and creation links', async ({ page }) => {
  await page.goto('/')
  const nav = page.getByRole('navigation', { name: 'メインナビゲーション' })
  const toggle = nav.locator('summary')
  const problem = nav.getByRole('link', { name: '新規問題' })
  const blog = nav.getByRole('link', { name: '新規記事' })
  await expect(problem).toBeHidden()
  await toggle.focus()
  await page.keyboard.press('Enter')
  await expect(problem).toBeVisible()
  await expect(problem).toHaveAttribute('href', '/problems/new?fresh=1')
  await expect(blog).toHaveAttribute('href', '/blog/new')
  await page.keyboard.press('Escape')
  await expect(problem).toBeHidden()
  await expect(toggle).toBeFocused()
  await toggle.click()
  await page.locator('h1').click()
  await expect(problem).toBeHidden()
  await page.setViewportSize({ width: 375, height: 812 })
  await toggle.click()
  await expect(blog).toBeInViewport()
  await expect(problem).toBeInViewport()
  await blog.click()
  await expect(page).toHaveURL(/\/blog\/new$/)
})

test('footer lists the contact address and X account', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 })
  await page.goto('/')
  const footer = page.locator('.site-footer')
  await expect(footer.getByRole('link', { name: 'shareoj2026@gmail.com' })).toHaveAttribute('href', 'mailto:shareoj2026@gmail.com')
  const x = footer.getByRole('link', { name: /@share_oj/ })
  await expect(x).toHaveAttribute('href', 'https://x.com/share_oj')
  await expect(x).toHaveAttribute('target', '_blank')
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320)
})
