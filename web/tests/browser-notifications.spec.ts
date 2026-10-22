import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { fakeNotifications } from './fixtures/notifications'

const cookieName = 'openoj-browser-notifications'
const kindLabels = ['質問と回答（全体公開を含む）', 'コンテストの開始と終了', '定期便の出題', '定期便の解説公開']
const denied = 'ブラウザで通知が拒否されています。ブラウザのサイトの設定で通知を許可してください。'
const receive = (page: Page) => page.getByRole('switch', { name: 'このブラウザで通知を受け取る', exact: true })
const kind = (page: Page, index: number) => page.getByRole('switch', { name: kindLabels[index], exact: true })

test('allowing notifications turns every kind on', async ({ page }) => {
  await fakeNotifications(page, { permission: 'default', answer: 'granted' })
  await page.goto('/my/settings?tab=notifications')
  await expect(page.getByText('ShareOJを開いているあいだ、このブラウザに通知を出します。閉じているときは届きません。')).toBeVisible()
  await expect(page.getByText('質問と回答と結果の公開は、ベルにも残ります。')).toBeVisible()
  await expect(receive(page)).not.toBeChecked()
  await expect(kind(page, 0)).toHaveCount(0)
  await receive(page).click()
  await expect(receive(page)).toBeChecked()
  for (const index of kindLabels.keys()) await expect(kind(page, index)).toBeChecked()
  await page.reload()
  await expect(receive(page)).toBeChecked()
  for (const index of kindLabels.keys()) await expect(kind(page, index)).toBeChecked()
  await kind(page, 2).click()
  await page.reload()
  await expect(kind(page, 2)).not.toBeChecked()
  await expect(kind(page, 0)).toBeChecked()
})

test('a refusal leaves the switch off and explains', async ({ page }) => {
  await fakeNotifications(page, { permission: 'default', answer: 'denied' })
  await page.goto('/my/settings?tab=notifications')
  await receive(page).click()
  await expect(page.getByText(denied)).toBeVisible()
  await expect(receive(page)).not.toBeChecked()
  await expect(kind(page, 0)).toHaveCount(0)
})

test('a revoked permission explains instead of listing kinds', async ({ page, context, baseURL }) => {
  const value = encodeURIComponent(JSON.stringify({ enabled: true, kinds: { clarifications: true, contests: true, featured: true, editorials: true } }))
  await context.addCookies([{ name: cookieName, value, url: baseURL! }])
  await fakeNotifications(page, { permission: 'denied', answer: 'denied' })
  await page.goto('/my/settings?tab=notifications')
  await expect(page.getByText(denied)).toBeVisible()
  await expect(kind(page, 0)).toHaveCount(0)
})

test.describe('phones', () => {
  test.use({ userAgent: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/126.0 Mobile Safari/537.36' })
  test('point to the bell', async ({ page }) => {
    await fakeNotifications(page, { permission: 'default', answer: 'granted' })
    await page.goto('/my/settings?tab=notifications')
    await expect(page.getByText('このブラウザでは通知を出せません。ベルで確かめてください。')).toBeVisible()
    await expect(page.getByRole('switch')).toHaveCount(0)
  })
})

test('the notification tab fits a phone screen', async ({ page }) => {
  await fakeNotifications(page, { permission: 'default', answer: 'granted' })
  await page.setViewportSize({ width: 375, height: 800 })
  await page.goto('/my/settings?tab=notifications')
  await receive(page).click()
  await expect(kind(page, 3)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
