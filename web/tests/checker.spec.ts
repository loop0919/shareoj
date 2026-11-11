import type { Page } from '@playwright/test'
import { test, expect } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

async function openChecker(page: Page) {
  await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
  await page.getByRole('group', { name: '生成・検証・判定の種類' }).getByRole('button', { name: '判定方法', exact: true }).click()
}

for (const [method, label] of [['special', '検証コード'], ['interactive', '対話用ジャッジ']] as const) {
  test(`${method} defaults to testlib for C++23 and preserves its saved protocol`, async ({ page }) => {
    await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }] } }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
    await page.getByLabel('問題のタイトル').fill('testlib形式の保存')
    await openChecker(page)
    await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption(method)
    await expect(page.getByLabel(`${label}の言語`)).toHaveValue('cpp23-gcc')
    await expect(page.getByLabel('判定コードの形式')).toHaveValue('testlib')
    await page.getByLabel(label, { exact: true }).fill('#include "testlib.h"\nint main(){}')
    await page.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
    await page.reload()
    await openChecker(page)
    await expect(page.getByLabel('判定コードの形式')).toHaveValue('testlib')
    await page.getByLabel(`${label}の言語`).selectOption('python314')
    await expect(page.getByLabel('判定コードの形式')).toHaveValue('legacy')
    await expect(page.getByLabel(label, { exact: true })).toContainText('#include "testlib.h"')
    await expect(page.getByLabel('判定コードの形式').locator('option')).toHaveCount(1)
  })
}

for (const [method, label] of [['special', '検証コード'], ['interactive', '対話用ジャッジ']] as const) {
  test(`${method} code survives switching to normal and back`, async ({ page }) => {
    await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [{ id: 'cpp17', label: 'C++17' }, { id: 'python314', label: 'Python' }] } }))
    await page.goto('/problems/new?fresh=1')
    await page.getByLabel('問題のタイトル').fill('判定方法の切り替え')
    await openChecker(page)
    const selector = page.getByRole('combobox', { name: '判定方法', exact: true })
    await selector.selectOption(method)
    await page.getByLabel(`${label}の言語`).selectOption('python314')
    for (const source of ['print(42)', 'print(43)']) {
      await page.getByLabel(label, { exact: true }).fill(source)
      await selector.selectOption('normal')
      await expect(page.getByLabel(label, { exact: true })).toHaveCount(0)
      const saved = page.waitForResponse(response => response.url().includes('/api/my/problems/') && response.request().method() === 'PUT')
      await page.getByRole('button', { name: '保存', exact: true }).click()
      const body = (await saved).request().postDataJSON()
      expect(body.draft.checker).toBeNull()
      expect(body.draft.interactor).toBeNull()
      await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
      await page.getByRole('button', { name: '問題文', exact: true }).click()
      await openChecker(page)
      await selector.selectOption(method)
      await expect(page.getByLabel(`${label}の言語`)).toHaveValue('python314')
      await expect(page.getByLabel(label, { exact: true })).toHaveText(source)
    }
  })
}

