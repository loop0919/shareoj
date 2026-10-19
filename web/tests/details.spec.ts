import type { Locator } from '@playwright/test'
import { expect, test } from './fixtures/account'

// Replace the source the way a person does: CodeMirror handles key and input events directly, while fill() swaps the
// DOM in one step, which a loaded CI runner was seen to miss.
async function write(source: Locator, text: string) {
  await source.click()
  await source.press('ControlOrMeta+a')
  await source.page().keyboard.insertText(text)
  await expect(source).toHaveText(text, { useInnerText: true })
}

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
    await write(source, `${fold('A', '内容A')}\n${fold('B', '内容B')}`)
    await expect(folds).toHaveCount(2)
    await folds.nth(1).locator('summary').click()
    await write(source, `${fold('A', '内容A')}\n${fold('B', '内容Bを編集')}`)
    await expect(folds.nth(1).locator('p')).toHaveText('内容Bを編集')
    await expect(folds.nth(1)).toHaveJSProperty('open', true)
    await expect(folds.nth(0)).toHaveJSProperty('open', false)
    await write(source, `${fold('A', '内容A')}\n${fold('Bの新しい題', '内容B')}`)
    await expect(folds.nth(1).locator('summary')).toHaveText('Bの新しい題')
    await expect(folds.nth(1)).toHaveJSProperty('open', true)
    await write(source, `${fold('追加', '内容')}\n${fold('A', '内容A')}\n${fold('Bの新しい題', '内容B')}`)
    await expect(folds).toHaveCount(3)
    expect(await folds.evaluateAll(nodes => nodes.map(node => (node as HTMLDetailsElement).open))).toEqual([false, false, true])
  })
}

test('statement and editorial previews keep their own folds', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/problems/new')
  const source = page.locator('#problem-source')
  const preview = page.locator('.preview-document details')
  await write(source, fold('問題のヒント', '内容'))
  await preview.locator('summary').click()
  await page.getByRole('button', { name: '解説', exact: true }).click()
  await write(source, fold('解説の補足', '内容'))
  await expect(preview.locator('summary')).toHaveText('解説の補足')
  await expect(preview).toHaveJSProperty('open', false)
})

test('fold titles render math and inline Markdown, but not links or HTML', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/problems/new')
  const source = page.locator('#problem-source')
  const details = page.locator('.preview-document details')
  const summary = details.locator('summary')
  const title = (bound: string) => `$N \\le ${bound}$ の **小課題** と \`code\` と [リンク](https://example.com) <script>x</script>`
  await write(source, fold(title('10^5'), '内容'))
  await expect(summary.locator('.katex')).toHaveCount(1)
  await expect(summary.locator('strong')).toHaveText('小課題')
  await expect(summary.locator('code')).toHaveText('code')
  await expect(summary.locator('a, script')).toHaveCount(0)
  await expect(summary).toContainText('リンク')
  await expect(summary).toContainText('<script>x</script>')
  await summary.click()
  await expect(details).toHaveJSProperty('open', true)
  await write(source, fold(title('2 \\times 10^5'), '内容'))
  await expect(summary.locator('.katex')).toContainText('×')
  await expect(details).toHaveJSProperty('open', true)
  await write(source, ':::details\n内容\n:::\n')
  await expect(summary).toHaveText('詳細')
})
