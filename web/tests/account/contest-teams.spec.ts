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
  // bob goes from the overview to the team page, creates the team and reads its code there.
  await bob.goto(`/contests/${shortId(id)}`)
  await expect(bob.getByText('チーム戦（1チーム3人まで）', { exact: true })).toBeVisible()
  await bob.locator('.contest-participation').getByRole('link', { name: 'チーム参加', exact: true }).click()
  await expect(bob).toHaveURL(`/contests/${shortId(id)}/team`)
  await bob.getByLabel('チーム名', { exact: true }).fill('Rocket')
  await bob.getByRole('button', { name: '参加する', exact: true }).click()
  const panel = bob.getByRole('region', { name: '所属チーム', exact: true })
  await expect(panel.locator('.team-name strong')).toHaveText('Rocket')
  await expect(panel.getByLabel('参加コード', { exact: true })).toHaveValue(/^[A-HJKMNP-TV-Z2-9]{10}$/)
  const code = await panel.getByLabel('参加コード', { exact: true }).inputValue()
  // The code never reaches the public contest or the standings.
  expect(await (await guest.request.get(`/api/contests/${id}`)).text()).not.toContain(code)
  expect(await (await guest.request.get(`/api/contests/${id}/standings`)).text()).not.toContain(code)
  // dave mistypes the code, then joins with it in lower case and hyphens, as read out loud.
  await dave.goto(`/contests/${shortId(id)}/team`)
  await dave.getByRole('group', { name: '参加のしかた' }).getByRole('radio', { name: 'チームに入る' }).check()
  await dave.getByLabel('チームの参加コード', { exact: true }).fill('ZZZZZZZZZZ')
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByRole('alert')).toHaveText('参加コードが違います。チームのメンバーに確かめてください。')
  await expect(dave.getByText('参加済み', { exact: true })).toHaveCount(0)
  await dave.getByLabel('チームの参加コード', { exact: true }).fill(`${code.slice(0, 5).toLowerCase()}-${code.slice(5).toLowerCase()}`)
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByRole('region', { name: '所属チーム', exact: true }).getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['bob外す', 'dave自分'])
  // erin forms another team, which frank joins and leaves again before the start.
  expect((await join(erin, id, { team: { action: 'create', name: 'Comet' } })).status()).toBe(200)
  // A name differing only in width and case is taken.
  const taken = await join(frank, id, { team: { action: 'create', name: 'ＲＯＣＫＥＴ' } })
  expect(taken.status()).toBe(409)
  expect((await taken.json()).data).toEqual({ code: 'team_name_taken' })
  const comet = (await (await erin.request.get(`/api/my/contests/${id}/team`)).json()).code as string
  expect((await join(frank, id, { team: { action: 'join', code: comet } })).status()).toBe(200)
  await frank.goto(`/contests/${shortId(id)}`)
  await frank.locator('.contest-participation').getByRole('link', { name: 'チームのページ', exact: true }).click()
  frank.once('dialog', dialog => void dialog.accept())
  await frank.getByRole('region', { name: '所属チーム', exact: true }).getByRole('button', { name: 'チームを抜ける' }).click()
  await expect(frank.locator('.toast').getByRole('status')).toHaveText('チームを抜けました')
  await expect(frank.getByRole('group', { name: '参加のしかた' })).toBeVisible()
  expect((await (await frank.request.get(`/api/contests/${id}`)).json()).participating).toBe(false)
  // The standings list teams with their members from before the start.
  await guest.goto(`/contests/${shortId(id)}?view=standings`)
  const rows = guest.locator('.standings-table tbody tr')
  await expect(rows).toHaveCount(2)
  await expect(guest.locator('.standings-table thead th').nth(1)).toHaveText('チーム・個人')
  await expect(rows.filter({ hasText: 'Rocket' }).locator('.standing-members')).toHaveText('bob、dave')
  await expect(rows.filter({ hasText: 'Comet' }).locator('.standing-members')).toHaveText('erin')
  // Once it starts, bob submits and dave sees it as the team's.
  await expect.poll(async () => (await (await guest.request.get(`/api/contests/${id}`)).json()).status, { timeout: 60000, intervals: [1000] }).toBe('running')
  const source = '// rocket team\nint main(){}'
  await expect.poll(async () => (await bob.request.post('/api/my/submissions', { headers: { origin }, data: { problemId: problem, contestId: id, runtime: 'cpp17', source } })).status(), { timeout: 15000 }).toBe(202)
  await dave.goto(`/contests/${shortId(id)}/A?view=my-submissions`)
  await expect(dave.getByRole('navigation', { name: '問題メニュー' }).getByRole('link')).toHaveText(['問題文', 'チームの提出'])
  const teamList = dave.getByRole('region', { name: 'チームの提出' })
  await expect(teamList.locator('tbody tr')).toHaveCount(1)
  await expect(teamList.locator('tbody tr').getByRole('cell').nth(1)).toHaveText('bob')
  await teamList.getByRole('link', { name: '詳細' }).click()
  await expect(dave).toHaveURL(/\/submissions\/[0-9A-Za-z]{22}\?from=team$/)
  await expect(dave.locator('#submitted-code')).toContainText('rocket team')
  await expect(dave.getByRole('link', { name: 'この問題のチームの提出' })).toBeVisible()
  const submission = dave.url().split('/submissions/')[1]!.split('?')[0]!
  // The contest's 「チームの提出」 shows dave the team's submissions to every problem while it runs (ADR 0030).
  await dave.goto(`/contests/${shortId(id)}?view=my-submissions`)
  await expect(dave.getByText('このコンテストの全問題への、チームの提出です。練習提出も掲載します。')).toBeVisible()
  await expect(dave.locator('#my-submissions tbody tr')).toHaveCount(1)
  // erin's team sees none of it, and nobody outside the team opens it.
  await erin.goto(`/contests/${shortId(id)}?view=my-submissions`)
  await expect(erin.locator('#my-submissions').getByText('提出はまだありません。')).toBeVisible()
  await erin.goto(`/contests/${shortId(id)}/A?view=my-submissions`)
  await expect(erin.getByRole('region', { name: 'チームの提出' }).getByText('提出はまだありません。')).toBeVisible()
  for (const page of [erin, frank, guest]) expect((await page.request.get(`/api/contests/${id}/submissions/${fullId(submission)}`)).status()).toBe(404)
  // After the start the team is fixed.
  await bob.goto(`/contests/${shortId(id)}/team`)
  await expect(bob.getByRole('region', { name: '所属チーム', exact: true }).getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['bob自分', 'dave'])
  await expect(bob.getByRole('region', { name: '所属チーム', exact: true }).getByRole('button', { name: /外す|抜ける/ })).toHaveCount(0)
  expect((await bob.request.delete(`/api/my/contests/${id}/team/members/dave`, { headers: { origin } })).status()).toBe(409)
  for (const page of [carol, bob, dave, erin, frank, guest]) await page.context().close()
})

