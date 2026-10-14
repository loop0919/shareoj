import { expect, test } from './fixtures/account'

const pid = '11111111-1111-4111-8111-111111111111'
const sid = '22222222-2222-4222-8222-222222222222'
const cid = '88888888-8888-4888-8888-888888888888'

for (const scope of ['problem', 'contest']) {
  for (const guest of [true, false]) {
    test(`${guest ? 'guest' : 'another user'} follows a shared my link to the ${scope} submission`, async ({ page }) => {
      if (guest) await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
      await page.route(`**/api/my/submissions/${sid}`, route => route.fulfill({ status: guest ? 401 : 404, json: {} }))
      await page.route(`**/api/submissions/${sid}`, route => route.fulfill({ json: { id: sid, problemId: pid, ...(scope === 'contest' ? { contestId: cid } : {}) } }))
      await page.goto(`/my/submissions/${sid}?from=problem`)
      await expect(page).toHaveURL(scope === 'contest' ? `/contests/${cid}/submissions/${sid}` : `/problems/${pid}/submissions/${sid}`)
      await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText('int main(){}')
      await expect(page.getByRole('link', { name: 'すべての提出', exact: true })).toBeVisible()
    })
  }
}

test('a restricted shared submission stays hidden and offers login to its owner', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.route(`**/api/my/submissions/${sid}`, route => route.fulfill({ status: 401, json: {} }))
  await page.route(`**/api/submissions/${sid}`, route => route.fulfill({ status: 404, json: {} }))
  await page.goto(`/my/submissions/${sid}`)
  await expect(page.getByRole('alert')).toContainText('まだ公開されていません')
  await expect(page).toHaveURL(`/my/submissions/${sid}`)
  await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveCount(0)
  await expect(page.getByRole('alert').getByRole('link', { name: 'ログイン' })).toHaveAttribute('href', `/login?next=/my/submissions/${sid}`)
})
