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
    await page.route(`**/api/my/${kind}/*`, route => route.request().method() === 'PUT'
      ? route.fulfill({ status: 429, json: failure })
      : route.fallback())
    if (kind === 'contests') {
      await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [{
        id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', title: 'コンテスト用の問題', updatedAt: new Date().toISOString(),
      }], nextCursor: '' } }))
    }
    await page.goto(kind === 'posts' ? '/blog/new' : kind === 'contests' ? '/my/contests/new' : '/problems/new')
    const title = kind === 'problems' ? page.locator('#problem-title') : page.getByLabel(kind === 'contests' ? 'コンテストタイトル' : 'タイトル', { exact: true })
    await title.fill('保存しておきたいタイトル')
    if (kind === 'contests') {
      await page.getByRole('button', { name: 'コンテスト設定', exact: true }).click()
      await page.getByLabel('開始日時').fill('2099-10-01T12:00')
      await page.getByLabel('終了日時').fill('2099-10-01T14:00')
      await page.getByRole('button', { name: '問題・配点', exact: true }).click()
      await page.getByRole('button', { name: 'コンテスト用の問題を追加', exact: true }).click()
    }
    await page.getByRole('button', { name: kind === 'contests' ? 'コンテストを作成' : '保存', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('約120分後に1件分回復します。')
    await expect(title).toHaveValue('保存しておきたいタイトル')
    await expect(page.getByRole('button', { name: kind === 'contests' ? 'コンテストを作成' : '保存', exact: true })).toBeEnabled()
  })
}
