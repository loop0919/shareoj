import { test, expect, type Locator, type Page } from '@playwright/test'

// The folder view stays with the account, so only folders and folders_many turn it on. The tests build on each other:
// folders keeps the problem, post and folders made here, and later tests start from the view left on at the end.
test.describe.configure({ mode: 'serial' })

const origin = 'http://127.0.0.1:13002'

async function login(page: Page, name: string) {
  await page.getByLabel('メールアドレス', { exact: true }).fill(`${name}@example.test`)
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
}

async function signIn(page: Page, name = 'folders') {
  await page.goto('/login')
  await login(page, name)
  await expect(page).toHaveURL('/my')
}

const tabs = (page: Page) => page.getByRole('navigation', { name: 'マイページのコンテンツ' }).getByRole('button')
const viewSwitch = (page: Page) => page.getByRole('switch', { name: 'フォルダ表示（ベータ）' })
const library = (page: Page) => page.getByRole('table', { name: '問題と記事', exact: true })
const rows = (page: Page) => library(page).locator('tbody tr')
const row = (page: Page, title: string) => rows(page).filter({ hasText: title })
// A folder's button reads its name and then its count.
const folder = (page: Page, name: string) => page.getByRole('navigation', { name: 'フォルダ', exact: true }).getByRole('button', { name: new RegExp(`^${name} \\d+$`) })
const filter = (group: Locator, label: string) => group.getByRole('button', { name: new RegExp(`^${label}(?: \\d+)?$`) })
const kinds = (page: Page) => page.getByRole('group', { name: '種類で絞り込む' })
const statuses = (page: Page) => page.getByRole('group', { name: '状態で絞り込む' })

async function toggleView(page: Page) {
  await Promise.all([page.waitForResponse(response => response.url().endsWith('/api/my/folder-view') && response.ok()), viewSwitch(page).click()])
}

async function createFolder(page: Page, name: string, parentId: string | null) {
  const response = await page.request.post('/api/my/folders', { headers: { origin }, data: { name, parentId } })
  expect(response.status()).toBe(201)
  return (await response.json()).id as string
}

async function problemId(page: Page, title: string) {
  const list = await (await page.request.get('/api/my/problems')).json() as { items: { id: string, title: string }[] }
  return list.items.find(item => item.title === title)!.id
}

test('while the folder view is off, the my page has its tabs and asks for no folders', async ({ page }) => {
  const folderRequests: string[] = []
  page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/my/folder')) folderRequests.push(request.url()) })
  await signIn(page)
  await expect(tabs(page)).toHaveText(['自分の問題', 'テスト中の問題', '記事', 'コンテスト', '通知'])
  await expect(page.getByRole('button', { name: '問題と記事' })).toHaveCount(0)
  await expect(viewSwitch(page)).not.toBeChecked()
  await expect(page.getByRole('link', { name: /意見を送る/ })).toHaveAttribute('href', 'https://github.com/loop0919/shareoj/issues/new?template=feature_request.yml')
  await expect(page.getByText('保存した問題はまだありません。')).toBeVisible()
  await page.getByRole('button', { name: '記事', exact: true }).click()
  await expect(page.getByText('保存した記事はまだありません。')).toBeVisible()
  expect(folderRequests).toEqual([])
})

test('turning the folder view on lists problems and posts in one table', async ({ page }) => {
  await signIn(page)
  await page.goto('/problems/new?fresh=1')
  await page.locator('#problem-title').fill('フォルダの問題A')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.draft-status').getByRole('status')).toHaveText('保存済み')
  await page.goto('/blog/new')
  await page.getByLabel('タイトル', { exact: true }).fill('フォルダの記事A')
  await page.getByLabel('本文（Markdown）').fill('フォルダに入れる記事')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/post=/)
  await page.goto('/my')
  await toggleView(page)
  await expect(viewSwitch(page)).toBeChecked()
  await expect(tabs(page)).toHaveText(['問題と記事', 'コンテスト', '通知'])
  await expect(rows(page)).toHaveCount(2)
  await expect(row(page, 'フォルダの問題A').getByRole('cell').first()).toHaveText('自分の問題')
  await expect(row(page, 'フォルダの記事A').getByRole('cell').first()).toHaveText('記事')
  await expect(page.getByText('フォルダを作ると、問題と記事を出す先ごとに分けられます。')).toBeVisible()
  await expect(page.getByRole('main').getByRole('link', { name: '新規問題', exact: true })).toHaveAttribute('href', '/problems/new?fresh=1')
  await expect(page.getByRole('main').getByRole('link', { name: '新規記事', exact: true })).toHaveAttribute('href', '/blog/new')
  await page.reload()
  await expect(viewSwitch(page)).toBeChecked()
  await expect(tabs(page)).toHaveText(['問題と記事', 'コンテスト', '通知'])
  await expect(rows(page)).toHaveCount(2)
})

