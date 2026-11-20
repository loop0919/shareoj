import { expect, test } from './fixtures/account'

for (const modifier of ['Control', 'Meta']) {
  test(`${modifier}+S saves immediately and prevents browser save`, async ({ page }) => {
    await page.goto('/problems/new?fresh=1')
    await expect(page.locator('#problem-source')).toBeEnabled()
    await page.locator('#problem-title').fill(`${modifier} で保存`)
    await page.locator('#problem-source').fill('## ショートカットで保存')
    await page.evaluate(() => {
      window.addEventListener('keydown', event => {
        if (event.key.toLowerCase() === 's') document.body.dataset.savePrevented = String(event.defaultPrevented)
      })
    })
    const savedResponse = page.waitForResponse(response => response.request().method() === 'PUT' && response.url().includes('/api/my/problems/'))
    await page.keyboard.press(`${modifier}+s`)
    expect((await savedResponse).ok()).toBe(true)
    await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
    await expect(page.locator('body')).toHaveAttribute('data-save-prevented', 'true')
    const stored = await page.evaluate(prefix => {
      const key = Object.keys(sessionStorage).find(key => key.startsWith(prefix))!
      return JSON.parse(sessionStorage.getItem(key)!).draft
    }, 'openoj.problem-cache.v1.')
    expect(stored.markdown).toBe('## ショートカットで保存')
    await expect(page).toHaveURL(/problem=/)
    await page.getByRole('link', { name: 'ShareOJ マイページ' }).click()
    await expect(page).toHaveURL('/my')
    const prevented = await page.evaluate(() => {
      const event = new KeyboardEvent('keydown', { key: 's', ctrlKey: true, cancelable: true, bubbles: true })
      window.dispatchEvent(event)
      return event.defaultPrevented
    })
    expect(prevented).toBe(false)
  })
}
