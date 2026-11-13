import { expect, test } from '@playwright/test'
import { docPages, legacyDocPaths } from '../shared/docs'

for (const [slug, codeBlocks, tables] of [['generator', 3, 1], ['checkers', 7, 5], ['languages', 0, 11]] as const) {
  test(`${slug} renders Markdown, highlighted code and stable section links`, async ({ page, request }, testInfo) => {
    const path = `/docs/${slug}`
    const response = await request.get(path)
    expect(response.status()).toBe(200)
    if (codeBlocks) expect(await response.text()).toContain('class="hljs-')
    await page.goto(path)
    const body = page.locator('article .markdown-body')
    await expect(body.locator('pre.highlighted-code')).toHaveCount(codeBlocks)
    await expect(body.locator('table')).toHaveCount(tables)
    // The run-twice examples on the checkers page are in Python; every other example is C++.
    for (const code of await body.locator('pre code').all()) {
      await expect(code).toHaveClass(/^language-(cpp|python)$/)
      expect(await code.locator('[class^="hljs-"]').count()).toBeGreaterThan(0)
      await expect(code).toContainText((await code.getAttribute('class')) === 'language-cpp' ? '#include' : 'import')
    }
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', `https://judge.example${path}`)
    for (const link of await page.getByRole('navigation', { name: 'ガイドの一覧' }).getByRole('list', { name: 'このページの目次' }).getByRole('link').all()) {
      const target = (await link.getAttribute('href'))!
      await expect(page.locator(target)).toHaveCount(1)
      await link.click()
      await expect(page).toHaveURL(new RegExp(`${target}$`))
      await expect(page.locator(target)).toBeInViewport()
    }
    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 900 })
      await body.locator(codeBlocks ? 'pre' : 'table').first().scrollIntoViewIfNeeded()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: testInfo.outputPath(`${slug}-${width}.png`) })
    }
  })
}

test('the guide index lists every guide and the sidebar follows the reader', async ({ page, request }) => {
  expect((await request.get('/docs/unknown')).status()).toBe(404)
  await page.goto('/docs')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('ガイド')
  const index = page.locator('.docs-index')
  for (const doc of docPages) await expect(index.getByRole('link', { name: doc.title, exact: true })).toHaveAttribute('href', `/docs/${doc.slug}`)
  await index.getByRole('link', { name: 'コンテストのルール', exact: true }).click()
  await expect(page).toHaveURL('/docs/contest-rules')
  const sidebar = page.getByRole('navigation', { name: 'ガイドの一覧' })
  await expect(sidebar.getByRole('link', { name: 'コンテストのルール', exact: true })).toHaveAttribute('aria-current', 'page')
  // Only the open guide lists its headings, right under its link.
  const contents = sidebar.getByRole('list', { name: 'このページの目次' })
  await expect(contents).toHaveCount(1)
  await expect(sidebar.getByRole('listitem').filter({ has: page.getByRole('link', { name: 'コンテストのルール', exact: true }) }).getByRole('list', { name: 'このページの目次' })).toBeVisible()
  await expect(contents.getByRole('link', { name: '公式順位に入る提出' })).toHaveAttribute('href', '#official')
  // The sidebar switches guides and their headings follow.
  await sidebar.getByRole('link', { name: '提出と判定結果', exact: true }).click()
  await expect(page).toHaveURL('/docs/submissions')
  await expect(contents.getByRole('link', { name: '解答を提出する' })).toHaveAttribute('href', '#submit')
  await expect(contents.getByRole('link', { name: '公式順位に入る提出' })).toHaveCount(0)
  // A link in the Markdown body moves within the app, keeping the sidebar.
  await page.locator('article .markdown-body').getByRole('link', { name: '使える言語と実行環境の仕様' }).click()
  await expect(page).toHaveURL('/docs/languages')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('使える言語と実行環境の仕様')
  await expect(sidebar.getByRole('link', { name: '使える言語', exact: true })).toHaveAttribute('aria-current', 'page')
  const next = docPages[docPages.findIndex(doc => doc.slug === 'languages') + 1]!
  await page.getByRole('navigation', { name: '前後のガイド' }).getByRole('link', { name: new RegExp(`次のガイド\\s*${next.label}`) }).click()
  await expect(page).toHaveURL(`/docs/${next.slug}`)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(next.title)
})

