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
// A row starts with its checkbox, so the kind is the second cell.
const kindCell = (row: Locator) => row.getByRole('cell').nth(1)
const tree = (page: Page) => page.getByRole('navigation', { name: 'フォルダ', exact: true })
// A folder's button reads its name and then its count.
const folder = (page: Page, name: string) => tree(page).getByRole('button', { name: new RegExp(`^${name} \\d+$`) })
// Below 40rem the folder list folds away under this button, which reads 「フォルダ: 」 and the open folder's name.
const choosing = (page: Page) => page.getByRole('button', { name: /^フォルダ: / })
const dialog = (page: Page, name: string) => page.getByRole('dialog', { name, exact: true })
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
  await expect(kindCell(row(page, 'フォルダの問題A'))).toHaveText('自分の問題')
  await expect(kindCell(row(page, 'フォルダの記事A'))).toHaveText('記事')
  await expect(page.getByText('「フォルダを作る」でフォルダを作り、行をドラッグするか「フォルダへ移動」で問題や記事を入れられます。', { exact: true })).toBeVisible()
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
  await expect(page.getByText('「フォルダを作る」でフォルダを作り、行をドラッグするか「フォルダへ移動」で問題や記事を入れられます。', { exact: true })).toHaveCount(0)
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
    await expect(kindCell(tested)).toHaveText('テスト中')
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
  await expect(page.getByRole('alert')).toHaveText('保存できませんでした。もう一度お試しください。何も変わっていません。')
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
  // The switch keeps the focus while its change is saved, and ignores presses until the save ends.
  let release!: () => void
  const held = new Promise<void>(resolve => { release = resolve })
  let saves = 0
  await page.route('**/api/my/folder-view', async route => { saves++; await held; await route.continue() })
  await viewSwitch(page).focus()
  await page.keyboard.press('Space')
  await expect(viewSwitch(page)).toBeChecked()
  await expect(viewSwitch(page)).toBeFocused()
  await expect(viewSwitch(page)).toHaveAttribute('aria-busy', 'true')
  await page.keyboard.press('Space')
  await expect(viewSwitch(page)).toBeChecked()
  const saved = page.waitForResponse(response => response.url().endsWith('/api/my/folder-view') && response.ok())
  release()
  await saved
  await expect(tabs(page)).toHaveText(['問題と記事', 'コンテスト', '通知'])
  await expect(viewSwitch(page)).not.toHaveAttribute('aria-busy')
  await expect(viewSwitch(page)).toBeChecked()
  await expect(viewSwitch(page)).toBeFocused()
  expect(saves).toBe(1)
  await page.unroute('**/api/my/folder-view')
  await folder(page, 'コンテスト用').click()
  await folder(page, '第3回').click()
  await expect(folder(page, '第3回')).toHaveAccessibleName('第3回 1')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの問題A')
})

// The tests below manage folders on the page. They start from what the tests above leave: the view on, 第3回 inside
// コンテスト用 holding フォルダの問題A, and the post and the tested problem in 未整理.
const createButton = (page: Page) => page.getByRole('button', { name: 'フォルダを作る', exact: true })
const moveButton = (page: Page, title: string) => row(page, title).getByRole('button', { name: `${title}をフォルダへ移動`, exact: true })
const chosenMove = (page: Page) => page.getByRole('button', { name: /^選んだ\d+件をフォルダへ移動$/ })

async function makeFolder(page: Page, name: string) {
  await createButton(page).click()
  const naming = dialog(page, 'フォルダを作る')
  await naming.getByLabel('フォルダの名前', { exact: true }).fill(name)
  await naming.getByRole('button', { name: '作る', exact: true }).click()
  await expect(naming).toHaveCount(0)
}

async function folderAction(page: Page, name: string, action: '名前を変更' | '移動' | '削除') {
  await tree(page).getByRole('button', { name: `${name}の操作`, exact: true }).click()
  await tree(page).getByRole('button', { name: action, exact: true }).click()
}

