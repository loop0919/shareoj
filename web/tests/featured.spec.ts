import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

const id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const hidden = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const updatedAt = '2026-09-26T00:00:00Z'

test('featured editions show two slots, revivals, vacancies and the Ultimate label', async ({ page }, testInfo) => {
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: [] } }))
  await page.goto('/featured')
  await expect(page.getByRole('heading', { name: '定期便', exact: true })).toBeVisible()
  const notes = page.locator('.delivery-details')
  await expect(notes.locator('ul')).toBeHidden()
  await notes.locator('summary').focus()
  await page.keyboard.press('Enter')
  await expect(notes.locator('li')).toHaveCount(5)
  await expect(notes).toContainText('次回予定を含む')
  await page.keyboard.press('Enter')
  await expect(notes.locator('ul')).toBeHidden()
  await expect(page.locator('.featured-page > p')).toHaveCount(0)
  await expect(page.locator('.round')).toHaveCount(2)
  // The heading names the edition instead of a tagline.
  await expect(page.getByRole('heading', { name: '定期便 vol.13', exact: true })).toBeVisible()
  await expect(page.getByText('次の一問が、届く。')).toHaveCount(0)
  await expect(page.locator('.round h3')).toContainText(['vol.12', 'vol.11'])
  await expect(page.locator('.round').first().locator('.slot')).toHaveCount(2)
  await expect(page.getByRole('heading', { name: 'Ultimate', exact: true })).toBeVisible()
  await expect(page.getByText('欠番', { exact: true })).toBeVisible()
  await expect(page.getByText('解説公開済み', { exact: false }).first()).toBeVisible()
  await expect(page.getByRole('link', { name: '定期便の新作', exact: true })).toHaveAttribute('href', `/problems/${shortId(hidden)}`)
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`featured-${width}.png`), fullPage: true })
  }
})

