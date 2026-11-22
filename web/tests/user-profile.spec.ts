import { test, expect, type Page } from '@playwright/test'
import { shortId } from '../shared/utils/short-id'

// Records the requests for icons, and fails the single-profile route that the links used to call once per name.
async function watchIcons(page: Page, answer: (handles: string[]) => { status?: number, json: unknown }) {
  const batches: string[][] = []
  const single: string[] = []
  await page.route(/\/api\/users\/[^/?]+$/, route => { single.push(route.request().url()); return route.abort() })
  await page.route(/\/api\/users\?handles=/, route => {
    const handles = new URL(route.request().url()).searchParams.get('handles')!.split(',')
    batches.push(handles)
    return route.fulfill(answer(handles))
  })
  return { batches, single }
}

test('user links read the icons of a page in one request and keep initials for names without one', async ({ page }) => {
  const avatar = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6V6kAAAAASUVORK5CYII='
  // carol has no profile, so the answer leaves her out.
  const { batches, single } = await watchIcons(page, handles => ({ json: { items: handles.filter(handle => handle !== 'carol').map(handle => ({ handle, avatar: handle === 'alice' ? avatar : '' })) } }))
  await page.goto('/problems/11111111-1111-4111-8111-111111111111?view=submissions')
  const authors = page.getByRole('link', { name: 'alice', exact: true })
  await expect(authors).toHaveCount(2)
  for (const author of await authors.all()) {
    await expect(author.locator('img')).toHaveAttribute('src', avatar)
    await expect(author.locator('.user-avatar')).toHaveAttribute('aria-hidden', 'true')
  }
  for (const handle of ['bob', 'carol']) {
    const link = page.getByRole('link', { name: handle, exact: true })
    await expect(link).toHaveText(handle)
    await expect(link).toHaveAccessibleName(handle)
    await expect.poll(() => link.locator('[data-initial]').evaluate(element => getComputedStyle(element, '::before').content)).toBe(`"${handle[0]!.toUpperCase()}"`)
  }
  expect(batches.map(handles => handles.slice().sort())).toEqual([['alice', 'bob', 'carol']])
  // The next page shows a name it already knows without asking again.
  await page.getByRole('link', { name: '詳細', exact: true }).click()
  await expect(page.getByRole('row', { name: '提出者 alice', exact: true }).locator('img')).toHaveAttribute('src', avatar)
  expect(batches).toHaveLength(1)
  expect(single).toEqual([])
  for (const width of [320, 768]) {
    await page.setViewportSize({ width, height: 800 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('user links keep initials when the icons cannot be read, and the next page asks again', async ({ page }) => {
  const avatar = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6V6kAAAAASUVORK5CYII='
  let failing = true
  const { batches } = await watchIcons(page, handles => failing ? { status: 502, json: {} } : { json: { items: handles.map(handle => ({ handle, avatar })) } })
  await page.goto('/problems/11111111-1111-4111-8111-111111111111?view=submissions')
  await expect.poll(() => batches.length).toBe(1)
  const authors = page.getByRole('link', { name: 'alice', exact: true })
  await expect(authors).toHaveCount(2)
  await expect(authors.first().locator('[data-initial]')).toBeVisible()
  await expect(authors.first().locator('img')).toHaveCount(0)
  failing = false
  await page.getByRole('link', { name: '詳細', exact: true }).click()
  await expect(page.getByRole('row', { name: '提出者 alice', exact: true }).locator('img')).toHaveAttribute('src', avatar)
  expect(batches).toHaveLength(2)
})

test('the icons route answers many handles at once and refuses a malformed list', async ({ request }) => {
  const response = await request.get('/api/users?handles=bob,alice,missing')
  expect(response.status()).toBe(200)
  expect(response.headers()['cache-control']).toBe('no-store')
  expect(await response.json()).toEqual({ items: [{ handle: 'alice', avatar: '' }, { handle: 'bob', avatar: '' }] })
  const tooMany = Array.from({ length: 21 }, (_, index) => `user_${index}`).join(',')
  for (const query of ['', '?handles=', '?handles=BAD', '?handles=alice,,bob', '?handles=alice&handles=bob', `?handles=${tooMany}`]) {
    expect((await request.get(`/api/users${query}`)).status(), query).toBe(400)
  }
})

test('public author and tester profiles are reachable without signing in', async ({ page }) => {
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  await page.getByRole('link', { name: 'alice', exact: true }).click()
  await expect(page).toHaveURL(/\/users\/alice$/)
  await expect(page.getByRole('heading', { name: 'alice', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader')).toHaveText(['公開日', 'タイトル', '作成者', 'TL / ML', '正解者数', '難易度'])
  await expect(page.getByRole('cell', { name: '2026/9/10', exact: true })).toBeVisible()
  await expect(page.getByRole('cell', { name: '2 秒・256 MiB', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '記事', exact: true }).click()
  await expect(page.getByRole('columnheader')).toHaveText(['タイトル', '投稿者', '公開日'])
  await expect(page.getByRole('link', { name: '公開記事', exact: true })).toHaveAttribute('href', `/blog/${shortId('77777777-7777-4777-8777-777777777777')}`)
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  await page.getByRole('link', { name: 'bob', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'bob', exact: true })).toBeVisible()
  await expect(page.getByText('公開された問題はまだありません。')).toBeVisible()
  await page.getByRole('button', { name: '記事', exact: true }).click()
  await expect(page.getByText('公開された記事はまだありません。')).toBeVisible()
  for (const width of [320, 375, 414, 768]) {
    await page.setViewportSize({ width, height: 800 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await page.screenshot({ path: '/tmp/user-profile.png', fullPage: true })
  const response = await page.goto('/users/missing')
  expect(response?.status()).toBe(404)
})


test('the back button returns from a problem to the user page it was opened from', async ({ page }) => {
  await page.goto('/users/alice')
  const problem = page.getByRole('link', { name: 'A + B', exact: true })
  await expect(problem).toHaveAttribute('href', `/problems/${shortId('11111111-1111-4111-8111-111111111111')}`)
  await problem.click()
  await expect(page.getByRole('heading', { name: 'A + B', exact: true })).toBeVisible()
  await page.goBack()
  await expect(page).toHaveURL(/\/users\/alice$/)
  await expect(page.getByRole('heading', { name: 'alice', exact: true })).toBeVisible()
})

test('own username leads to the public user page like other users', async ({ page, context }) => {
  const response = await context.request.post('/api/auth/login', {
    headers: { Origin: 'https://judge.example' }, data: { username: 'user', password: 'password' },
  })
  expect(response.ok()).toBe(true)
  await page.goto('/problems/11111111-1111-4111-8111-111111111111')
  const author = page.getByRole('link', { name: 'alice', exact: true })
  await expect(author).toHaveAttribute('href', '/users/alice')
  await expect(page.getByRole('link', { name: 'bob', exact: true })).toHaveAttribute('href', '/users/bob')
  await author.click()
  await expect(page).toHaveURL(/\/users\/alice$/)
  await expect(page.getByText('ユーザーページ', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'alice', exact: true })).toBeVisible()
  // My page still offers the public page, so authors can check what others see.
  await page.goto('/my')
  await expect(page.getByRole('heading', { name: 'alice', exact: true })).toBeVisible()
  await page.getByRole('link', { name: '公開ページを見る', exact: true }).click()
  await expect(page).toHaveURL(/\/users\/alice$/)
  await expect(page.getByText('ユーザーページ', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'alice', exact: true })).toBeVisible()
})


test('yukicoder links show the fetched user name and retain the ID when unavailable', async ({ page, request }) => {
  for (const id of ['', '../123', 'abc', '1&id=2']) {
    expect((await request.get(`/api/accounts/yukicoder?id=${id}`)).status()).toBe(400)
  }
  await page.route('**/api/accounts/yukicoder?*', route => route.fulfill({ json: { name: 'ゆきユーザー' } }))
  await page.goto('/users/yuki')
  const link = page.locator('a[href="https://yukicoder.me/users/123"]')
  await expect(link).toHaveText('ゆきユーザー')
  await page.route('**/api/accounts/yukicoder?*', route => route.fulfill({ status: 503, json: {} }))
  await page.reload()
  await expect(link).toHaveText('123')
  await page.goto('/users/bob')
  await expect(page.locator('a[href^="https://yukicoder.me/users/"]')).toHaveCount(0)
})