test('a folder is made from the folder list and renamed from its row', async ({ page }) => {
  await signIn(page)
  await expect(folder(page, 'すべて')).toHaveAttribute('aria-current', 'true')
  await createButton(page).click()
  const naming = dialog(page, 'フォルダを作る')
  await expect(naming).toHaveAccessibleDescription('最上位に作ります。')
  const name = naming.getByLabel('フォルダの名前', { exact: true })
  await expect(name).toBeFocused()
  await expect(name).toHaveAttribute('maxlength', '40')
  const make = naming.getByRole('button', { name: '作る', exact: true })
  await expect(make).toBeDisabled()
  await name.fill('   ')
  await expect(make).toBeDisabled()
  await name.fill('定期便用')
  await make.click()
  await expect(naming).toHaveCount(0)
  await expect(folder(page, '定期便用')).toHaveAccessibleName('定期便用 0')
  await expect(createButton(page)).toBeFocused()
  // The same name in the same place is refused inside the dialog, which stays open with the name for another try.
  await createButton(page).click()
  await name.fill('定期便用')
  // While the save runs, the dialog cannot be cancelled.
  let release!: () => void
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/my/folders', async route => { await held; await route.continue() })
  await make.click()
  await expect(naming.getByRole('button', { name: '保存中…', exact: true })).toBeDisabled()
  await expect(naming.getByRole('button', { name: 'キャンセル', exact: true })).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(naming).toBeVisible()
  release()
  await expect(naming.getByRole('alert')).toHaveText('同じ場所に同じ名前のフォルダがあります。別の名前にしてください。何も変わっていません。')
  await expect(page.getByRole('alert')).toHaveCount(1)
  await expect(name).toHaveValue('定期便用')
  await expect(name).toBeFocused()
  await expect(folder(page, '定期便用')).toHaveCount(1)
  await page.unroute('**/api/my/folders')
  await name.fill('メモ')
  await make.click()
  await expect(naming).toHaveCount(0)
  // The refusal was the dialog's, so nothing is left above the table.
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 0')
  await expect(folder(page, '定期便用')).toHaveCount(1)
  await folderAction(page, '定期便用', '名前を変更')
  const renaming = dialog(page, 'フォルダの名前を変更')
  await expect(renaming.getByLabel('フォルダの名前', { exact: true })).toHaveValue('定期便用')
  await renaming.getByLabel('フォルダの名前', { exact: true }).fill('定期便')
  await renaming.getByRole('button', { name: '変更する', exact: true }).click()
  await expect(renaming).toHaveCount(0)
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 0')
  await expect(folder(page, '定期便用')).toHaveCount(0)
  await expect(tree(page).getByRole('button', { name: '定期便の操作', exact: true })).toBeFocused()
  await page.reload()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 0')
  await expect(folder(page, '定期便用')).toHaveCount(0)
})

test('a row goes into a folder with the keyboard alone', async ({ page }) => {
  await signIn(page)
  await page.goto('/my?tab=library&folder=unsorted')
  await moveButton(page, 'フォルダの記事A').focus()
  await page.keyboard.press('Enter')
  const picker = dialog(page, '「フォルダの記事A」をフォルダへ移動')
  // The choice starts where the row is now; the folders follow in the list's order.
  const unsorted = picker.getByRole('radio', { name: '未整理', exact: true })
  await expect(unsorted).toBeChecked()
  await expect(unsorted).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(picker.getByRole('radio', { name: 'コンテスト用', exact: true })).toBeChecked()
  await page.keyboard.press('ArrowDown')
  await expect(picker.getByRole('radio', { name: 'コンテスト用 / 第3回', exact: true })).toBeChecked()
  await page.keyboard.press('ArrowDown')
  await expect(picker.getByRole('radio', { name: 'メモ', exact: true })).toBeChecked()
  await page.keyboard.press('ArrowDown')
  await expect(picker.getByRole('radio', { name: '定期便', exact: true })).toBeChecked()
  await page.keyboard.press('Enter')
  await expect(picker).toHaveCount(0)
  await expect(row(page, 'フォルダの記事A')).toHaveCount(0)
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 1')
  // The row that took its place has the focus, so the next move needs no mouse either.
  await expect(moveButton(page, 'フォルダで受けたテストの問題')).toBeFocused()
  await page.reload()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 1')
  await expect(row(page, 'フォルダの記事A')).toHaveCount(0)
  await folder(page, '定期便').focus()
  await page.keyboard.press('Enter')
  await expect(folder(page, '定期便')).toHaveAttribute('aria-current', 'true')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの記事A')
})

