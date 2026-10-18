import { expect, test, type Browser, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { fullId, shortId } from '../../shared/utils/short-id'

// Teams and the onsite division against the real API (ADR 0024). carol sets the contests; bob, dave, erin and frank
// take part.
const origin = 'http://127.0.0.1:13002'
async function user(browser: Browser, name: string) {
  const context = await browser.newContext({ baseURL: origin })
  const page = await context.newPage()
  await page.goto('/login')
  await page.getByLabel('メールアドレス', { exact: true }).fill(`${name}@example.test`)
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/my')
  return page
}
async function seed(page: Page, title: string) {
  const id = randomUUID()
  const response = await page.request.put(`/api/my/problems/${id}`, { headers: { origin }, data: { version: 0, draft: { title, difficulty: 3, markdown: '## 本文\n整数を出力してください。', editorial: '## 解説\n答えは2です。', timeLimitMs: '1500', memoryLimitMb: '256', testCases: [{ input: '1', output: '2', isSample: true }] } } })
  expect(response.status()).toBe(200)
  return id
}
async function contest(setter: Page, settings: Record<string, unknown>) {
  const id = randomUUID()
  const response = await setter.request.put(`/api/my/contests/${id}`, { headers: { origin }, data: { description: '', version: 0, ...settings } })
  expect(response.status()).toBe(200)
  return id
}
const join = (page: Page, id: string, data?: unknown) => page.request.post(`/api/my/contests/${id}/participation`, { headers: { origin }, ...data ? { data } : {} })

test('a team forms by its join code, teammates see each other\'s submissions during the contest, and other teams do not', async ({ browser }) => {
  test.setTimeout(120000)
  const carol = await user(browser, 'carol')
  const bob = await user(browser, 'bob')
  const dave = await user(browser, 'dave')
  const erin = await user(browser, 'erin')
  const frank = await user(browser, 'frank')
  const guest = await (await browser.newContext({ baseURL: origin })).newPage()
  const problem = await seed(carol, 'チーム戦の問題')
  const now = Date.now()
  const id = await contest(carol, {
    title: 'チーム戦コンテスト', teamMode: true, maxTeamSize: 3,
    startsAt: new Date(now + 40000).toISOString(), endsAt: new Date(now + 3600000).toISOString(), problems: [{ id: problem, points: 100 }],
  })
  // bob creates the team on the overview and reads its code there.
  await bob.goto(`/contests/${shortId(id)}`)
  await expect(bob.getByText('チーム戦（1チーム3人まで）', { exact: true })).toBeVisible()
  await bob.getByLabel('チーム名', { exact: true }).fill('Rocket')
  await bob.getByRole('button', { name: '参加する', exact: true }).click()
  const panel = bob.getByRole('region', { name: 'チーム' })
  await expect(panel.locator('.team-name strong')).toHaveText('Rocket')
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveValue(/^[A-HJKMNP-TV-Z2-9]{10}$/)
  const code = await panel.getByLabel('参加コード', { exact: true }).inputValue()
  // The code never reaches the public contest or the standings.
  expect(await (await guest.request.get(`/api/contests/${id}`)).text()).not.toContain(code)
  expect(await (await guest.request.get(`/api/contests/${id}/standings`)).text()).not.toContain(code)
  // dave mistypes the code, then joins with it in lower case and hyphens, as read out loud.
  await dave.goto(`/contests/${shortId(id)}`)
  await dave.getByRole('group', { name: '参加のしかた' }).getByRole('radio', { name: 'チームに入る' }).check()
  await dave.getByLabel('チームの参加コード', { exact: true }).fill('ZZZZZZZZZZ')
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByRole('alert')).toHaveText('参加コードが違います。チームのメンバーに確かめてください。')
  await expect(dave.getByText('参加済み', { exact: true })).toHaveCount(0)
  await dave.getByLabel('チームの参加コード', { exact: true }).fill(`${code.slice(0, 5).toLowerCase()}-${code.slice(5).toLowerCase()}`)
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByRole('region', { name: 'チーム' }).getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['bob外す', 'dave自分'])
  // erin forms another team, which frank joins and leaves again before the start.
  expect((await join(erin, id, { team: { action: 'create', name: 'Comet' } })).status()).toBe(200)
  // A name differing only in width and case is taken.
  const taken = await join(frank, id, { team: { action: 'create', name: 'ＲＯＣＫＥＴ' } })
  expect(taken.status()).toBe(409)
  expect((await taken.json()).data).toEqual({ code: 'team_name_taken' })
  const comet = (await (await erin.request.get(`/api/my/contests/${id}/team`)).json()).code as string
  expect((await join(frank, id, { team: { action: 'join', code: comet } })).status()).toBe(200)
  await frank.goto(`/contests/${shortId(id)}`)
  frank.once('dialog', dialog => void dialog.accept())
  await frank.getByRole('region', { name: 'チーム' }).getByRole('button', { name: 'チームを抜ける' }).click()
  await expect(frank.locator('.toast').getByRole('status')).toHaveText('チームを抜けました')
  await expect(frank.getByRole('group', { name: '参加のしかた' })).toBeVisible()
  expect((await (await frank.request.get(`/api/contests/${id}`)).json()).participating).toBe(false)
  // The standings list teams with their members from before the start.
  await guest.goto(`/contests/${shortId(id)}?view=standings`)
  const rows = guest.locator('.standings-table tbody tr')
  await expect(rows).toHaveCount(2)
  await expect(guest.locator('.standings-table thead th').nth(1)).toHaveText('チーム')
  await expect(rows.filter({ hasText: 'Rocket' }).locator('.standing-members')).toHaveText('bob、dave')
  await expect(rows.filter({ hasText: 'Comet' }).locator('.standing-members')).toHaveText('erin')
  // Once it starts, bob submits and dave sees it as the team's.
  await expect.poll(async () => (await (await guest.request.get(`/api/contests/${id}`)).json()).status, { timeout: 60000, intervals: [1000] }).toBe('running')
  const source = '// rocket team\nint main(){}'
  await expect.poll(async () => (await bob.request.post('/api/my/submissions', { headers: { origin }, data: { problemId: problem, contestId: id, runtime: 'cpp17', source } })).status(), { timeout: 15000 }).toBe(202)
  await dave.goto(`/contests/${shortId(id)}/A?view=my-submissions`)
  await expect(dave.getByRole('navigation', { name: '問題メニュー' }).getByRole('link')).toHaveText(['問題文', 'チームの提出', 'すべての提出'])
  const teamList = dave.getByRole('region', { name: 'チームの提出' })
  await expect(teamList.locator('tbody tr')).toHaveCount(1)
  await expect(teamList.locator('tbody tr').getByRole('cell').nth(1)).toHaveText('bob')
  await teamList.getByRole('link', { name: '詳細' }).click()
  await expect(dave).toHaveURL(/\/submissions\/[0-9A-Za-z]{22}\?from=team$/)
  await expect(dave.locator('#submitted-code')).toContainText('rocket team')
  await expect(dave.getByRole('link', { name: 'この問題のチームの提出' })).toBeVisible()
  const submission = dave.url().split('/submissions/')[1]!.split('?')[0]!
  // The contest's list shows dave the team's submissions while it runs.
  await dave.goto(`/contests/${shortId(id)}?view=submissions`)
  await expect(dave.getByText('開催中は、チームの提出だけを表示しています。提出コードを閲覧できます。')).toBeVisible()
  await expect(dave.locator('#submissions tbody tr')).toHaveCount(1)
  // erin's team sees none of it, and nobody outside the team opens it.
  await erin.goto(`/contests/${shortId(id)}?view=submissions`)
  await expect(erin.getByText('提出はまだありません。')).toBeVisible()
  await erin.goto(`/contests/${shortId(id)}/A?view=my-submissions`)
  await expect(erin.getByRole('region', { name: 'チームの提出' }).getByText('提出はまだありません。')).toBeVisible()
  for (const page of [erin, frank, guest]) expect((await page.request.get(`/api/contests/${id}/submissions/${fullId(submission)}`)).status()).toBe(404)
  // After the start the team is fixed.
  await bob.goto(`/contests/${shortId(id)}`)
  await expect(bob.getByRole('region', { name: 'チーム' }).getByRole('button', { name: /外す|抜ける/ })).toHaveCount(0)
  expect((await bob.request.delete(`/api/my/contests/${id}/team/members/dave`, { headers: { origin } })).status()).toBe(409)
  for (const page of [carol, bob, dave, erin, frank, guest]) await page.context().close()
})

