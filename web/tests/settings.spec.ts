import { expect, test } from './fixtures/account'

const profile = { handle: 'ui_test', avatar: '', version: 1, createdAt: '2026-09-10T00:00:00Z', accounts: { x: '', atcoder: 'tourist', codeforces: '', yukicoder: '' } }
const failure = (status: number, code: string) => ({ status, json: { statusCode: status, data: { code } } })

test('settings split the profile into tabs reached from the account menu', async ({ page }) => {
  let saved: Record<string, unknown> | undefined
  await page.route('**/api/my/profile', route => {
    if (route.request().method() === 'PUT') saved = route.request().postDataJSON()
    return route.fulfill({ json: { profile: { ...profile, version: saved ? 2 : 1 } } })
  })
  await page.goto('/problems')
  await page.locator('.site-header').getByRole('button', { name: 'アカウントメニュー', exact: true }).click()
  await page.getByRole('navigation', { name: 'アカウント', exact: true }).getByRole('link', { name: '設定', exact: true }).click()
  await expect(page).toHaveURL('/my/settings')
  const tabs = page.getByRole('navigation', { name: '設定の項目', exact: true })
  await expect(tabs.getByRole('button')).toHaveText(['プロフィール', '外部サービス', '認証設定', 'プッシュ通知', 'アカウント削除'])
  await expect(tabs.getByRole('button', { name: 'プロフィール', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByLabel('ユーザーID', { exact: true })).toHaveValue('ui_test')
  await expect(page.getByLabel('AtCoder ID', { exact: true })).toHaveCount(0)
  // The reserved placeholder of deleted accounts cannot be chosen.
  await page.getByLabel('ユーザーID', { exact: true }).fill('deleted_0123456789a')
  await page.getByRole('button', { name: '変更を保存', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('このユーザーIDは使えません。別のIDを入力してください。')
  expect(saved).toBeUndefined()
  await page.getByLabel('ユーザーID', { exact: true }).fill('ui_renamed')
  await page.getByRole('button', { name: '変更を保存', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '保存しました' })).toBeVisible()
  await expect(page).toHaveURL('/my/settings')
  // Each save sends the whole profile, so the other tab's values stay.
  expect(saved).toMatchObject({ handle: 'ui_renamed', version: 1, accounts: { atcoder: 'tourist' } })
  await tabs.getByRole('button', { name: '外部サービス', exact: true }).click()
  await expect(page).toHaveURL('/my/settings?tab=services')
  await expect(page.getByLabel('ユーザーID', { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('AtCoder ID', { exact: true })).toHaveValue('tourist')
  await page.getByLabel('AtCoder ID', { exact: true }).fill('')
  await page.getByRole('button', { name: '変更を保存', exact: true }).click()
  // The first save's toast is still fading out, so wait for the second request itself.
  await expect.poll(() => saved).toMatchObject({ version: 2, accounts: { atcoder: '' } })
  await expect(page.getByRole('status').filter({ hasText: '保存しました' })).toBeVisible()
  for (const width of [320, 375, 768]) {
    await page.setViewportSize({ width, height: 800 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('password users change their password; Google users are told why they cannot', async ({ page }) => {
  let provider = 'password'
  await page.route('**/api/my/account', route => route.fulfill({ json: { email: 'ui@example.test', provider } }))
  await page.route('**/api/my/password', route => {
    const body = route.request().postDataJSON()
    if (body.current !== 'Current-password-1') return route.fulfill(failure(400, 'incorrect_password'))
    if (body.proposed.length < 12) return route.fulfill(failure(400, 'invalid_password'))
    return route.fulfill({ status: 204 })
  })
  await page.goto('/my/settings?tab=security')
  await expect(page.locator('.sign-in')).toContainText('メールアドレスとパスワード')
  await expect(page.locator('.sign-in')).toContainText('ui@example.test')
  const current = page.getByLabel('現在のパスワード')
  const proposed = page.getByLabel('新しいパスワード', { exact: true })
  const confirmation = page.getByLabel('新しいパスワード（確認）')
  const submit = page.getByRole('button', { name: 'パスワードを変更', exact: true })
  await current.fill('Current-password-1')
  await proposed.fill('New-password-123!')
  await confirmation.fill('New-password-124!')
  await submit.click()
  await expect(page.getByRole('alert')).toHaveText('確認用のパスワードが一致しません。')
  await current.fill('wrong')
  await confirmation.fill('New-password-123!')
  await submit.click()
  await expect(page.getByRole('alert')).toHaveText('現在のパスワードが正しくありません。')
  await current.fill('Current-password-1')
  await submit.click()
  await expect(page.getByRole('status').filter({ hasText: 'パスワードを変更しました' })).toBeVisible()
  await expect(current).toHaveValue('')
  provider = 'google'
  await page.reload()
  await expect(page.locator('.sign-in')).toContainText('Google アカウント')
  await expect(page.getByText(/ShareOJ のパスワードはありません/)).toBeVisible()
  await expect(page.getByLabel('現在のパスワード')).toHaveCount(0)
})

test('deletion asks for the handle and explains a contest whose results are not final', async ({ page }) => {
  let active = true
  await page.route('**/api/my/account', route => route.request().method() !== 'DELETE'
    ? route.fallback()
    : active ? route.fulfill(failure(409, 'account_has_active_contest')) : route.fulfill({ status: 204 }))
  await page.goto('/my/settings?tab=deletion')
  await expect(page.getByRole('heading', { name: 'アカウント削除', exact: true })).toBeVisible()
  const remove = page.getByRole('button', { name: 'アカウントを削除', exact: true })
  await expect(remove).toBeDisabled()
  await expect(remove).toHaveCSS('opacity', '0.55')
  await page.getByLabel(/確認のため/).fill('ui_test')
  await expect(remove).toHaveCSS('opacity', '1')
  await remove.click()
  await expect(page.getByRole('alert')).toHaveText('結果が確定していないコンテストがあるため削除できません。コンテストの終了後、順位表を凍結した場合は結果を公開してから削除してください。')
  active = false
  await remove.click()
  await expect(page).toHaveURL('/')
  await expect(page.locator('.site-header').getByRole('link', { name: 'ログイン', exact: true })).toBeVisible()
})

test('deletion passes through the Cognito logout the server returns', async ({ page }) => {
  const cognito = 'https://example.auth.ap-northeast-1.amazoncognito.com'
  let visited = ''
  await page.route('**/api/my/account', route => route.request().method() !== 'DELETE'
    ? route.fallback()
    : route.fulfill({ json: { logoutUrl: `${cognito}/logout?client_id=c` } }))
  await page.route(`${cognito}/**`, route => {
    visited = route.request().url()
    return route.fulfill({ status: 302, headers: { location: 'http://127.0.0.1:13000/' } })
  })
  await page.goto('/my/settings?tab=deletion')
  await page.getByLabel(/確認のため/).fill('ui_test')
  // The top page loads afresh after Cognito, so it must see the signed-out state.
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.getByRole('button', { name: 'アカウントを削除', exact: true }).click()
  await expect(page).toHaveURL('http://127.0.0.1:13000/')
  expect(visited).toBe(`${cognito}/logout?client_id=c`)
  await expect(page.locator('.site-header').getByRole('link', { name: 'ログイン', exact: true })).toBeVisible()
})

test('deleted authors read as a former user without a profile link', async ({ page }) => {
  await page.route('**/api/my/contests/drafts**', route => route.fulfill({ json: { items: [], hasMore: false } }))
  await page.route('**/api/my/contests?**', route => route.fulfill({ json: { items: [{ id: 'ffffffff-ffff-4fff-8fff-ffffffffffff', author: 'deleted_0123456789a', problemAuthors: [], testers: [], title: '残ったコンテスト', description: '', startsAt: '2026-09-01T00:00:00Z', endsAt: '2026-09-01T02:00:00Z', penaltyMinutes: 5, version: 1, status: 'ended', canEdit: false, participating: false, official: false, canViewSubmissions: true, problems: [] }], hasMore: false } }))
  await page.goto('/my?tab=contests')
  const row = page.getByRole('row', { name: /残ったコンテスト/ })
  await expect(row).toContainText('退会したユーザー')
  await expect(row.locator('a[href^="/users/"]')).toHaveCount(0)
})

test('profile and password refusals name their reason', async ({ page }) => {
  await page.route('**/api/my/profile', route => route.request().method() === 'PUT' ? route.fulfill(failure(400, 'invalid_avatar')) : route.fulfill({ json: { profile } }))
  await page.goto('/my/settings')
  await page.getByRole('button', { name: '変更を保存', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('アイコンの画像を保存できませんでした。別の画像を選ぶか、アイコンを削除してから保存してください。')
  await page.route('**/api/my/account', route => route.fulfill({ json: { email: 'ui@example.test', provider: 'password' } }))
  await page.route('**/api/my/password', route => route.fulfill(failure(409, 'password_unavailable')))
  await page.goto('/my/settings?tab=security')
  await page.getByLabel('現在のパスワード').fill('Current-password-1')
  await page.getByLabel('新しいパスワード', { exact: true }).fill('New-password-123!')
  await page.getByLabel('新しいパスワード（確認）').fill('New-password-123!')
  await page.getByRole('button', { name: 'パスワードを変更', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('外部アカウントでログインしているため、パスワードは変更できません。')
})

test('buttons are flat: the primary darkens on hover and a disabled button stays still', async ({ page }) => {
  // Canvas turns any color syntax, such as the color-mix() result, into comparable RGB.
  const rgb = (button: ReturnType<typeof page.getByRole>, property: 'backgroundColor' | 'borderTopColor' = 'backgroundColor') => button.evaluate((element, key) => {
    const context = document.createElement('canvas').getContext('2d')!
    context.fillStyle = getComputedStyle(element)[key]
    context.fillRect(0, 0, 1, 1)
    return [...context.getImageData(0, 0, 1, 1).data.slice(0, 3)]
  }, property)
  const lightness = (color: number[]) => color.reduce((a, b) => a + b, 0)
  await page.goto('/my/settings')
  const save = page.getByRole('button', { name: '変更を保存', exact: true })
  await expect(save).toHaveCSS('background-image', 'none')
  await expect(save).toHaveCSS('box-shadow', 'none')
  const accent = await rgb(save)
  await save.hover()
  await expect(save).toHaveCSS('text-decoration-line', 'none')
  expect(lightness(await rgb(save))).toBeLessThan(lightness(accent))
  expect(await rgb(save, 'borderTopColor')).toEqual(await rgb(save))
  await page.goto('/my/settings?tab=deletion')
  const remove = page.getByRole('button', { name: 'アカウントを削除', exact: true })
  await expect(remove).toBeDisabled()
  const paper = await rgb(remove)
  await remove.hover()
  expect(await rgb(remove)).toEqual(paper)
  await page.getByLabel(/確認のため/).fill('ui_test')
  await remove.hover()
  expect(await rgb(remove)).not.toEqual(paper)
})