test('chosen rows move together', async ({ page }) => {
  await signIn(page)
  const chooseProblem = row(page, 'フォルダの問題A').getByRole('checkbox', { name: 'フォルダの問題Aを選択', exact: true })
  await chooseProblem.check()
  await expect(chosenMove(page)).toHaveText('選んだ1件をフォルダへ移動')
  // A row the filter hides is no longer chosen.
  await filter(kinds(page), '記事').click()
  await expect(chosenMove(page)).toHaveCount(0)
  await filter(kinds(page), 'すべて').click()
  await expect(chooseProblem).not.toBeChecked()
  // Choosing all takes every row shown, and 選択を解除 lets them go.
  await library(page).getByRole('checkbox', { name: '表示している問題と記事をすべて選択', exact: true }).check()
  await expect(chosenMove(page)).toHaveText('選んだ3件をフォルダへ移動')
  await page.getByRole('button', { name: '選択を解除', exact: true }).click()
  await expect(library(page).getByRole('checkbox', { checked: true })).toHaveCount(0)
  await expect(chosenMove(page)).toHaveCount(0)
  const choosePost = row(page, 'フォルダの記事A').getByRole('checkbox', { name: 'フォルダの記事Aを選択', exact: true })
  await chooseProblem.check()
  await choosePost.check()
  const picker = dialog(page, '選んだ2件をフォルダへ移動')
  // A move the API refuses leaves both where they were, still chosen, and says so above the table.
  await page.route('**/api/my/folder-items', route => route.fulfill({ status: 503 }))
  await chosenMove(page).click()
  await picker.getByRole('radio', { name: '未整理', exact: true }).check()
  await picker.getByRole('button', { name: '移動する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('保存できませんでした。もう一度お試しください。何も変わっていません。')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 1')
  await expect(chosenMove(page)).toHaveText('選んだ2件をフォルダへ移動')
  await page.unroute('**/api/my/folder-items')
  await chosenMove(page).click()
  // The two are in different folders, so nothing is chosen yet.
  await expect(picker.getByRole('radio', { checked: true })).toHaveCount(0)
  await picker.getByRole('radio', { name: '未整理', exact: true }).check()
  await picker.getByRole('button', { name: '移動する', exact: true }).click()
  await expect(picker).toHaveCount(0)
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(chosenMove(page)).toHaveCount(0)
  await expect(library(page).getByRole('checkbox', { checked: true })).toHaveCount(0)
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 3')
  // コンテスト用 counts what 第3回 inside it holds.
  await expect(folder(page, 'コンテスト用')).toHaveAccessibleName('コンテスト用 0')
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 0')
  await folder(page, '未整理').click()
  await expect(row(page, 'フォルダの問題A')).toHaveCount(1)
  await expect(row(page, 'フォルダの記事A')).toHaveCount(1)
  // In 未整理 the moved rows leave the table at once; when the API refuses they come back, still chosen.
  await page.goto('/my?tab=library&folder=unsorted')
  await chooseProblem.check()
  await choosePost.check()
  await page.route('**/api/my/folder-items', route => route.fulfill({ status: 503 }))
  await chosenMove(page).click()
  await expect(picker.getByRole('radio', { name: '未整理', exact: true })).toBeChecked()
  await picker.getByRole('radio', { name: '定期便', exact: true }).check()
  await picker.getByRole('button', { name: '移動する', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('保存できませんでした。もう一度お試しください。何も変わっていません。')
  await expect(rows(page)).toHaveCount(3)
  await expect(chooseProblem).toBeChecked()
  await expect(choosePost).toBeChecked()
  await expect(chosenMove(page)).toHaveText('選んだ2件をフォルダへ移動')
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 0')
  await page.unroute('**/api/my/folder-items')
  // Opening another folder clears the message.
  await folder(page, 'すべて').click()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('a folder moves into another folder, but not into one below itself', async ({ page }) => {
  await signIn(page)
  await expect(folder(page, 'コンテスト用')).toHaveCount(1)
  await expect(folder(page, '定期便')).toHaveCount(1)
  // With the keyboard: the row's actions, then 移動, then the folder.
  const actions = tree(page).getByRole('button', { name: '定期便の操作', exact: true })
  await actions.focus()
  await page.keyboard.press('Enter')
  await page.keyboard.press('Tab')
  await page.keyboard.press('Tab')
  await expect(tree(page).getByRole('button', { name: '移動', exact: true })).toBeFocused()
  await page.keyboard.press('Enter')
  const picker = dialog(page, '「定期便」を移動')
  await expect(picker.getByRole('radio', { name: '最上位', exact: true })).toBeChecked()
  await expect(picker.getByRole('radio', { name: '定期便', exact: true })).toBeDisabled()
  await page.keyboard.press('ArrowDown')
  await expect(picker.getByRole('radio', { name: 'コンテスト用', exact: true })).toBeChecked()
  await page.keyboard.press('Enter')
  await expect(picker).toHaveCount(0)
  await expect(actions).toBeFocused()
  // Indented under コンテスト用, level with 第3回.
  const left = async (name: string) => (await folder(page, name).boundingBox())!.x
  await expect(folder(page, '第3回')).toBeVisible()
  expect(await left('定期便')).toBeGreaterThan(await left('コンテスト用'))
  expect(await left('定期便')).toBe(await left('第3回'))
  await folderAction(page, 'コンテスト用', '移動')
  const outer = dialog(page, '「コンテスト用」を移動')
  await expect(outer.getByRole('radio', { name: 'コンテスト用 / 第3回', exact: true })).toBeDisabled()
  await expect(outer.getByRole('radio', { name: 'コンテスト用 / 定期便', exact: true })).toBeDisabled()
  await expect(outer.getByRole('radio', { name: '最上位', exact: true })).toBeEnabled()
  await page.keyboard.press('Escape')
  await expect(outer).toHaveCount(0)
  await page.reload()
  await expect(folder(page, '定期便')).toHaveCount(0)
  await tree(page).getByRole('button', { name: 'コンテスト用の中のフォルダ', exact: true }).click()
  await expect(folder(page, '定期便')).toBeVisible()
  expect(await left('定期便')).toBe(await left('第3回'))
})

test('deleting a folder puts what it held back in 未整理', async ({ page }) => {
  await signIn(page)
  // 第3回 holds the problem again, so the deletion has something to give back.
  await moveButton(page, 'フォルダの問題A').click()
  const picker = dialog(page, '「フォルダの問題A」をフォルダへ移動')
  await picker.getByRole('radio', { name: 'コンテスト用 / 第3回', exact: true }).check()
  await picker.getByRole('button', { name: '移動する', exact: true }).click()
  await expect(folder(page, 'コンテスト用')).toHaveAccessibleName('コンテスト用 1')
  await expect(folder(page, 'すべて')).toHaveAccessibleName('すべて 3')
  await expect(filter(kinds(page), '自分の問題')).toHaveAccessibleName('自分の問題 1')
  await folderAction(page, 'コンテスト用', '削除')
  const confirm = dialog(page, '「コンテスト用」を削除しますか？')
  await expect(confirm).toHaveAccessibleDescription('中のフォルダ2個も削除されます。問題と記事1件は「未整理」に戻り、削除されません。')
  await confirm.getByRole('button', { name: '削除する', exact: true }).click()
  await expect(confirm).toHaveCount(0)
  await expect(folder(page, 'コンテスト用')).toHaveCount(0)
  await expect(folder(page, '第3回')).toHaveCount(0)
  await expect(folder(page, '定期便')).toHaveCount(0)
  await expect(folder(page, 'すべて')).toBeFocused()
  await expect(folder(page, 'すべて')).toHaveAccessibleName('すべて 3')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 3')
  await expect(filter(kinds(page), '自分の問題')).toHaveAccessibleName('自分の問題 1')
  await folder(page, '未整理').click()
  await expect(row(page, 'フォルダの問題A')).toHaveCount(1)
  await page.reload()
  await expect(folder(page, 'コンテスト用')).toHaveCount(0)
  await expect(row(page, 'フォルダの問題A')).toHaveCount(1)
  await expect(filter(kinds(page), '自分の問題')).toHaveAccessibleName('自分の問題 1')
})

test('the third level holds no more folders', async ({ page }) => {
  await signIn(page)
  await makeFolder(page, '段1')
  await folder(page, '段1').click()
  await createButton(page).click()
  await expect(dialog(page, 'フォルダを作る')).toHaveAccessibleDescription('「段1」の中に作ります。')
  await page.keyboard.press('Escape')
  await makeFolder(page, '段2')
  await folder(page, '段2').click()
  await makeFolder(page, '段3')
  await expect(createButton(page)).toBeEnabled()
  await folder(page, '段3').click()
  await expect(page.getByRole('navigation', { name: '今いる場所' })).toHaveText('段1 / 段2 / 段3')
  const reason = 'フォルダの入れ子は3段までです。このフォルダの中には作れません。'
  await expect(createButton(page)).toBeDisabled()
  await expect(createButton(page)).toHaveAttribute('title', reason)
  await expect(page.getByText(reason, { exact: true })).toBeVisible()
  // Deleting the open folder, which holds no folders, goes back to すべて.
  await folderAction(page, '段3', '削除')
  const confirm = dialog(page, '「段3」を削除しますか？')
  await expect(confirm).toHaveAccessibleDescription('問題と記事0件は「未整理」に戻り、削除されません。')
  await confirm.getByRole('button', { name: '削除する', exact: true }).click()
  await expect(folder(page, '段3')).toHaveCount(0)
  await expect(folder(page, 'すべて')).toHaveAttribute('aria-current', 'true')
  await expect(page).not.toHaveURL(/folder=/)
  await expect(createButton(page)).toBeEnabled()
})

test('a renamed folder keeps the focus where its new name sorts it', async ({ page }) => {
  await signIn(page)
  const top = async (name: string) => (await folder(page, name).boundingBox())!.y
  expect(await top('メモ')).toBeLessThan(await top('段1'))
  await folderAction(page, '段1', '名前を変更')
  const renaming = dialog(page, 'フォルダの名前を変更')
  await renaming.getByLabel('フォルダの名前', { exact: true }).fill('アーカイブ')
  await renaming.getByRole('button', { name: '変更する', exact: true }).click()
  await expect(renaming).toHaveCount(0)
  // The renamed folder now comes before メモ, and its actions button still has the focus.
  await expect(tree(page).getByRole('button', { name: 'アーカイブの操作', exact: true })).toBeFocused()
  expect(await top('アーカイブ')).toBeLessThan(await top('メモ'))
})

// The tests below drag rows and folders. They start from what the tests above leave: メモ and アーカイブ, holding 段2,
// at the top, and the problem, the post and the tested problem in 未整理.
// A folder list row is marked while something is dragged; the tests read the marks only to see which places are offered.
const treeRow = (page: Page, name: string) => tree(page).getByRole('listitem').filter({ has: page.getByRole('button', { name: new RegExp(`^${name} \\d+$`) }) })

// Drags from one place and holds it over another without letting go, so the marks can be read; page.mouse.up() drops.
async function hold(page: Page, from: Locator, over: Locator) {
  await from.hover()
  await page.mouse.down()
  await over.hover()
}

// What the page puts on each drag it starts, and the text of any image it shows in place of the row.
type SeenDrags = { starts: { effect: string, text: string }[], images: string[] }
async function watchDrags(page: Page) {
  await page.evaluate(() => {
    const seen: SeenDrags = { starts: [], images: [] }
    Object.assign(window, { seenDrags: seen })
    document.addEventListener('dragstart', event => { seen.starts.push({ effect: event.dataTransfer?.effectAllowed ?? '', text: event.dataTransfer?.getData('text/plain') ?? '' }) })
    const setDragImage = DataTransfer.prototype.setDragImage
    DataTransfer.prototype.setDragImage = function (image: Element, x: number, y: number) {
      seen.images.push(image.textContent ?? '')
      setDragImage.call(this, image, x, y)
    }
  })
}
const seenDrags = (page: Page) => page.evaluate(() => (window as unknown as { seenDrags: SeenDrags }).seenDrags)

test('a row dragged onto a folder goes into it', async ({ page }) => {
  await signIn(page)
  await createFolder(page, '定期便', null)
  await page.goto('/my?tab=library&folder=unsorted')
  await expect(rows(page)).toHaveCount(3)
  await watchDrags(page)
  await row(page, 'フォルダの記事A').dragTo(folder(page, '定期便'))
  await expect(row(page, 'フォルダの記事A')).toHaveCount(0)
  await expect(rows(page)).toHaveCount(2)
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 1')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 2')
  // One row shows itself while dragged.
  expect(await seenDrags(page)).toEqual({ starts: [{ effect: 'move', text: '1件を移動' }], images: [] })
  await page.reload()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 1')
  await expect(rows(page)).toHaveCount(2)
  await expect(row(page, 'フォルダの記事A')).toHaveCount(0)
})

test('chosen rows are dragged together', async ({ page }) => {
  await signIn(page)
  await expect(folder(page, 'すべて')).toHaveAttribute('aria-current', 'true')
  const chooseProblem = row(page, 'フォルダの問題A').getByRole('checkbox', { name: 'フォルダの問題Aを選択', exact: true })
  const chooseTested = row(page, 'フォルダで受けたテストの問題').getByRole('checkbox', { name: 'フォルダで受けたテストの問題を選択', exact: true })
  await chooseProblem.check()
  await chooseTested.check()
  await watchDrags(page)
  await hold(page, row(page, 'フォルダで受けたテストの問題'), folder(page, '定期便'))
  // Both are unsorted, so 未整理 is not offered.
  await expect(treeRow(page, '定期便')).toContainClass('drop-target drop-over')
  await expect(treeRow(page, '未整理')).not.toContainClass('drop-target')
  await page.mouse.up()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 3')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 0')
  // Once moved, the rows are no longer chosen, as after a move from the dialog.
  await expect(library(page).getByRole('checkbox', { checked: true })).toHaveCount(0)
  await expect(chosenMove(page)).toHaveCount(0)
  expect(await seenDrags(page)).toEqual({ starts: [{ effect: 'move', text: '2件を移動' }], images: ['2件を移動'] })
  // The image's element is on the page only while the browser takes its picture.
  await expect(page.locator('.library-drag-image')).toHaveCount(0)
  await page.reload()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 3')
  await folder(page, '定期便').click()
  await expect(rows(page)).toHaveCount(3)
})

test('a row dragged onto 未整理 leaves its folder', async ({ page }) => {
  await signIn(page)
  await folder(page, '定期便').click()
  await expect(rows(page)).toHaveCount(3)
  // A row that is not chosen goes alone, and the chosen row stays chosen.
  const chooseProblem = row(page, 'フォルダの問題A').getByRole('checkbox', { name: 'フォルダの問題Aを選択', exact: true })
  await chooseProblem.check()
  await hold(page, row(page, 'フォルダの記事A'), folder(page, '未整理'))
  // Rows go into any folder or 未整理 but the one they are in, and never into すべて.
  await expect(treeRow(page, '未整理')).toContainClass('drop-target drop-over')
  await expect(treeRow(page, 'メモ')).toContainClass('drop-target')
  await expect(treeRow(page, 'メモ')).not.toContainClass('drop-over')
  await expect(treeRow(page, '定期便')).not.toContainClass('drop-target')
  await expect(treeRow(page, 'すべて')).not.toContainClass('drop-target')
  await page.mouse.up()
  await expect(row(page, 'フォルダの記事A')).toHaveCount(0)
  await expect(rows(page)).toHaveCount(2)
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 2')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 1')
  await expect(chooseProblem).toBeChecked()
  await expect(chosenMove(page)).toHaveText('選んだ1件をフォルダへ移動')
  await expect(treeRow(page, '未整理')).not.toContainClass('drop-target')
  // Dropped on 定期便, where it already is, the chosen row sends nothing and stays chosen.
  const sent: string[] = []
  page.on('request', request => { if (request.url().endsWith('/api/my/folder-items')) sent.push(request.method()) })
  await row(page, 'フォルダの問題A').dragTo(folder(page, '定期便'))
  await expect(rows(page)).toHaveCount(2)
  await expect(chooseProblem).toBeChecked()
  await expect(chosenMove(page)).toHaveText('選んだ1件をフォルダへ移動')
  await page.reload()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 2')
  await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 1')
  await expect(row(page, 'フォルダの記事A')).toHaveCount(0)
  expect(sent).toEqual([])
})

test('a folder dragged onto another goes inside it, but not where it would be too deep', async ({ page }) => {
  await signIn(page)
  const left = async (name: string) => (await folder(page, name).boundingBox())!.x
  await watchDrags(page)
  await folder(page, '定期便').dragTo(folder(page, 'メモ'))
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 2')
  // メモ opens to show where it went.
  await expect(folder(page, '定期便')).toBeVisible()
  expect(await left('定期便')).toBeGreaterThan(await left('メモ'))
  expect(await seenDrags(page)).toEqual({ starts: [{ effect: 'move', text: '定期便' }], images: [] })
  await page.reload()
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 2')
  await tree(page).getByRole('button', { name: 'メモの中のフォルダ', exact: true }).click()
  await tree(page).getByRole('button', { name: 'アーカイブの中のフォルダ', exact: true }).click()
  expect(await left('定期便')).toBe(await left('段2'))
  // メモ now takes two levels: it fits in アーカイブ but not in 段2, the second level, nor in itself or the folder below it.
  await hold(page, folder(page, 'メモ'), folder(page, '段2'))
  await expect(treeRow(page, 'アーカイブ')).toContainClass('drop-target')
  await expect(treeRow(page, '段2')).not.toContainClass('drop-target')
  await expect(treeRow(page, '段2')).not.toContainClass('drop-over')
  await expect(treeRow(page, 'メモ')).not.toContainClass('drop-target')
  await expect(treeRow(page, '定期便')).not.toContainClass('drop-target')
  await expect(treeRow(page, '未整理')).not.toContainClass('drop-target')
  await expect(treeRow(page, 'すべて')).not.toContainClass('drop-target')
  await page.mouse.up()
  await expect(treeRow(page, 'アーカイブ')).not.toContainClass('drop-target')
  await expect(folder(page, '段2')).toHaveAccessibleName('段2 0')
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 2')
  expect(await left('メモ')).toBe(await left('アーカイブ'))
  // Dropped on the folder list's blank space, below its button, a folder goes to the top.
  await createButton(page).evaluate(button => button.scrollIntoView({ block: 'center' }))
  await folder(page, '定期便').hover()
  const button = (await createButton(page).boundingBox())!
  await page.mouse.down()
  await page.mouse.move(button.x + 8, button.y + button.height + 16)
  // The mark is on the list around the navigation. メモ, where the folder is, is not offered, as it would change nothing.
  await expect(page.locator('.folder-tree', { has: tree(page) })).toContainClass('drop-over')
  await expect(treeRow(page, 'アーカイブ')).toContainClass('drop-target')
  await expect(treeRow(page, 'メモ')).not.toContainClass('drop-target')
  await page.mouse.up()
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 0')
  expect(await left('定期便')).toBe(await left('メモ'))
  await page.reload()
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 0')
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 2')
  expect(await left('定期便')).toBe(await left('メモ'))
  await expect(tree(page).getByRole('button', { name: 'メモの中のフォルダ', exact: true })).toHaveCount(0)
})

test('a folder drop the API refuses leaves the folder where it was', async ({ page }) => {
  await signIn(page)
  const left = async (name: string) => (await folder(page, name).boundingBox())!.x
  await page.route('**/api/my/folders/*', route => route.fulfill({ status: 503 }))
  await folder(page, 'メモ').dragTo(folder(page, 'アーカイブ'))
  await expect(page.getByRole('alert')).toHaveText('保存できませんでした。もう一度お試しください。何も変わっていません。')
  // Still at the top, level with アーカイブ.
  expect(await left('メモ')).toBe(await left('アーカイブ'))
  await page.unroute('**/api/my/folders/*')
  await page.reload()
  await tree(page).getByRole('button', { name: 'アーカイブの中のフォルダ', exact: true }).click()
  await expect(folder(page, '段2')).toBeVisible()
  expect(await left('メモ')).toBe(await left('アーカイブ'))
  expect(await left('メモ')).toBeLessThan(await left('段2'))
})

test('a drop the API refuses leaves the rows where they were', async ({ page }) => {
  await signIn(page)
  await folder(page, '定期便').click()
  await expect(rows(page)).toHaveCount(2)
  const chooseProblem = row(page, 'フォルダの問題A').getByRole('checkbox', { name: 'フォルダの問題Aを選択', exact: true })
  const chooseTested = row(page, 'フォルダで受けたテストの問題').getByRole('checkbox', { name: 'フォルダで受けたテストの問題を選択', exact: true })
  await chooseProblem.check()
  await chooseTested.check()
  await page.route('**/api/my/folder-items', route => route.fulfill({ status: 503 }))
  await row(page, 'フォルダの問題A').dragTo(folder(page, 'メモ'))
  await expect(page.getByRole('alert')).toHaveText('保存できませんでした。もう一度お試しください。何も変わっていません。')
  // Both come back, still chosen.
  await expect(rows(page)).toHaveCount(2)
  await expect(chooseProblem).toBeChecked()
  await expect(chooseTested).toBeChecked()
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 2')
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 0')
  await page.unroute('**/api/my/folder-items')
  await page.reload()
  await expect(folder(page, '定期便')).toHaveAttribute('aria-current', 'true')
  await expect(rows(page)).toHaveCount(2)
  await expect(folder(page, '定期便')).toHaveAccessibleName('定期便 2')
  await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 0')
})

test('a narrow screen drags neither rows nor folders', async ({ page }) => {
  await signIn(page)
  await expect(row(page, 'フォルダの記事A')).toHaveAttribute('draggable', 'true')
  await expect(treeRow(page, 'メモ')).toHaveAttribute('draggable', 'true')
  await page.setViewportSize({ width: 600, height: 900 })
  await expect(row(page, 'フォルダの記事A')).not.toHaveAttribute('draggable')
  // The folder list folds away on a narrow screen; opened, its folders do not drag either.
  await choosing(page).click()
  await expect(treeRow(page, 'メモ')).not.toHaveAttribute('draggable')
  await page.setViewportSize({ width: 1280, height: 720 })
  await expect(row(page, 'フォルダの記事A')).toHaveAttribute('draggable', 'true')
})

test('a touch screen moves rows through the dialog, not by dragging', async ({ browser }) => {
  const context = await browser.newContext({ baseURL: origin, hasTouch: true })
  try {
    const page = await context.newPage()
    await signIn(page)
    // The default viewport is wide, so only the coarse pointer turns dragging off.
    expect(await page.evaluate(() => [innerWidth, matchMedia('(pointer: coarse)').matches])).toEqual([1280, true])
    await expect(rows(page)).toHaveCount(3)
    await expect(library(page).locator('tbody tr[draggable]')).toHaveCount(0)
    await expect(tree(page).locator('li[draggable]')).toHaveCount(0)
    await moveButton(page, 'フォルダの記事A').tap()
    const picker = dialog(page, '「フォルダの記事A」をフォルダへ移動')
    await picker.getByRole('radio', { name: 'メモ', exact: true }).tap()
    await picker.getByRole('button', { name: '移動する', exact: true }).tap()
    await expect(picker).toHaveCount(0)
    await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 1')
    await expect(folder(page, '未整理')).toHaveAccessibleName('未整理 0')
    await page.reload()
    await expect(folder(page, 'メモ')).toHaveAccessibleName('メモ 1')
  } finally { await context.close() }
})

// The tests below look at a phone's width and at problems and posts made inside a folder. Each makes the folders it
// uses; フォルダの問題A, wherever the tests above left it, is the saved problem they move.
const newProblem = (page: Page) => page.getByRole('main').getByRole('link', { name: '新規問題', exact: true })
const newPost = (page: Page) => page.getByRole('main').getByRole('link', { name: '新規記事', exact: true })
const saveStatus = (page: Page) => page.locator('.draft-status').getByRole('status')
const fitsWidth = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

async function placeProblem(page: Page, folderId: string, title: string) {
  const response = await page.request.put('/api/my/folder-items', { headers: { origin }, data: { folderId, items: [{ kind: 'problem', id: await problemId(page, title) }] } })
  expect(response.status()).toBe(204)
}

// The bodies of the placements the page sends.
function watchPlacements(page: Page) {
  const sent: unknown[] = []
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/my/folder-items') sent.push(request.postDataJSON()) })
  return sent
}

// Where the API has the item now, or null when it is unsorted.
async function placement(page: Page, kind: 'problem' | 'post', id: string) {
  const tree = await (await page.request.get('/api/my/folders')).json() as { items: { kind: string, id: string, folderId: string }[] }
  return tree.items.find(item => item.kind === kind && item.id === id)?.folderId ?? null
}

test('a phone opens the folders from a button above the table and moves rows through the dialog', async ({ page }, testInfo) => {
  await signIn(page)
  const opened = await createFolder(page, 'スマホで開く', null)
  await createFolder(page, 'スマホで入れる', null)
  await placeProblem(page, opened, 'フォルダの問題A')
  await page.setViewportSize({ width: 375, height: 812 })
  await page.reload()
  // The list is folded away under a button that names the open folder, by the same words it shows.
  await expect(choosing(page)).toHaveText('フォルダ: すべて')
  await expect(choosing(page)).toHaveAccessibleName('フォルダ: すべて')
  await expect(choosing(page)).toHaveAttribute('aria-expanded', 'false')
  await expect(tree(page)).toBeHidden()
  await expect(rows(page)).toHaveCount(3)
  await choosing(page).click()
  await expect(choosing(page)).toHaveAttribute('aria-expanded', 'true')
  await expect(tree(page)).toBeVisible()
  expect((await tree(page).boundingBox())!.y).toBeGreaterThan((await choosing(page).boundingBox())!.y)
  expect(await fitsWidth(page)).toBe(true)
  await folder(page, 'スマホで開く').click()
  await expect(page).toHaveURL(new RegExp(`folder=${opened}`))
  await expect(tree(page)).toBeHidden()
  await expect(choosing(page)).toHaveAttribute('aria-expanded', 'false')
  await expect(choosing(page)).toHaveAccessibleName('フォルダ: スマホで開く')
  await expect(choosing(page)).toBeFocused()
  await expect(rows(page)).toHaveCount(1)
  // Each row is a card, led by its kind, and nothing is dragged.
  const card = row(page, 'フォルダの問題A')
  await expect(library(page).getByRole('columnheader', { name: '種類', exact: true, includeHidden: true })).toBeHidden()
  await expect(kindCell(card)).toHaveText('自分の問題')
  expect((await kindCell(card).boundingBox())!.y).toBeLessThan((await card.getByRole('rowheader').boundingBox())!.y)
  await expect(library(page).locator('tbody tr[draggable]')).toHaveCount(0)
  expect(await fitsWidth(page)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('folder-view-phone.png'), fullPage: true })
  await moveButton(page, 'フォルダの問題A').click()
  const picker = dialog(page, '「フォルダの問題A」をフォルダへ移動')
  await picker.getByRole('radio', { name: 'スマホで入れる', exact: true }).check()
  await picker.getByRole('button', { name: '移動する', exact: true }).click()
  await expect(picker).toHaveCount(0)
  await expect(rows(page)).toHaveCount(0)
  await expect(page.getByText('このフォルダは空です。', { exact: true })).toBeVisible()
  // The row took its button along and the folder's own button is folded away, so the list's button has the focus.
  await expect(choosing(page)).toBeFocused()
  // The folders' actions stay within reach while the list is open.
  await choosing(page).click()
  await expect(folder(page, 'スマホで入れる')).toHaveAccessibleName('スマホで入れる 1')
  await tree(page).getByRole('button', { name: 'スマホで入れるの操作', exact: true }).click()
  await expect(tree(page).getByRole('button', { name: '削除', exact: true })).toBeInViewport()
  expect(await fitsWidth(page)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('folder-list-phone.png'), fullPage: true })
  await page.keyboard.press('Escape')
  await folder(page, 'スマホで入れる').click()
  await expect(tree(page)).toBeHidden()
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの問題A')
  await page.reload()
  await expect(choosing(page)).toHaveAccessibleName('フォルダ: スマホで入れる')
  await expect(rows(page)).toContainText('フォルダの問題A')
  // A folder below the open one is a card too, led by フォルダ.
  await choosing(page).click()
  await folder(page, 'アーカイブ').click()
  const below = row(page, '段2')
  expect((await below.getByRole('cell', { name: 'フォルダ', exact: true }).boundingBox())!.y).toBeLessThan((await below.getByRole('button', { name: '段2', exact: true }).boundingBox())!.y)
  expect(await fitsWidth(page)).toBe(true)
  // Opening a folder from the table folds the list as well.
  await choosing(page).click()
  await expect(tree(page)).toBeVisible()
  await below.getByRole('button', { name: '段2', exact: true }).click()
  await expect(choosing(page)).toHaveAccessibleName('フォルダ: 段2')
  await expect(choosing(page)).toHaveAttribute('aria-expanded', 'false')
  await expect(tree(page)).toBeHidden()
  await expect(choosing(page)).toBeFocused()
})