test('checker languages match submissions and settings survive saving and reload', async ({ page }, testInfo) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [{ id: 'cpp17', label: 'C++17' }, { id: 'python314', label: 'Python' }] } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new')])
  await page.getByLabel('問題のタイトル').fill('構築問題')
  await openChecker(page)
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption({ label: 'スペシャルジャッジ' })
  await expect(page.getByLabel('検証コードの言語').locator('option')).toHaveText(['C++17', 'Python'])
  await page.getByLabel('検証コードの言語').selectOption('python314')
  await page.getByLabel('検証コード', { exact: true }).fill('import sys\nassert int(sys.stdin.read()) == 7')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  const id = new URL(page.url()).searchParams.get('problem')!
  expect(id).toBeTruthy()
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.reload()])
  await openChecker(page)
  await expect(page.getByLabel('検証コードの言語')).toHaveValue('python314')
  await expect(page.getByLabel('検証コード', { exact: true })).toContainText('assert int(sys.stdin.read()) == 7')
  await expect(page.getByRole('button', { name: '問題ページで提出する', exact: true })).toBeEnabled()
  await page.screenshot({ path: testInfo.outputPath('checker-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('checker-mobile.png'), fullPage: true })
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption({ label: '通常判定（空白区切りで比較）' })
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.reload()
  await openChecker(page)
  await expect(page.getByLabel('検証コードの言語')).toHaveCount(0)
})

test('interactive judging saves the selected language and mutually excludes special judging', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }] } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new')])
  await page.getByLabel('問題のタイトル').fill('対話問題')
  await openChecker(page)
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('interactive')
  await expect(page.getByLabel('対話用ジャッジの言語').locator('option')).toHaveText(['C++23', 'Python'])
  await page.getByLabel('対話用ジャッジの言語').selectOption('python314')
  await page.getByLabel('対話用ジャッジ', { exact: true }).fill('print(10, flush=True)\nassert int(input()) == 20')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.reload()
  await openChecker(page)
  await expect(page.getByRole('combobox', { name: '判定方法', exact: true })).toHaveValue('interactive')
  await expect(page.getByLabel('対話用ジャッジ', { exact: true })).toContainText('assert int(input()) == 20')
  await expect(page.getByText('各ケースCPU 5秒・256 MiB', { exact: false })).toBeHidden()
  await page.getByText('対話用ジャッジの仕様', { exact: true }).click()
  await expect(page.getByText('各ケースCPU 5秒・256 MiB', { exact: false })).toBeVisible()
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.reload()
  await openChecker(page)
  await expect(page.getByRole('combobox', { name: '判定方法', exact: true })).toHaveValue('special')
  await expect(page.getByLabel('検証コードの言語')).toHaveValue('python314')
})

test('judge tab saves the draft and opens the problem page to submit', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('判定を試す')
  await openChecker(page)
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
  const open = page.getByRole('button', { name: '問題ページで提出する', exact: true })
  await expect(open).toBeDisabled()
  await page.getByLabel('検証コード', { exact: true }).fill('int main(){}')
  const saved = page.waitForRequest(request => request.method() === 'PUT' && /^\/api\/my\/problems\/[^/]+$/.test(new URL(request.url()).pathname))
  await open.click()
  expect((await saved).postDataJSON().draft.checker.source).toBe('int main(){}')
  await expect(page).toHaveURL(/\/problems\/[0-9A-Za-z]{22}#submission-title$/)
})

const runTwiceLabel = '2ステップ用ジャッジ'
const runtimes = [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }]

test('2ステップ形式を選ぶと下書きに runTwice が入る', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: runtimes } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
  await page.getByLabel('問題のタイトル').fill('2ステップ問題')
  await openChecker(page)
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption({ label: '2ステップ形式（Run-twice）' })
  await page.getByLabel(`${runTwiceLabel}の言語`).selectOption('python314')
  await page.getByLabel(runTwiceLabel, { exact: true }).fill('print(1)')
  const saved = page.waitForResponse(response => response.url().includes('/api/my/problems/') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const body = (await saved).request().postDataJSON()
  expect(body.draft.runTwice).toEqual({ runtime: 'python314', source: 'print(1)', protocol: 'legacy' })
  expect(body.draft.checker).toBeNull()
  expect(body.draft.interactor).toBeNull()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.reload()
  await openChecker(page)
  await expect(page.getByRole('combobox', { name: '判定方法', exact: true })).toHaveValue('runTwice')
})

test('対話形式から2ステップ形式へ切り替えるとコードが引き継がれる', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: runtimes } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
  await page.getByLabel('問題のタイトル').fill('判定方法の切り替え')
  await openChecker(page)
  const selector = page.getByRole('combobox', { name: '判定方法', exact: true })
  await selector.selectOption('interactive')
  await page.getByLabel('対話用ジャッジの言語').selectOption('python314')
  await page.getByLabel('対話用ジャッジ', { exact: true }).fill('print(7)')
  await selector.selectOption('runTwice')
  await expect(page.getByLabel(`${runTwiceLabel}の言語`)).toHaveValue('python314')
  await expect(page.getByLabel(runTwiceLabel, { exact: true })).toHaveText('print(7)')
  const saved = page.waitForResponse(response => response.url().includes('/api/my/problems/') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const body = (await saved).request().postDataJSON()
  expect(body.draft.interactor).toBeNull()
  expect(body.draft.checker).toBeNull()
  expect(body.draft.runTwice.source).toBe('print(7)')
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
})