test('authors choose a problem and preference, see where it lands, apply and withdraw', async ({ page }, testInfo) => {
  let applied = false
  let attempts = 0
  let withdrawals = 0
  const plans: URLSearchParams[] = []
  await page.route('**/api/my/featured', route => route.fulfill({ json: { items: applied ? [{ problemId: id, title: '応募する問題', preference: 'later', enteredAt: updatedAt }] : [] } }))
  await page.route('**/api/my/featured/schedule**', async route => {
    const query = new URL(route.request().url()).searchParams
    plans.push(query)
    const json = await (await page.request.get('/api/featured/schedule')).json()
    // The chosen problem lands in the second round when soon, the fourth when later.
    const round = query.get('problem') === id ? query.get('preference') === 'later' ? 3 : 1 : -1
    if (round >= 0) json.rounds[round].slots[0] = { slot: 'easy', kind: 'new', difficulty: 4, mine: applied, title: '応募する問題', trial: true }
    return route.fulfill({ json })
  })
  await page.route(/\/api\/my\/problems(?:\?.*)?$/, route => route.fulfill({ json: { items: [
    { id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', title: '過去に公開済み', everPublished: true, difficulty: 3, updatedAt, readiness: { publish: [], contest: [], featured: ['ever_published'] } },
    { id, title: '応募する問題', featuredPreference: applied ? 'later' : '', difficulty: 4, updatedAt, readiness: { publish: [], contest: [], featured: [] } },
    { id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', title: 'コンテスト用', contestId: 'contest', difficulty: 6, updatedAt, readiness: { publish: ['in_contest'], contest: ['in_contest'], featured: ['in_contest', 'editorial_missing'] } },
    { id: 'dddddddd-dddd-4ddd-8ddd-dddddddddddd', title: '公開中', publishedVersion: 1, difficulty: 5, updatedAt },
  ], nextCursor: '' } }))
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: {
    id, version: 3, updatedAt, draft: { title: '応募する問題', markdown: '本文', editorial: '解説', difficulty: 4, timeLimitMs: '2000', memoryLimitMb: '256', testCases: [] },
  } }))
  await page.route(`**/api/my/problems/${id}/featured`, route => {
    const body = route.request().postDataJSON()
    expect(body.version).toBe(3)
    if (body.preference === '') {
      if (++withdrawals === 1) return route.fulfill({ status: 503, json: {} })
      applied = false
      return route.fulfill({ status: 204 })
    }
    expect(body.preference).toBe('later')
    if (++attempts === 1) return route.fulfill({ status: 400, json: { data: { code: 'featured_ineligible' } } })
    applied = true
    return route.fulfill({ status: 204 })
  })
  await page.goto('/featured')
  await page.getByRole('link', { name: '新作を応募' }).click()
  await expect(page).toHaveURL('/featured/apply')
  await expect(page.getByRole('heading', { name: '定期便に応募', level: 1 })).toBeVisible()
  await expect(page.getByText('応募枠 0 / 3')).toBeVisible()
  const choices = page.getByRole('group', { name: '問題' })
  // Choosable problems come first; published ones are not listed; blocked ones say why.
  await expect(choices.getByRole('radio')).toHaveCount(3)
  await expect(choices.locator('li').first()).toContainText('応募する問題')
  await expect(choices.getByRole('radio', { name: /過去に公開済み/ })).toBeDisabled()
  await expect(choices.getByText('一度公開した問題は定期便に応募できません')).toBeVisible()
  await expect(choices.getByText('コンテストに登録した問題は応募できません（ほか1件）')).toBeVisible()
  await expect(choices).not.toContainText('公開中')
  const submit = page.getByRole('button', { name: '応募する', exact: true })
  await expect(submit).toBeDisabled()
  await expect(page.getByRole('radio', { name: '早めに出したい' })).toBeDisabled()
  await expect(page.locator('.apply-schedule tbody tr')).toHaveCount(6)
  await expect(page.locator('.apply-schedule')).toContainText('確定')
  await choices.getByRole('radio', { name: /応募する問題/ }).check()
  await expect(page.locator('.apply-bar .bar-message')).toContainText('10/5')
  const placed = page.locator('.apply-schedule td[data-trial]')
  // The chosen problem shows by name, marked as a what-if until it is applied.
  await expect(placed).toContainText('応募する問題')
  await expect(placed).toContainText('応募した場合')
  await expect(page.locator('.apply-bar')).toContainText('「応募する問題」は10/5')
  await page.getByRole('radio', { name: 'ゆっくりで良い' }).check()
  await expect(page.locator('.apply-bar .bar-message')).toContainText('10/12')
  expect(plans.at(-1)?.get('preference')).toBe('later')
  expect(plans.at(-1)?.get('problem')).toBe(id)
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`apply-${width}.png`), fullPage: true })
  }
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.screenshot({ path: testInfo.outputPath('apply-dark.png'), fullPage: true })
  await page.emulateMedia({ colorScheme: 'light' })
  await submit.click()
  await expect(page.getByRole('alert')).toContainText('難易度・解説')
  await submit.click()
  await expect(page.getByRole('status')).toContainText('応募しました。「応募する問題」は10/12')
  await expect(placed).toContainText('応募中')
  await expect(page.getByRole('button', { name: '応募済み', exact: true })).toBeDisabled()
  await expect(page.getByText('応募枠 1 / 3')).toBeVisible()
  await page.getByRole('radio', { name: '早めに出したい' }).check()
  await expect(page.getByRole('button', { name: '希望を変更', exact: true })).toBeEnabled()
  await page.goto('/my?tab=problems')
  const row = page.getByRole('row').filter({ has: page.getByRole('rowheader', { name: '応募する問題', exact: true }) })
  await expect(row).toContainText('応募中（定期便）')
  await row.getByRole('img', { name: 'ゆっくりで良い' }).focus()
  await expect(row.getByRole('tooltip')).toHaveText('ゆっくりで良い')
  const withdrawButton = row.getByRole('button', { name: '応募する問題の定期便応募を取り下げる' })
  const confirmation = page.getByRole('dialog', { name: '定期便の応募を取り下げますか？' })
  await withdrawButton.click()
  await expect(confirmation).toContainText('応募する問題')
  await expect(confirmation.getByRole('button', { name: 'キャンセル' })).toBeFocused()
  expect(withdrawals).toBe(0)
  await confirmation.getByRole('button', { name: 'キャンセル' }).click()
  await expect(confirmation).toBeHidden()
  await expect(withdrawButton).toBeFocused()
  await expect(row).toContainText('応募中（定期便）')
  await withdrawButton.click()
  await page.keyboard.press('Escape')
  await expect(confirmation).toBeHidden()
  expect(withdrawals).toBe(0)
  await page.setViewportSize({ width: 375, height: 812 })
  await withdrawButton.click()
  const bounds = (await confirmation.boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(375)
  await confirmation.getByRole('button', { name: '取り下げる', exact: true }).click()
  await expect(confirmation.getByRole('alert')).toBeVisible()
  expect(applied).toBe(true)
  await expect(row).toContainText('応募中（定期便）')
  await confirmation.getByRole('button', { name: '取り下げる', exact: true }).click()
  await expect(confirmation).toBeHidden()
  expect(withdrawals).toBe(2)
  await expect(row).toContainText('準備中')
  await expect(row.getByRole('img')).toHaveCount(0)
})

