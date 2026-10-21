import { expect, test } from './fixtures/account'

for (const [path, selector, editorial] of [
  ['/problems/new', '#problem-source', false],
  ['/problems/new', '#problem-source', true],
  ['/blog/new', '#post-body', false],
] as const) {
  test(`content links confirm external destinations in ${path}, editorial=${editorial}`, async ({ page, context }, testInfo) => {
    let externalRequests = 0
    await context.route('https://outside.example/**', route => {
      externalRequests++
      return route.fulfill({ contentType: 'text/html', body: '<p>External destination</p>' })
    })
    await page.goto(path)
    if (editorial) await page.getByRole('button', { name: '解説', exact: true }).click()
    await page.locator(selector).fill('[**外部リンク**](https://outside.example/article?x=1&y=2)\n\n[内部リンク](/docs/markdown)\n\n[公式リンク](https://judge.example/blog/article)\n\n[見出し](#section)\n\n## 見出し {#section}')
    if (path === '/problems/new') await expect(page).toHaveURL(url => url.searchParams.has('problem'))
    const preview = page.locator('#preview-pane')
    const external = preview.getByRole('link', { name: '外部リンク', exact: true })
    const dialog = page.getByRole('dialog', { name: '外部サイトへ移動します' })
    await external.locator('strong').click()
    const sourceUrl = page.url()
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('https://outside.example/article?x=1&y=2')
    await expect(dialog.getByRole('button', { name: 'キャンセル' })).toBeFocused()
    await dialog.getByRole('button', { name: 'キャンセル' }).click()
    await expect(dialog).not.toBeVisible()
    expect(externalRequests).toBe(0)
    expect(context.pages()).toHaveLength(1)

    await external.click()
    await dialog.click({ position: { x: 4, y: 4 } })
    await expect(dialog).toBeVisible()
    const bounds = (await dialog.boundingBox())!
    await page.mouse.move(bounds.x + 4, bounds.y + 4)
    await page.mouse.down()
    await page.mouse.move(2, 2)
    await page.mouse.up()
    await expect(dialog).toBeVisible()
    await page.mouse.click(2, 2)
    await expect(dialog).toBeHidden()
    expect(externalRequests).toBe(0)
    await expect(page).toHaveURL(sourceUrl)

    await external.focus()
    await external.press('Enter')
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(external).toBeFocused()
    await external.click({ modifiers: ['ControlOrMeta'] })
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await external.click({ button: 'middle' })
    await expect(dialog).toBeVisible()
    expect(externalRequests).toBe(0)

    await page.setViewportSize({ width: 320, height: 700 })
    expect(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth)).toBe(true)
    await dialog.screenshot({ path: testInfo.outputPath('external-link-mobile.png') })
    const opened = context.waitForEvent('page')
    await dialog.getByRole('link', { name: '別タブで開く' }).click()
    const destination = await opened
    await destination.waitForLoadState()
    await expect(destination).toHaveURL('https://outside.example/article?x=1&y=2')
    expect(await destination.evaluate(() => window.opener)).toBeNull()
    await destination.close()
    await expect(dialog).not.toBeVisible()
    await expect(page).toHaveURL(sourceUrl)

    await page.setViewportSize({ width: 1280, height: 900 })
    await context.route('https://judge.example/**', route => route.fulfill({ body: 'ShareOJ' }))
    for (const name of ['内部リンク', '公式リンク']) {
      const opened = context.waitForEvent('page')
      await preview.getByRole('link', { name, exact: true }).click()
      const destination = await opened
      await destination.waitForLoadState()
      await expect(dialog).not.toBeVisible()
      await destination.close()
    }
    await preview.getByRole('link', { name: '見出し', exact: true }).click()
    await expect(page).toHaveURL(`${sourceUrl}#section`)
    expect(context.pages()).toHaveLength(1)

    for (const url of ['//outside.example/path', 'https://judge.example.outside.example/', 'https://judge.example@outside.example/']) {
      await page.locator(selector).fill(`[確認対象](${url})`)
      await preview.getByRole('link', { name: '確認対象' }).click()
      await expect(dialog).toBeVisible()
      await page.keyboard.press('Escape')
    }
  })
}
