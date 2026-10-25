import { expect, test } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

const id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const base = 'http://127.0.0.1:13000'

test('the author reads a featured editorial and all submissions before they unlock', async ({ page, context }) => {
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: base }])
  await page.goto(`/problems/${id}?view=editorial`)
  await expect(page.getByText(/作成者とテスターは公開前から閲覧できます/)).toBeVisible()
  await expect(page.getByText('定期便の解説本文')).toBeVisible()
  await expect(page.getByText('解説は公開待ちです。')).toHaveCount(0)
  await page.getByRole('link', { name: 'すべての提出', exact: true }).click()
  await expect(page.locator('.problem-submissions')).toContainText('carol')
  await expect(page.getByText(/他者の提出は解禁後に閲覧できます/)).toHaveCount(0)
})

test('the writer and testers reveal announced problems with the eye button, hidden again on each visit', async ({ page, context }) => {
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: base }])
  const asked: string[] = []
  page.on('request', request => { if (request.url().includes('/api/my/featured/next')) asked.push(request.url()) })
  await page.goto('/featured')
  const [easy, hard] = [page.locator('.delivery-problem').nth(0), page.locator('.delivery-problem').nth(1)]
  const reveal = page.getByRole('button', { name: '問題名を表示' })
  await expect(reveal).toHaveCount(2)
  await expect(page.locator('.delivery-problem-title')).toHaveText(['???', '???'])
  // Names stay off the page until asked for, so a screenshot cannot catch them.
  expect(asked).toHaveLength(0)
  await easy.getByRole('button', { name: '問題名を表示' }).click()
  await expect(easy.getByRole('button', { name: '問題名を表示' })).toHaveAttribute('aria-pressed', 'true')
  await expect(easy.getByRole('link', { name: '予告の新作' })).toHaveAttribute('href', `/problems/${shortId('f1f1f1f1-f1f1-4f1f-8f1f-f1f1f1f1f1f1')}`)
  await expect(hard.locator('.delivery-problem-title')).toHaveText('???')
  await hard.getByRole('button', { name: '問題名を表示' }).click()
  await expect(hard.getByRole('link', { name: 'テスト中の新作' })).toHaveAttribute('href', `/problems/${shortId('f2f2f2f2-f2f2-4f2f-8f2f-f2f2f2f2f2f2')}`)
  await easy.getByRole('button', { name: '問題名を表示' }).click()
  await expect(easy.locator('.delivery-problem-title')).toHaveText('???')
  await expect(easy.getByRole('button', { name: '問題名を表示' })).toHaveAttribute('aria-pressed', 'false')
  await page.reload()
  await expect(page.locator('.delivery-problem-title')).toHaveText(['???', '???'])
})

test('without a session the featured editorial stays hidden', async ({ page }) => {
  await page.goto(`/problems/${id}?view=editorial`)
  await expect(page.getByText('解説は公開待ちです。')).toBeVisible()
  await expect(page.getByText('定期便の解説本文')).toHaveCount(0)
  await expect(page.getByText(/作成者とテスターは公開前から閲覧できます/)).toHaveCount(0)
})
