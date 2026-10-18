import { expect, test, type Browser, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { shortId } from '../../shared/utils/short-id'

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
  const response = await page.request.put(`/api/my/problems/${id}`, { headers: { origin }, data: { version: 0, draft: { title, difficulty: 3, markdown: '## 凍結の本文\n整数を出力してください。', editorial: '## 凍結の解説\n答えは2です。', timeLimitMs: '1500', memoryLimitMb: '256', testCases: [{ input: '1', output: '2', isSample: true }] } } })
  expect(response.status()).toBe(200)
  return id
}
// The page names the date too when the contest crosses midnight in Japan.
function freezeText(contest: { startsAt: string, endsAt: string, freezeAt: string }) {
  const day = (value: string) => new Intl.DateTimeFormat('ja-JP', { dateStyle: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
  const date = day(contest.startsAt) !== day(contest.endsAt) ? { month: 'long', day: 'numeric' } as const : {}
  return `${new Intl.DateTimeFormat('ja-JP', { ...date, hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' }).format(new Date(contest.freezeAt))}以降の提出の結果は、結果発表まで伏せています。`
}

test('frozen standings hide runs from the public, staff switch to the live table, and only setters and operators publish the results', async ({ browser }) => {
  // The shortest freeze is one minute, so the contests last one minute and the test waits for their end.
  test.setTimeout(150000)
  const carol = await user(browser, 'carol')
  const bob = await user(browser, 'bob')
  const alice = await user(browser, 'alice')
  const guest = await (await browser.newContext({ baseURL: origin })).newPage()
  const [pending, quiet] = [randomUUID(), randomUUID()]
  const problems = { [pending]: await seed(carol, '凍結中に提出する問題'), [quiet]: await seed(carol, '提出のない問題') }
  const now = Date.now()
  for (const id of [pending, quiet]) {
    // The whole contest is frozen: one minute long with a one-minute freeze.
    const response = await carol.request.put(`/api/my/contests/${id}`, { headers: { origin }, data: {
      title: id === pending ? '判定待ちが残るコンテスト' : '結果を公開するコンテスト', description: '', version: 0, freezeMinutes: 1,
      startsAt: new Date(now + 5000).toISOString(), endsAt: new Date(now + 65000).toISOString(), problems: [{ id: problems[id], points: 100 }],
    } })
    expect(response.status()).toBe(200)
    expect((await bob.request.post(`/api/my/contests/${id}/participation`, { headers: { origin } })).status()).toBe(200)
  }
  const contest = await (await guest.request.get(`/api/contests/${pending}`)).json()
  expect(contest).toMatchObject({ freezeMinutes: 1, revealed: false, resultsFinal: false, canViewLiveStandings: false, canRevealResults: false })
  await guest.goto(`/contests/${shortId(pending)}`)
  await expect(guest.getByText('終了1分前から順位表を凍結します。', { exact: true })).toBeVisible()
  // bob's run during the freeze stays queued, since this fixture has no judge.
  await expect.poll(async () => (await bob.request.post('/api/my/submissions', { headers: { origin }, data: { problemId: problems[pending], contestId: pending, runtime: 'cpp17', source: 'int main(){}' } })).status(), { timeout: 15000 }).toBe(202)
  // The public table counts it as ?1 and says since when results are hidden.
  await guest.goto(`/contests/${shortId(pending)}?view=standings`)
  await expect(guest.getByText(freezeText(contest), { exact: true })).toBeVisible()
  const guestRow = guest.locator('.standings-table tbody tr').filter({ hasText: 'bob' })
  await expect(guestRow.locator('.standing-frozen')).toHaveText('?1凍結後の提出 1 件')
  await expect(guestRow.getByRole('img', { name: '判定待ち' })).toHaveCount(0)
  await expect(guest.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  // A participant gets the frozen table even when asking for the live one.
  await bob.goto(`/contests/${shortId(pending)}?view=standings`)
  await expect(bob.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  await expect(bob.locator('.standings-table tbody tr').filter({ hasText: 'bob' }).locator('.standing-frozen')).toHaveText('?1凍結後の提出 1 件')
  const asked = await (await bob.request.get(`/api/contests/${pending}/standings?live=1`)).json()
  expect(asked[0].problems[problems[pending]!]).toMatchObject({ frozen: 1, pending: 0 })
  // The setter starts on the frozen table and switches to the live one, where the run is pending.
  await carol.goto(`/contests/${shortId(pending)}?view=standings`)
  const modes = carol.getByRole('group', { name: '順位表の表示' })
  await expect(modes.getByRole('radio', { name: '凍結した順位表' })).toBeChecked()
  const carolRow = carol.locator('.standings-table tbody tr').filter({ hasText: 'bob' })
  await expect(carolRow.locator('.standing-frozen')).toHaveText('?1凍結後の提出 1 件')
  await modes.getByRole('radio', { name: '凍結なし' }).check()
  await expect(carolRow.getByRole('img', { name: '判定待ち' })).toBeVisible()
  await expect(carolRow.locator('.standing-frozen')).toHaveCount(0)
  await expect(carol.getByText('凍結なしの順位表を表示しています。スタッフだけが見られます。', { exact: true })).toBeVisible()
  // Nobody publishes the results before the end.
  await expect(carol.getByRole('button', { name: '結果を公開', exact: true })).toHaveCount(0)
  await expect.poll(async () => (await (await guest.request.get(`/api/contests/${quiet}`)).json()).status, { timeout: 80000, intervals: [2000] }).toBe('ended')
  // The queued run blocks the setter.
  await carol.reload()
  carol.once('dialog', dialog => void dialog.accept())
  await carol.getByRole('button', { name: '結果を公開', exact: true }).click()
  await expect(carol.getByRole('alert')).toHaveText('判定待ちの提出が残っているため、まだ結果を公開できません。判定が終わってから、もう一度お試しください。')
  // On the contest without runs, a participant cannot publish, while an operator who does not set it can.
  await bob.goto(`/contests/${shortId(quiet)}?view=standings`)
  await expect(bob.getByText(freezeText((await (await guest.request.get(`/api/contests/${quiet}`)).json())), { exact: true })).toBeVisible()
  await expect(bob.getByRole('button', { name: '結果を公開', exact: true })).toHaveCount(0)
  expect((await bob.request.post(`/api/my/contests/${quiet}/reveal`, { headers: { origin } })).status()).toBe(404)
  expect((await guest.request.post(`/api/my/contests/${quiet}/reveal`, { headers: { origin } })).status()).toBe(401)
  expect((await alice.request.post(`/api/my/contests/${quiet}/reveal`, { headers: { origin: 'https://attacker.example' } })).status()).toBe(403)
  await alice.goto(`/contests/${shortId(quiet)}?view=standings`)
  await expect(alice.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  alice.once('dialog', dialog => void dialog.accept())
  await alice.getByRole('button', { name: '結果を公開', exact: true }).click()
  await expect(alice.getByRole('button', { name: '結果を公開', exact: true })).toHaveCount(0)
  await expect(alice.getByText('以降の提出の結果は、結果発表まで伏せています。')).toHaveCount(0)
  expect(await (await guest.request.get(`/api/contests/${quiet}`)).json()).toMatchObject({ revealed: true, resultsFinal: true })
  await carol.goto(`/contests/${shortId(quiet)}?view=standings`)
  await expect(carol.locator('.standings-table tbody tr').filter({ hasText: 'bob' })).toBeVisible()
  await expect(carol.getByRole('button', { name: '結果を公開', exact: true })).toHaveCount(0)
  await expect(carol.getByRole('group', { name: '順位表の表示' })).toHaveCount(0)
  for (const page of [carol, bob, alice, guest]) await page.context().close()
})