test('2ステップ形式の説明に引数が出る', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: runtimes } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
  await openChecker(page)
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('runTwice')
  await page.getByText(`${runTwiceLabel}の仕様`, { exact: true }).click()
  await expect(page.getByLabel('判定コードの形式')).toHaveValue('testlib')
  await expect(page.getByText('1回目に tout へ書いた内容が、2回目の入力（inf）になります。', { exact: false })).toBeVisible()
  await expect(page.getByText('各ケースCPU 5秒・256 MiB', { exact: false })).toBeVisible()
  const example = page.getByRole('link', { name: 'コード例 ↗' })
  await expect(example).toHaveAttribute('href', '/docs/checkers#run-twice')
  await page.getByLabel('判定コードの形式').selectOption('legacy')
  await expect(page.getByText('何回目か（1か2）、引き継ぎファイルのパス（この順）。', { exact: false })).toBeVisible()
  await expect(page.getByText('引き継ぎファイルは1回目に書き、2回目に読みます。', { exact: false })).toBeVisible()
  const secondRunScore = page.getByText('読むのは2回目に書いた内容だけです。', { exact: false })
  await expect(secondRunScore).toBeVisible()
  // An interactor runs once, so its help keeps the single-run score and the testlib example.
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('interactive')
  await expect(secondRunScore).toHaveCount(0)
  await page.getByLabel('判定コードの形式').selectOption('testlib')
  await expect(example).toHaveAttribute('href', '/docs/checkers#testlib')
})

test('問題ページに2ステップ形式と表示する', async ({ page }) => {
  const notice = '2ステップ形式（Run-twice）：プログラムは1ケースにつき2回実行されます。応答を待つ前に出力をflushしてください。'
  await page.goto(`/problems/${shortId('adadadad-adad-4dad-8dad-adadadadadad')}`)
  await expect(page.getByText(notice, { exact: true })).toBeVisible()
  await page.goto(`/contests/${shortId('aeaeaeae-aeae-4eae-8eae-aeaeaeaeaeae')}/A`)
  await expect(page.getByText(notice, { exact: true })).toBeVisible()
  await page.goto(`/problems/${shortId('11111111-1111-4111-8111-111111111111')}`)
  await expect(page.getByText('2ステップ形式', { exact: false })).toHaveCount(0)
})

// Saves the draft with the save button and returns the draft the request carried.
async function saveWithButton(page: Page) {
  const saved = page.waitForResponse(response => response.url().includes('/api/my/problems/') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const body = (await saved).request().postDataJSON()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  return body.draft
}
// The judging methods a draft sets; at most one of the four.
const judges = (draft: Record<string, unknown>) => ['checker', 'interactor', 'runTwice', 'errorJudge'].filter(key => draft[key] !== null)
const errorJudgeDefaults = { mode: 'either', epsilon: '1e-6', marginPercent: '0.1' }

function errorJudgeFields(page: Page) {
  return {
    method: page.getByRole('combobox', { name: '判定方法', exact: true }),
    kind: page.getByRole('combobox', { name: '種類', exact: true }),
    epsilon: page.getByRole('combobox', { name: '許容誤差', exact: true }),
    margin: page.getByRole('textbox', { name: '判定の余裕', exact: true }),
  }
}

test('誤差ジャッジを選ぶと下書きに errorJudge が入る', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: runtimes } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
  await page.getByLabel('問題のタイトル').fill('誤差ジャッジの問題')
  await openChecker(page)
  const { method, kind, epsilon, margin } = errorJudgeFields(page)
  await method.selectOption({ label: '誤差ジャッジ' })
  await expect(kind).toHaveValue('either')
  await expect(kind.locator('option')).toHaveText(['絶対誤差', '相対誤差', '絶対誤差または相対誤差'])
  await expect(epsilon).toHaveValue('1e-6')
  await expect(epsilon.locator('option')).toHaveText(Array.from({ length: 12 }, (_, index) => `10^-${index + 1}`))
  await expect(margin).toHaveValue('0.1')
  await expect(page.getByText('%', { exact: true })).toBeVisible()
  // No author code: neither the language, the protocol, the code nor its spec shows.
  for (const label of ['検証コード', '検証コードの言語', '判定コードの形式']) await expect(page.getByLabel(label, { exact: true })).toHaveCount(0)
  await expect(page.getByText('検証コードの仕様', { exact: true })).toHaveCount(0)
  await expect(page.getByText('数として読めるトークンだけを誤差で比べ、それ以外は完全一致で比べます。', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: '誤差ジャッジの説明 ↗' })).toHaveAttribute('href', '/docs/checkers#error-judge')
  let draft = await saveWithButton(page)
  expect(draft.errorJudge).toEqual(errorJudgeDefaults)
  expect(draft.checker).toBeNull()
  expect(draft.interactor).toBeNull()
  expect(draft.runTwice).toBeNull()
  await kind.selectOption({ label: '相対誤差' })
  await epsilon.selectOption({ label: '10^-9' })
  await margin.fill('0.5')
  draft = await saveWithButton(page)
  expect(draft.errorJudge).toEqual({ mode: 'relative', epsilon: '1e-9', marginPercent: '0.5' })
  await page.reload()
  await openChecker(page)
  await expect(method).toHaveValue('errorJudge')
  await expect(kind).toHaveValue('relative')
  await expect(epsilon).toHaveValue('1e-9')
  await expect(margin).toHaveValue('0.5')
  // A draft without the margin, as the CLI may save it, shows the default the judge uses.
  const id = new URL(page.url()).searchParams.get('problem')!
  await page.evaluate(async id => {
    const stored = await (await fetch(`/api/my/problems/${id}`)).json()
    const draft = { ...stored.draft, errorJudge: { mode: 'absolute', epsilon: '1e-3' } }
    await fetch(`/api/my/problems/${id}`, { method: 'PUT', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ version: stored.version, draft }) })
  }, id)
  await page.reload()
  await openChecker(page)
  await expect(margin).toHaveValue('0.1')
  await expect(page.getByText('期待出力との誤差が 1.001 × 10-3 以下なら正解になります。', { exact: true })).toBeVisible()
})

