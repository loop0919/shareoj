import { expect, test } from '@playwright/test'

const problemId = '11111111-1111-4111-8111-111111111111'
const submissionId = '22222222-2222-4222-8222-222222222222'

for (const scope of ['problem', 'contest']) {
  test(`${scope} public submission uses the full result view`, async ({ page }) => {
    const contestId = '88888888-8888-4888-8888-888888888888'
    const problemPath = scope === 'contest' ? `/contests/${contestId}/problems/${problemId}` : `/problems/${problemId}`
    const detailPath = scope === 'contest' ? `/contests/${contestId}/submissions/${submissionId}?from=problem` : `/problems/${problemId}/submissions/${submissionId}`
    if (scope === 'problem') {
      await page.goto(`${problemPath}?view=submissions`)
      await page.getByRole('link', { name: '詳細', exact: true }).click()
      await expect(page).toHaveURL(detailPath)
    } else {
      await page.goto(detailPath)
    }
    await expect(page.getByRole('heading', { name: '提出結果', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: '提出者 alice', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: '言語 C++17 (GCC)', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: 'コード長 12 bytes', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: '実行時間 2 ms', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: 'メモリ使用量 1.05 MB', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: '正解したケース 1 / 1', exact: true })).toBeVisible()
    await expect(page.getByRole('row', { name: 'sample_1 AC 2 ms 4 ms 1.05 MB', exact: true })).toBeVisible()
    const source = page.getByRole('textbox', { name: 'ソースコード', exact: true })
    await expect(source).toHaveText('int main(){}')
    await expect(source).toHaveAttribute('aria-readonly', 'true')
    await page.getByRole('button', { name: '広げる', exact: true }).click()
    await expect(page.getByRole('button', { name: '折りたたむ' })).toHaveAttribute('aria-expanded', 'true')
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.getByRole('button', { name: 'コピー', exact: true }).click()
    await expect(page.getByText('コピーしました。', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('int main(){}')
    await expect(page.getByRole('link', { name: 'この問題のすべての提出', exact: true })).toHaveAttribute('href', `${problemPath}?view=submissions`)
    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 900 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    }
    await page.reload()
    await expect(source).toHaveText('int main(){}')
  })
}

test('language selection survives reloads and returning to the problem', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.goto(`/problems/${problemId}`)
  const language = page.getByRole('combobox', { name: '言語' })
  await expect(language).toHaveValue('cpp17')
  await language.selectOption('python314')
  await page.reload()
  await expect(language).toHaveValue('python314')
  await page.goto('/problems')
  await page.getByRole('link', { name: 'A + B', exact: true }).click()
  await expect(language).toHaveValue('python314')
  await language.selectOption('cpp17')
  await page.reload()
  await expect(language).toHaveValue('cpp17')
})

test('unselected language disables both actions and survives reloads', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.goto(`/problems/${problemId}`)
  const language = page.getByRole('combobox', { name: '言語' })
  const submit = page.getByRole('button', { name: '提出する', exact: true })
  const sample = page.getByRole('button', { name: 'サンプル検証', exact: true })
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  await language.selectOption({ label: '-- 未選択 --' })
  await expect(submit).toBeDisabled()
  await expect(sample).toBeDisabled()
  await language.selectOption('cpp17')
  await expect(submit).toBeEnabled()
  await expect(sample).toBeEnabled()
  await language.selectOption('')
  await page.reload()
  await expect(language).toHaveValue('')
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  await expect(submit).toBeDisabled()
  await expect(sample).toBeDisabled()
})

test('an unavailable saved language falls back to the default', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.addInitScript(() => localStorage.setItem('openoj.submission-runtime', 'removed-runtime'))
  await page.goto(`/problems/${problemId}`)
  await expect(page.getByRole('combobox', { name: '言語' })).toHaveValue('cpp17')
})

