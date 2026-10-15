import { expect, test } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

const problemId = '11111111-1111-4111-8111-111111111111'
for (const path of [
  `/problems/${shortId(problemId)}`, `/contests/${shortId('88888888-8888-4888-8888-888888888888')}`, `/contests/${shortId('99999999-9999-4999-8999-999999999999')}`,
  `/contests/${shortId('88888888-8888-4888-8888-888888888888')}/A`, `/blog/${shortId('77777777-7777-4777-8777-777777777777')}`,
  '/blog/markdown-guide', '/blog/generator-guide', '/blog/language-guide', '/blog/contest-rules',
]) {
  test(`Tweet shares the title, canonical URL and hashtag: ${path}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await page.setViewportSize({ width: 320, height: 900 })
    await page.goto(`${path}?ref=test#top`)
    const link = page.getByRole('link', { name: 'Xでシェア（新しいタブで開く）' })
    await expect(link).toBeVisible()
    await expect(link).toHaveText('')
    await expect(link.locator('svg')).toBeVisible()
    await expect(link).toHaveCSS('color', 'rgb(25, 36, 49)')
    await page.getByRole('button', { name: 'ダークモードに切り替え', exact: true }).click()
    await expect(link).toHaveCSS('color', 'rgb(230, 237, 243)')
    await expect(link.locator('svg')).toHaveCSS('fill', 'rgb(230, 237, 243)')
    await page.getByRole('button', { name: 'ライトモードに切り替え', exact: true }).click()
    await expect(link).toHaveCSS('color', 'rgb(25, 36, 49)')
    const row = await page.locator('.breadcrumb-row').boundingBox()
    const button = await link.boundingBox()
    expect(Math.abs(button!.x + button!.width - row!.x - row!.width)).toBeLessThan(1)
    expect(Math.abs(button!.y + button!.height / 2 - row!.y - row!.height / 2)).toBeLessThan(1)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const url = new URL((await link.getAttribute('href'))!)
    expect(url.origin + url.pathname).toBe('https://x.com/intent/tweet')
    expect(url.searchParams.get('text')).toBe(await page.getByRole('heading', { level: 1 }).textContent())
    expect(url.searchParams.get('url')).toBe(`https://judge.example${path}`)
    expect(url.searchParams.get('hashtags')).toBe('ShareOJ')
    await expect(link).toHaveAttribute('target', '_blank')
    // Intercept X so the check never sends a real request or publishes a post.
    await page.context().route('https://x.com/**', route => route.fulfill({ body: 'X compose' }))
    const popup = page.waitForEvent('popup')
    await link.click()
    await expect(await popup).toHaveURL(url.href)
  })
}

test('private and pre-start problems have no Tweet button', async ({ page, context }) => {
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }])
  for (const path of ['/problems/55555555-5555-4555-8555-555555555555', `/contests/99999999-9999-4999-8999-999999999999/problems/${problemId}`]) {
    await page.goto(path)
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Xでシェア（新しいタブで開く）' })).toHaveCount(0)
    await context.clearCookies()
  }
})