test('the venue passphrase makes participants onsite, a mistake registers nothing, and setters move them onsite and replace it', async ({ browser }) => {
  test.setTimeout(90000)
  const carol = await user(browser, 'carol')
  const bob = await user(browser, 'bob')
  const dave = await user(browser, 'dave')
  const erin = await user(browser, 'erin')
  const guest = await (await browser.newContext({ baseURL: origin })).newPage()
  const problem = await seed(carol, 'オンサイトの問題')
  // Only letters, digits and underscores make a passphrase.
  const refused = await carol.request.put(`/api/my/contests/${randomUUID()}`, { headers: { origin }, data: { title: '使えない合言葉', description: '', version: 0, onsiteDivision: true, onsitePassphrase: 'Venue-2026', startsAt: new Date(Date.now() + 3600000).toISOString(), endsAt: new Date(Date.now() + 7200000).toISOString(), problems: [{ id: problem, points: 100 }] } })
  expect(refused.status()).toBe(400)
  const id = await contest(carol, {
    title: 'オンサイトのコンテスト', onsiteDivision: true, onsitePassphrase: 'Venue_2026',
    startsAt: new Date(Date.now() + 3600000).toISOString(), endsAt: new Date(Date.now() + 7200000).toISOString(), problems: [{ id: problem, points: 100 }],
  })
  // The passphrase stays out of every public response.
  expect(await (await guest.request.get(`/api/contests/${id}`)).text()).not.toContain('Venue_2026')
  expect((await (await guest.request.get(`/api/contests/${id}`)).json()).onsiteDivision).toBe(true)
  await dave.goto(`/contests/${shortId(id)}`)
  const field = dave.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })
  // The browser names the rule for other characters before anything is sent.
  await field.fill('Venue-2026')
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  expect(await field.evaluate((input: HTMLInputElement) => input.validationMessage)).toBe('会場の合言葉は、英数字とアンダースコア（_）の4〜64文字で入力してください。')
  await field.fill('venue_2025')
  await dave.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(dave.getByRole('alert')).toHaveText('会場の合言葉が違います。会場で配られた合言葉を確かめてください。')
  await expect(field).toHaveValue('venue_2025')
  expect((await (await dave.request.get(`/api/contests/${id}`)).json()).participating).toBe(false)
  // Full-width characters and stray spaces are folded, as for a passphrase read from paper.
  await field.fill(' Ｖｅｎｕｅ＿２０２６ ')
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
  // carol sees the passphrase, when and how dave became onsite, and moves bob onsite herself, for good.
  await carol.goto(`/contests/${shortId(id)}`)
  await carol.locator('details.setter-tools summary').click()
  const venue = carol.getByRole('region', { name: '会場の合言葉' })
  await expect(venue.getByRole('textbox', { name: '会場の合言葉' })).toHaveValue('Venue_2026')
  const table = carol.getByRole('region', { name: '参加者の一覧' })
  await expect(table.locator('tbody tr').filter({ hasText: 'dave' }).locator('td').nth(1)).toContainText('（合言葉）')
  await expect(table.getByRole('button', { name: 'daveをオンサイト参加にする' })).toHaveCount(0)
  await expect(table.locator('tbody tr').filter({ hasText: 'bob' }).locator('td').nth(1)).toHaveText('—')
  carol.once('dialog', dialog => void dialog.accept())
  await table.getByRole('button', { name: 'bobをオンサイト参加にする' }).click()
  await expect(table.locator('tbody tr').filter({ hasText: 'bob' }).locator('td').nth(1)).toContainText('（作成者）')
  await expect(table.locator('tbody tr').filter({ hasText: 'bob' }).locator('td').first()).toHaveText('オンサイト')
  await expect(table.getByRole('button', { name: 'bobをオンサイト参加にする' })).toHaveCount(0)
  const back = await carol.request.put(`/api/my/contests/${id}/participants/bob/onsite`, { headers: { origin }, data: { onsite: false } })
  expect(back.status()).toBe(400)
  expect((await back.json()).data).toEqual({ code: 'onsite_irreversible' })
  // The onsite table holds both, and the filter stays in the URL.
  await guest.goto(`/contests/${shortId(id)}?view=standings&scope=onsite`)
  await expect(guest.locator('.standings-table tbody tr')).toHaveCount(2)
  await expect(guest.getByRole('switch', { name: 'オンサイトのみ表示' })).toBeChecked()
  // Replacing the passphrase checks the rule and stops the old one.
  await venue.getByRole('button', { name: '差し替える', exact: true }).click()
  const fresh = venue.getByLabel('新しい会場の合言葉', { exact: true })
  await fresh.fill('Second Venue')
  await venue.getByRole('button', { name: '保存', exact: true }).click()
  expect(await fresh.evaluate((input: HTMLInputElement) => input.validationMessage)).toBe('会場の合言葉は、英数字とアンダースコア（_）の4〜64文字で入力してください。')
  await fresh.fill('Second_Venue')
  await venue.getByRole('button', { name: '保存', exact: true }).click()
  await expect(venue.getByRole('textbox', { name: '会場の合言葉' })).toHaveValue('Second_Venue')
  const old = await join(erin, id, { onsitePassphrase: 'Venue_2026' })
  expect(old.status()).toBe(400)
  expect((await old.json()).data).toEqual({ code: 'incorrect_venue_passphrase' })
  // After ten mistakes the page names how long to wait, even for the right passphrase.
  for (let i = 0; i < 9; i++) expect((await join(erin, id, { onsitePassphrase: `wrong_${i}` })).status()).toBe(400)
  await erin.goto(`/contests/${shortId(id)}`)
  await erin.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true }).fill('Second_Venue')
  await erin.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(erin.getByRole('alert')).toHaveText(/^参加コードか会場の合言葉の誤りが続いたため、一時的に試せません。(約\d+分|\d+秒)後にもう一度お試しください。$/)
  // Others are unaffected and the passphrase works for them.
  const frank = await user(browser, 'frank')
  expect((await join(frank, id, { onsitePassphrase: 'Second_Venue' })).status()).toBe(200)
  for (const page of [carol, bob, dave, erin, frank, guest]) await page.context().close()
})