test('language selection works when browser storage is disabled', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.addInitScript(() => {
    Object.defineProperty(window, 'localStorage', { get() { throw new DOMException('Blocked', 'SecurityError') } })
  })
  const errors: Error[] = []
  page.on('pageerror', error => errors.push(error))
  await page.goto(`/problems/${problemId}`)
  const language = page.getByRole('combobox', { name: '言語' })
  await expect(language).toHaveValue('cpp17')
  await language.selectOption('python314')
  await expect(language).toHaveValue('python314')
  expect(errors).toEqual([])
})

test('unauthenticated visitors see a login card instead of the submission form', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto(`/problems/${problemId}`)
  await expect(page.getByRole('heading', { name: 'ログインして解答を提出' })).toBeVisible()
  await expect(page.getByLabel('ソースコード', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: '言語' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '提出する', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'ログインする', exact: true }).click()
  await expect(page).toHaveURL('/login')
})

test('C++ submission opens its result and polls until completion', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
  const item = { id: submissionId, problemId, problemVersion: 2, problemTitle: 'A + B', runtime: 'cpp17-local', source: 'int main(){}', status: 'QUEUED', result: null, createdAt: '2026-09-01T00:00:00Z' }
  await page.route('**/api/my/submissions', async route => {
    expect(route.request().method()).toBe('POST')
    expect(route.request().postDataJSON()).toEqual({ problemId, runtime: 'cpp17', source: 'int main(){}' })
    await route.fulfill({ status: 202, json: item })
  })
  let reads = 0
  await page.route(`**/api/my/submissions/${submissionId}`, route => route.fulfill({ json: ++reads === 1 ? item : { ...item, status: 'DONE', result: { verdict: 'AC', passed: 2, total: 2, cpuTimeMs: 3.25, memoryBytes: 12345000, cases: [{ name: 'sample', verdict: 'AC' }, { name: 'large', verdict: 'AC' }] } } }))
  await page.goto(`/problems/${problemId}`)
  await expect(page.getByRole('heading', { name: 'ログインして解答を提出' })).toBeHidden()
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  await expect(page.getByText('12 / 65,536 bytes', { exact: true })).toBeVisible()
  const keyword = page.locator('.cm-line span').filter({ hasText: /^int$/ }).first()
  await expect(keyword).toBeVisible()
  expect(await keyword.evaluate(element => getComputedStyle(element).color !== getComputedStyle(element.closest('.cm-content')!).color)).toBe(true)
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(page).toHaveURL(`/my/submissions/${submissionId}`)
  await expect(page.getByRole('status')).toHaveText('AC：正解')
  await expect(page.getByRole('row', { name: '正解したケース 2 / 2' })).toBeVisible()
  await expect(page.getByRole('row', { name: '実行時間 4 ms', exact: true })).toBeVisible()
  await expect(page.getByRole('row', { name: 'メモリ使用量 12.35 MB', exact: true })).toBeVisible()
  await expect(page.getByRole('row', { name: '言語 C++17 (GCC)', exact: true })).toBeVisible()
  await expect(page.getByRole('row', { name: '問題の版', exact: false })).toHaveCount(0)
  await expect(page.getByRole('table', { name: 'テストケースごとの結果' }).getByRole('row', { name: 'sample AC' })).toBeVisible()
  await expect(page.getByRole('table', { name: 'テストケースごとの結果' }).getByRole('row', { name: 'large AC' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'ソースコード', exact: true })).toHaveAttribute('aria-readonly', 'true')
  await expect(page.getByRole('row', { name: 'コード長 12 bytes' })).toBeVisible()
  await page.getByRole('button', { name: '広げる', exact: true }).click()
  await expect(page.getByRole('button', { name: '折りたたむ' })).toHaveAttribute('aria-expanded', 'true')
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.getByRole('button', { name: 'コピー', exact: true }).click()
  await expect(page.getByText('コピーしました。', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('int main(){}')
})

for (const view of ['detail', 'history']) {
  for (const verdict of ['AC', 'WA', 'TLE', 'RE', 'MLE', 'OLE']) {
    test(`${view} ${verdict} displays actual preparation and case progress, then stops polling`, async ({ page }) => {
      await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
      await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
      const item = { id: submissionId, problemId, problemVersion: 2, problemTitle: 'A + B', runtime: 'python314-isolate', source: 'print(3)', status: 'QUEUED', result: null, createdAt: '2026-09-01T00:00:00Z' }
      const states = [item,
        { ...item, status: 'RUNNING', progress: { phase: 'PREPARING', completed: 0, total: 4 } },
        { ...item, status: 'RUNNING', progress: { phase: 'JUDGING', completed: 0, total: 4 } },
        { ...item, status: 'RUNNING', progress: { phase: 'JUDGING', completed: 2, total: 4, ...(verdict === 'AC' ? {} : { verdict }) } },
        { ...item, status: 'DONE', progress: null, result: { verdict, passed: verdict === 'AC' ? 4 : 1, total: 4 } },
      ]
      let reads = 0
      const endpoint = `/api/my/submissions${view === 'detail' ? `/${submissionId}` : ''}`
      await page.route(`**${endpoint}`, route => {
        const state = states[Math.min(reads++, states.length - 1)]
        return route.fulfill({ json: view === 'detail' ? state : { items: [state] } })
      })
      await page.goto(`/my/submissions${view === 'detail' ? `/${submissionId}` : ''}`)
      await expect(page.getByRole('cell', { name: 'Python (CPython 3.14)', exact: true })).toBeVisible()
      const badge = page.locator('.verdict-badge').first()
      await expect(badge).toHaveText('WJ')
      await expect(badge.locator('.judge-spinner')).toBeVisible()
      await badge.focus()
      await expect(page.getByRole('tooltip')).toHaveText('ジャッジ中')
      await expect(badge).toHaveAccessibleDescription('ジャッジ中')
      for (const [index, label] of ['WJ', 'WJ', '0/4', verdict === 'AC' ? '2/4' : `${verdict} 2/4`, verdict].entries()) {
        await expect.poll(() => reads).toBeGreaterThanOrEqual(index + 1)
        await expect(badge).toHaveText(label)
        if (index === 3) {
          await expect(badge.locator('.judge-spinner')).toBeVisible()
          if (verdict !== 'AC') await expect(badge).toHaveAttribute('data-verdict', verdict)
        }
      }
      await expect(badge.locator('.judge-spinner')).toHaveCount(0)
      await page.clock.install()
      await page.clock.fastForward(6000)
      expect(reads).toBe(5)
    })
  }
}

test('unready judging settings explain why submission was rejected', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.route('**/api/my/favorites/*', route => route.fulfill({ status: 503, json: {} }))
  await page.route('**/api/my/submissions', route => route.fulfill({ status: 409, json: { data: { code: 'tests_not_ready' } } }))
  await page.goto(`/problems/${problemId}`)
  await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'お気に入りを取得できませんでした。' })).toBeVisible()
  await expect(page.getByRole('region', { name: '提出', exact: true }).getByRole('alert')).toContainText('テストケース、検証コード、利用できる言語の設定を確認してください。')
  await expect(page.getByLabel('ソースコード', { exact: true })).toHaveText('int main(){}')
  await expect(page.getByLabel('ソースコード', { exact: true })).toBeEditable()
})


