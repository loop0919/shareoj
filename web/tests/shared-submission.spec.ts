import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const pid = '11111111-1111-4111-8111-111111111111'
const sid = '22222222-2222-4222-8222-222222222222'
const cid = '88888888-8888-4888-8888-888888888888'

for (const scope of ['problem', 'contest', 'contest-own']) {
  for (const guest of [true, false]) {
    test(`${guest ? 'guest' : 'another user'} follows a shared my link to the ${scope} submission`, async ({ page }) => {
      if (guest) await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
      await page.route(`**/api/my/submissions/${sid}`, route => route.fulfill({ status: guest ? 401 : 404, json: {} }))
      await page.route(`**/api/submissions/${sid}`, route => route.fulfill({ json: { id: sid, problemId: pid, ...(scope !== 'problem' ? { contestId: cid } : {}) } }))
      await page.goto(scope === 'contest-own' ? `/contests/${cid}/my-submissions/${sid}` : `/my/submissions/${sid}`)
      await expect(page).toHaveURL(scope !== 'problem' ? `/contests/${shortId(cid)}/submissions/${shortId(sid)}` : `/problems/${shortId(pid)}/submissions/${shortId(sid)}`)
      await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText('int main(){}')
      await expect(page.getByRole('link', { name: 'すべての提出', exact: true })).toBeVisible()
    })
  }
}

for (const path of [`/my/submissions/${shortId(sid)}`, `/contests/${shortId(cid)}/my-submissions/${shortId(sid)}`]) {
  test(`a restricted shared submission stays hidden and offers login to its owner: ${path}`, async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
    await page.route(`**/api/my/submissions/${sid}`, route => route.fulfill({ status: 401, json: {} }))
    await page.route(`**/api/submissions/${sid}`, route => route.fulfill({ status: 404, json: {} }))
    await page.goto(path)
    await expect(page.getByRole('alert')).toContainText('まだ公開されていません')
    await expect(page).toHaveURL(path)
    await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveCount(0)
    await expect(page.getByRole('alert').getByRole('link', { name: 'ログイン' })).toHaveAttribute('href', `/login?next=${path}`)
  })
}
