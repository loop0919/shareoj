import { expect, test } from './fixtures/account'

for (const path of ['/problems/new', '/blog/new']) {
  test(`details can be inserted and toggled in ${path}`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.goto(path)
    const source = page.locator('.markdown-source-editor .cm-content')
    await source.fill('')
    await page.getByRole('button', { name: '折りたたみ', exact: true }).click()
    await expect(source).toHaveText('\n:::details タイトル\n内容\n:::\n', { useInnerText: true })
    const details = page.locator('.markdown-body details')
    await expect(details.locator('p')).toBeHidden()
    await details.locator('summary').click()
    await expect(details.locator('p')).toBeVisible()
    await details.locator('summary').focus()
    await page.keyboard.press('Enter')
    await expect(details.locator('p')).toBeHidden()
  })
}

const fold = (title: string, body: string) => `:::details ${title}\n${body}\n:::\n`
for (const [path, selector] of [['/problems/new', '#problem-source'], ['/blog/new', '#post-body'], ['/my/contests/new', '#contest-description']]) {
  test(`opened folds stay open while the source is edited in ${path}`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.goto(path!)
    const source = page.locator(selector!)
    const folds = page.locator('.preview-document details')
    await source.fill(`${fold('A', '内容A')}\n${fold('B', '内容B')}`)
    await expect(folds).toHaveCount(2)
    await folds.nth(1).locator('summary').click()
    await source.fill(`${fold('A', '内容A')}\n${fold('B', '内容Bを編集')}`)
    await expect(folds.nth(1).locator('p')).toHaveText('内容Bを編集')
    await expect(folds.nth(1)).toHaveJSProperty('open', true)
    await expect(folds.nth(0)).toHaveJSProperty('open', false)
    await source.fill(`${fold('A', '内容A')}\n${fold('Bの新しい題', '内容B')}`)
    await expect(folds.nth(1).locator('summary')).toHaveText('Bの新しい題')
    await expect(folds.nth(1)).toHaveJSProperty('open', true)
    await source.fill(`${fold('追加', '内容')}\n${fold('A', '内容A')}\n${fold('Bの新しい題', '内容B')}`)
    await expect(folds).toHaveCount(3)
    expect(await folds.evaluateAll(nodes => nodes.map(node => (node as HTMLDetailsElement).open))).toEqual([false, false, true])
  })
}

test('statement and editorial previews keep their own folds', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/problems/new')
  const source = page.locator('#problem-source')
  const preview = page.locator('.preview-document details')
  await source.fill(fold('問題のヒント', '内容'))
  await preview.locator('summary').click()
  await page.getByRole('button', { name: '解説', exact: true }).click()
  await source.fill(fold('解説の補足', '内容'))
  await expect(preview.locator('summary')).toHaveText('解説の補足')
  await expect(preview).toHaveJSProperty('open', false)
})