test('Sample validation polls inline and retains the source for a full submission', async ({ page }) => {
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  const item = { id: submissionId, problemId, problemVersion: 2, problemTitle: 'A + B', runtime: 'cpp17', status: 'QUEUED', result: null, createdAt: '2026-09-01T00:00:00Z' }
  const requests: Record<string, unknown>[] = []
  await page.route('**/api/my/submissions', async route => {
    requests.push(route.request().postDataJSON())
    await route.fulfill({ status: 202, json: { ...item, easyTest: requests.length === 1 } })
  })
  await page.route(`**/api/my/submissions/${submissionId}`, route => route.fulfill({ json: { ...item, status: 'DONE', result: { verdict: 'AC', passed: 1, total: 1, cases: [{ name: 'sample_1', verdict: 'AC', sampleDetails: { input: { text: '1 2\n', truncated: false }, expectedOutput: { text: '3\n', truncated: false }, actualOutput: { text: '7\n', truncated: false } } }] } } }))
  await page.goto(`/problems/${problemId}`)
  const source = page.getByLabel('ソースコード', { exact: true })
  await source.fill('int main(){}')
  const help = page.getByRole('button', { name: 'サンプル検証の説明', exact: true })
  const tooltip = page.getByRole('tooltip').filter({ hasText: 'サンプルケースを検証する機能です。' })
  await expect(tooltip).toBeHidden()
  const helpBox = await help.boundingBox()
  const buttonBox = await page.getByRole('button', { name: 'サンプル検証', exact: true }).boundingBox()
  expect(helpBox!.width).toBe(20)
  expect(helpBox!.height).toBe(20)
  expect(Math.abs(helpBox!.y + helpBox!.height - buttonBox!.y - buttonBox!.height)).toBeLessThan(1)
  await help.hover()
  await expect(tooltip).toBeVisible()
  await source.hover()
  await expect(tooltip).toBeHidden()
  await help.focus()
  await expect(tooltip).toBeVisible()
  await page.getByRole('button', { name: 'サンプル検証', exact: true }).click()
  await expect(page.getByText('sample_1: AC', { exact: true })).toBeVisible()
  const details = page.locator('.sample-case')
  await expect(details.locator('dt')).toHaveText(['入力', '期待される出力', '実際の出力'])
  await expect(details.locator('pre')).toHaveText(['1 2\n', '3\n', '7\n'])
  await expect(page).toHaveURL(`/problems/${problemId}`)
  await expect(source).toHaveText('int main(){}')
  await expect(source).toBeEditable()
  await page.setViewportSize({ width: 375, height: 900 })
  await help.hover()
  await expect(tooltip).toBeVisible()
  const tooltipBox = await tooltip.boundingBox()
  expect(tooltipBox!.x).toBeGreaterThanOrEqual(0)
  expect(tooltipBox!.x + tooltipBox!.width).toBeLessThanOrEqual(375)
  await source.hover()
  expect(requests[0]).toEqual({ problemId, runtime: 'cpp17', source: 'int main(){}', easyTest: true })
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(page).toHaveURL(`/my/submissions/${submissionId}`)
  expect(requests[1]).toEqual({ problemId, runtime: 'cpp17', source: 'int main(){}' })
})


