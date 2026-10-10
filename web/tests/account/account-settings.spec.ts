import { expect, test, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

const origin = 'http://127.0.0.1:13002'
async function login(page: Page, password: string) {
  await page.goto('/login')
  await page.getByLabel('メールアドレス').fill('leaver@example.test')
  await page.getByLabel('パスワード', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
}

test('password changes and account deletion keep public work under an anonymous author', async ({ page, browser }) => {
  // leaver exists only for this test, because deletion takes the account with it.
  await login(page, 'test-password')
  await expect(page).toHaveURL('/my')
  const post = randomUUID()
  expect((await page.request.put(`/api/my/posts/${post}`, { headers: { origin }, data: { version: 0, title: '残る記事', markdown: '公開した本文' } })).status()).toBe(200)
  expect((await page.request.put(`/api/my/posts/${post}/publication`, { headers: { origin }, data: { version: 1, publish: true } })).status()).toBe(200)
  const draft = randomUUID()
  expect((await page.request.put(`/api/my/posts/${draft}`, { headers: { origin }, data: { version: 0, title: '消える下書き', markdown: '非公開' } })).status()).toBe(200)

  await page.locator('.site-header .account-nav').hover()
  await page.getByRole('navigation', { name: 'アカウント', exact: true }).getByRole('link', { name: '設定', exact: true }).click()
  await expect(page).toHaveURL('/my/settings')
  await page.getByRole('button', { name: '認証設定', exact: true }).click()
  await expect(page.locator('.sign-in')).toContainText('メールアドレスとパスワード')
  await expect(page.locator('.sign-in')).toContainText('leaver@example.test')
  await page.getByLabel('現在のパスワード').fill('wrong-password')
  await page.getByLabel('新しいパスワード', { exact: true }).fill('New-password-123!')
  await page.getByLabel('新しいパスワード（確認）').fill('New-password-123!')
  await page.getByRole('button', { name: 'パスワードを変更' }).click()
  await expect(page.getByRole('alert')).toHaveText('現在のパスワードが正しくありません。')
  await page.getByLabel('現在のパスワード').fill('test-password')
  await page.getByRole('button', { name: 'パスワードを変更' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'パスワードを変更しました' })).toBeVisible()
  await expect(page.getByLabel('現在のパスワード')).toHaveValue('')

  // The new password signs in; the old one no longer does.
  await page.goto('/my')
  await page.locator('#main').getByRole('button', { name: 'ログアウト', exact: true }).click()
  await login(page, 'test-password')
  await expect(page.getByRole('alert')).toBeVisible()
  await login(page, 'New-password-123!')
  await expect(page).toHaveURL('/my')

  await page.goto('/my/settings?tab=deletion')
  const remove = page.getByRole('button', { name: 'アカウントを削除', exact: true })
  await expect(remove).toBeDisabled()
  await page.getByLabel(/確認のため/).fill('leave')
  await expect(remove).toBeDisabled()
  await page.getByLabel(/確認のため/).fill('leaver')
  await remove.click()
  await expect(page).toHaveURL('/')
  await expect(page.locator('.site-header').getByRole('link', { name: 'ログイン', exact: true })).toBeVisible()

  // Published work stays under an anonymous author; private drafts, the profile page and the sign-in are gone.
  const guest = await browser.newContext({ baseURL: origin })
  const reader = await guest.newPage()
  await reader.goto(`/blog/${post}`)
  await expect(reader.getByRole('heading', { name: '残る記事', exact: true })).toBeVisible()
  await expect(reader.getByText('退会したユーザー', { exact: true })).toBeVisible()
  await expect(reader.locator('a[href^="/users/deleted_"]')).toHaveCount(0)
  expect((await guest.request.get('/api/users/leaver')).status()).toBe(404)
  expect((await page.request.get(`/api/my/posts/${draft}`)).status()).toBe(401)
  await login(page, 'New-password-123!')
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page).toHaveURL(/\/login/)
  await guest.close()
})