test('a long list scrolls inside itself, can hide blocked problems, and keeps the button on screen', async ({ page }, testInfo) => {
  const items = Array.from({ length: 30 }, (_, index) => ({
    id: `aaaaaaaa-aaaa-4aaa-8aaa-${String(index).padStart(12, '0')}`, title: `問題 ${index + 1}`, difficulty: index % 10 + 1, updatedAt,
    readiness: { publish: [], contest: [], featured: index % 3 ? ['editorial_missing'] : [] },
  }))
  const last = items[27]!
  await page.route(/\/api\/my\/problems(?:\?.*)?$/, route => route.fulfill({ json: { items, nextCursor: '' } }))
  await page.route('**/api/my/featured/schedule**', async route => route.fulfill({ json: await (await page.request.get('/api/featured/schedule')).json() }))
  for (const size of [{ width: 1280, height: 800 }, { width: 375, height: 812 }]) {
    await page.setViewportSize(size)
    await page.goto(`/featured/apply?problem=${last.id}`)
    const radio = page.getByRole('radio', { name: /問題 28/ })
    await expect(radio).toBeChecked()
    // The problem chosen by the link is scrolled into the list's view, not the page's.
    await expect(radio).toBeInViewport()
    await expect(page.getByRole('button', { name: '応募する', exact: true })).toBeInViewport()
    await expect(page.getByRole('radio', { name: 'ゆっくりで良い' })).toBeInViewport()
    await expect(page.locator('.apply-bar')).toContainText('「問題 28」は')
    if (size.width === 1280) {
      // The button closes the page at the bottom right, under the schedule.
      const button = (await page.getByRole('button', { name: '応募する', exact: true }).boundingBox())!
      const schedule = (await page.locator('.apply-schedule').boundingBox())!
      expect(button.x).toBeGreaterThan(schedule.x)
    }
    expect(await page.evaluate(() => scrollY)).toBe(0)
    await page.screenshot({ path: testInfo.outputPath(`apply-long-${size.width}.png`) })
  }
  const toggle = page.getByRole('switch', { name: /応募不可の問題を隠す/ })
  await expect(toggle).not.toBeChecked()
  await expect(page.locator('.choices-switch')).toContainText('20件')
  await expect(page.getByRole('radio')).toHaveCount(32)
  await toggle.click()
  await expect(toggle).toBeChecked()
  await expect(page.getByRole('group', { name: '問題' }).getByRole('radio')).toHaveCount(10)
  await expect(page.getByRole('group', { name: '問題' }).getByRole('status')).toHaveText('20件を隠しています')
  await expect(page.getByRole('radio', { name: /問題 28/ })).toBeChecked()
  await page.screenshot({ path: testInfo.outputPath('apply-switch-on.png') })
  // The keyboard turns it back off.
  await toggle.focus()
  await page.keyboard.press('Space')
  await expect(toggle).not.toBeChecked()
  await expect(page.getByRole('group', { name: '問題' }).getByRole('radio')).toHaveCount(30)
})

