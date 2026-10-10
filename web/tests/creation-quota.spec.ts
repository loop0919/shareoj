import { expect, test } from './fixtures/account'

for (const kind of ['problems', 'contests', 'posts']) {
  test(`${kind} creation quota keeps the draft and shows the recovery time`, async ({ page, request }) => {
    const response = await request.put(`/api/my/${kind}/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa`, {
      headers: { Cookie: 'openoj_access=valid-access', Origin: 'https://judge.example' },
      data: { version: 0 },
    })
    expect(response.status()).toBe(429)
    expect(response.headers()['retry-after']).toBe('7200')
    const failure = await response.json()
    expect(failure.data).toEqual({ code: 'creation_quota_exceeded', retryAfter: 7200 })
    // A new contest starts as a draft, so its first save is the one that uses the quota.
    await page.route(kind === 'contests' ? '**/api/my/contests/*/draft' : `**/api/my/${kind}/*`, route => route.request().method() === 'PUT'
      ? route.fulfill({ status: 429, json: failure })
      : route.fallback())
    await page.goto(kind === 'posts' ? '/blog/new' : kind === 'contests' ? '/my/contests/new' : '/problems/new')
    const title = kind === 'problems' ? page.locator('#problem-title') : page.getByLabel(kind === 'contests' ? 'コンテストタイトル' : 'タイトル', { exact: true })
    await title.fill('保存しておきたいタイトル')
    const save = page.getByRole('button', { name: kind === 'contests' ? '下書きを保存' : '保存', exact: true })
    await save.click()
    await expect(page.getByRole('alert')).toContainText('約120分後に1件分回復します。')
    await expect(title).toHaveValue('保存しておきたいタイトル')
    await expect(save).toBeEnabled()
  })
}
