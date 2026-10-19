import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'

// The fixture keeps what the editor saved; reading it back shows exactly what the server would receive (ADR 0025).
async function stored(page: Page) {
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  const id = new URL(page.url()).searchParams.get('problem')!
  return (await page.evaluate(async id => (await fetch(`/api/my/problems/${id}`)).json(), id)).draft
}

async function addCases(page: Page, count: number) {
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  for (let i = 0; i < count; i++) await page.getByRole('button', { name: 'テストケースを追加' }).click()
}

test('the scoring editor switches the method and the aggregator, edits groups and membership, and keeps them through saving and reload', async ({ page }, testInfo) => {
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }] } }))
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('部分点の問題')
  await addCases(page, 3)
  await page.getByRole('button', { name: 'case_001.txt', exact: true }).click()
  await page.getByRole('checkbox', { name: 'サンプルケースにする' }).check()
  // An all-or-nothing problem has no groups to join.
  await expect(page.getByRole('group', { name: '所属するグループ' })).toHaveCount(0)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const methods = page.getByRole('group', { name: '採点方式' })
  const aggregate = page.getByRole('switch', { name: '集計コードで採点する' })
  await expect(methods.locator('label')).toHaveText(['一括採点', '部分点採点'])
  await expect(methods.getByRole('radio', { name: '一括採点' })).toBeChecked()
  await expect(page.locator('.scoring-mode .scoring-note')).toHaveText(['すべてのケースでACなら正解です。部分点はありません。'])
  await expect(aggregate).toHaveCount(0)
  // 部分点採点 starts with the switch off and one group of every case but the samples, which still scores like 一括採点.
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await expect(aggregate).not.toBeChecked()
  await expect(aggregate).toHaveAccessibleDescription('グループに属するケースがすべてACなら、そのグループの配点を与えます。')
  await expect(page.locator('.scoring-mode .scoring-note')).toHaveText([
    'ケースをグループに分け、グループごとに得点を付けます。提出の得点は、グループの得点の合計です。',
    'グループに属するケースがすべてACなら、そのグループの配点を与えます。',
  ])
  await expect(page.getByRole('heading', { name: '集計コード', exact: true })).toHaveCount(0)
  await expect(page.getByLabel('グループ1の名前')).toHaveValue('小課題1')
  await expect(page.getByLabel('グループ1の配点')).toHaveValue('100')
  await expect(page.locator('.scoring-heading')).toHaveText('グループ満点 100 点')
  await expect(page.locator('.group-count')).toHaveText(['2 件'])
  let draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 100 }] })
  expect(draft.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([undefined, ['小課題1'], ['小課題1']])

  // A new group starts at 0 points with its name selected for typing.
  await page.getByRole('button', { name: 'グループを追加' }).click()
  const second = page.getByLabel('グループ2の名前')
  await expect(second).toBeFocused()
  await second.fill('小課題1')
  await second.press('Enter')
  await expect(page.getByRole('alert').filter({ hasText: 'グループ名「小課題1」が重複しています。' })).toBeVisible()
  await second.fill('大きいケース')
  await second.press('Enter')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await page.getByLabel('グループ1の配点').fill('30')
  await page.getByLabel('グループ2の配点').fill('70')
  await expect(page.locator('.scoring-heading')).toHaveText('グループ満点 100 点')
  // An empty group is pointed out, and so is a case in no group.
  await expect(page.locator('.group-count').nth(1)).toHaveText('0 件')
  await page.getByLabel('グループ2の配点').fill('')
  await page.getByLabel('グループ2の配点').blur()
  await expect(page.getByRole('alert')).toHaveText('配点は0〜1,000,000点の整数で入力してください。')
  await page.getByLabel('グループ2の配点').fill('70')
  await expect(page.getByRole('alert')).toHaveCount(0)

  // Cases join groups with chips; membership follows the groups' order.
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  await page.getByRole('button', { name: 'case_003.txt', exact: true }).click()
  const chips = page.getByRole('group', { name: '所属するグループ' })
  await chips.getByRole('checkbox', { name: '大きいケース' }).check()
  await expect(chips.getByRole('checkbox', { name: '小課題1' })).toBeChecked()
  await page.getByRole('button', { name: 'case_002.txt', exact: true }).click()
  await chips.getByRole('checkbox', { name: '小課題1' }).uncheck()
  await expect(page.getByRole('button', { name: 'case_002.txt 無所属', exact: true })).toBeVisible()
  await expect(chips).toContainText('サンプル以外のケースは、どれかのグループに入れてください。')
  // New cases join no group.
  await page.getByRole('button', { name: 'テストケースを追加' }).click()
  await expect(chips.getByRole('checkbox')).toHaveCount(2)
  await expect(chips.getByRole('checkbox', { checked: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'case_004.txt 無所属', exact: true }).click()
  await chips.getByRole('checkbox', { name: '小課題1' }).check()
  await page.getByRole('button', { name: 'case_002.txt 無所属', exact: true }).click()
  await chips.getByRole('checkbox', { name: '大きいケース' }).check()
  await chips.getByRole('checkbox', { name: '小課題1' }).check()
  draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 30 }, { name: '大きいケース', points: 70 }] })
  expect(draft.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([undefined, ['小課題1', '大きいケース'], ['小課題1', '大きいケース'], ['小課題1']])

  // Renaming a group carries its members along; removing one takes it out of every case.
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await page.getByLabel('グループ2の名前').fill('  large  ')
  await page.getByLabel('グループ2の名前').blur()
  await expect(page.getByLabel('グループ2の名前')).toHaveValue('large')
  draft = await stored(page)
  expect(draft.scoring.groups).toEqual([{ name: '小課題1', points: 30 }, { name: 'large', points: 70 }])
  expect(draft.testCases[1].groups).toEqual(['小課題1', 'large'])
  await page.getByRole('button', { name: 'グループを追加' }).click()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '「小課題3」を削除' }).click()
  await expect(page.getByLabel('グループ3の名前')).toHaveCount(0)

  // Turning the switch on keeps the groups and adds the aggregator; the checker explains the case scores.
  await aggregate.check()
  await expect(methods.getByRole('radio', { name: '部分点採点' })).toBeChecked()
  await expect(aggregate).toHaveAccessibleDescription('集計コードが、ケースの判定と点からグループごとの得点を決めます。コードは下の「集計コード」に書きます。')
  await expect(page.getByRole('heading', { name: '集計コード', exact: true })).toBeVisible()
  await expect(page.getByLabel('集計コードの言語')).toHaveValue('cpp23-gcc')
  await page.getByLabel('集計コードの言語').selectOption('python314')
  await expect(page.locator('.scoring-block').getByRole('alert')).toContainText('集計コードを1〜65,536バイトで')
  await page.getByText('集計コードの仕様').click()
  await expect(page.locator('.scoring-editor .code-spec dl')).toContainText('SKIPPED（未実行）')
  await page.getByRole('textbox', { name: '集計コード', exact: true }).fill('n, g = map(int, input().split())')
  draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'aggregate', groups: [{ name: '小課題1', points: 30 }, { name: 'large', points: 70 }], aggregator: { runtime: 'python314', source: 'n, g = map(int, input().split())' } })
  await page.screenshot({ path: testInfo.outputPath('scoring-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('scoring-mobile.png'), fullPage: true })
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
  await page.getByRole('group', { name: '生成・検証・判定の種類' }).getByRole('button', { name: '判定方法', exact: true }).click()
  await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
  await expect(page.getByLabel('判定コードの形式')).toHaveValue('testlib')
  await expect(page.locator('.checker-settings .code-spec dl')).toContainText('quitp に整数（0〜1,000,000）を渡すと、正解とし、その値をケースの点にします。')
  await page.getByLabel('判定コードの形式').selectOption('legacy')
  await expect(page.locator('.checker-settings .code-spec dl')).toContainText('書いた整数（0〜1018）をケースの点として集計コードに渡します。')

  // Everything comes back after a reload.
  await page.getByLabel('検証コード', { exact: true }).fill('int main() {}')
  await stored(page)
  await page.reload()
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await expect(methods.getByRole('radio', { name: '部分点採点' })).toBeChecked()
  await expect(aggregate).toBeChecked()
  await expect(page.getByLabel('グループ2の名前')).toHaveValue('large')
  await expect(page.getByLabel('グループ1の配点')).toHaveValue('30')
  await expect(page.getByLabel('集計コードの言語')).toHaveValue('python314')
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('n, g = map(int, input().split())')
  await expect(page.locator('.group-count')).toHaveText(['3 件', '2 件'])

  // Turning the switch off takes the aggregator out of the draft, so it asks first; declining keeps everything.
  const messages: string[] = []
  page.once('dialog', dialog => { messages.push(dialog.message()); return dialog.dismiss() })
  await aggregate.click()
  await expect(aggregate).toBeChecked()
  await expect(methods.getByRole('radio', { name: '部分点採点' })).toBeChecked()
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('n, g = map(int, input().split())')
  page.once('dialog', dialog => { messages.push(dialog.message()); return dialog.accept() })
  await aggregate.uncheck()
  await expect(aggregate).toHaveAccessibleDescription('グループに属するケースがすべてACなら、そのグループの配点を与えます。')
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveCount(0)
  draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 30 }, { name: 'large', points: 70 }] })
  // Back to 一括採点 the membership goes too, after confirming. The page keeps both while it is open, in any section.
  page.once('dialog', dialog => { messages.push(dialog.message()); return dialog.accept() })
  await methods.getByRole('radio', { name: '一括採点' }).check()
  await expect(page.getByLabel('グループ1の名前')).toHaveCount(0)
  await expect(aggregate).toHaveCount(0)
  expect(messages).toEqual([
    '「集計コードで採点する」をオフにすると、集計コードを下書きから外します。このページを開いている間は、オンに戻すと元に戻ります。オフにしますか？',
    '「集計コードで採点する」をオフにすると、集計コードを下書きから外します。このページを開いている間は、オンに戻すと元に戻ります。オフにしますか？',
    '一括採点にすると、グループと配点、ケースの所属を下書きから外します。このページを開いている間は、部分点採点に戻すと元に戻ります。一括採点にしますか？',
  ])
  draft = await stored(page)
  expect(draft.scoring).toBeNull()
  expect(draft.testCases.every((item: { groups?: string[] }) => item.groups === undefined)).toBe(true)
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await expect(methods.getByRole('radio', { name: '一括採点' })).toBeChecked()
  // 部分点採点 comes back with the switch as it was left, and the aggregator taken out before comes back with it on.
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await expect(aggregate).not.toBeChecked()
  await expect(page.getByLabel('グループ2の名前')).toHaveValue('large')
  await aggregate.check()
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('n, g = map(int, input().split())')
  draft = await stored(page)
  expect(draft.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([undefined, ['小課題1', 'large'], ['小課題1', 'large'], ['小課題1']])
})