test('Sample validation detail displays empty and truncated output safely', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
  await page.route(`**/api/my/submissions/${submissionId}`, route => route.fulfill({ json: {
    id: submissionId, easyTest: true, problemId, problemVersion: 1, problemTitle: 'A + B', runtime: 'cpp17',
    status: 'DONE', createdAt: '2026-09-01T00:00:00Z', result: {
      verdict: 'WA', passed: 0, total: 1, cases: [{ name: 'sample_1', verdict: 'WA', sampleDetails: {
        input: { text: '<script>alert(1)</script>', truncated: false },
        expectedOutput: { text: '', truncated: false }, actualOutput: { text: 'partial', truncated: true },
      } }],
    },
  } }))
  await page.goto(`/my/submissions/${submissionId}`)
  await expect(page.getByRole('heading', { name: 'サンプル検証の結果', exact: true })).toBeVisible()
  const details = page.locator('.sample-case')
  await expect(details.locator('pre')).toHaveText(['<script>alert(1)</script>', 'partial'])
  await expect(details.getByText('（空）', { exact: true })).toBeVisible()
  await expect(details.getByText('長いため、先頭部分のみ表示しています。')).toBeVisible()
  await expect(details.locator('script')).toHaveCount(0)
})


for (const failed of [false, true]) {
  test(`Interactive sample displays diagnostics instead of output comparison (failed=${failed})`, async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
    await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
    await page.route(`**/api/my/submissions/${submissionId}`, route => route.fulfill({ json: {
      id: submissionId, easyTest: true, problemId, problemVersion: 1, problemTitle: 'Interactive', runtime: 'cpp17',
      status: 'DONE', createdAt: '2026-09-01T00:00:00Z', result: {
        interactive: true, verdict: failed ? 'JE' : 'AC', passed: failed ? 0 : 1, total: 1,
        checkerLog: '対話の診断',
        ...(failed ? {} : { cases: [{ name: 'sample_1', verdict: 'AC', checkerLog: { text: '対話判定: AC', truncated: false } }] }),
      },
    } }))
    await page.goto(`/my/submissions/${submissionId}`)
    await expect(page.getByRole('heading', { name: 'ジャッジコードの診断', exact: true })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: failed ? '対話の診断' : '対話判定: AC' })).toBeVisible()
    await expect(page.getByText('期待される出力', { exact: true })).toHaveCount(0)
    await expect(page.getByText('実際の出力', { exact: true })).toHaveCount(0)
    await expect(page.getByText('このケースの入出力は記録されていません。')).toHaveCount(0)
  })
}