test('folders count what they hold, and a folder opens into the folders below it', async ({ page }, testInfo) => {
  await signIn(page)
  const problem = await problemId(page, 'フォルダの問題A')
  const contest = await createFolder(page, 'コンテスト用', null)
  const third = await createFolder(page, '第3回', contest)
  expect((await page.request.put('/api/my/folder-items', { headers: { origin }, data: { folderId: third, items: [{ kind: 'problem', id: problem }] } })).status()).toBe(204)
  await page.reload()
  await expect(folder(page, 'コンテスト用')).toHaveAccessibleName('コンテスト用 1')
  await expect(page.getByText('フォルダを作ると、問題と記事を出す先ごとに分けられます。')).toHaveCount(0)
  await expect(folder(page, 'すべて')).toHaveAccessibleName('すべて 2')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 1')
  await folder(page, '未整理').click()
  await expect(page).toHaveURL(/folder=unsorted/)
  await expect(folder(page, '未整理')).toHaveAttribute('aria-current', 'true')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの記事A')
  await folder(page, 'コンテスト用').click()
  await expect(page).toHaveURL(new RegExp(`folder=${contest}`))
  await expect(rows(page)).toHaveCount(1)
  await library(page).getByRole('button', { name: '第3回', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`folder=${third}`))
  const location = page.getByRole('navigation', { name: '今いる場所' })
  await expect(location).toHaveText('コンテスト用 / 第3回')
  await expect(folder(page, '第3回')).toHaveAttribute('aria-current', 'true')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの問題A')
  await page.screenshot({ path: testInfo.outputPath('folder-view.png'), fullPage: true })
  await location.getByRole('button', { name: 'コンテスト用', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`folder=${contest}`))
  await expect(location).toHaveText('コンテスト用')
})

test('kind and status filters combine', async ({ page }) => {
  await signIn(page)
  await expect(folder(page, 'すべて')).toHaveAttribute('aria-current', 'true')
  await expect(rows(page)).toHaveCount(2)
  await expect(filter(kinds(page), '記事')).toHaveAccessibleName('記事 1')
  await filter(kinds(page), '記事').click()
  await expect(page).toHaveURL(/kind=post/)
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの記事A')
  // A draft post is 準備中, like an unfinished problem.
  await filter(statuses(page), '準備中').click()
  await expect(page).toHaveURL(/status=preparing/)
  await expect(page).toHaveURL(/kind=post/)
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの記事A')
  await expect(filter(kinds(page), 'すべて')).toHaveAccessibleName('すべて 2')
})

