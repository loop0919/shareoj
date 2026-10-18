import { expect, test } from './fixtures/account'

const problemId = '11111111-1111-4111-8111-111111111111'
const postId = '77777777-7777-4777-8777-777777777777'
const tagsLoaded = (page: import('@playwright/test').Page) => page.waitForResponse(response => new URL(response.url()).pathname === '/api/tags')

// Problem tags are hidden by default; these tests turn them on as the settings do.
const showProblemTags = (page: import('@playwright/test').Page) => page.context().addCookies([{ name: 'openoj-problem-tags', value: 'show', url: 'http://127.0.0.1:13000' }])

test('problem tags link to the problems that carry them', async ({ page }, testInfo) => {
  await showProblemTags(page)
  await page.goto(`/problems/${problemId}`)
  const tags = page.locator('.problem-meta').getByRole('list', { name: 'タグ' })
  await expect(tags.getByRole('link')).toHaveText(['DP', 'グラフ'])
  await tags.getByRole('link', { name: 'DP', exact: true }).click()
  await expect(page).toHaveURL('/problems?tag=DP')
  await expect(page).toHaveTitle('タグ「DP」の問題 | ShareOJ')
  // The search box shows the tag in use, with a button beside it to clear the filter.
  const search = page.getByRole('combobox', { name: 'タグで絞り込む' })
  await expect(search).toHaveValue('DP')
  const table = page.getByRole('table')
  await expect(table.getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(table.getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'グラフ'])
  await page.screenshot({ path: testInfo.outputPath('problems-filtered.png') })
  await page.getByRole('link', { name: '絞り込みを解除' }).click()
  await expect(page).toHaveURL('/problems')
  await expect(search).toHaveValue('')
  await expect(page.getByRole('link', { name: '絞り込みを解除' })).toHaveCount(0)
})

test('problem tags stay hidden until the viewer turns them on', async ({ page }, testInfo) => {
  await page.goto(`/problems/${problemId}`)
  await expect(page.getByRole('heading', { name: 'A + B', level: 1 })).toBeVisible()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
  // The server reads the preference, so hidden tags never flash before hydration.
  expect(await (await page.request.get(`/problems/${problemId}`)).text()).not.toContain('aria-label="タグ"')
  await page.goto('/problems')
  await expect(page.getByRole('table').getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
  // Post tags are not spoilers and are always shown.
  await page.goto(`/blog/${postId}`)
  await expect(page.getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'Rust'])

  await page.goto('/my/settings?tab=display')
  const toggle = page.getByRole('switch', { name: '問題のタグを表示する' })
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await page.screenshot({ path: testInfo.outputPath('display-settings.png') })
  await page.reload()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await page.goto(`/problems/${problemId}`)
  await expect(page.locator('.problem-meta').getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'グラフ'])
  expect(await (await page.request.get(`/problems/${problemId}`)).text()).toContain('aria-label="タグ"')

  // The problem list switches the same preference.
  await page.goto('/problems')
  const listToggle = page.getByRole('switch', { name: 'タグを表示' })
  await expect(listToggle).toBeChecked()
  await listToggle.click()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
  await expect(page.getByRole('search')).toHaveCount(0)
  await page.goto('/my/settings?tab=display')
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
})