test('declining a switch keeps the method and the switch where they were, with focus on the control used', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('取り消す切り替え')
  await addCases(page, 2)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const methods = page.getByRole('group', { name: '採点方式' })
  const aggregate = page.getByRole('switch', { name: '集計コードで採点する' })
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await aggregate.check()
  await page.getByRole('textbox', { name: '集計コード', exact: true }).fill('print(100)')
  await stored(page)
  const messages: string[] = []
  page.on('dialog', dialog => { messages.push(dialog.message()); return dialog.dismiss() })
  const unchanged = async (focused: 'method' | 'switch') => {
    await expect(methods.getByRole('radio', { name: '部分点採点' })).toBeChecked()
    await expect(methods.getByRole('radio', { name: '一括採点' })).not.toBeChecked()
    await expect(aggregate).toBeChecked()
    await expect(focused === 'method' ? methods.getByRole('radio', { name: '部分点採点' }) : aggregate).toBeFocused()
  }
  // With the pointer.
  await methods.getByRole('radio', { name: '一括採点' }).click()
  await unchanged('method')
  await aggregate.click()
  await unchanged('switch')
  // With the keyboard: the arrow keys move focus and selection together, and Space toggles the switch.
  await methods.getByRole('radio', { name: '部分点採点' }).focus()
  await page.keyboard.press('ArrowLeft')
  await unchanged('method')
  await aggregate.focus()
  await page.keyboard.press('Space')
  await unchanged('switch')
  expect(messages).toEqual([
    '一括採点にすると、グループと配点、ケースの所属、集計コードを下書きから外します。このページを開いている間は、部分点採点に戻すと元に戻ります。一括採点にしますか？',
    '「集計コードで採点する」をオフにすると、集計コードを下書きから外します。このページを開いている間は、オンに戻すと元に戻ります。オフにしますか？',
    '一括採点にすると、グループと配点、ケースの所属、集計コードを下書きから外します。このページを開いている間は、部分点採点に戻すと元に戻ります。一括採点にしますか？',
    '「集計コードで採点する」をオフにすると、集計コードを下書きから外します。このページを開いている間は、オンに戻すと元に戻ります。オフにしますか？',
  ])
  await expect(page.getByLabel('グループ1の名前')).toHaveValue('小課題1')
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('print(100)')
  const draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'aggregate', groups: [{ name: '小課題1', points: 100 }], aggregator: { runtime: 'cpp23-gcc', source: 'print(100)' } })
  expect(draft.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([['小課題1'], ['小課題1']])
})