test('a problem made inside a folder goes into it', async ({ page }) => {
  await signIn(page)
  const made = await createFolder(page, '作った問題', null)
  // すべて and 未整理 make what is new unsorted.
  await expect(newProblem(page)).toHaveAttribute('href', '/problems/new?fresh=1')
  await page.goto('/my?tab=library&folder=unsorted')
  await expect(newProblem(page)).toHaveAttribute('href', '/problems/new?fresh=1')
  await expect(newPost(page)).toHaveAttribute('href', '/blog/new')
  await page.goto(`/my?tab=library&folder=${made}`)
  await expect(newProblem(page)).toHaveAttribute('href', `/problems/new?fresh=1&folder=${made}`)
  await expect(newPost(page)).toHaveAttribute('href', `/blog/new?folder=${made}`)
  const placements = watchPlacements(page)
  await newProblem(page).click()
  await expect(page).toHaveURL(`/problems/new?fresh=1&folder=${made}`)
  await page.getByLabel('問題のタイトル', { exact: true }).fill('フォルダの中で作った問題')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(saveStatus(page)).toHaveText('保存済み')
  await expect(page).toHaveURL(/\/problems\/new\?problem=[0-9a-f-]{36}$/)
  const id = new URL(page.url()).searchParams.get('problem')!
  expect(placements).toEqual([{ folderId: made, items: [{ kind: 'problem', id }] }])
  // Only the save that made it places it.
  await page.getByLabel('問題のタイトル', { exact: true }).fill('フォルダの中で作った問題（改）')
  await expect(saveStatus(page)).toHaveText('未保存の変更があります')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(saveStatus(page)).toHaveText('保存済み')
  expect(placements).toHaveLength(1)
  await page.goto('/my')
  await folder(page, '作った問題').click()
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの中で作った問題（改）')
})

