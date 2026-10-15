import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const problemId = '11111111-1111-4111-8111-111111111111'
const postId = '77777777-7777-4777-8777-777777777777'

test('problem editorial tab and post link each other', async ({ page }) => {
  await page.goto(`/problems/${problemId}?view=editorial`)
  const section = page.getByRole('region', { name: 'ユーザー解説' })
  await expect(section.getByRole('link', { name: 'ユーザー解説を書く', exact: true })).toHaveAttribute('href', `/blog/new?problem=${problemId}`)
  await section.getByRole('link', { name: '日本語 & #記事 + 共有', exact: true }).click()
  await expect(page).toHaveURL(`/blog/${shortId(postId)}`)
  await expect(page.getByText('解説している問題')).toBeVisible()
  await page.locator('.post-problems').getByRole('link', { name: 'A + B', exact: true }).click()
  await expect(page).toHaveURL(`/problems/${shortId(problemId)}`)
})

test('hidden featured editorials have no user editorials', async ({ page }) => {
  await page.goto('/problems/eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee?view=editorial')
  await expect(page.getByText('解説は公開待ちです。')).toBeVisible()
  await expect(page.getByRole('region', { name: 'ユーザー解説' })).toHaveCount(0)
})

test('post editor saves the problems it explains', async ({ page }) => {
  const saved: { problemIds?: string[] }[] = []
  await page.route('**/api/my/posts/*', route => {
    const request = route.request()
    if (request.method() !== 'PUT') return route.fulfill({ status: 404, json: {} })
    const body = request.postDataJSON()
    saved.push(body)
    return route.fulfill({ json: {
      id: new URL(request.url()).pathname.split('/').pop(), title: body.title, markdown: body.markdown, version: saved.length,
      publishedVersion: 0, publishedAt: null, updatedAt: '2026-10-11T00:00:00Z', isOperator: false,
      problems: body.problemIds.map((id: string) => ({ id, title: 'A + B' })),
    } })
  })
  // "Write a user editorial" on a problem page starts with that problem.
  await page.goto(`/blog/new?problem=${problemId}`)
  const problems = page.getByRole('list', { name: '解説する問題' })
  await expect(problems.getByRole('listitem')).toHaveText(['A + B'])
  const input = page.getByRole('textbox', { name: /^解説する問題/ })
  const add = page.getByRole('button', { name: '追加', exact: true })
  for (const [value, message] of [
    [`https://judge.example/contests/88888888-8888-4888-8888-888888888888/problems/${problemId}`, 'この問題はすでに追加されています。'],
    ['https://judge.example/problems/eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee', 'この問題は公式解説の公開後に追加できます。'],
    ['12345678-1234-4234-8234-123456789012', '公開されている問題が見つかりません。コンテストの問題は終了後に追加できます。'],
    ['https://judge.example/blog', '問題ページのURLか問題IDを入力してください。'],
  ]) {
    await input.fill(value)
    await add.click()
    await expect(page.getByRole('alert')).toHaveText(message)
  }
  await page.getByLabel('タイトル', { exact: true }).fill('A + B のユーザー解説')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('保存済み')
  expect(saved.at(-1)?.problemIds).toEqual([problemId])
  await problems.getByRole('button', { name: 'A + Bを外す' }).click()
  await expect(page.getByRole('status')).toHaveText('未保存の変更があります')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('保存済み')
  expect(saved.at(-1)?.problemIds).toEqual([])
})

test('user editorial notifications open the editorial tab', async ({ page }) => {
  const notifications = { notifications: [{ id: '55555555-5555-4555-8555-555555555555', problemId, title: 'A + B', actor: 'bob', kind: 'user_editorial', createdAt: '2026-10-11T00:00:00Z' }] }
  await page.route('**/api/my/notifications**', route => route.fulfill({ json: notifications }))
  await page.goto('/')
  await page.getByLabel('通知（未読1件）').click()
  await page.getByRole('link', { name: /bobがユーザー解説を書きました/ }).click()
  await expect(page).toHaveURL(`/problems/${shortId(problemId)}?view=editorial`)
  await expect(page.getByRole('region', { name: 'ユーザー解説' })).toBeVisible()
})