test('history labels cover all runtimes, historical aliases and unknown IDs', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  const languages = [
    ['cpp17-isolate', 'C++17 (GCC)'], ['cpp17-local', 'C++17 (GCC)'],
    ['c23-gcc-isolate', 'C23 (GCC)'], ['c23-clang-isolate', 'C23 (Clang)'],
    ['cpp23-gcc-isolate', 'C++23 (GCC)'], ['cpp23-clang-isolate', 'C++23 (Clang)'],
    ['python314-isolate', 'Python (CPython 3.14)'], ['python314', 'Python (CPython 3.14)'],
    ['pypy311-isolate', 'Python (PyPy 3.11)'], ['codon020-isolate', 'Python (Codon 0.20)'],
    ['rust2024-isolate', 'Rust (Edition 2024)'], ['java24-isolate', 'Java (OpenJDK 24)'],
    ['future-runtime-isolate', 'future-runtime-isolate'],
  ]
  // Labels must not depend on which languages currently accept submissions.
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: [] } }))
  await page.route('**/api/my/submissions', route => route.fulfill({ json: { items: languages.map(([runtime], index) => ({
    id: String(index), problemId, problemTitle: 'A + B', runtime, status: 'DONE', result: { verdict: 'AC', passed: 1, total: 1 }, createdAt: '2026-09-01T00:00:00Z',
  })) } }))
  await page.goto('/my/submissions')
  await expect(page.locator('.submission-language')).toHaveText(languages.map(([, label]) => label!))
})

test('submission frequency limit preserves the wait time and styles both actions', async ({ page, context, request }) => {
  const response = await request.post('/api/my/submissions', {
    headers: { Cookie: 'openoj_access=valid-access', Origin: 'https://judge.example' },
    data: { source: '// rate-limit-fixture' },
  })
  expect(response.status()).toBe(429)
  expect(response.headers()['retry-after']).toBe('42')
  expect((await response.json()).data).toEqual({ code: 'submission_rate_limited', retryAfter: 42 })
  await context.addCookies([{ name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }])
  await page.route('**/api/my/submissions', async route => {
    const upstream = await route.fetch({ headers: { ...route.request().headers(), origin: 'https://judge.example' } })
    await route.fulfill({ response: upstream })
  })
  await page.goto(`/problems/${problemId}`)
  await page.getByLabel('ソースコード', { exact: true }).fill('// rate-limit-fixture')
  for (const name of ['提出する', 'サンプル検証']) {
    await page.getByRole('button', { name, exact: true }).click()
    const alert = page.getByRole('region', { name: '提出', exact: true }).getByRole('alert')
    await expect(alert).toHaveText('提出頻度制限に到達しました。42秒後に再度試してください。')
    await expect(alert).toHaveCSS('background-color', 'rgb(255, 243, 242)')
    await expect(alert).toHaveCSS('border-inline-start-width', '3px')
    await expect(page.getByLabel('ソースコード', { exact: true })).toHaveText('// rate-limit-fixture')
  }
})

