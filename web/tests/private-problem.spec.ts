import { test, expect } from '@playwright/test'

const id = '55555555-5555-4555-8555-555555555555'
const base = 'http://127.0.0.1:13000'

test('owner can read a private problem and editorial, then submit', async ({ page, context }) => {
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: base }])
  const response = await page.goto(`/problems/${id}`)
  expect(response?.status()).toBe(200)
  expect(response?.headers()['cache-control']).toBe('no-store')
  expect(response?.headers().vary).toContain('Cookie')
  await expect(page.getByRole('heading', { name: '非公開の練習問題' })).toBeVisible()
  await expect(page.getByText('保存済みの問題文')).toBeVisible()
  await page.getByRole('link', { name: '作問画面へ', exact: true }).click()
  await expect(page).toHaveURL(`/problems/new?problem=${id}`)
  await expect(page.locator('#problem-title')).toHaveValue('非公開の練習問題')
  await page.getByRole('link', { name: '問題を見る', exact: true }).click()
  await expect(page).toHaveURL(`/problems/${id}`)
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content', 'noindex, nofollow')
  expect(await response!.text()).not.toContain('secret-input')
  const api = await context.request.get(`/api/problems/${id}`)
  expect(api.headers()['cache-control']).toBe('no-store')
  expect(await api.text()).not.toContain('secret-output')
  await page.getByRole('link', { name: '解説', exact: true }).click()
  await expect(page.getByText('非公開の解説')).toBeVisible()
  await page.getByRole('link', { name: '問題', exact: true }).last().click()
  await page.route('**/api/my/submissions', async route => {
    expect(route.request().postDataJSON()).toEqual({ problemId: id, runtime: 'cpp17', source: 'int main(){}' })
    await route.fulfill({ status: 202, json: { id: 'private-submission' } })
  })
  await page.route('**/api/my/submissions/private-submission', route => route.fulfill({ json: {
    id: 'private-submission', problemId: id, problemTitle: '非公開の練習問題', runtime: 'cpp17', source: 'int main(){}',
    status: 'DONE', result: { verdict: 'AC', passed: 1, total: 1 }, createdAt: '2026-09-10T00:00:00Z',
  } }))
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(page).toHaveURL('/my/submissions/private-submission')
  await expect(page.getByRole('status')).toHaveText('AC：正解')
})

test('private content is unavailable without a session and unknown owner resources stay hidden', async ({ context }) => {
  expect((await context.request.get(`/api/problems/${id}`)).status()).toBe(404)
  expect((await context.request.get(`/problems/${id}`)).status()).toBe(404)
  await context.addCookies([{ name: 'openoj_access', value: 'invalid-access', url: base }])
  expect((await context.request.get(`/api/problems/${id}`)).status()).toBe(404)
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: base }])
  expect((await context.request.get('/api/problems/66666666-6666-4666-8666-666666666666')).status()).toBe(404)
})

test('private viewing can refresh an expired session', async ({ context }) => {
  await context.addCookies([{ name: 'openoj_refresh', value: 'valid-refresh', url: base }])
  const response = await context.request.get(`/api/problems/${id}`)
  expect(response.status()).toBe(200)
  expect((await response.json()).isPrivate).toBe(true)
})
