import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import { preferredRuntime, usableRuntimes } from '../app/utils/judge-catalog'

// Opens new checker and aggregator code and checks the runtime each starts in, the runtimes offered and the protocol.
async function expectNewCode(page: Page, runtime: string, options: string[], protocol: string) {
  await page.getByRole('group', { name: '生成・検証・判定の種類' }).getByRole('button', { name: '判定方法', exact: true }).click()
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
  await expect(page.getByLabel('検証コードの言語')).toHaveValue(runtime)
  await expect(page.getByLabel('検証コードの言語').locator('option')).toHaveText(options)
  await expect(page.getByLabel('判定コードの形式')).toHaveValue(protocol)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
  await page.getByRole('switch', { name: '集計コードで採点する' }).check()
  await expect(page.getByLabel('集計コードの言語')).toHaveValue(runtime)
  await expect(page.getByLabel('集計コードの言語').locator('option')).toHaveText(options)
}

// Every screen that picks a judge runtime offers none while the judge cannot take work, and new judge code starts in
// C++23 (GCC) when it is offered. The catalog arrives after hydration, so each test waits until it shows.
const unavailable = [
  // The server lists no runtime in maintenance; listing some shows that the screens follow the flag themselves.
  { state: 'is in maintenance', status: 200, json: { items: [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }], maintenance: true },
    banner: 'ジャッジ機能のメンテナンスを行っています。この期間中は提出等ができません。' },
  { state: 'cannot be reached', status: 502, json: {}, banner: 'ジャッジ機能の状態を確認できません。現在、提出等を利用できません。' },
]

for (const { state, status, json, banner } of unavailable) {
  test(`while the judge ${state}, the editors offer no runtime and new code names C++23 (GCC) as unavailable`, async ({ page }) => {
    await page.route('**/api/runtimes', route => route.fulfill({ status, json }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
    await expect(page.locator('.judge-banner')).toHaveText(banner)
    await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
    await expect(page.locator('.generator-options select option')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '生成する', exact: true })).toBeDisabled()
    await expectNewCode(page, 'cpp23-gcc', ['cpp23-gcc（現在利用できません）'], 'testlib')
  })

  test(`while the judge ${state}, the submission form offers no runtime`, async ({ page }) => {
    await page.route('**/api/runtimes', route => route.fulfill({ status, json }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/11111111-1111-4111-8111-111111111111')])
    await expect(page.locator('.judge-banner')).toHaveText(banner)
    await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
    await expect(page.getByRole('combobox', { name: '言語' }).locator('option')).toHaveText(['-- 未選択 --'])
    await expect(page.getByText('現在、提出受付を停止しています。', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '提出する', exact: true })).toBeDisabled()
  })
}

for (const { items, preferred, protocol } of [
  { items: [{ id: 'python314', label: 'Python' }, { id: 'cpp23-gcc', label: 'C++23' }], preferred: 'cpp23-gcc', protocol: 'testlib' },
  { items: [{ id: 'cpp17', label: 'C++17' }, { id: 'python314', label: 'Python' }], preferred: 'cpp17', protocol: 'legacy' },
]) {
  test(`new checker and aggregator code start in ${preferred} when the judge offers ${items.map(item => item.id).join(', ')}`, async ({ page }) => {
    const labels = items.map(item => item.label)
    await page.route('**/api/runtimes', route => route.fulfill({ json: { items } }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
    await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
    // These labels differ from the server-rendered catalog, so they show that the offered runtimes arrived.
    await expect(page.locator('.generator-options select option')).toHaveText(labels)
    await expect(page.getByRole('button', { name: '生成する', exact: true })).toBeEnabled()
    await expectNewCode(page, preferred, labels, protocol)
  })
}

test('no runtime is usable while the judge is in maintenance or its catalog could not be fetched', () => {
  const items = [{ id: 'cpp17', label: 'C++17' }, { id: 'python314', label: 'Python' }]
  expect(usableRuntimes({ items, maintenance: false }, undefined)).toBe(items)
  expect(usableRuntimes({ items }, null)).toBe(items)
  expect(usableRuntimes({ items, maintenance: true }, undefined)).toEqual([])
  expect(usableRuntimes({ items, maintenance: false }, new Error('502'))).toEqual([])
  for (const catalog of [undefined, null, {}]) expect(usableRuntimes(catalog, undefined)).toEqual([])
})

test('new judge code prefers C++23 (GCC), then the first runtime, and names C++23 (GCC) when none is offered', () => {
  expect(preferredRuntime([{ id: 'python314' }, { id: 'cpp23-gcc' }])).toBe('cpp23-gcc')
  expect(preferredRuntime([{ id: 'cpp17' }, { id: 'python314' }])).toBe('cpp17')
  expect(preferredRuntime([])).toBe('cpp23-gcc')
  expect(preferredRuntime([{ id: '' }, { id: 'cpp17' }])).toBe('')
})
