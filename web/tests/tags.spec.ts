import { expect, test } from './fixtures/account'

const problemId = '11111111-1111-4111-8111-111111111111'
const postId = '77777777-7777-4777-8777-777777777777'
const tagsLoaded = (page: import('@playwright/test').Page) => page.waitForResponse(response => new URL(response.url()).pathname === '/api/tags')

test('problem tags link to the problems that carry them', async ({ page }) => {
  await page.goto(`/problems/${problemId}`)
  const tags = page.locator('.problem-meta').getByRole('list', { name: 'タグ' })
  await expect(tags.getByRole('link')).toHaveText(['DP', 'グラフ'])
  await tags.getByRole('link', { name: 'DP', exact: true }).click()
  await expect(page).toHaveURL('/problems?tag=DP')
  await expect(page).toHaveTitle('タグ「DP」の問題 | ShareOJ')
  await expect(page.getByText('タグ「DP」で絞り込んでいます。')).toBeVisible()
  const table = page.getByRole('table')
  await expect(table.getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(table.getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'グラフ'])
  await page.getByRole('link', { name: '絞り込みを解除' }).click()
  await expect(page).toHaveURL('/problems')
  await expect(page.getByText('で絞り込んでいます。')).toHaveCount(0)
})

test('the tag setting hides problem tags from the first render', async ({ page }) => {
  await page.goto('/my/settings?tab=display')
  const toggle = page.getByRole('switch', { name: '問題のタグを表示する' })
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await page.reload()
  await expect(toggle).toHaveAttribute('aria-checked', 'false')

  await page.goto(`/problems/${problemId}`)
  await expect(page.getByRole('heading', { name: 'A + B', level: 1 })).toBeVisible()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
  // The server reads the preference, so the tags never flash before hydration.
  expect(await (await page.request.get(`/problems/${problemId}`)).text()).not.toContain('aria-label="タグ"')
  await page.goto('/problems')
  await expect(page.getByRole('table').getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
  // Post tags are not spoilers and stay visible.
  await page.goto(`/blog/${postId}`)
  await expect(page.getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'Rust'])
})

test('contest problems never show tags', async ({ page }) => {
  await page.goto(`/contests/88888888-8888-4888-8888-888888888888/problems/${problemId}`)
  await expect(page.getByRole('heading', { name: 'A + B', level: 1 })).toBeVisible()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
})

test('post tags link to the posts that carry them', async ({ page }) => {
  await page.goto(`/blog/${postId}`)
  await page.getByRole('list', { name: 'タグ' }).getByRole('link', { name: 'Rust', exact: true }).click()
  await expect(page).toHaveURL('/blog?tag=Rust')
  await expect(page).toHaveTitle('タグ「Rust」の記事 | ShareOJ')
  const row = page.getByRole('row').filter({ hasText: '公開記事' })
  await expect(row.getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'Rust'])
  await page.goto('/blog?tag=Go')
  await expect(page.getByText('「Go」のタグが付いた記事はまだありません。')).toBeVisible()
})

test('the tag filter reuses the spelling in use', async ({ page }) => {
  const loaded = tagsLoaded(page)
  await page.goto('/problems')
  await loaded
  await expect(page.locator('#problem-tag-filter-options option')).toHaveCount(3)
  await page.getByRole('combobox', { name: 'タグで絞り込む' }).fill('ｄｐ')
  await page.getByRole('button', { name: '絞り込む', exact: true }).click()
  await expect(page).toHaveURL('/problems?tag=DP')
  expect((await page.goto('/problems?tag=a,b'))?.status()).toBe(404)
})

test('post editor suggests the tags in use', async ({ page }) => {
  const saved: { tags?: string[] }[] = []
  await page.route('**/api/my/posts/*', route => {
    const request = route.request()
    if (request.method() !== 'PUT') return route.fulfill({ status: 404, json: {} })
    const body = request.postDataJSON()
    saved.push(body)
    return route.fulfill({ json: {
      id: new URL(request.url()).pathname.split('/').pop(), title: body.title, markdown: body.markdown, version: saved.length,
      publishedVersion: 0, publishedAt: null, updatedAt: '2026-10-11T00:00:00Z', isOperator: false, problems: [], tags: body.tags,
    } })
  })
  const loaded = tagsLoaded(page)
  await page.goto('/blog/new')
  await loaded
  const input = page.getByRole('combobox', { name: /^タグ/ })
  const options = page.getByRole('listbox', { name: 'タグの候補' }).getByRole('option')
  const chips = page.getByRole('list', { name: 'タグ' }).getByRole('listitem')

  // The same tag in another case is picked by Enter, with the spelling in use.
  await input.fill('dp')
  await expect(options).toHaveText([/^DP/])
  await expect(options.first()).toHaveAttribute('aria-selected', 'true')
  await input.press('Enter')
  await expect(chips).toHaveText(['DP'])

  // Half-width kana find a similar tag; creating a new spelling is the last choice.
  await input.fill('ｾｸﾞﾒﾝﾄ')
  await expect(options).toHaveText([/^セグメント木問題 2$/, /^セグメント新しいタグ$/])
  await expect(options.first()).toHaveAttribute('aria-selected', 'false')
  await input.press('ArrowDown')
  await expect(options.first()).toHaveAttribute('aria-selected', 'true')
  await input.press('Enter')
  await expect(chips).toHaveText(['DP', 'セグメント木'])

  await input.fill('dp')
  await expect(options).toHaveCount(0)
  await input.press('Enter')
  await expect(page.getByRole('alert')).toHaveText('「DP」はすでに追加されています。')

  // Commas separate pasted tags; the text after the last comma stays in the field.
  await input.fill('rust, 新しい　 タグ,グ')
  await expect(chips).toHaveText(['DP', 'セグメント木', 'Rust', '新しい タグ'])
  await expect(input).toHaveValue('グ')
  await input.fill('')
  await page.getByRole('list', { name: 'タグ' }).getByRole('button', { name: '新しい タグを外す' }).click()

  // A post or problem has five tags at most.
  await input.fill('x,y,z,')
  await expect(chips).toHaveText(['DP', 'セグメント木', 'Rust', 'x', 'y'])
  await expect(page.getByRole('alert')).toHaveText('タグは5個までです。')
  for (const tag of ['x', 'y']) await page.getByRole('list', { name: 'タグ' }).getByRole('button', { name: `${tag}を外す`, exact: true }).click()

  await page.getByLabel('タイトル', { exact: true }).fill('タグ付きの記事')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('保存済み')
  expect(saved.at(-1)?.tags).toEqual(['DP', 'セグメント木', 'Rust'])
})

test('problem editor saves tags with the draft', async ({ page }) => {
  const loaded = tagsLoaded(page)
  await page.goto('/problems/new')
  await loaded
  const input = page.getByRole('combobox', { name: /^タグ/ })
  const saved = page.waitForRequest(request => request.method() === 'PUT' && request.url().includes('/api/my/problems/'))
  await input.fill('ぐらふ')
  await page.getByRole('listbox', { name: 'タグの候補' }).getByRole('option', { name: /^グラフ/ }).click()
  await expect(page.getByRole('list', { name: 'タグ' }).getByRole('listitem')).toHaveText(['グラフ'])
  expect((await saved).postDataJSON().draft.tags).toEqual(['グラフ'])
})