test('the switch starts off, turns on and off without asking while there is no code, and brings the code back', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('スイッチの往復')
  await addCases(page, 1)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const aggregate = page.getByRole('switch', { name: '集計コードで採点する' })
  let asked = 0
  page.on('dialog', dialog => { asked++; return dialog.accept() })
  await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
  await expect(aggregate).not.toBeChecked()
  expect((await stored(page)).scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 100 }] })
  // On shows the aggregator below with a fresh, empty code; off again asks nothing while the code is empty.
  await aggregate.check()
  await expect(page.getByLabel('集計コードの言語')).toBeVisible()
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('')
  expect((await stored(page)).scoring).toEqual({ mode: 'aggregate', groups: [{ name: '小課題1', points: 100 }], aggregator: { runtime: 'cpp23-gcc', source: '' } })
  await aggregate.uncheck()
  await expect(page.getByLabel('集計コードの言語')).toHaveCount(0)
  expect(asked).toBe(0)
  expect((await stored(page)).scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 100 }] })
  // With code, off asks; the code comes back when the switch is on again, in another section's round trip too.
  await aggregate.check()
  await page.getByRole('textbox', { name: '集計コード', exact: true }).fill('print(100)')
  expect((await stored(page)).scoring.aggregator).toEqual({ runtime: 'cpp23-gcc', source: 'print(100)' })
  await aggregate.uncheck()
  expect(asked).toBe(1)
  await expect(aggregate).not.toBeChecked()
  expect((await stored(page)).scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 100 }] })
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await expect(aggregate).not.toBeChecked()
  await aggregate.check()
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('print(100)')
  expect((await stored(page)).scoring).toEqual({ mode: 'aggregate', groups: [{ name: '小課題1', points: 100 }], aggregator: { runtime: 'cpp23-gcc', source: 'print(100)' } })
})