test('the schedule lists coming rounds by level without writers and fits narrow screens', async ({ page }, testInfo) => {
  await page.goto('/featured')
  await page.getByRole('link', { name: '予定', exact: true }).click()
  await expect(page).toHaveURL('/featured/schedule')
  await expect(page.getByRole('heading', { name: '定期便の予定', level: 1 })).toBeVisible()
  await expect(page.locator('.schedule-waiting')).toContainText('Easy 3問')
  const rows = page.locator('.featured-schedule tbody tr')
  await expect(rows).toHaveCount(8)
  await expect(rows.first()).toContainText('確定')
  await expect(rows.nth(1)).not.toContainText('確定')
  await expect(rows.first().locator('td').first()).toHaveText('Lv.4')
  await expect(rows.first().locator('td').nth(1)).toHaveText('Lv.7')
  // The schedule's Hard column keeps its name even for a Lv.9+ problem: a column can hold both.
  await expect(page.locator('.featured-schedule thead th').last()).toHaveText('Hard')
  await expect(rows.nth(1).locator('td').nth(1)).toHaveText('Lv.10')
  await expect(rows.nth(1).locator('td').nth(1)).toHaveAttribute('data-slot', 'Hard')
  // The schedule never names writers.
  await expect(page.locator('.featured-schedule a')).toHaveCount(0)
  await expect(rows.nth(3)).toContainText('募集中')
  await expect(rows.nth(3)).toContainText('復刻')
  // No problem names reach the public schedule.
  await expect(page.locator('.featured-schedule')).not.toContainText('A + B')
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`schedule-${width}.png`), fullPage: true })
  }
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.screenshot({ path: testInfo.outputPath('schedule-dark.png'), fullPage: true })
  await page.emulateMedia({ colorScheme: 'light' })
  await page.getByRole('link', { name: '新作を応募' }).click()
  await expect(page).toHaveURL('/featured/apply')
})

test('guests see the schedule on the application page and a login link back to it', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto('/featured/apply')
  await expect(page.locator('.apply-form').getByRole('link', { name: 'ログイン' })).toHaveAttribute('href', '/login?next=/featured/apply')
  await expect(page.locator('.apply-schedule tbody tr')).toHaveCount(6)
  await expect(page.getByText(/応募枠/)).toHaveCount(0)
})

test('hidden editorial and other submissions show the unlock time while own results stay accessible', async ({ page }) => {
  await page.goto(`/problems/${hidden}?view=editorial`)
  await expect(page.getByText('解説は公開待ちです。')).toBeVisible()
  await expect(page.locator('.notice')).toHaveText('定期便の新作です。解説と他者の提出は2099/09/29 22:00（日本時間）に公開します。')
  await page.getByRole('link', { name: 'すべての提出', exact: true }).click()
  await expect(page.getByText(/他者の提出は解禁後に閲覧できます/)).toBeVisible()
  await expect(page.locator('.problem-submissions')).toHaveCount(0)
  await page.getByRole('link', { name: '自分の提出', exact: true }).click()
  await expect(page.getByRole('heading', { name: '自分の提出', exact: true })).toBeVisible()
})

test('featured history has a recoverable error state', async ({ page }) => {
  await page.goto('/problems')
  await page.route('**/api/featured**', route => route.fulfill({ status: 502, json: {} }))
  await page.locator('header').getByRole('link', { name: '定期便', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: '定期便を取得できませんでした' })).toBeVisible()
  await expect(page.getByRole('button', { name: '再試行', exact: true }).first()).toBeVisible()
})


test('home and featured share upcoming credits and waiting counts without preview titles', async ({ page }, testInfo) => {
  await page.goto('/')
  const delivery = page.getByRole('region', { name: 'ShareOJ定期便' })
  await expect(delivery).toContainText('次回予告')
  await expect(delivery.locator('.delivery-problem')).toHaveCount(2)
  await expect(delivery.locator('.delivery-waiting')).toContainText('Easy 3問')
  await expect(delivery.locator('.delivery-waiting')).toContainText('Hard 2問')
  await expect(delivery.locator('.delivery-problem').first()).toContainText('writeralice')
  await expect(delivery.locator('.delivery-problem').first()).toContainText('testerbob')
  await expect(delivery.locator('a[href^="/problems/"]')).toHaveCount(0)
  await expect(delivery).not.toContainText('定期便の新作')
  for (const width of [320, 375, 414, 768, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`home-preview-${width}.png`), fullPage: true })
  }
  await delivery.getByRole('link', { name: '定期便・新作の応募' }).focus()
  await expect(delivery.getByRole('link', { name: '定期便・新作の応募' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/featured$/)
  await expect(delivery).toContainText('次回予告')
  await expect(delivery.locator('.delivery-waiting')).toContainText('Easy 3問')
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.screenshot({ path: testInfo.outputPath('featured-preview-dark.png'), fullPage: true })
})