test('しきい値の説明が入力に合わせて変わる', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await openChecker(page)
  const { method, kind, epsilon, margin } = errorJudgeFields(page)
  await method.selectOption('errorJudge')
  const line = (text: string) => page.getByText(text, { exact: true })
  const widened = line('想定解の出力も真の値からわずかにずれるので、その分だけ広げて比べます。')
  const comparison = page.getByText('数として読めるトークンだけを誤差で比べ、それ以外は完全一致で比べます。', { exact: false })
  // The default kind is 絶対誤差または相対誤差.
  const initial = line('期待出力との絶対誤差または相対誤差が 1.001 × 10-6 以下なら正解になります。')
  await expect(initial).toBeVisible()
  await expect(initial.locator('sup')).toHaveText('-6')
  await expect(widened).toBeVisible()
  await expect(comparison).toBeVisible()
  await kind.selectOption('absolute')
  await expect(line('期待出力との誤差が 1.001 × 10-6 以下なら正解になります。')).toBeVisible()
  await epsilon.selectOption('1e-9')
  await margin.fill('0.5')
  await expect(line('期待出力との誤差が 1.005 × 10-9 以下なら正解になります。')).toBeVisible()
  await kind.selectOption('relative')
  const relative = line('期待出力との相対誤差が 1.005 × 10-9 以下なら正解になります。')
  await expect(relative).toBeVisible()
  await expect(relative.locator('sup')).toHaveText('-9')
  for (const [value, factor] of [['12.345678', '1.12345678'], ['100', '2'], ['0.000001', '1.00000001']]) {
    await margin.fill(value)
    await expect(line(`期待出力との相対誤差が ${factor} × 10-9 以下なら正解になります。`)).toBeVisible()
  }
  // Without a margin the two lines become one, for each kind, and the power of ten stands alone.
  await margin.fill('0')
  for (const [value, phrase] of [['relative', '相対誤差が'], ['absolute', '誤差が'], ['either', '絶対誤差または相対誤差が']] as const) {
    await kind.selectOption(value)
    const strict = line(`期待出力との${phrase} 10-9 を少しでも超えると不正解になります。`)
    await expect(strict).toBeVisible()
    await expect(strict.locator('sup')).toHaveText('-9')
    await expect(page.getByText('以下なら正解になります。', { exact: false })).toHaveCount(0)
    await expect(widened).toHaveCount(0)
    await expect(comparison).toBeVisible()
  }
  await kind.selectOption('absolute')
  // An invalid margin replaces the threshold line with the error, for each kind.
  await margin.fill('101')
  for (const value of ['absolute', 'relative', 'either']) {
    await kind.selectOption(value)
    await expect(line('判定の余裕は0〜100の数で、小数は6桁までです。')).toBeVisible()
    await expect(page.getByText('以下なら正解になります。', { exact: false })).toHaveCount(0)
    await expect(page.getByText('を少しでも超えると不正解になります。', { exact: false })).toHaveCount(0)
    await expect(widened).toBeVisible()
    await expect(comparison).toBeVisible()
  }
  await margin.fill('0.1')
  await expect(line('期待出力との絶対誤差または相対誤差が 1.001 × 10-9 以下なら正解になります。')).toBeVisible()
  await expect(line('判定の余裕は0〜100の数で、小数は6桁までです。')).toHaveCount(0)
})

