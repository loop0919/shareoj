import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

// The fixture contest is running and its problem A has three favorites. A contest problem takes favorites before it
// is published, and shows the count to everyone (ADR 0040).
const path = `/contests/${shortId('88888888-8888-4888-8888-888888888888')}/A`

test('a contest problem is favorited during the contest', async ({ page }) => {
  let favorited = false
  const sent: string[] = []
  await page.route('**/api/my/favorites/*', route => {
    if (route.request().method() === 'PUT') {
      favorited = route.request().postDataJSON().favorited
      sent.push(new URL(route.request().url()).pathname)
    }
    return route.fulfill({ json: { favorited, favoriteCount: favorited ? 4 : 3 } })
  })
  await page.goto(path)
  const button = page.locator('.problem-header').getByRole('button', { name: /お気に入り/ })
  await expect(button).toBeEnabled()
  await expect(button).toHaveAttribute('aria-pressed', 'false')
  await expect(button).toContainText('3')
  await button.click()
  await expect(button).toHaveAttribute('aria-pressed', 'true')
  await expect(button).toContainText('4')
  expect(sent).toEqual(['/api/my/favorites/11111111-1111-4111-8111-111111111111'])
  // The editorial of the same problem keeps the button.
  await page.goto(`${path}?view=editorial`)
  await expect(button).toHaveAttribute('aria-pressed', 'true')
})

test('a signed-out viewer reads the count and returns to the contest problem after login', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto(path)
  const link = page.locator('.problem-header').getByRole('link', { name: /お気に入り/ })
  await expect(link).toContainText('3')
  const href = await link.getAttribute('href')
  expect(new URL(href!, 'http://127.0.0.1').pathname).toBe('/login')
  expect(new URL(href!, 'http://127.0.0.1').searchParams.get('next')).toBe(path)
})