for (const view of ['all', 'mine', 'history']) {
  test(`${view} submission list shows code size and measured resource usage`, async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
    await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-01T00:00:00Z' } } }))
    const base = { id: submissionId, problemId, problemTitle: 'A + B', problemVersion: 1, author: 'alice', runtime: 'python314', createdAt: '2026-09-01T00:00:00Z', status: 'DONE' }
    const items = [
      { ...base, sourceBytes: 1234, result: { verdict: 'AC', passed: 2, total: 2, cpuTimeMs: 1234.1, memoryBytes: 12345678 } },
      { ...base, id: 'zero', sourceBytes: 0, result: { verdict: 'AC', passed: 1, total: 1, cpuTimeMs: 0, memoryBytes: 0 } },
      { ...base, id: 'missing', result: { verdict: 'CE', passed: 0, total: 1 } },
      { ...base, id: 'pending', sourceBytes: 42, status: 'RUNNING', result: null },
    ]
    const endpoint = view === 'history' ? '**/api/my/submissions' : `**/api/problems/${problemId}/submissions?*`
    await page.route(endpoint, route => route.fulfill({ json: { items, hasMore: false } }))
    await page.goto(view === 'history' ? '/my/submissions' : `/problems/${problemId}?view=${view === 'mine' ? 'my-submissions' : 'submissions'}`)
    await expect(page.getByRole('columnheader', { name: 'コード長', exact: true })).toBeVisible()
    for (const size of ['1,234 bytes', '0 bytes', '42 bytes', '—']) {
      await expect(page.getByRole('cell', { name: size, exact: true })).toBeVisible()
    }
    await expect(page.getByRole('columnheader', { name: '実行時間・メモリ', exact: true })).toBeVisible()
    await expect(page.getByRole('cell', { name: '1235 ms・ 12.35 MB', exact: true })).toBeVisible()
    await expect(page.getByRole('cell', { name: '0 ms・ 0.00 MB', exact: true })).toBeVisible()
    await expect(page.getByRole('cell', { name: '—・ —', exact: true })).toHaveCount(2)
    await page.setViewportSize({ width: 375, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

for (const scope of ['problem', 'contest']) {
  test(`${scope} without samples disables sample validation and explains why`, async ({ page }) => {
    const base = scope === 'contest' ? '/contests/88888888-8888-4888-8888-888888888888' : ''
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
    await page.route(`**/api${base}/problems/${problemId}`, async route => {
      const response = await route.fetch()
      await route.fulfill({ json: { ...await response.json(), hasSamples: false } })
    })
    await page.goto(scope === 'contest' ? `${base}?view=problems` : '/problems')
    await page.locator(`a[href="${base}/problems/${problemId}"]`).first().click()
    await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
    const sample = page.getByRole('button', { name: 'サンプル検証', exact: true })
    await expect(sample).toBeDisabled()
    await expect(page.getByRole('button', { name: '提出する', exact: true })).toBeEnabled()
    await sample.hover()
    await expect(page.getByRole('tooltip')).toHaveText('この問題は利用可能なサンプルがありません')
    await expect(page.getByRole('tooltip')).toBeVisible()
    await page.getByLabel('ソースコード', { exact: true }).hover()
    await page.getByRole('button', { name: 'サンプル検証の説明', exact: true }).focus()
    await expect(page.getByRole('tooltip')).toBeVisible()
  })
}