test('余裕の誤りは保存しない', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('余裕の誤り')
  await openChecker(page)
  const { method, margin } = errorJudgeFields(page)
  await method.selectOption('errorJudge')
  expect((await saveWithButton(page)).errorJudge).toEqual(errorJudgeDefaults)
  const writes: unknown[] = []
  page.on('request', request => { if (request.method() === 'PUT' && /^\/api\/my\/problems\/[^/]+$/.test(new URL(request.url()).pathname)) writes.push(request.postDataJSON().draft.errorJudge) })
  const message = '判定の余裕は0〜100の数で、小数は6桁までです。'
  const inline = page.getByRole('region', { name: '判定方法' }).getByText(message, { exact: true })
  // A refused save says why where other refused saves do.
  const refusal = page.getByRole('alert').filter({ hasText: message })
  const state = page.locator('[data-save-state]')
  const open = page.getByRole('button', { name: '問題ページで提出する', exact: true })
  await expect(open).toBeEnabled()
  for (const value of ['', '101', '0.1.2', '0.1234567', '-1', '1,5']) {
    await margin.fill(value)
    await expect(inline).toBeVisible()
    await expect(margin).toHaveAttribute('aria-invalid', 'true')
    await expect(page.getByText('以下なら正解になります。', { exact: false })).toHaveCount(0)
    await expect(open).toBeDisabled()
  }
  // The invalid margin is an unsaved edit that the autosave refuses.
  await expect(state).toHaveAttribute('data-save-state', 'dirty')
  await expect(refusal).toBeVisible()
  // Saving by hand, by the button or the shortcut, sends nothing and shows no success.
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await page.keyboard.press('Control+s')
  // Wait beyond the autosave debounce to prove no write occurs.
  await page.waitForTimeout(900)
  expect(writes).toEqual([])
  await expect(state).toHaveAttribute('data-save-state', 'dirty')
  await expect(refusal).toBeVisible()
  await expect(page.getByText('保存しました。', { exact: true })).toHaveCount(0)
  await expect(margin).toHaveValue('1,5')
  // A corrected margin saves as usual.
  await margin.fill('2.5')
  await expect(inline).toHaveCount(0)
  await expect(margin).not.toHaveAttribute('aria-invalid', 'true')
  await expect(open).toBeEnabled()
  expect((await saveWithButton(page)).errorJudge).toEqual({ ...errorJudgeDefaults, marginPercent: '2.5' })
  await expect(refusal).toHaveCount(0)
  expect(writes.length).toBeGreaterThan(0)
  expect(writes.every(value => (value as { marginPercent: string }).marginPercent === '2.5')).toBe(true)
  // Going back to the saved margin after an invalid one leaves nothing to save and clears the refusal.
  const count = writes.length
  await margin.fill('x')
  await expect(refusal).toBeVisible()
  await margin.fill('2.5')
  await expect(refusal).toHaveCount(0)
  await expect(state).toHaveAttribute('data-save-state', 'saved')
  expect(writes.length).toBe(count)
})

