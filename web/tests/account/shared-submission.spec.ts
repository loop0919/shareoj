import { randomUUID } from 'node:crypto'
import { expect, test, type Page } from '@playwright/test'
import { shortId } from '../../shared/utils/short-id'

async function login(page: Page, name: string) {
  await page.goto('/login')
  await page.getByLabel('メールアドレス').fill(`${name}@example.test`)
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/my')
}

test('old my links resolve public submissions for guests and other users while draft runs stay private', async ({ page, browser }) => {
  // Keep the submission quota independent of the contest tests' Alice account.
  await login(page, 'shared_submission')
  const origin = 'http://127.0.0.1:13002'
  const headers = { origin }
  const pid = randomUUID()
  expect((await page.request.put(`/api/my/problems/${pid}`, { headers, data: { version: 0, draft: {
    title: '共有する提出', markdown: '整数を出力してください。', difficulty: 3, timeLimitMs: '1000', memoryLimitMb: '256', testCases: [{ input: '1', output: '2' }],
  } } })).status()).toBe(200)
  const submit = async () => {
    const response = await page.request.post('/api/my/submissions', { headers, data: { problemId: pid, runtime: 'cpp17', source: 'int main(){}' } })
    expect(response.status()).toBe(202)
    return (await response.json()).id as string
  }
  const privateSid = await submit()
  expect((await page.request.put(`/api/my/problems/${pid}/publication`, { headers, data: { version: 1, publish: true } })).status()).toBe(200)
  const sid = await submit()
  await page.goto(`/my/submissions/${sid}`)
  await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText('int main(){}')
  await expect(page).toHaveURL(`/my/submissions/${shortId(sid)}`)
  const context = await browser.newContext({ baseURL: origin })
  const reader = await context.newPage()
  for (const viewer of ['guest', 'bob']) {
    if (viewer === 'bob') await login(reader, viewer)
    await reader.goto(`/my/submissions/${sid}`)
    await expect(reader).toHaveURL(`/problems/${shortId(pid)}/submissions/${shortId(sid)}`)
    await expect(reader.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveText('int main(){}')
    await reader.goto(`/my/submissions/${privateSid}`)
    await expect(reader.getByRole('alert')).toContainText('まだ公開されていません')
    await expect(reader.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveCount(0)
    expect((await reader.request.get(`/api/submissions/${privateSid}`)).status()).toBe(404)
  }
  await context.close()
})