test('部分点採点 comes back as 一括採点 left it, and starts afresh when nothing was taken out', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('戻す部分点')
  await addCases(page, 2)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const methods = page.getByRole('group', { name: '採点方式' })
  const aggregate = page.getByRole('switch', { name: '集計コードで採点する' })
  let asked = 0
  page.on('dialog', dialog => { asked++; return dialog.accept() })
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await page.getByLabel('グループ1の名前').fill('全体')
  await page.getByLabel('グループ1の名前').press('Enter')
  await aggregate.check()
  await page.getByRole('textbox', { name: '集計コード', exact: true }).fill('print(100)')
  await methods.getByRole('radio', { name: '一括採点' }).check()
  expect(asked).toBe(1)
  await expect(aggregate).toHaveCount(0)
  expect((await stored(page)).scoring).toBeNull()
  // The switch, the code, the groups and the membership all return at once.
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await expect(aggregate).toBeChecked()
  await expect(page.getByLabel('グループ1の名前')).toHaveValue('全体')
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('print(100)')
  let draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'aggregate', groups: [{ name: '全体', points: 100 }], aggregator: { runtime: 'cpp23-gcc', source: 'print(100)' } })
  expect(draft.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([['全体'], ['全体']])

  // With no groups and no code left, leaving asks nothing, and the next switch starts off with the usual first group.
  await aggregate.uncheck()
  await page.getByRole('button', { name: '「全体」を削除' }).click()
  await expect(page.getByLabel('グループ1の名前')).toHaveCount(0)
  expect(asked).toBe(3)
  await methods.getByRole('radio', { name: '一括採点' }).check()
  expect(asked).toBe(3)
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await expect(aggregate).not.toBeChecked()
  await expect(page.getByLabel('グループ1の名前')).toHaveValue('小課題1')
  draft = await stored(page)
  expect(draft.scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 100 }] })
  expect(draft.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([['小課題1'], ['小課題1']])
  // The aggregator taken out on the way still comes back with the switch.
  await aggregate.check()
  await expect(page.getByRole('textbox', { name: '集計コード', exact: true })).toHaveText('print(100)')
})