test('team contests take individuals beside teams, who switch to a team on the team page only before the start', async ({ browser }) => {
  test.setTimeout(180000)
  const carol = await user(browser, 'carol')
  const bob = await user(browser, 'bob')
  const dave = await user(browser, 'dave')
  const erin = await user(browser, 'erin')
  const frank = await user(browser, 'frank')
  const guest = await (await browser.newContext({ baseURL: origin })).newPage()
  const problem = await seed(carol, '個人参加の問題')
  const now = Date.now()
  const id = await contest(carol, {
    title: '個人参加のあるチーム戦', teamMode: true, onsiteDivision: true, onsitePassphrase: 'Venue_2026',
    startsAt: new Date(now + 60000).toISOString(), endsAt: new Date(now + 3600000).toISOString(), problems: [{ id: problem, points: 100 }],
  })
  // erin enters alone from the overview, as in an individual contest.
  await erin.goto(`/contests/${shortId(id)}`)
  await erin.locator('.contest-participation').getByRole('button', { name: '個人参加', exact: true }).click()
  await expect(erin.locator('.participation-state')).toHaveText('参加済みオープン参加個人参加')
  // bob types the passphrase on the overview, goes on to the team page with it, and forms an onsite team.
  await bob.goto(`/contests/${shortId(id)}`)
  await bob.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true }).fill('Venue_2026')
  await bob.locator('.contest-participation').getByRole('link', { name: 'チーム参加', exact: true }).click()
  await expect(bob.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true })).toHaveValue('Venue_2026')
  await bob.getByLabel('チーム名', { exact: true }).fill('Rocket')
  await bob.getByRole('button', { name: '参加する', exact: true }).click()
  await expect(bob.locator('.participation-state')).toHaveText('参加済みオンサイト参加')
  const code = await bob.getByRole('region', { name: '所属チーム', exact: true }).getByLabel('参加コード', { exact: true }).inputValue()
  // dave first enters alone onsite, then switches into bob's team before the start.
  await dave.goto(`/contests/${shortId(id)}`)
  await dave.getByLabel('会場の合言葉（オンサイト参加者のみ）', { exact: true }).fill('Venue_2026')
  await dave.locator('.contest-participation').getByRole('button', { name: '個人参加', exact: true }).click()
  await expect(dave.locator('.participation-state')).toHaveText('参加済みオンサイト参加個人参加')
  await dave.locator('.contest-participation').getByRole('link', { name: 'チーム参加に切り替える', exact: true }).click()
  await expect(dave.locator('.participation-form .notice')).toContainText('個人参加の登録はチームのメンバーとしての登録に置き換わります。')
  await dave.getByRole('group', { name: '参加のしかた' }).getByRole('radio', { name: 'チームに入る' }).check()
  await dave.getByLabel('チームの参加コード', { exact: true }).fill(code)
  await dave.getByRole('button', { name: 'チーム参加に切り替える', exact: true }).click()
  await expect(dave.getByRole('region', { name: '所属チーム', exact: true }).getByRole('list', { name: 'メンバー' }).getByRole('listitem')).toHaveText(['bob外す', 'dave自分'])
  // frank switches into a team of his own, leaves it, and is then unregistered until he enters alone.
  expect((await join(frank, id)).status()).toBe(200)
  await frank.goto(`/contests/${shortId(id)}/team`)
  await frank.getByLabel('チーム名', { exact: true }).fill('Comet')
  await frank.getByRole('button', { name: 'チーム参加に切り替える', exact: true }).click()
  frank.once('dialog', dialog => void dialog.accept())
  await frank.getByRole('region', { name: '所属チーム', exact: true }).getByRole('button', { name: 'チームを抜ける' }).click()
  await expect(frank.locator('.toast').getByRole('status')).toHaveText('チームを抜けました')
  expect((await (await frank.request.get(`/api/contests/${id}`)).json()).participating).toBe(false)
  await frank.locator('.participation-form').getByRole('link', { name: '概要ページ' }).click()
  await frank.locator('.contest-participation').getByRole('button', { name: '個人参加', exact: true }).click()
  await expect(frank.locator('.participation-state')).toHaveText('参加済みオープン参加個人参加')
  // The standings show the team and the individuals side by side.
  await guest.goto(`/contests/${shortId(id)}?view=standings`)
  const rows = guest.locator('.standings-table tbody tr')
  await expect(rows).toHaveCount(3)
  await expect(guest.locator('.standings-table thead th').nth(1)).toHaveText('チーム・個人')
  await expect(rows.filter({ hasText: 'Rocket' }).locator('th')).toHaveText('Rocketbob、daveオンサイト')
  await expect(rows.filter({ hasText: 'erin' }).locator('th')).toHaveText('erin')
  // carol lists both kinds and moves erin onsite by handle.
  await carol.goto(`/contests/${shortId(id)}`)
  await carol.locator('details.setter-tools summary').click()
  await expect(carol.getByRole('heading', { name: 'チーム・個人 チーム 1・個人 2' })).toBeVisible()
  carol.once('dialog', dialog => void dialog.accept())
  await carol.getByRole('button', { name: 'erinをオンサイト参加にする', exact: true }).click()
  await expect(carol.getByRole('region', { name: '参加者の一覧' }).locator('tbody tr').filter({ hasText: 'erin' }).locator('td').first()).toHaveText('オンサイト')
  // Once it starts, erin stays alone: no switch on the overview or the team page, and the server refuses a team.
  await expect.poll(async () => (await (await guest.request.get(`/api/contests/${id}`)).json()).status, { timeout: 90000, intervals: [1000] }).toBe('running')
  await erin.goto(`/contests/${shortId(id)}`)
  await expect(erin.locator('.participation-state')).toHaveText('参加済みオンサイト参加個人参加')
  await expect(erin.locator('.contest-participation').getByRole('link')).toHaveCount(0)
  await erin.goto(`/contests/${shortId(id)}/team`)
  await expect(erin.locator('.contest-participation')).toContainText('開始後は、個人参加からチーム参加に切り替えられません。')
  await expect(erin.getByRole('group', { name: '参加のしかた' })).toHaveCount(0)
  const locked = await join(erin, id, { team: { action: 'join', code } })
  expect(locked.status()).toBe(409)
  expect((await locked.json()).data).toEqual({ code: 'contest_individual_locked' })
  // Her own submissions stay hers: the problem page says 自分の提出, not the team's.
  await erin.goto(`/contests/${shortId(id)}/A?view=my-submissions`)
  await expect(erin.getByRole('navigation', { name: '問題メニュー' }).getByRole('link')).toHaveText(['問題文', '自分の提出'])
  for (const page of [carol, bob, dave, erin, frank, guest]) await page.context().close()
})