test('the old tabs open the library narrowed to their kind', async ({ page }) => {
  await signIn(page)
  await page.goto('/my?tab=posts')
  await expect(page.getByRole('button', { name: '問題と記事', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(filter(kinds(page), '記事')).toHaveAttribute('aria-pressed', 'true')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの記事A')
})

test('a folder that is gone shows everything', async ({ page }) => {
  await signIn(page)
  // Without the folders nothing could be counted, so the list waits for a retry.
  await page.route('**/api/my/folders', route => route.fulfill({ status: 503 }))
  await page.goto('/my?tab=library&folder=00000000-0000-4000-8000-000000000000')
  await expect(page.getByRole('alert')).toHaveText('問題と記事を読み込めませんでした。もう一度お試しください。')
  await expect(library(page)).toHaveCount(0)
  await page.unroute('**/api/my/folders')
  await page.getByRole('button', { name: '再試行', exact: true }).click()
  await expect(folder(page, 'すべて')).toHaveAttribute('aria-current', 'true')
  await expect(rows(page)).toHaveCount(2)
  await expect(row(page, 'フォルダの問題A')).toHaveCount(1)
  await expect(row(page, 'フォルダの記事A')).toHaveCount(1)
})

test('a tested problem shows its author', async ({ page, browser }) => {
  // alice stays off the folder view; she only writes the problem and the invitation.
  const context = await browser.newContext({ baseURL: origin })
  try {
    const author = await context.newPage()
    await signIn(author, 'alice')
    await author.goto('/problems/new?fresh=1')
    await author.locator('#problem-title').fill('フォルダで受けたテストの問題')
    await author.getByRole('button', { name: '問題管理', exact: true }).click()
    await author.getByRole('button', { name: 'リンクを発行', exact: true }).click()
    const link = await author.getByLabel('招待リンク', { exact: true }).inputValue()
    await page.goto(link)
    await expect(page).toHaveURL(/\/login\?next=/)
    await login(page, 'folders')
    await expect(page).toHaveURL(link)
    await page.getByRole('button', { name: '許可する', exact: true }).click()
    await expect(page).toHaveURL('/my?tab=testing')
    await expect(page.getByRole('button', { name: '問題と記事', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(filter(kinds(page), 'テスト中')).toHaveAttribute('aria-pressed', 'true')
    const tested = row(page, 'フォルダで受けたテストの問題')
    await expect(tested.getByRole('cell').first()).toHaveText('テスト中')
    await expect(tested).toContainText('作者: alice')
    await expect(rows(page)).toHaveCount(1)
    // folders joining reached alice; reading it leaves her bell empty for the specs that follow.
    const unread = await author.request.post('/api/my/notifications/read', { headers: { origin } })
    expect((await unread.json()).notifications).toEqual([expect.objectContaining({ actor: 'folders', kind: 'tester', title: 'フォルダで受けたテストの問題' })])
  } finally { await context.close() }
})

test('counts read every page of a long list', async ({ browser }) => {
  const context = await browser.newContext({ baseURL: origin })
  try {
    const page = await context.newPage()
    await signIn(page, 'folders_many')
    await toggleView(page)
    await expect(folder(page, 'すべて')).toHaveAccessibleName('すべて 51')
    await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 51')
    await expect(filter(statuses(page), '準備中')).toHaveAccessibleName('準備中 51')
    // The oldest problem is past the first page of 50.
    await expect(library(page).getByText('大量の問題 01', { exact: true })).toBeVisible()
  } finally { await context.close() }
})

test('turning the folder view off brings the old tabs back, and the folders stay', async ({ page }) => {
  await signIn(page)
  // A switch that could not be saved goes back, and the page says so.
  await page.route('**/api/my/folder-view', route => route.fulfill({ status: 503 }))
  await viewSwitch(page).click()
  await expect(page.getByRole('alert')).toContainText('フォルダ表示の切り替えを保存できませんでした。')
  await expect(viewSwitch(page)).toBeChecked()
  await expect(tabs(page)).toHaveText(['問題と記事', 'コンテスト', '通知'])
  await page.unroute('**/api/my/folder-view')
  await toggleView(page)
  await expect(viewSwitch(page)).not.toBeChecked()
  await expect(tabs(page)).toHaveText(['自分の問題', 'テスト中の問題', '記事', 'コンテスト', '通知'])
  await expect(page.getByRole('table', { name: '自分の問題', exact: true }).getByText('フォルダの問題A')).toBeVisible()
  await page.goto('/my?tab=library')
  await expect(page.getByRole('button', { name: '自分の問題', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('table', { name: '自分の問題', exact: true }).getByText('フォルダの問題A')).toBeVisible()
  await toggleView(page)
  await expect(tabs(page)).toHaveText(['問題と記事', 'コンテスト', '通知'])
  await folder(page, 'コンテスト用').click()
  await folder(page, '第3回').click()
  await expect(folder(page, '第3回')).toHaveAccessibleName('第3回 1')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの問題A')
})