test('the method and the switch are locked while the editor is busy', async ({ page }) => {
  await page.route('**/api/my/submissions', route => route.fulfill({ json: { id: 'generation-job', status: 'QUEUED' } }))
  await page.route('**/api/my/submissions/generation-job', route => route.fulfill({ json: { id: 'generation-job', status: 'QUEUED' } }))
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('生成中の採点')
  await addCases(page, 1)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const methods = page.getByRole('group', { name: '採点方式' })
  const aggregate = page.getByRole('switch', { name: '集計コードで採点する' })
  await methods.getByRole('radio', { name: '部分点採点' }).check()
  await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
  await page.getByLabel('入力生成のコード', { exact: true }).fill('int main() {}')
  await page.getByRole('button', { name: '生成する', exact: true }).click()
  await expect(page.getByText('実行待ちです…')).toBeVisible()
  await page.getByRole('button', { name: '採点', exact: true }).click()
  for (const radio of await methods.getByRole('radio').all()) await expect(radio).toBeDisabled()
  await expect(aggregate).toBeDisabled()
  await expect(methods.getByRole('radio', { name: '部分点採点' })).toBeChecked()
  await expect(aggregate).not.toBeChecked()
  await expect(page.getByLabel('グループ1の名前')).toBeDisabled()
  // A click on the disabled switch's label changes nothing.
  await page.locator('.scoring-switch').click({ force: true })
  await expect(aggregate).not.toBeChecked()
  await expect(page.getByLabel('集計コードの言語')).toHaveCount(0)
})

test('the readiness names the cases in no group and the empty groups, and opens where to fix them', async ({ page }) => {
  const id = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'
  const readiness = ['aggregator_source_missing', 'scoring_group_empty', 'test_case_ungrouped']
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
    id, version: 2, updatedAt: '2026-10-10T00:00:00Z', readiness: { publish: readiness, contest: readiness, featured: readiness },
    draft: {
      title: '部分点の準備', markdown: '本文', difficulty: 3, timeLimitMs: '2000', memoryLimitMb: '256',
      scoring: { mode: 'aggregate', groups: [{ name: '小課題1', points: 40 }, { name: '小課題2', points: 60 }], aggregator: { runtime: 'cpp23-gcc', source: '' } },
      testCases: [{ name: 'sample', isSample: true, input: '1', output: '1' }, { name: '01', input: '2', output: '2', groups: ['小課題1'] }, { name: '02', input: '3', output: '3' }],
    },
  } }))
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.locator('#problem-title')).toHaveValue('部分点の準備')
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const issues = page.getByRole('list', { name: '足りない項目' })
  await expect(issues.getByRole('listitem')).toHaveText([
    '集計コードが空です採点を開く',
    'テストケースが1件も属していないグループがあります：小課題2テストケースを開く',
    'どのグループにも属していないテストケースがあります（サンプルを除く）：02テストケースを開く',
  ])
  await issues.getByRole('listitem').nth(2).getByRole('button', { name: 'テストケースを開く' }).click()
  await expect(page.getByRole('heading', { name: /^テストケース/ })).toBeVisible()
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await issues.getByRole('button', { name: '採点を開く' }).click()
  await expect(page.getByRole('heading', { name: '採点', exact: true })).toBeVisible()
  await expect(page.locator('.scoring-warning')).toContainText('どのグループにも属していないケース（1件）：02')
  await page.locator('.scoring-warning').getByRole('button', { name: 'テストケースを開く' }).click()
  await expect(page.getByRole('heading', { name: /^テストケース/ })).toBeVisible()
})