test('signed-out viewers can show problem tags from the problem list', async ({ page }, testInfo) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto(`/problems/${problemId}`)
  await expect(page.getByRole('link', { name: 'ログイン', exact: true }).first()).toBeVisible()
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)

  await page.goto('/problems')
  const toggle = page.getByRole('switch', { name: 'タグを表示' })
  await expect(toggle).not.toBeChecked()
  await expect(toggle).toHaveAccessibleDescription('解法のヒントになることがあります。')
  await page.screenshot({ path: testInfo.outputPath('problems-tags-off.png') })
  await toggle.click()
  const row = page.getByRole('row').filter({ has: page.getByRole('link', { name: 'A + B', exact: true }) })
  await expect(row.getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'グラフ'])
  await expect(page.getByRole('search', { name: '問題をタグで絞り込む' })).toBeVisible()
  await expect(page.getByText('解法のヒントになることがあります。')).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('problems-tags-on.png') })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.screenshot({ path: testInfo.outputPath('problems-tags-on-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.screenshot({ path: testInfo.outputPath('problems-tags-on-390.png') })

  // The browser keeps the choice, so the server renders the tags on the next visit.
  await page.goto(`/problems/${problemId}`)
  await expect(page.locator('.problem-meta').getByRole('list', { name: 'タグ' }).getByRole('link')).toHaveText(['DP', 'グラフ'])
  expect(await (await page.request.get(`/problems/${problemId}`)).text()).toContain('aria-label="タグ"')
})

test('contest problems never show tags', async ({ page }) => {
  await showProblemTags(page)
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

test('the tag filter reuses the spelling in use', async ({ page }, testInfo) => {
  await showProblemTags(page)
  const loaded = tagsLoaded(page)
  await page.goto('/problems')
  await loaded
  await expect(page.locator('#problem-tag-filter-options option')).toHaveCount(3)
  const search = page.getByRole('combobox', { name: 'タグで絞り込む' })
  await search.fill('ｄｐ')
  await page.getByRole('button', { name: '絞り込む', exact: true }).click()
  await expect(page).toHaveURL('/problems?tag=DP')
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.getByRole('search').screenshot({ path: testInfo.outputPath('tag-filter-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' })
  // Emptying the box and pressing Enter clears the filter too.
  await search.fill('')
  await search.press('Enter')
  await expect(page).toHaveURL('/problems')
  expect((await page.goto('/problems?tag=a,b'))?.status()).toBe(404)
})

test('viewers who hide problem tags get no problem tag search', async ({ page }) => {
  await page.goto('/problems')
  await expect(page.getByRole('table').getByRole('link', { name: 'A + B', exact: true })).toBeVisible()
  await expect(page.getByRole('search')).toHaveCount(0)
  // A link to a filtered list still shows the tag and a way to clear it.
  await page.goto('/problems?tag=DP')
  await expect(page.getByRole('combobox', { name: 'タグで絞り込む' })).toHaveValue('DP')
  await page.getByRole('link', { name: '絞り込みを解除' }).click()
  await expect(page).toHaveURL('/problems')
  await expect(page.getByRole('search')).toHaveCount(0)
  // Post tags are not spoilers, so the post list keeps its search.
  await page.goto('/blog')
  await expect(page.getByRole('search', { name: '記事をタグで絞り込む' })).toBeVisible()
})

test('post editor suggests the tags in use', async ({ page }, testInfo) => {
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
  await page.screenshot({ path: testInfo.outputPath('post-editor-five-tags.png') })
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

test('wide editors put the tags beside the title', async ({ page }, testInfo) => {
  const top = async (selector: string) => (await page.locator(selector).boundingBox())!.y
  // The problem editor shares its first row with the limits, so it needs a wider screen than the post editor.
  for (const [path, title, narrow] of [['/problems/new', '#problem-title', 1024], ['/blog/new', '#post-title', 800]] as const) {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(path)
    await expect(page.locator(title)).toBeEnabled()
    const height = async (selector: string) => (await page.locator(selector).boundingBox())!.height
    expect(Math.abs(await top('.tag-box') - await top(title))).toBeLessThan(2)
    expect(Math.abs(await height('.tag-box') - await height(title))).toBeLessThan(2)
    await page.screenshot({ path: testInfo.outputPath(`${path.split('/')[1]}-1440.png`) })
    // Narrower screens give the tags a row of their own instead of squeezing the title.
    await page.setViewportSize({ width: narrow, height: 768 })
    expect(await top('.tag-box')).toBeGreaterThan(await top(title) + 20)
  }
})
