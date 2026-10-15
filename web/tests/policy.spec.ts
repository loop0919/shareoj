import { expect, test } from './fixtures/account'

test('the footer links the terms and the privacy policy', async ({ page }) => {
  await page.goto('/problems')
  const footer = page.locator('.site-footer')
  await footer.getByRole('link', { name: '利用規約', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('利用規約')
  await expect(page.getByRole('heading', { name: '第15条（準拠法・裁判管轄）', exact: true })).toBeVisible()
  await footer.getByRole('link', { name: 'プライバシーポリシー', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('プライバシーポリシー')
  await expect(page.getByRole('link', { name: 'shareoj2026@gmail.com', exact: true })).toHaveAttribute('href', 'mailto:shareoj2026@gmail.com')
})

test('users registered before the terms see the notice until they close it', async ({ page }) => {
  await page.goto('/problems')
  const notice = page.getByRole('region', { name: '利用規約とプライバシーポリシーのお知らせ', exact: true })
  await expect(notice).toContainText('2026年10月15日に利用規約とプライバシーポリシーを定めました。')
  await expect(notice.getByRole('link', { name: '利用規約', exact: true })).toHaveAttribute('href', '/terms')
  await notice.getByRole('button', { name: '閉じる', exact: true }).click()
  await expect(notice).toHaveCount(0)
  await page.reload()
  await expect(page.locator('.site-footer')).toBeVisible()
  await expect(notice).toHaveCount(0)
})

test('users registered under the terms do not see the notice', async ({ page }) => {
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'ui_test', avatar: '', version: 1, createdAt: '2026-10-15T00:00:00+09:00' } } }))
  await page.goto('/problems')
  await expect(page.locator('.site-footer')).toBeVisible()
  await expect(page.locator('.site-header').getByRole('button', { name: 'アカウントメニュー', exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '利用規約とプライバシーポリシーのお知らせ', exact: true })).toHaveCount(0)
})
