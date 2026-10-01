import { expect, test } from '@playwright/test'

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

test('without a session the featured editorial stays hidden', async ({ page }) => {
  await page.goto(`/problems/${id}?view=editorial`)
  await expect(page.getByText('解説は公開待ちです。')).toBeVisible()
  await expect(page.getByText('定期便の解説本文')).toHaveCount(0)
  await expect(page.getByText(/作成者とテスターは公開前から閲覧できます/)).toHaveCount(0)
})
