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

test('the footer groups its links and opens external ones in a new tab', async ({ page }) => {
  await page.goto('/problems')
  const footer = page.locator('.site-footer')
  const groups = footer.getByRole('navigation')
  await expect(groups).toHaveCount(3)
  for (const [index, name] of ['ガイド', '規約', '外部リンク'].entries()) await expect(groups.nth(index)).toHaveAccessibleName(name)
  const external = footer.getByRole('navigation', { name: '外部リンク', exact: true })
  await expect(external.getByRole('link', { name: 'X（@share_oj） ↗', exact: true })).toHaveAttribute('href', 'https://x.com/share_oj')
  await expect(external.getByRole('link', { name: 'GitHub ↗', exact: true })).toHaveAttribute('href', 'https://github.com/loop0919/shareoj')
  for (const link of await external.getByRole('link').all()) {
    await expect(link).toHaveAttribute('target', '_blank')
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  }
})

test('users registered before the terms see the notice until they close it', async ({ page }) => {
  await page.goto('/problems')
  const notice = page.getByRole('region', { name: '利用規約とプライバシーポリシーのお知らせ', exact: true })
  await expect(notice).toContainText('2026年10月16日に利用規約とプライバシーポリシーを定めました。')
  await expect(notice.getByRole('link', { name: '利用規約', exact: true })).toHaveAttribute('href', '/terms')
  await notice.getByRole('button', { name: '閉じる', exact: true }).click()
  await expect(notice).toHaveCount(0)
  await page.reload()
  await expect(page.locator('.site-footer')).toBeVisible()
  await expect(notice).toHaveCount(0)
})

test('users registered under the terms do not see the notice', async ({ page }) => {
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'ui_test', avatar: '', version: 1, createdAt: '2026-10-16T00:00:00+09:00' } } }))
  await page.goto('/problems')
  await expect(page.locator('.site-footer')).toBeVisible()
  await expect(page.locator('.site-header').getByRole('button', { name: 'アカウントメニュー', exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '利用規約とプライバシーポリシーのお知らせ', exact: true })).toHaveCount(0)
})