test('a save refused by the contest lock says what may still change', async ({ page }) => {
  const id = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'
  await page.route(`**/api/my/problems/${id}`, route => route.request().method() === 'PUT'
    ? route.fulfill({ status: 409, json: { statusCode: 409, data: { code: 'problem_scoring_locked' } } })
    : route.fulfill({ json: {
      id, version: 2, updatedAt: '2026-10-10T00:00:00Z', contestId: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc',
      draft: { title: 'コンテストの問題', markdown: '本文', timeLimitMs: '2000', memoryLimitMb: '256', scoring: { mode: 'groups', groups: [{ name: '小課題1', points: 100 }] }, testCases: [{ name: '01', input: '1', output: '1', groups: ['小課題1'] }] },
    } }))
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.locator('#problem-title')).toHaveValue('コンテストの問題')
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await expect(page.getByText('コンテストの開始後は、採点方式、集計コードで採点するかどうか、グループの名前・配点・順番を変えられません。')).toBeVisible()
  await page.getByLabel('グループ1の配点').fill('50')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.editor-notices').getByRole('alert')).toHaveText('コンテストが始まったため、採点方式、集計コードで採点するかどうか、グループの名前・配点・順番は変えられません。ケースの所属、テストケース、集計コードは直せます。')
})

test('a draft whose full score is out of range is not sent', async ({ page }) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('満点のない問題')
  await addCases(page, 1)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
  await stored(page)
  let puts = 0
  page.on('request', request => { if (request.method() === 'PUT') puts++ })
  await page.getByLabel('グループ1の配点').fill('0')
  await expect(page.locator('.scoring-block [role=alert]')).toHaveText('満点（配点の合計）を1〜1,000,000点にしてください。')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.editor-notices').getByRole('alert')).toHaveText('満点（配点の合計）を1〜1,000,000点にしてください。')
  expect(puts).toBe(0)
})

test('a problem in the aggregate mode without an aggregator, as the CLI may push it, gets the aggregator editor', async ({ page }) => {
  const id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
  let sent: { scoring?: unknown } | undefined
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }] } }))
  await page.route(`**/api/my/problems/${id}`, async (route) => {
    const draft = {
      title: 'CLIの問題', markdown: '本文', difficulty: 3, timeLimitMs: '2000', memoryLimitMb: '256',
      scoring: { mode: 'aggregate', groups: [{ name: '小課題1', points: 100 }] },
      testCases: [{ name: '01', input: '1', output: '1', groups: ['小課題1'] }],
    }
    if (route.request().method() === 'PUT') {
      sent = route.request().postDataJSON().draft
      return route.fulfill({ json: { id, version: 3, updatedAt: '2026-10-10T00:00:00Z', draft: sent } })
    }
    return route.fulfill({ json: { id, version: 2, updatedAt: '2026-10-10T00:00:00Z', draft } })
  })
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.locator('#problem-title')).toHaveValue('CLIの問題')
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await expect(page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' })).toBeChecked()
  await expect(page.getByRole('switch', { name: '集計コードで採点する' })).toBeChecked()
  await expect(page.getByLabel('集計コードの言語')).toHaveValue('cpp23-gcc')
  await expect(page.locator('.scoring-block').getByRole('alert')).toContainText('集計コードを1〜65,536バイトで')
  // Opening the section changes nothing (an automatic save would follow within a second); the first edit creates the
  // aggregator.
  await page.waitForTimeout(1000)
  expect(sent).toBeUndefined()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.getByRole('textbox', { name: '集計コード', exact: true }).fill('print(100)')
  await page.getByLabel('集計コードの言語').selectOption('python314')
  await expect.poll(() => sent?.scoring).toEqual({ mode: 'aggregate', groups: [{ name: '小課題1', points: 100 }], aggregator: { runtime: 'python314', source: 'print(100)' } })
})