test('a post made inside a folder goes into it', async ({ page }) => {
  await signIn(page)
  const made = await createFolder(page, '書いた記事', null)
  await page.goto(`/my?tab=library&folder=${made}`)
  const placements = watchPlacements(page)
  await newPost(page).click()
  await expect(page).toHaveURL(`/blog/new?folder=${made}`)
  await page.getByLabel('タイトル', { exact: true }).fill('フォルダの中で書いた記事')
  await page.getByLabel('本文（Markdown）').fill('フォルダの中で書く')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(saveStatus(page)).toHaveText('保存済み')
  await expect(page).toHaveURL(/\/blog\/new\?post=[0-9a-f-]{36}$/)
  const id = new URL(page.url()).searchParams.get('post')!
  expect(placements).toEqual([{ folderId: made, items: [{ kind: 'post', id }] }])
  await page.getByLabel('タイトル', { exact: true }).fill('フォルダの中で書いた記事（改）')
  await expect(saveStatus(page)).toHaveText('未保存の変更があります')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(saveStatus(page)).toHaveText('保存済み')
  expect(placements).toHaveLength(1)
  await page.goto('/my')
  await folder(page, '書いた記事').click()
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの中で書いた記事（改）')
})

test('a saved problem opened with another folder in its address stays where it is', async ({ page }) => {
  await signIn(page)
  const home = await createFolder(page, '元の場所', null)
  const other = await createFolder(page, '別の場所', null)
  await placeProblem(page, home, 'フォルダの問題A')
  const id = await problemId(page, 'フォルダの問題A')
  const placements = watchPlacements(page)
  await page.goto(`/problems/new?problem=${id}&folder=${other}`)
  await expect(saveStatus(page)).toHaveText('保存済み')
  await expect(page.getByLabel('問題のタイトル', { exact: true })).toHaveValue('フォルダの問題A')
  // The save button writes a new version even with nothing changed.
  await Promise.all([
    page.waitForResponse(response => response.url().endsWith(`/api/my/problems/${id}`) && response.request().method() === 'PUT' && response.ok()),
    page.getByRole('button', { name: '保存', exact: true }).click(),
  ])
  await expect(saveStatus(page)).toHaveText('保存済み')
  expect(placements).toEqual([])
  expect(await placement(page, 'problem', id)).toBe(home)
  await page.goto(`/my?tab=library&folder=${home}`)
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page)).toContainText('フォルダの問題A')
  await expect(folder(page, '別の場所')).toHaveAccessibleName('別の場所 0')
})
