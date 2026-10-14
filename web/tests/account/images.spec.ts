import { expect, test } from '@playwright/test'

test('images drop into all editors, deduplicate, follow publication, and can be reclaimed', async ({ page, browser, baseURL }, testInfo) => {
  await page.goto('/login')
  await page.getByLabel('メールアドレス').fill('alice@example.test')
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/my')
  page.on('dialog', dialog => dialog.accept())
  let imageURL = ''
  for (const [path, selector] of [
    ['/problems/new', '#problem-source'],
    ['/my/contests/new', '#contest-description'],
    ['/blog/new', '#post-body'],
  ]) {
    await page.goto(path!)
    const editor = page.locator(selector!)
    await expect(editor).toBeEditable()
    const transfer = await page.evaluateHandle(async () => {
      const canvas = document.createElement('canvas')
      canvas.width = 2400; canvas.height = 1200
      const ctx = canvas.getContext('2d')!
      ctx.fillStyle = 'red'; ctx.fillRect(0, 0, 2000, 1000)
      const blob = await new Promise<Blob>(resolve => canvas.toBlob(value => resolve(value!), 'image/png'))
      const transfer = new DataTransfer()
      transfer.items.add(new File([blob], 'diagram.png', { type: 'image/png' }))
      return transfer
    })
    await editor.dispatchEvent('drop', { dataTransfer: transfer })
    await expect(editor).toContainText('![画像の説明](/api/images/')
    const url = (await editor.innerText()).match(/\/api\/images\/[a-f0-9-]+/)![0]
    if (imageURL) expect(url).toBe(imageURL)
    imageURL = url
    const image = page.locator(`.markdown-body img[src="${url}"]`)
    await expect.poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(2048)
    expect((await page.request.get(url)).headers()['cache-control']).toBe('no-store')
    await transfer.dispose()
  }
  const guest = await browser.newContext({ baseURL })
  expect((await guest.request.get(imageURL)).status()).toBe(404)
  expect((await page.request.post('/api/my/images', { headers: { origin: 'https://attacker.example' }, data: { data: '' } })).status()).toBe(403)
  const library = await (await page.request.get('/api/my/images')).json()
  expect(library.items).toHaveLength(1)
  expect(library.items[0].size).toBeLessThanOrEqual(512 * 1024)
  await page.getByLabel('タイトル', { exact: true }).fill('画像のある記事')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page).toHaveURL(/post=/)
  await page.getByRole('button', { name: '記事管理', exact: true }).click()
  await page.getByRole('button', { name: '公開する', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('公開しました')
  const publicImage = await guest.request.get(imageURL)
  expect(publicImage.status()).toBe(200)
  expect(publicImage.headers()['content-type']).toBe('image/png')
  const deleteURL = imageURL.replace('/api/images/', '/api/my/images/')
  const headers = { origin: baseURL! }
  expect((await page.request.delete(deleteURL, { headers })).status()).toBe(409)
  await page.getByRole('button', { name: '非公開に戻す', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('非公開に戻しました')
  expect((await guest.request.get(imageURL)).status()).toBe(404)
  await page.getByRole('button', { name: '本文', exact: true }).click()
  await page.locator('#post-body').fill('画像を外した本文')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.draft-status').getByRole('status')).toHaveText('保存済み')
  await page.getByRole('button', { name: '画像', exact: true }).click()
  await expect(page.getByRole('region', { name: '保存した画像' })).toBeVisible()
  const panel = page.getByRole('region', { name: '保存した画像' })
  await expect(panel.getByRole('button', { name: '画像を追加', exact: true })).toHaveText('追加')
  await expect(panel.getByRole('button', { name: '閉じる', exact: true })).toHaveText('')
  await expect(panel.getByRole('button', { name: '削除', exact: true })).toHaveText('')
  await panel.getByRole('button', { name: '画像を本文に挿入', exact: true }).click()
  await expect(page.locator('#post-body')).toContainText(imageURL)
  await expect(panel.getByRole('button', { name: '削除', exact: true })).toBeDisabled()
  await page.locator('#post-body').press('ControlOrMeta+z')
  await expect(page.locator('#post-body')).toHaveText('画像を外した本文')
  await page.getByRole('button', { name: 'ダークモードに切り替え' }).click()
  for (const width of [1280, 320, 375, 414, 768]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`image-panel-${width}.png`) })
  }
  await page.getByRole('region', { name: '保存した画像' }).getByRole('button', { name: '削除', exact: true }).click()
  await expect(page.getByText('保存した画像はありません。')).toBeVisible()
  expect((await page.request.get(imageURL)).status()).toBe(404)
  await guest.close()
})