test('on narrow screens the method, the switch and each group stay usable without sideways scrolling', async ({ page }, testInfo) => {
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル').fill('狭い画面の採点')
  await addCases(page, 1)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
  await page.getByRole('switch', { name: '集計コードで採点する' }).check()
  await page.getByRole('button', { name: 'グループを追加' }).click()
  for (const width of [320, 375, 414, 768, 1024, 1280]) {
    await page.setViewportSize({ width, height: 800 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    // The method keeps its parts on one row, inside the screen, with their labels whole.
    const parts = await page.getByRole('group', { name: '採点方式' }).locator('label').evaluateAll(labels => labels.map((label) => {
      const box = label.getBoundingClientRect()
      return { top: box.top, right: box.right, height: box.height, clipped: label.scrollWidth > label.clientWidth }
    }))
    expect(parts).toHaveLength(2)
    expect(parts[0]!.top).toBe(parts[1]!.top)
    for (const part of parts) {
      expect(part.right).toBeLessThanOrEqual(width)
      expect(part.height).toBeGreaterThanOrEqual(44)
      expect(part.clipped).toBe(false)
    }
    // The switch keeps its own layout: the track beside its text on one line, a full-size target inside the screen.
    const toggle = await page.locator('.scoring-switch').evaluate((label) => {
      const box = label.getBoundingClientRect()
      const track = label.querySelector('.switch-track')!.getBoundingClientRect()
      return { display: getComputedStyle(label).display, right: box.right, height: box.height, track: { width: track.width, height: track.height, middle: track.top + track.height / 2 - box.top } }
    })
    expect(toggle.display).toBe('inline-flex')
    expect(toggle.right).toBeLessThanOrEqual(width)
    expect(toggle.height).toBeGreaterThanOrEqual(44)
    expect(toggle.height).toBeLessThan(56)
    expect(toggle.track.width).toBeGreaterThan(toggle.track.height)
    expect(Math.abs(toggle.track.middle - toggle.height / 2)).toBeLessThanOrEqual(1)
    for (const label of ['グループ1の名前', 'グループ2の名前']) {
      const name = (await page.getByLabel(label).boundingBox())!
      expect(name.width).toBeGreaterThanOrEqual(8 * 14)
    }
    const remove = (await page.getByRole('button', { name: '「小課題2」を削除' }).boundingBox())!
    expect(remove.x + remove.width).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`scoring-groups-${width}.png`), fullPage: true })
  }
  // The table still reads as a table with its column names.
  await expect(page.getByRole('columnheader', { name: '配点' })).toHaveCount(1)
  await expect(page.getByRole('row')).toHaveCount(3)
})

test.describe('touch screens', () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 375, height: 800 } })
  test('the link to the groups is a full-size touch target', async ({ page }) => {
    await page.goto('/problems/new?fresh=1')
    await addCases(page, 1)
    await page.getByRole('button', { name: '採点', exact: true }).click()
    await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
    await page.getByRole('button', { name: 'テストケース', exact: true }).click()
    const link = page.getByRole('button', { name: 'グループを編集' })
    expect((await link.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    await link.click()
    await expect(page.getByRole('heading', { name: '採点', exact: true })).toBeVisible()
  })
})
