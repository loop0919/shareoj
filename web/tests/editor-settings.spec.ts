import { expect, test } from './fixtures/account'

const problem = '/problems/11111111-1111-4111-8111-111111111111'

for (const [path, selector] of [['/problems/new', '#problem-source'], ['/blog/new', '#post-body']]) {
  test(`Markdown indentation and settings work in ${path}`, async ({ page }) => {
    await page.goto(path!)
    const editor = page.locator(selector!)
    const toolbar = page.locator('.editor-toolbar')
    const settingsButton = page.getByRole('button', { name: 'エディタ設定', exact: true })
    const toolbarBox = (await toolbar.boundingBox())!
    const settingsBox = (await settingsButton.boundingBox())!
    expect(Math.abs(toolbarBox.x + toolbarBox.width - settingsBox.x - settingsBox.width - 8)).toBeLessThan(2)
    await editor.fill('one\ntwo\nthree')
    await editor.press('ControlOrMeta+Home')
    await editor.press('Shift+ArrowDown')
    await editor.press('Shift+End')
    await editor.press('Tab')
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual('  one\n  two\nthree'.split('\n'))
    await editor.press('Shift+Tab')
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual('one\ntwo\nthree'.split('\n'))
    await editor.fill('  one')
    await editor.press('Home')
    await editor.press('Shift+Tab')
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual('one'.split('\n'))
    await editor.press('Tab')
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual('  one'.split('\n'))
    await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'エディタ設定（Markdown）', exact: true })
    await expect(dialog.getByLabel('インデント方式')).toHaveValue('space')
    await expect(dialog.getByLabel('インデント幅')).toHaveValue('2')
    await dialog.getByLabel('インデント方式').selectOption('tab')
    await dialog.getByLabel('インデント幅').selectOption('8')
    await dialog.press('Escape')
    await expect(dialog).toBeHidden()
    await expect(page.getByRole('button', { name: 'エディタ設定', exact: true })).toBeFocused()
    await editor.fill('one')
    await editor.press('Tab')
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual('\tone'.split('\n'))
    await expect(editor).toHaveCSS('tab-size', '8')
    await expect(page.locator('.markdown-source-editor .cm-content')).toHaveCSS('tab-size', '8')
    await editor.press('Shift+Tab')
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual('one'.split('\n'))
    await editor.press('Escape')
    await editor.press('Tab')
    await expect(editor).not.toBeFocused()
  })
}

test('source and Markdown settings apply immediately and persist independently', async ({ page }) => {
  await page.goto(problem)
  const editor = page.getByLabel('ソースコード', { exact: true })
  await editor.fill('int main(){}')
  await editor.press('Tab')
  await expect(editor).toHaveText('    int main(){}')
  await editor.press('Shift+Tab')
  await expect(editor).toHaveText('int main(){}')
  await editor.press('ControlOrMeta+z')
  await expect(editor).toHaveText('    int main(){}')
  await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'エディタ設定' })
  await expect(dialog).toHaveAccessibleName('エディタ設定（Code）')
  await dialog.getByLabel('インデント幅').selectOption('2')
  await expect(dialog.getByRole('button', { name: '閉じる', exact: true })).toHaveCount(0)
  await page.mouse.click(2, 2)
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('button', { name: 'エディタ設定', exact: true })).toBeFocused()
  await editor.fill('x')
  await editor.press('Tab')
  await expect(editor).toHaveText('  x')
  await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
  await dialog.getByLabel('インデント方式').selectOption('tab')
  await page.mouse.click(2, 2)
  await editor.fill('x')
  await editor.press('Tab')
  expect(await editor.textContent()).toBe('\tx')
  await page.reload()
  await editor.fill('x')
  await editor.press('Tab')
  expect(await editor.textContent()).toBe('\tx')
  await editor.press('Escape')
  await editor.press('Tab')
  await expect(editor).not.toBeFocused()
  await page.goto('/blog/new')
  const markdown = page.getByLabel('本文（Markdown）')
  await markdown.fill('x')
  await markdown.press('Tab')
  await expect.poll(() => markdown.locator('.cm-line').allTextContents()).toEqual('  x'.split('\n'))
  await expect(markdown).toHaveCSS('tab-size', '2')
  await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
  await dialog.getByLabel('インデント幅').selectOption('8')
  await page.mouse.click(2, 2)
  await page.reload()
  await markdown.fill('x')
  await markdown.press('Tab')
  await expect.poll(() => markdown.locator('.cm-line').allTextContents()).toEqual('        x'.split('\n'))
  await page.goto(problem)
  await editor.fill('x')
  await editor.press('Tab')
  expect(await editor.textContent()).toBe('\tx')
  await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
  await expect(dialog.getByLabel('インデント方式')).toHaveValue('tab')
  await expect(dialog.getByLabel('インデント幅')).toHaveValue('2')
})