test('every screenshot in the guides is served and has a description', async ({ page, request }) => {
  for (const doc of docPages) {
    await page.goto(`/docs/${doc.slug}`)
    for (const image of await page.locator('article .markdown-body img').all()) {
      expect(await image.getAttribute('alt')).toBeTruthy()
      const src = (await image.getAttribute('src'))!
      expect(src).toMatch(/^\/images\/docs\/[a-z-]+\.png$/)
      expect((await request.get(src)).status()).toBe(200)
    }
  }
})

test('old guide URLs redirect permanently to the guides', async ({ request }) => {
  for (const [path, slug] of Object.entries(legacyDocPaths)) {
    const response = await request.get(path, { maxRedirects: 0 })
    expect(response.status()).toBe(301)
    expect(response.headers().location).toBe(`/docs/${slug}`)
  }
})

test('narrow screens fold the guide list behind a button and never scroll sideways', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 375, height: 800 })
  await page.goto('/docs/markdown')
  const toggle = page.getByRole('button', { name: 'ガイドの一覧' })
  const sidebar = page.getByRole('navigation', { name: 'ガイドの一覧' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(sidebar.getByRole('link', { name: '使える言語', exact: true })).toBeHidden()
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await sidebar.getByRole('link', { name: '使える言語', exact: true }).click()
  await expect(page).toHaveURL('/docs/languages')
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  for (const path of ['/docs', ...docPages.map(doc => `/docs/${doc.slug}`)]) {
    await page.goto(path)
    for (const width of [320, 768]) {
      await page.setViewportSize({ width, height: 800 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${path} at ${width}px`).toBe(true)
    }
  }
  await page.setViewportSize({ width: 1280, height: 900 })
  await expect(toggle).toBeHidden()
  await expect(sidebar.getByRole('link', { name: '使える言語', exact: true })).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('docs-1280.png') })
})

test('links between guides point at existing pages and headings', async ({ page }) => {
  const targets = new Set<string>()
  for (const doc of docPages) {
    await page.goto(`/docs/${doc.slug}`)
    for (const href of await page.locator('article .markdown-body a[href^="/docs/"], article .markdown-body a[href^="#"]').evaluateAll(links => links.map(link => link.getAttribute('href')!))) {
      targets.add(href.startsWith('#') ? `/docs/${doc.slug}${href}` : href)
    }
  }
  expect(targets.size).toBeGreaterThan(10)
  for (const target of targets) {
    const [path, hash] = target.split('#')
    const response = await page.goto(path!)
    expect(response?.status(), target).toBe(200)
    if (hash) await expect(page.locator(`#${hash}`), target).toHaveCount(1)
  }
})

test('the contest guide says start times are typed in JST', async ({ page }) => {
  await page.goto('/docs/hosting-contests')
  await expect(page.locator('article .markdown-body')).toContainText('開始日時はJSTで入力します。')
})

test('the featured guide gives the delivery time in JST', async ({ page }) => {
  await page.goto('/docs/featured')
  await expect(page.locator('article .markdown-body')).toContainText('22:00 (JST)')
})

test('each guide names its reader above the title and in the lists', async ({ page }) => {
  for (const [slug, reader] of [['submissions', '参加者向け'], ['contest-rules', '参加者向け'], ['hosting-contests', '作問者向け'], ['cli', '作問者向け'], ['api', '開発者向け']] as const) {
    await page.goto(`/docs/${slug}`)
    await expect(page.locator('article .eyebrow')).toHaveText(reader)
    await expect(page.getByRole('navigation', { name: 'パンくずリスト' })).toContainText(reader)
  }
  await page.goto('/docs')
  await expect(page.locator('.docs-index').getByRole('heading', { level: 2 })).toHaveText(['参加者向け', '作問者向け', '開発者向け'])
})

test('the contest guides split joining from ranking', async ({ page }) => {
  await page.goto('/docs/contest-participation')
  const joining = page.locator('article .markdown-body')
  for (const id of ['join', 'team', 'onsite', 'clarifications', 'my-submissions']) await expect(joining.locator(`#${id}`)).toHaveCount(1)
  await expect(joining.locator('img[src="/images/docs/contest-join.png"]')).toHaveCount(1)
  await expect(joining.locator('#penalty')).toHaveCount(0)
  await expect(joining).toContainText('チームメイトには表示しません')
  await expect(joining).toContainText('質問した人の名前')

  await page.goto('/docs/contest-rules')
  const rules = page.locator('article .markdown-body')
  await expect(rules.locator('h2')).toHaveCount(5)
  for (const id of ['official', 'scoring', 'penalty', 'freeze', 'visibility']) await expect(rules.locator(`#${id}`)).toHaveCount(1)
  // The terms and the privacy policy defer to this page for when contest submissions become public.
  await expect(rules.locator('#visibility ~ table tbody tr').filter({ hasText: '終了後' })).toContainText('誰でも見られます')
  // Readers arriving at a section that moved find the way from the top of the page.
  await expect(rules.locator('p').first().getByRole('link', { name: 'コンテストに参加する' })).toHaveAttribute('href', '/docs/contest-participation')
  await expect(rules.locator('p').first()).toContainText('質問')
  // Testers and authors see only their own problems before the start.
  await expect(rules).toContainText('その問題の作者とテスター')
  await expect(rules).not.toContainText('回答')
  await expect(rules.locator('img')).toHaveCount(0)
  // Verdicts show as the coloured badges used on the submission pages.
  await expect(rules.locator('.verdict-badge')).toHaveText(['AC', 'WA', 'RE', 'TLE', 'MLE', 'OLE', 'CE', 'JE'])
})

test('the hosting guide carries what staff need to know', async ({ page }) => {
  await page.goto('/docs/hosting-contests')
  const body = page.locator('article .markdown-body')
  for (const id of ['problems', 'create', 'publish', 'changes', 'setters', 'staff', 'clarifications', 'after']) await expect(body.locator(`#${id}`)).toHaveCount(1)
  await expect(body).toContainText('スタッフは参加登録できません')
  await expect(body).toContainText('オープン参加には戻せません')
  await expect(body.locator('ol').first()).toBeVisible()
})

test('the featured guides split solving from applying', async ({ page }) => {
  await page.goto('/docs/featured')
  const solving = page.locator('article .markdown-body')
  for (const id of ['schedule', 'solve', 'standings']) await expect(solving.locator(`#${id}`)).toHaveCount(1)
  await expect(solving.locator('#apply')).toHaveCount(0)
  await expect(solving.locator('p').first().getByRole('link', { name: '定期便に応募する' })).toHaveAttribute('href', '/docs/featured-apply')
  await expect(solving).toContainText('Hardの点数')

  await page.goto('/docs/featured-apply')
  await expect(page.locator('article .eyebrow')).toHaveText('作問者向け')
  const applying = page.locator('article .markdown-body')
  for (const id of ['apply', 'conditions', 'selection', 'withdraw']) await expect(applying.locator(`#${id}`)).toHaveCount(1)
  await expect(applying.locator('img[src="/images/docs/featured-apply.png"]')).toHaveCount(1)
})

test('the submission guide leads with the steps and keeps the verdict table', async ({ page }) => {
  await page.goto('/docs/submissions')
  const body = page.locator('article .markdown-body')
  await expect(body.locator('#submit ~ ol').first().locator('li')).toHaveCount(3)
  await expect(body.locator('table').filter({ hasText: '正解' }).first().locator('tbody tr')).toHaveCount(9)
  await expect(body.locator('#usage')).toHaveCount(0)
  await expect(body).not.toContainText('1,048,576')
  await expect(body.locator('tr').filter({ hasText: 'ジャッジ中' })).toContainText('コンパイルしています')
})

test('the authoring guide keeps its anchors and drops the sample marker details', async ({ page }) => {
  await page.goto('/docs/authoring')
  const body = page.locator('article .markdown-body')
  for (const id of ['start', 'statement', 'tests', 'scoring', 'save', 'readiness', 'publish', 'after-publish', 'rejudge', 'testers', 'editorials']) await expect(body.locator(`#${id}`)).toHaveCount(1)
  await expect(body.locator('img')).toHaveCount(11)
  await expect(body).not.toContainText('sample-begin')
  await expect(body.locator('#publish ~ ol').first()).toBeVisible()
})
