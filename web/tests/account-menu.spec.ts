import { expect, test } from './fixtures/account'

test('clicking the avatar opens the account menu, whose items lead on', async ({ page }) => {
  let loggedOut = false
  await page.route('**/api/auth/logout', route => { loggedOut = true; return route.fulfill({ status: 204 }) })
  await page.goto('/problems')
  const header = page.locator('.site-header')
  const avatar = header.getByRole('button', { name: 'アカウントメニュー', exact: true })
  const menu = header.getByRole('navigation', { name: 'アカウント', exact: true })
  await expect(menu).toBeHidden()
  // Hovering no longer opens it; a click does, and the page stays put.
  await avatar.hover()
  await expect(menu).toBeHidden()
  await avatar.click()
  await expect(menu).toBeVisible()
  await expect(avatar).toHaveAttribute('aria-expanded', 'true')
  await expect(menu).toContainText('ui_test')
  await expect(page).toHaveURL(/\/problems$/)
  await avatar.click()
  await expect(menu).toBeHidden()
  await avatar.click()
  await page.getByRole('heading', { level: 1 }).click()
  await expect(menu).toBeHidden()
  // Keyboard: Enter opens, Tab walks the items, Escape returns to the avatar.
  await avatar.focus()
  await page.keyboard.press('Enter')
  await expect(menu).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(menu.getByRole('link', { name: 'マイページ', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await expect(avatar).toBeFocused()
  await page.keyboard.press('Enter')
  for (let i = 0; i < 4; i++) await page.keyboard.press('Tab')
  await expect(menu).toBeHidden()
  await avatar.click()
  await menu.getByRole('link', { name: 'マイページ', exact: true }).click()
  await expect(page).toHaveURL(/\/my$/)
  await expect(menu).toBeHidden()
  await expect(page.locator('#main').getByRole('button', { name: 'ログアウト' })).toHaveCount(0)
  await avatar.click()
  const logout = menu.getByRole('button', { name: 'ログアウト', exact: true })
  // Logout alone uses the error color.
  const errorColor = await page.evaluate(() => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-error)'
    document.body.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })
  await expect(logout).toHaveCSS('color', errorColor)
  await expect(menu.getByRole('link', { name: '設定', exact: true })).not.toHaveCSS('color', errorColor)
  await logout.click()
  await expect(page).toHaveURL(/\/login$/)
  expect(loggedOut).toBe(true)
})

test.describe('touch', () => {
  test.use({ hasTouch: true, viewport: { width: 375, height: 800 } })
  test('a tap opens the menu instead of leaving the page', async ({ page }) => {
    await page.goto('/problems')
    await page.locator('.site-header').getByRole('button', { name: 'アカウントメニュー', exact: true }).tap()
    const menu = page.getByRole('navigation', { name: 'アカウント', exact: true })
    await expect(menu).toBeVisible()
    await expect(page).toHaveURL(/\/problems$/)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await menu.getByRole('link', { name: 'マイページ', exact: true }).tap()
    await expect(page).toHaveURL(/\/my$/)
  })
})