test('余裕の誤りがあるうちは移動の前に確かめる', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('余裕の誤りと移動')
  await openChecker(page)
  const { method, margin } = errorJudgeFields(page)
  await method.selectOption('errorJudge')
  await saveWithButton(page)
  let writes = 0
  page.on('request', request => { if (request.method() === 'PUT' && /^\/api\/my\/problems\/[^/]+$/.test(new URL(request.url()).pathname)) writes++ })
  await margin.fill('101')
  await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '保存して移動', exact: true }).click()
  await expect(dialog.getByText('保存できませんでした。編集を続けるか、保存せずに移動してください。', { exact: true })).toBeVisible()
  expect(writes).toBe(0)
  await dialog.getByRole('button', { name: '保存せずに移動', exact: true }).click()
  await expect(page).toHaveURL('/my')
  expect(writes).toBe(0)
})

test('誤差ジャッジからほかの判定方法へ切り替えると errorJudge が null になる', async ({ page }) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: runtimes } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
  await page.getByLabel('問題のタイトル').fill('判定方法の切り替え')
  await openChecker(page)
  const { method, kind, margin } = errorJudgeFields(page)
  await method.selectOption('errorJudge')
  await method.selectOption('special')
  await expect(page.getByLabel('検証コード', { exact: true })).toBeVisible()
  await expect(kind).toHaveCount(0)
  await expect(margin).toHaveCount(0)
  await page.getByLabel('検証コードの言語').selectOption('python314')
  await page.getByLabel('検証コード', { exact: true }).fill('print(1)')
  let draft = await saveWithButton(page)
  expect(judges(draft)).toEqual(['checker'])
  expect(draft.errorJudge).toBeNull()
  expect(draft.checker).toEqual({ runtime: 'python314', source: 'print(1)', protocol: 'legacy' })
  // Back to the error judge, which starts from its defaults; the code is set aside, as for 通常判定.
  await method.selectOption('errorJudge')
  await expect(page.getByLabel('検証コード', { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('検証コードの言語')).toHaveCount(0)
  await expect(kind).toHaveValue('either')
  draft = await saveWithButton(page)
  expect(judges(draft)).toEqual(['errorJudge'])
  expect(draft.errorJudge).toEqual(errorJudgeDefaults)
  for (const [value, key, label] of [['interactive', 'interactor', '対話用ジャッジ'], ['runTwice', 'runTwice', runTwiceLabel], ['special', 'checker', '検証コード']] as const) {
    await method.selectOption(value)
    await expect(page.getByLabel(label, { exact: true })).toHaveText('print(1)')
    draft = await saveWithButton(page)
    expect(judges(draft)).toEqual([key])
    await method.selectOption('errorJudge')
    draft = await saveWithButton(page)
    expect(judges(draft)).toEqual(['errorJudge'])
  }
  await method.selectOption('normal')
  draft = await saveWithButton(page)
  expect(judges(draft)).toEqual([])
})

test('問題ページに誤差ジャッジの条件を表示する', async ({ page, context }) => {
  const notice = '誤差ジャッジ：絶対誤差または相対誤差が10-6以下なら正解です。'
  await page.goto(`/problems/${shortId('afafafaf-afaf-4faf-8faf-afafafafafaf')}`)
  await expect(page.getByText(notice, { exact: true })).toBeVisible()
  await expect(page.getByText(notice, { exact: true }).locator('sup')).toHaveText('-6')
  await page.goto(`/contests/${shortId('b0b0b0b0-b0b0-4b0b-8b0b-b0b0b0b0b0b0')}/A`)
  await expect(page.getByText(notice, { exact: true })).toBeVisible()
  await expect(page.getByText(notice, { exact: true }).locator('sup')).toHaveText('-6')
  await page.goto(`/problems/${shortId('11111111-1111-4111-8111-111111111111')}`)
  await expect(page.getByText('誤差ジャッジ', { exact: false })).toHaveCount(0)
  // The author's preview of an unpublished problem shows the condition too, and never the margin.
  const draftId = 'b1b1b1b1-b1b1-4b1b-8b1b-b1b1b1b1b1b1'
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }])
  await page.goto(`/problems/${shortId(draftId)}`)
  await expect(page.getByText('誤差ジャッジ：相対誤差が10-9以下なら正解です。', { exact: true })).toBeVisible()
  const preview = await (await context.request.get(`/api/problems/${draftId}`)).json()
  expect(preview.errorJudge).toEqual({ mode: 'relative', epsilon: '1e-9' })
})