test('settings work without local storage and the modal fits a narrow viewport', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'localStorage', { get() { throw new DOMException('Blocked', 'SecurityError') } })
  })
  await page.setViewportSize({ width: 320, height: 700 })
  await page.goto(problem)
  await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'エディタ設定' })
  await dialog.getByLabel('インデント幅').selectOption('8')
  const bounds = (await dialog.boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(320)
  await dialog.getByRole('button', { name: 'エディタ設定を閉じる', exact: true }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('button', { name: 'エディタ設定', exact: true })).toBeFocused()
  const editor = page.getByLabel('ソースコード', { exact: true })
  await editor.fill('x')
  await editor.press('Tab')
  expect(await editor.textContent()).toBe('        x')
})


test('legacy shared settings are retained only for the code editor', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('openoj.editor-settings', JSON.stringify({ style: 'tab', width: 4 })))
  await page.goto('/problems/new')
  const markdown = page.locator('#problem-source')
  await markdown.fill('x')
  await markdown.press('Tab')
  await expect.poll(() => markdown.locator('.cm-line').allTextContents()).toEqual('  x'.split('\n'))
  await page.goto(problem)
  const code = page.getByLabel('ソースコード', { exact: true })
  await code.fill('x')
  await code.press('Tab')
  expect(await code.textContent()).toBe('\tx')
})


test('language switching updates highlighting and keeps indentation settings per language', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [
    { id: 'cpp17', label: 'C++' }, { id: 'python314', label: 'Python' },
    { id: 'javascript-node24', label: 'JavaScript' }, { id: 'go127', label: 'Go' },
  ] } }))
  await page.goto(problem)
  const editor = page.getByLabel('ソースコード', { exact: true })
  const language = page.getByLabel('言語', { exact: true })
  await editor.fill('# hello')
  await language.selectOption('python314')
  await expect(editor).toHaveText('# hello')
  await expect(editor.locator('span').filter({ hasText: '# hello' })).toHaveCount(1)
  await editor.fill('x')
  await editor.press('Tab')
  expect(await editor.textContent()).toBe('    x')
  await page.getByRole('button', { name: 'エディタ設定', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'エディタ設定' })
  await dialog.getByLabel('インデント幅').selectOption('8')
  await page.mouse.click(2, 2)
  for (const [runtime, indent] of [['javascript-node24', '  '], ['go127', '\t'], ['cpp17', '    '], ['python314', '        ']]) {
    await language.selectOption(runtime!)
    await editor.fill('x')
    await editor.press('Tab')
    expect(await editor.textContent()).toBe(`${indent}x`)
    await editor.press('Shift+Tab')
    await expect(editor).toHaveText('x')
  }
  await page.reload()
  await language.selectOption('python314')
  await editor.fill('x')
  await editor.press('Tab')
  expect(await editor.textContent()).toBe('        x')
  await language.selectOption('')
  await editor.fill('# hello')
  await expect(editor.locator('span')).toHaveCount(0)
})


test('all runtime families highlight comments using their selected language', async ({ page }) => {
  const samples = [
    ['c23-clang', '// hello'], ['cpp23-gcc', '// hello'], ['pypy311', '# hello'],
    ['codon020', '# hello'], ['rust2024', '// hello'], ['java25', '// hello'],
    ['csharp14', '// hello'], ['nim22', '#[ outer #[ inner ]# end ]#'],
    ['go127', '// hello'], ['haskell-ghc910', '-- hello'],
    ['javascript-deno29', '// hello'], ['typescript-bun14', '// hello'], ['ruby-truffle40', '# hello'],
  ]
  await page.route('**/api/runtimes', route => route.fulfill({ json: {
    items: samples.map(([id]) => ({ id, label: id })),
  } }))
  await page.goto(problem)
  const editor = page.getByLabel('ソースコード', { exact: true })
  for (const [runtime, comment] of samples) {
    await page.getByLabel('言語', { exact: true }).selectOption(runtime!)
    await editor.fill(comment!)
    await expect(editor.locator('span').filter({ hasText: comment! })).toHaveCount(1)
  }
})
