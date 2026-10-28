import { randomUUID } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { shortId } from '../../shared/utils/short-id'
import { moveContest } from './schedule'

// Partial scoring against the real API (ADR 0025). This fixture has no judge, so scores are covered by the UI tests.
test('a grouped problem saves through the editor, names what is missing, takes its full score into a contest and is locked from the start', async ({ page }) => {
  test.setTimeout(90000)
  const origin = 'http://127.0.0.1:13002'
  await page.goto('/login')
  // frank only joins other setters' contests, so his own problems and contest stay apart from theirs.
  await page.getByLabel('メールアドレス').fill('frank@example.test')
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/my')
  page.on('dialog', dialog => void dialog.accept())
  await page.goto('/problems/new')
  await page.getByLabel('問題のタイトル').fill('部分点のテスト')
  await page.getByRole('combobox', { name: '難易度（作成者設定）' }).click()
  await page.getByRole('option', { name: 'Lv.1', exact: true }).click()
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  for (let i = 0; i < 3; i++) await page.getByRole('button', { name: 'テストケースを追加' }).click()
  await page.getByRole('button', { name: 'case_001.txt', exact: true }).click()
  await page.getByRole('checkbox', { name: 'サンプルケースにする' }).check()
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
  await expect(page.getByRole('switch', { name: '集計コードで採点する' })).not.toBeChecked()
  await page.getByLabel('グループ1の配点').fill('30')
  await page.getByRole('button', { name: 'グループを追加' }).click()
  await page.getByLabel('グループ2の名前').fill('大きいケース')
  await page.getByLabel('グループ2の名前').press('Enter')
  await page.getByLabel('グループ2の配点').fill('70')
  const status = page.locator('.draft-status').getByRole('status')
  await expect(status).toHaveText('保存済み')
  const id = new URL(page.url()).searchParams.get('problem')!

  // The server's readiness names the reason; the editor adds the group and the case it is about.
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const issues = page.getByRole('list', { name: '足りない項目' })
  await expect(issues.getByRole('listitem')).toHaveText(['テストケースが1件も属していないグループがあります：大きいケーステストケースを開く'])
  await issues.getByRole('button', { name: 'テストケースを開く' }).click()
  const chips = page.getByRole('group', { name: '所属するグループ' })
  await page.getByRole('button', { name: 'case_003.txt', exact: true }).click()
  await chips.getByRole('checkbox', { name: '大きいケース' }).check()
  await page.getByRole('button', { name: 'テストケースを追加' }).click()
  await expect(status).toHaveText('保存済み')
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await expect(issues.getByRole('listitem')).toHaveText(['どのグループにも属していないテストケースがあります（サンプルを除く）：case_004.txtテストケースを開く'])
  await issues.getByRole('button', { name: 'テストケースを開く' }).click()
  await page.getByRole('button', { name: 'case_004.txt 無所属', exact: true }).click()
  await chips.getByRole('checkbox', { name: '小課題1' }).check()
  await expect(status).toHaveText('保存済み')
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await expect(page.getByText('公開・コンテスト・定期便のどれにも出せます。')).toBeVisible()
  const saved = (await (await page.request.get(`/api/my/problems/${id}`)).json()).draft
  expect(saved.scoring).toEqual({ mode: 'groups', groups: [{ name: '小課題1', points: 30 }, { name: '大きいケース', points: 70 }] })
  expect(saved.testCases.map((item: { groups?: string[] }) => item.groups)).toEqual([undefined, ['小課題1'], ['小課題1', '大きいケース'], ['小課題1']])
  // The switch saves the aggregate mode with its code, and turning it off (confirmed) saves the groups mode again.
  const savedScoring = async () => (await (await page.request.get(`/api/my/problems/${id}`)).json()).draft.scoring
  await page.getByRole('button', { name: '採点', exact: true }).click()
  const aggregate = page.getByRole('switch', { name: '集計コードで採点する' })
  await aggregate.check()
  await page.getByRole('textbox', { name: '集計コード', exact: true }).fill('print(100)')
  await expect.poll(savedScoring).toMatchObject({ mode: 'aggregate', groups: saved.scoring.groups, aggregator: { source: 'print(100)' } })
  await aggregate.uncheck()
  await expect.poll(savedScoring).toEqual(saved.scoring)
  await expect(status).toHaveText('保存済み')
  await page.goto(`/problems/${shortId(id)}`)
  await expect(page.locator('.problem-header .problem-scoring-total')).toHaveText('部分点あり（満点100点）')

  // The contest editor takes the full score as the points instead of asking for them.
  await page.goto('/my/contests/new')
  await page.getByRole('button', { name: '問題・配点', exact: true }).click()
  await page.getByRole('button', { name: '部分点のテストを追加', exact: true }).click()
  await expect(page.locator('.selected li .points-fixed')).toHaveText('配点 100 点（満点）')
  await expect(page.getByRole('heading', { name: /出題順・配点/ })).toContainText('1 問 · 合計 100 点')

  // From the start, the groups and their points stay as they are; membership may still change.
  const contestId = randomUUID()
  const now = Date.now()
  const created = await page.request.put(`/api/my/contests/${contestId}`, { headers: { origin }, data: {
    title: '部分点のコンテスト', description: '', version: 0, startsAt: new Date(now + 4000).toISOString(), endsAt: new Date(now + 3600000).toISOString(), problems: [{ id, points: 0 }],
  } })
  expect(created.status()).toBe(200)
  expect(await created.json()).toMatchObject({ partialScoring: true, problems: [{ id, points: 100, partial: true }] })
  await moveContest(page.request, contestId, { startsIn: 0 })
  expect((await (await page.request.get(`/api/contests/${contestId}`)).json()).status).toBe('running')
  await page.goto(`/problems/new?problem=${id}`)
  await page.getByRole('button', { name: '採点', exact: true }).click()
  await page.getByLabel('グループ1の配点').fill('40')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(status).toHaveText('保存に失敗しました')
  await expect(page.locator('.editor-notices').getByRole('alert')).toHaveText('コンテストが始まったため、採点方式、集計コードで採点するかどうか、グループの名前・配点・順番は変えられません。ケースの所属、テストケース、集計コードは直せます。')
  await page.getByLabel('グループ1の配点').fill('30')
  await page.getByRole('button', { name: 'テストケース', exact: true }).click()
  await page.getByRole('button', { name: 'case_002.txt', exact: true }).click()
  await page.getByRole('group', { name: '所属するグループ' }).getByRole('checkbox', { name: '大きいケース' }).check()
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(status).toHaveText('保存済み')
  expect((await (await page.request.get(`/api/my/problems/${id}`)).json()).draft.testCases[1].groups).toEqual(['小課題1', '大きいケース'])
  await page.goto(`/contests/${shortId(contestId)}`)
  await expect(page.locator('.contest-scoring').first()).toHaveText('誤答ペナルティ 5 分')
})