test('published edition shows both titles and credits, then switches back to preview', async ({ page }, testInfo) => {
  await page.clock.install()
  await page.goto('/problems')
  const payload = await (await page.request.get('/api/featured')).json()
  payload.current = payload.items[0]
  await page.route('**/api/featured**', route => route.fulfill({ json: payload }))
  await page.locator('header').getByRole('link', { name: /ShareOJ/ }).click()
  const delivery = page.getByRole('region', { name: 'ShareOJ定期便' })
  await expect(delivery).toContainText('公開中の問題')
  await expect(delivery.getByRole('link', { name: 'A + B' })).toBeVisible()
  await expect(delivery.getByRole('link', { name: '定期便の新作' })).toHaveAttribute('href', `/problems/${shortId(hidden)}`)
  await expect(delivery).toContainText('Ultimate')
  await expect(delivery).toContainText('解説・他者の提出')
  for (const width of [375, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    await page.screenshot({ path: testInfo.outputPath(`home-current-${width}.png`), fullPage: true })
  }
  await delivery.getByRole('link', { name: '定期便・新作の応募' }).click()
  await expect(delivery).toContainText('公開中の問題')
  await page.screenshot({ path: testInfo.outputPath('featured-current.png'), fullPage: true })
  payload.current = null
  await page.clock.fastForward(61000)
  await expect(delivery).toContainText('次回予告')
  await expect(delivery.locator('a[href^="/problems/"]')).toHaveCount(0)
  await expect(page.locator('.round').first()).toContainText('定期便の新作')
})

test('empty inventory and long credits fit a narrow screen', async ({ page }) => {
  await page.goto('/problems')
  const payload = await (await page.request.get('/api/featured')).json()
  payload.waiting = { easy: 0, hard: 0 }
  payload.nextSlots[0] = { slot: 'easy', kind: 'missing', writer: '', testers: [], difficulty: null }
  payload.nextSlots[1] = { slot: 'hard', kind: 'revival', writer: 'a'.repeat(32), testers: ['b'.repeat(32), 'c'.repeat(32)], difficulty: 10 }
  await page.route('**/api/featured**', route => route.fulfill({ json: payload }))
  await page.setViewportSize({ width: 320, height: 1000 })
  await page.locator('header').getByRole('link', { name: '定期便', exact: true }).click()
  const delivery = page.getByRole('region', { name: 'ShareOJ定期便' })
  await expect(delivery).toContainText('出題する問題を募集中です。')
  await expect(delivery).toContainText('復刻')
  await expect(delivery.locator('.delivery-waiting')).toContainText('Easy 0問')
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320)
})

test('home announces 定期便 with a link that fits narrow screens', async ({ page }, testInfo) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await page.screenshot({ path: testInfo.outputPath('home-news-1280.png') })
  await page.setViewportSize({ width: 320, height: 800 })
  const news = page.getByRole('complementary', { name: 'お知らせ' })
  await expect(news).toContainText('新機能「定期便」が追加されました！')
  await expect(news.getByRole('link', { name: '定期便を見る' })).toHaveAttribute('href', '/featured')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await news.screenshot({ path: testInfo.outputPath('home-news-320.png') })
})

test('home keeps its navigation and retries when the delivery API fails', async ({ page }) => {
  await page.goto('/problems')
  let failing = true
  const payload = await (await page.request.get('/api/featured')).json()
  await page.route('**/api/featured**', route => failing ? route.fulfill({ status: 502, json: {} }) : route.fulfill({ json: payload }))
  await page.locator('header').getByRole('link', { name: /ShareOJ/ }).click()
  await expect(page.getByRole('heading', { level: 1 })).toContainText('考える楽しさを、')
  await expect(page.getByRole('alert')).toContainText('定期便を取得できませんでした')
  failing = false
  await page.getByRole('button', { name: '再試行' }).click()
  await expect(page.getByRole('region', { name: 'ShareOJ定期便' })).toContainText('次回予告')
})