test('the venue passphrase makes participants onsite, a mistake registers nothing, and setters switch and replace it', async ({ browser }) => {
  test.setTimeout(90000)
  const carol = await user(browser, 'carol')
  const bob = await user(browser, 'bob')
  const dave = await user(browser, 'dave')
  const erin = await user(browser, 'erin')
  const guest = await (await browser.newContext({ baseURL: origin })).newPage()
  const problem = await seed(carol, 'オンサイトの問題')
  const id = await contest(carol, {
    title: 'オンサイトのコンテスト', onsiteDivision: true, onsitePassphrase: 'Venue-2026',
    startsAt: new Date(Date.now() + 3600000).toISOString(), endsAt: new Date(Date.now() + 7200000).toISOString(), problems: [{ id: problem, points: 100 }],
  })
  // The passphrase stays out of every public response.
  expect(await (await guest.request.get(`/api/contests/${id}`)).text()).not.toContain('Venue-2026')
  expect((await (await guest.request.get(`/api/contests/${id}`)).json()).onsiteDivision).toBe(true)
  await dave.goto(`/contests/${shortId(id)}`)
  const field = dave.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  await field.fill('venue-2025')
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByRole('alert')).toHaveText('会場の合言葉が違います。会場で配られた合言葉を確かめてください。')
  await expect(field).toHaveValue('venue-2025')
  expect((await (await dave.request.get(`/api/contests/${id}`)).json()).participating).toBe(false)
  // Full-width characters and stray spaces are folded, as for a passphrase read from paper.
  await field.fill(' Ｖｅｎｕｅ－２０２６ ')
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByText('参加済み', { exact: true })).toBeVisible()
  await expect(dave.locator('.participation-division')).toHaveText('オンサイト参加')
  // bob joins open from a problem-free view, where no passphrase is asked.
  await bob.goto(`/contests/${shortId(id)}?view=standings`)
  await expect(bob.getByLabel('会場の合言葉（オンサイト参加者のみ）')).toHaveCount(0)
  await bob.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(bob.locator('.participation-division')).toHaveText('オープン参加')
  await expect(bob.locator('.standings-table tbody th').filter({ hasText: 'dave' })).toContainText('オンサイト')
  await expect(bob.locator('.standings-table tbody th').filter({ hasText: 'bob' })).not.toContainText('オンサイト')
  // carol sees the passphrase, when and how dave became onsite, and moves bob onsite herself.
  await carol.goto(`/contests/${shortId(id)}`)
  await carol.locator('details.setter-tools summary').click()
  const venue = carol.getByRole('region', { name: '会場の合言葉' })
  await expect(venue.getByRole('textbox', { name: '会場の合言葉' })).toHaveValue('Venue-2026')
  const table = carol.getByRole('region', { name: '参加者の一覧' })
  await expect(table.locator('tbody tr').filter({ hasText: 'dave' }).locator('td').nth(1)).toContainText('（合言葉）')
  await expect(table.locator('tbody tr').filter({ hasText: 'bob' }).locator('td').nth(1)).toHaveText('—')
  await table.getByRole('switch', { name: 'bobをオンサイト参加にする' }).click()
  await expect(table.locator('tbody tr').filter({ hasText: 'bob' }).locator('td').nth(1)).toContainText('（作成者）')
  await expect(table.getByRole('switch', { name: 'bobをオンサイト参加にする' })).toBeChecked()
  // The onsite table now holds both; the setter turns bob back, and the filter stays in the URL.
  await guest.goto(`/contests/${shortId(id)}?view=standings&scope=onsite`)
  await expect(guest.locator('.standings-table tbody tr')).toHaveCount(2)
  await table.getByRole('switch', { name: 'bobをオンサイト参加にする' }).click()
  await expect(table.getByRole('switch', { name: 'bobをオンサイト参加にする' })).not.toBeChecked()
  await guest.reload()
  await expect(guest.locator('.standings-table tbody tr')).toHaveCount(1)
  await expect(guest.locator('.standings-table tbody tr')).toContainText('dave')
  await expect(guest.getByRole('group', { name: '表示する参加者' }).getByRole('radio', { name: 'オンサイトのみ' })).toBeChecked()
  // Replacing the passphrase stops the old one.
  await venue.getByRole('button', { name: '差し替える', exact: true }).click()
  await venue.getByLabel('新しい会場の合言葉', { exact: true }).fill('Second-Venue')
  await venue.getByRole('button', { name: '保存', exact: true }).click()
  await expect(venue.getByRole('textbox', { name: '会場の合言葉' })).toHaveValue('Second-Venue')
  const old = await join(erin, id, { onsitePassphrase: 'Venue-2026' })
  expect(old.status()).toBe(400)
  expect((await old.json()).data).toEqual({ code: 'incorrect_venue_passphrase' })
  // After ten mistakes the page names how long to wait, even for the right passphrase.
  for (let i = 0; i < 9; i++) expect((await join(erin, id, { onsitePassphrase: `wrong-${i}` })).status()).toBe(400)
  await erin.goto(`/contests/${shortId(id)}`)
  await erin.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true }).fill('Second-Venue')
  await erin.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(erin.getByRole('alert')).toHaveText(/^参加コードか会場の合言葉の誤りが続いたため、一時的に試せません。(約\d+分|\d+秒)後にもう一度お試しください。$/)
  // Others are unaffected and the passphrase works for them.
  expect((await join(bob, id, { onsitePassphrase: 'Second-Venue' })).status()).toBe(200)
  for (const page of [carol, bob, dave, erin, guest]) await page.context().close()
})
